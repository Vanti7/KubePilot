// Package helmops runs real Helm 3 operations (upgrade, rollback) against a
// cluster's releases via the Helm Go SDK. Kept separate from internal/k8sops
// (which patches Kubernetes objects directly): the Helm SDK is a much
// heavier dependency, and a Helm upgrade can touch any resource the chart
// manages, not just the handful of types k8sops knows about.
package helmops

import (
	"fmt"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	memcached "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// restClientGetter implements genericclioptions.RESTClientGetter around a
// *rest.Config we already have in hand (from CollectorManager.GetRESTConfig)
// instead of a kubeconfig file — the pattern used by Go programs that embed
// the Helm SDK without going through Helm's own CLI settings/config-file
// loading, which assumes a kubeconfig path rather than an in-memory config.
type restClientGetter struct {
	config    *rest.Config
	namespace string
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return g.config, nil
}

func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(g.config)
	if err != nil {
		return nil, err
	}
	return memcached.NewMemCacheClient(dc), nil
}

func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(dc), nil
}

// ToRawKubeConfigLoader satisfies the interface with a minimal in-memory
// config carrying just the namespace. We drive every Helm action with an
// explicit namespace ourselves, so nothing in this package ever reads the
// namespace back out of this loader — it exists only so restClientGetter
// compiles against genericclioptions.RESTClientGetter.
func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	apiConfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{"cluster": {}},
		Contexts: map[string]*clientcmdapi.Context{
			"context": {Cluster: "cluster", AuthInfo: "user", Namespace: g.namespace},
		},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"user": {}},
		CurrentContext: "context",
	}
	return clientcmd.NewDefaultClientConfig(apiConfig, &clientcmd.ConfigOverrides{})
}

// NewActionConfig builds a Helm action.Configuration for a single upgrade or
// rollback call, targeting the given namespace over restConfig. Uses the
// "secrets" storage driver — the same one `helm` CLI defaults to and the one
// the collector already reads release data from (collector/kubernetes.go's
// collectHelmReleases lists Secrets labelled owner=helm).
func NewActionConfig(restConfig *rest.Config, namespace string, logger *zap.Logger) (*action.Configuration, error) {
	getter := &restClientGetter{config: restConfig, namespace: namespace}

	cfg := new(action.Configuration)
	debugLog := func(format string, v ...interface{}) {
		logger.Debug(fmt.Sprintf(format, v...))
	}
	if err := cfg.Init(getter, namespace, "secrets", debugLog); err != nil {
		return nil, fmt.Errorf("init helm action config: %w", err)
	}
	return cfg, nil
}
