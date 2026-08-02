package k8sops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	memcached "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// fieldManager identifies KubePilot's own changes in a resource's
// managedFields, distinguishing them from Helm, kubectl or other controllers
// (relevant for the Force option below, which resolves a conflict against a
// *different* field manager).
const fieldManager = "kubepilot"

// Apply outcomes for a single document in a manifest.
const (
	ApplyOperationCreated = "created"
	ApplyOperationUpdated = "updated"
	ApplyOperationError   = "error"
)

// ApplyResult reports what happened to one resource out of a (possibly
// multi-document) manifest.
type ApplyResult struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Operation string `json:"operation"`
	Error     string `json:"error,omitempty"`
}

// BuildDynamicClient derives a dynamic client and RESTMapper from a
// *rest.Config already in hand (from CollectorManager.GetRESTConfig) —
// mirrors helmops.restClientGetter's ToDiscoveryClient/ToRESTMapper, kept
// separate rather than shared since that type is otherwise tied to the Helm
// SDK's genericclioptions.RESTClientGetter interface.
func BuildDynamicClient(restConfig *rest.Config) (dynamic.Interface, meta.RESTMapper, error) {
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("build dynamic client: %w", err)
	}
	dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("build discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memcached.NewMemCacheClient(dc))
	return dyn, mapper, nil
}

// ApplyManifest server-side applies every document in manifestYAML
// independently — one document failing does not stop the rest, the same way
// `kubectl apply -f multi.yaml` reports per-resource results rather than
// aborting. Documents are parsed up front; a structurally invalid manifest
// (bad YAML, missing apiVersion/kind) is rejected before anything is sent to
// the cluster.
func ApplyManifest(ctx context.Context, dynamicClient dynamic.Interface, mapper meta.RESTMapper, manifestYAML, defaultNamespace string, dryRun, force bool) ([]ApplyResult, error) {
	objs, err := splitYAMLDocuments(manifestYAML)
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, fmt.Errorf("manifest contains no resources")
	}

	results := make([]ApplyResult, 0, len(objs))
	for _, obj := range objs {
		results = append(results, applyOne(ctx, dynamicClient, mapper, obj, defaultNamespace, dryRun, force))
	}
	return results, nil
}

// splitYAMLDocuments decodes a (possibly multi-document) YAML string one
// document at a time. Uses the decode-until-EOF approach rather than
// splitting on "---" — a literal "---" can appear inside a string value, so
// a naive split would corrupt that document.
func splitYAMLDocuments(manifestYAML string) ([]*unstructured.Unstructured, error) {
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifestYAML), 4096)
	var objs []*unstructured.Unstructured
	for i := 1; ; i++ {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("document %d: %w", i, err)
		}
		if len(raw) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{Object: raw}
		if obj.GetAPIVersion() == "" || obj.GetKind() == "" {
			return nil, fmt.Errorf("document %d: missing apiVersion or kind", i)
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func applyOne(ctx context.Context, dynamicClient dynamic.Interface, mapper meta.RESTMapper, obj *unstructured.Unstructured, defaultNamespace string, dryRun, force bool) ApplyResult {
	gvk := obj.GroupVersionKind()
	result := ApplyResult{Kind: gvk.Kind, Name: obj.GetName()}

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		result.Operation = ApplyOperationError
		result.Error = fmt.Sprintf("resolve resource type: %v", err)
		return result
	}

	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = defaultNamespace
		}
		if ns == "" {
			result.Operation = ApplyOperationError
			result.Error = "namespace is required for this resource"
			return result
		}
		obj.SetNamespace(ns)
		result.Namespace = ns
		ri = dynamicClient.Resource(mapping.Resource).Namespace(ns)
	} else {
		ri = dynamicClient.Resource(mapping.Resource)
	}

	// Checked up front rather than relying on Patch(ApplyPatchType) alone to
	// create a not-yet-existing object: a real API server allows that, but an
	// explicit Create when we already know the object is absent is just as
	// correct and lets this be exercised with the fake dynamic client used in
	// manifest_test.go, whose ObjectTracker requires the target to already
	// exist for any Patch (regardless of patch type).
	_, getErr := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if getErr != nil && !apierrors.IsNotFound(getErr) {
		result.Operation = ApplyOperationError
		result.Error = fmt.Sprintf("check existing resource: %v", getErr)
		return result
	}

	if apierrors.IsNotFound(getErr) {
		createOpts := metav1.CreateOptions{FieldManager: fieldManager}
		if dryRun {
			createOpts.DryRun = []string{metav1.DryRunAll}
		}
		if _, err := ri.Create(ctx, obj, createOpts); err != nil {
			result.Operation = ApplyOperationError
			result.Error = err.Error()
			return result
		}
		result.Operation = ApplyOperationCreated
		return result
	}

	data, err := json.Marshal(obj.Object)
	if err != nil {
		result.Operation = ApplyOperationError
		result.Error = fmt.Sprintf("encode resource: %v", err)
		return result
	}

	patchOpts := metav1.PatchOptions{FieldManager: fieldManager, Force: &force}
	if dryRun {
		patchOpts.DryRun = []string{metav1.DryRunAll}
	}

	if _, err := ri.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, patchOpts); err != nil {
		result.Operation = ApplyOperationError
		result.Error = err.Error()
		return result
	}
	result.Operation = ApplyOperationUpdated
	return result
}
