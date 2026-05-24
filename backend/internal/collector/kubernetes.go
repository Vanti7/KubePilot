package collector

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"gorm.io/datatypes"
)

// KubernetesCollector collects workloads, nodes and helm releases from a single cluster.
type KubernetesCollector struct {
	clusterID string
	clientset *kubernetes.Clientset
	store     *store.Store
	logger    *zap.Logger
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewKubernetesCollector builds a KubernetesCollector for the given cluster model.
// It resolves the kubeconfig from the KubeconfigRef field or falls back to in-cluster config.
func NewKubernetesCollector(cluster *models.Cluster, s *store.Store, logger *zap.Logger) (*KubernetesCollector, error) {
	var restConfig *rest.Config
	var err error

	if cluster.KubeconfigRef != "" {
		// KubeconfigRef is treated as the literal kubeconfig content (base64-encoded or raw YAML).
		raw, decErr := base64.StdEncoding.DecodeString(cluster.KubeconfigRef)
		if decErr != nil {
			// Assume it's already raw YAML.
			raw = []byte(cluster.KubeconfigRef)
		}
		restConfig, err = clientcmd.RESTConfigFromKubeConfig(raw)
	} else if cluster.APIEndpoint != "" && cluster.TLSInsecure {
		// Only use bare endpoint config when TLS verification is explicitly disabled.
		restConfig = &rest.Config{
			Host:            cluster.APIEndpoint,
			TLSClientConfig: rest.TLSClientConfig{Insecure: true},
		}
	} else {
		// In-cluster config: loads SA token + cluster CA cert from pod-mounted secrets.
		restConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("build rest config for cluster %s: %w", cluster.ID, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset for cluster %s: %w", cluster.ID, err)
	}

	return &KubernetesCollector{
		clusterID: cluster.ID.String(),
		clientset: clientset,
		store:     s,
		logger:    logger.With(zap.String("cluster_id", cluster.ID.String()), zap.String("cluster_name", cluster.Name)),
		stopCh:    make(chan struct{}),
	}, nil
}

// Start begins collection goroutines.
func (kc *KubernetesCollector) Start(ctx context.Context) {
	kc.logger.Info("starting kubernetes collector")

	kc.wg.Add(1)
	go func() {
		defer kc.wg.Done()
		kc.runPeriodicCollection(ctx)
	}()
}

// runPeriodicCollection performs a full collection pass every 60 seconds, then watches for changes.
func (kc *KubernetesCollector) runPeriodicCollection(ctx context.Context) {
	kc.collectAll(ctx)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			kc.collectAll(ctx)
		case <-kc.stopCh:
			kc.logger.Info("kubernetes collector stopped")
			return
		case <-ctx.Done():
			return
		}
	}
}

// collectAll runs all collection routines sequentially.
func (kc *KubernetesCollector) collectAll(ctx context.Context) {
	start := time.Now()
	kc.logger.Debug("collection pass started")

	// Update Kubernetes version from server version.
	if sv, err := kc.clientset.Discovery().ServerVersion(); err == nil {
		_ = kc.store.UpdateK8sVersion(ctx, kc.clusterID, sv.GitVersion)
	}

	kc.collectNamespaces(ctx)
	kc.collectNodes(ctx)
	kc.collectWorkloads(ctx)
	kc.collectHelmReleases(ctx)

	// Mark stale resources (not seen in last 10 minutes).
	staleThreshold := time.Now().Add(-10 * time.Minute)
	_ = kc.store.DeleteWorkloadsNotSeenSince(ctx, kc.clusterID, staleThreshold)
	_ = kc.store.DeleteHelmReleasesNotSeenSince(ctx, kc.clusterID, staleThreshold)
	_ = kc.store.DeleteNodesNotSeenSince(ctx, kc.clusterID, staleThreshold)

	_ = kc.store.UpdateClusterStatus(ctx, kc.clusterID, models.ClusterStatusHealthy, time.Now())

	kc.logger.Info("collection pass completed", zap.Duration("duration", time.Since(start)))
}

// collectNamespaces lists namespaces and watches for changes.
func (kc *KubernetesCollector) collectNamespaces(ctx context.Context) {
	list, err := kc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("list namespaces", zap.Error(err))
		return
	}

	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for _, ns := range list.Items {
		labelsJSON, _ := json.Marshal(ns.Labels)
		model := &models.Namespace{
			ClusterID: clusterUUID,
			Name:      ns.Name,
			Status:    string(ns.Status.Phase),
			Labels:    datatypes.JSON(labelsJSON),
		}
		if err := kc.store.UpsertNamespace(ctx, model); err != nil {
			kc.logger.Error("upsert namespace", zap.String("name", ns.Name), zap.Error(err))
		}
	}

	kc.wg.Add(1)
	go func() {
		defer kc.wg.Done()
		kc.watchNamespaces(ctx)
	}()
}

func (kc *KubernetesCollector) watchNamespaces(ctx context.Context) {
	watcher, err := kc.clientset.CoreV1().Namespaces().Watch(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("watch namespaces", zap.Error(err))
		return
	}
	defer watcher.Stop()

	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return
			}
			ns, ok := event.Object.(*corev1.Namespace)
			if !ok {
				continue
			}
			if event.Type == watch.Deleted {
				continue
			}
			labelsJSON, _ := json.Marshal(ns.Labels)
			model := &models.Namespace{
				ClusterID: clusterUUID,
				Name:      ns.Name,
				Status:    string(ns.Status.Phase),
				Labels:    datatypes.JSON(labelsJSON),
			}
			_ = kc.store.UpsertNamespace(ctx, model)
		case <-kc.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// collectNodes lists all nodes and upserts them.
func (kc *KubernetesCollector) collectNodes(ctx context.Context) {
	list, err := kc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("list nodes", zap.Error(err))
		return
	}

	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for _, n := range list.Items {
		model := nodeFromK8s(clusterUUID, &n)
		if err := kc.store.UpsertNode(ctx, model); err != nil {
			kc.logger.Error("upsert node", zap.String("name", n.Name), zap.Error(err))
		}
	}

	kc.wg.Add(1)
	go func() {
		defer kc.wg.Done()
		kc.watchNodes(ctx)
	}()
}

func (kc *KubernetesCollector) watchNodes(ctx context.Context) {
	watcher, err := kc.clientset.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("watch nodes", zap.Error(err))
		return
	}
	defer watcher.Stop()

	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return
			}
			node, ok := event.Object.(*corev1.Node)
			if !ok {
				continue
			}
			if event.Type == watch.Deleted {
				continue
			}
			model := nodeFromK8s(clusterUUID, node)
			_ = kc.store.UpsertNode(ctx, model)
		case <-kc.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// collectWorkloads collects Deployments, DaemonSets and StatefulSets.
func (kc *KubernetesCollector) collectWorkloads(ctx context.Context) {
	clusterUUID, _ := uuid.Parse(kc.clusterID)

	// Deployments
	deployments, err := kc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("list deployments", zap.Error(err))
	} else {
		for _, d := range deployments.Items {
			d := d
			wl := workloadFromDeployment(clusterUUID, &d)
			if err := kc.store.UpsertWorkload(ctx, wl); err != nil {
				kc.logger.Error("upsert deployment", zap.String("name", d.Name), zap.Error(err))
				continue
			}
			kc.upsertContainerImages(ctx, wl, d.Spec.Template.Spec)
		}
	}

	// DaemonSets
	daemonsets, err := kc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("list daemonsets", zap.Error(err))
	} else {
		for _, ds := range daemonsets.Items {
			ds := ds
			wl := workloadFromDaemonSet(clusterUUID, &ds)
			if err := kc.store.UpsertWorkload(ctx, wl); err != nil {
				kc.logger.Error("upsert daemonset", zap.String("name", ds.Name), zap.Error(err))
				continue
			}
			kc.upsertContainerImages(ctx, wl, ds.Spec.Template.Spec)
		}
	}

	// StatefulSets
	statefulsets, err := kc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("list statefulsets", zap.Error(err))
	} else {
		for _, ss := range statefulsets.Items {
			ss := ss
			wl := workloadFromStatefulSet(clusterUUID, &ss)
			if err := kc.store.UpsertWorkload(ctx, wl); err != nil {
				kc.logger.Error("upsert statefulset", zap.String("name", ss.Name), zap.Error(err))
				continue
			}
			kc.upsertContainerImages(ctx, wl, ss.Spec.Template.Spec)
		}
	}
}

// upsertContainerImages extracts container images from a pod spec and upserts them.
func (kc *KubernetesCollector) upsertContainerImages(ctx context.Context, wl *models.Workload, spec corev1.PodSpec) {
	for _, c := range spec.Containers {
		img := parseImage(wl.ID, c.Name, c.Image, false)
		if err := kc.store.UpsertContainerImage(ctx, img); err != nil {
			kc.logger.Error("upsert container image", zap.String("image", c.Image), zap.Error(err))
		}
	}
	for _, c := range spec.InitContainers {
		img := parseImage(wl.ID, c.Name, c.Image, true)
		if err := kc.store.UpsertContainerImage(ctx, img); err != nil {
			kc.logger.Error("upsert init container image", zap.String("image", c.Image), zap.Error(err))
		}
	}
}

// collectHelmReleases reads Helm 3 secrets from all namespaces and upserts releases.
func (kc *KubernetesCollector) collectHelmReleases(ctx context.Context) {
	secrets, err := kc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm,status=deployed",
	})
	if err != nil {
		kc.logger.Error("list helm secrets", zap.Error(err))
		return
	}

	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for _, secret := range secrets.Items {
		release, err := decodeHelmSecret(&secret)
		if err != nil {
			kc.logger.Warn("decode helm secret", zap.String("secret", secret.Name), zap.Error(err))
			continue
		}
		release.ClusterID = clusterUUID
		if err := kc.store.UpsertHelmRelease(ctx, release); err != nil {
			kc.logger.Error("upsert helm release", zap.String("release", release.Name), zap.Error(err))
		}
	}
}

// Stop signals the collector to shut down and waits for goroutines to finish.
func (kc *KubernetesCollector) Stop() {
	close(kc.stopCh)
	kc.wg.Wait()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nodeFromK8s(clusterID uuid.UUID, n *corev1.Node) *models.Node {
	role := "worker"
	if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
		role = "master"
	}
	if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
		role = "control-plane"
	}

	status := "Ready"
	for _, cond := range n.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
			status = "NotReady"
		}
	}

	labelsJSON, _ := json.Marshal(n.Labels)
	taintsJSON, _ := json.Marshal(n.Spec.Taints)
	conditionsJSON, _ := json.Marshal(n.Status.Conditions)

	return &models.Node{
		ID:               uuid.New(),
		ClusterID:        clusterID,
		Name:             n.Name,
		Role:             role,
		Status:           status,
		K8sVersion:       n.Status.NodeInfo.KubeletVersion,
		OSImage:          n.Status.NodeInfo.OSImage,
		KernelVersion:    n.Status.NodeInfo.KernelVersion,
		ContainerRuntime: n.Status.NodeInfo.ContainerRuntimeVersion,
		Arch:             n.Status.NodeInfo.Architecture,
		CapacityCPU:      n.Status.Capacity.Cpu().String(),
		CapacityMemory:   n.Status.Capacity.Memory().String(),
		AllocatableCPU:   n.Status.Allocatable.Cpu().String(),
		AllocatableMemory: n.Status.Allocatable.Memory().String(),
		Labels:           datatypes.JSON(labelsJSON),
		Taints:           datatypes.JSON(taintsJSON),
		Conditions:       datatypes.JSON(conditionsJSON),
	}
}

func workloadFromDeployment(clusterID uuid.UUID, d *appsv1.Deployment) *models.Workload {
	health := healthFromReplicas(d.Status.ReadyReplicas, *d.Spec.Replicas)
	labelsJSON, _ := json.Marshal(d.Labels)
	annotationsJSON, _ := json.Marshal(d.Annotations)

	return &models.Workload{
		ID:              uuid.New(),
		ClusterID:       clusterID,
		Name:            d.Name,
		NamespaceName:   d.Namespace,
		Kind:            models.WorkloadKindDeployment,
		ReplicasDesired: *d.Spec.Replicas,
		ReplicasReady:   d.Status.ReadyReplicas,
		HealthStatus:    health,
		Labels:          datatypes.JSON(labelsJSON),
		Annotations:     datatypes.JSON(annotationsJSON),
	}
}

func workloadFromDaemonSet(clusterID uuid.UUID, ds *appsv1.DaemonSet) *models.Workload {
	desired := ds.Status.DesiredNumberScheduled
	ready := ds.Status.NumberReady
	health := healthFromReplicas(ready, desired)
	labelsJSON, _ := json.Marshal(ds.Labels)
	annotationsJSON, _ := json.Marshal(ds.Annotations)

	return &models.Workload{
		ID:              uuid.New(),
		ClusterID:       clusterID,
		Name:            ds.Name,
		NamespaceName:   ds.Namespace,
		Kind:            models.WorkloadKindDaemonSet,
		ReplicasDesired: desired,
		ReplicasReady:   ready,
		HealthStatus:    health,
		Labels:          datatypes.JSON(labelsJSON),
		Annotations:     datatypes.JSON(annotationsJSON),
	}
}

func workloadFromStatefulSet(clusterID uuid.UUID, ss *appsv1.StatefulSet) *models.Workload {
	health := healthFromReplicas(ss.Status.ReadyReplicas, *ss.Spec.Replicas)
	labelsJSON, _ := json.Marshal(ss.Labels)
	annotationsJSON, _ := json.Marshal(ss.Annotations)

	return &models.Workload{
		ID:              uuid.New(),
		ClusterID:       clusterID,
		Name:            ss.Name,
		NamespaceName:   ss.Namespace,
		Kind:            models.WorkloadKindStatefulSet,
		ReplicasDesired: *ss.Spec.Replicas,
		ReplicasReady:   ss.Status.ReadyReplicas,
		HealthStatus:    health,
		Labels:          datatypes.JSON(labelsJSON),
		Annotations:     datatypes.JSON(annotationsJSON),
	}
}

func healthFromReplicas(ready, desired int32) string {
	if desired == 0 {
		return models.WorkloadHealthUnknown
	}
	if ready == desired {
		return models.WorkloadHealthHealthy
	}
	if ready == 0 {
		return models.WorkloadHealthDegraded
	}
	return models.WorkloadHealthWarning
}

// parseImage splits an image reference into registry, repository, tag components.
func parseImage(workloadID uuid.UUID, containerName, imageRef string, isInit bool) *models.ContainerImage {
	registry := "docker.io"
	repository := imageRef
	tag := "latest"

	// Split tag.
	if idx := strings.LastIndex(imageRef, ":"); idx != -1 && !strings.Contains(imageRef[idx:], "/") {
		tag = imageRef[idx+1:]
		imageRef = imageRef[:idx]
	}

	// Digest notation.
	if idx := strings.Index(imageRef, "@"); idx != -1 {
		tag = imageRef[idx+1:]
		imageRef = imageRef[:idx]
	}

	// Split registry from repository.
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repository = parts[1]
	} else {
		repository = imageRef
	}

	return &models.ContainerImage{
		ID:              uuid.New(),
		WorkloadID:      workloadID,
		ContainerName:   containerName,
		Image:           fmt.Sprintf("%s/%s:%s", registry, repository, tag),
		Registry:        registry,
		Repository:      repository,
		Tag:             tag,
		IsInitContainer: isInit,
	}
}

// helmRelease is the minimal Helm release structure we decode from secrets.
type helmRelease struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Info    struct {
		Status      string    `json:"status"`
		LastDeployed time.Time `json:"last_deployed"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
	Namespace string          `json:"namespace"`
	Config    json.RawMessage `json:"config"`
}

// decodeHelmSecret decodes a Helm 3 storage secret into a HelmRelease model.
func decodeHelmSecret(secret *corev1.Secret) (*models.HelmRelease, error) {
	data, ok := secret.Data["release"]
	if !ok {
		return nil, fmt.Errorf("secret %s has no 'release' key", secret.Name)
	}

	// Base64 decode (Helm double-encodes).
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		decoded = data // Already decoded.
	}

	// Gzip decompress.
	gr, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("gzip open: %w", err)
	}
	defer gr.Close()

	raw, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}

	var hr helmRelease
	if err := json.Unmarshal(raw, &hr); err != nil {
		return nil, fmt.Errorf("unmarshal release: %w", err)
	}

	var values datatypes.JSON
	if hr.Config != nil {
		values = datatypes.JSON(hr.Config)
	} else {
		values = datatypes.JSON([]byte("{}"))
	}

	lastDeployed := hr.Info.LastDeployed
	return &models.HelmRelease{
		Name:           hr.Name,
		NamespaceName:  hr.Namespace,
		ChartName:      hr.Chart.Metadata.Name,
		ChartVersion:   hr.Chart.Metadata.Version,
		AppVersion:     hr.Chart.Metadata.AppVersion,
		Status:         hr.Info.Status,
		Revision:       hr.Version,
		Values:         values,
		LastDeployedAt: &lastDeployed,
	}, nil
}
