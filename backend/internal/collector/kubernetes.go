package collector

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	sshTunnel *sshTunnel // non-nil only for ssh connection mode; closed on Stop

	metrics   MetricsConfig
	prevNetMu sync.Mutex
	prevNet   map[string]netSample // node name -> last network counters, for rate calc
}

// netSample holds the cumulative network counters from a node's previous metric
// sample, used to derive an instantaneous throughput rate.
type netSample struct {
	rxBytes int64
	txBytes int64
	ts      time.Time
}

// NewKubernetesCollector builds a KubernetesCollector for the given cluster model.
// It resolves the kubeconfig from the KubeconfigRef field or falls back to in-cluster config.
func NewKubernetesCollector(cluster *models.Cluster, s *store.Store, logger *zap.Logger, metrics MetricsConfig) (*KubernetesCollector, error) {
	var restConfig *rest.Config
	var err error
	var tunnel *sshTunnel

	if cluster.ConnectionMode == models.ClusterConnSSH || cluster.SSHHost != "" {
		// SSH mode: connect to the node, read its kubeconfig, then tunnel all
		// Kubernetes API traffic through a self-healing SSH connection.
		tunnel, err = newSSHTunnel(cluster, logger)
		if err != nil {
			return nil, fmt.Errorf("ssh connect for cluster %s: %w", cluster.ID, err)
		}
		client, cErr := tunnel.ensureClient()
		if cErr != nil {
			tunnel.Close()
			return nil, fmt.Errorf("ssh client for cluster %s: %w", cluster.ID, cErr)
		}
		kubeconfigBytes, kErr := fetchRemoteKubeconfig(client, cluster)
		if kErr != nil {
			tunnel.Close()
			return nil, fmt.Errorf("fetch remote kubeconfig for cluster %s: %w", cluster.ID, kErr)
		}
		restConfig, err = clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
		if err != nil {
			tunnel.Close()
			return nil, fmt.Errorf("parse remote kubeconfig for cluster %s: %w", cluster.ID, err)
		}
		restConfig.Dial = tunnel.Dial
	} else if cluster.KubeconfigRef != "" {
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
		if tunnel != nil {
			tunnel.Close()
		}
		return nil, fmt.Errorf("build rest config for cluster %s: %w", cluster.ID, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		if tunnel != nil {
			tunnel.Close()
		}
		return nil, fmt.Errorf("create clientset for cluster %s: %w", cluster.ID, err)
	}

	return &KubernetesCollector{
		clusterID: cluster.ID.String(),
		clientset: clientset,
		store:     s,
		logger:    logger.With(zap.String("cluster_id", cluster.ID.String()), zap.String("cluster_name", cluster.Name)),
		stopCh:    make(chan struct{}),
		sshTunnel: tunnel,
		metrics:   metrics,
		prevNet:   make(map[string]netSample),
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

	// Start the long-lived resource watchers exactly once. Each watcher reconnects
	// internally when the API server closes its watch channel, so they must not be
	// re-spawned on every collection pass (that previously leaked goroutines).
	kc.wg.Add(3)
	go func() { defer kc.wg.Done(); kc.watchNamespaces(ctx) }()
	go func() { defer kc.wg.Done(); kc.watchNodes(ctx) }()
	go func() { defer kc.wg.Done(); kc.watchSecrets(ctx) }()

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

// Collect triggers an immediate, one-off collection pass (used by the manual sync endpoint).
func (kc *KubernetesCollector) Collect(ctx context.Context) {
	kc.collectAll(ctx)
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
	kc.collectNodeMetrics(ctx)
	kc.collectWorkloads(ctx)
	kc.collectHelmReleases(ctx)
	kc.collectSecrets(ctx)

	// Mark stale resources (not seen in last 10 minutes).
	staleThreshold := time.Now().Add(-10 * time.Minute)
	_ = kc.store.DeleteWorkloadsNotSeenSince(ctx, kc.clusterID, staleThreshold)
	_ = kc.store.DeleteHelmReleasesNotSeenSince(ctx, kc.clusterID, staleThreshold)
	_ = kc.store.DeleteNodesNotSeenSince(ctx, kc.clusterID, staleThreshold)
	_ = kc.store.DeleteSecretsNotSeenSince(ctx, kc.clusterID, staleThreshold)

	// Purge node-metric samples beyond the retention window.
	if kc.metrics.Enabled && kc.metrics.Retention > 0 {
		_ = kc.store.DeleteNodeMetricsBefore(ctx, time.Now().Add(-kc.metrics.Retention))
	}

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
}

// watchNamespaces watches namespace changes, reconnecting whenever the API server
// closes the watch channel, until the collector is stopped.
func (kc *KubernetesCollector) watchNamespaces(ctx context.Context) {
	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for {
		if kc.stopping(ctx) {
			return
		}
		watcher, err := kc.clientset.CoreV1().Namespaces().Watch(ctx, metav1.ListOptions{})
		if err != nil {
			kc.logger.Error("watch namespaces", zap.Error(err))
			if !kc.waitBeforeRetry(ctx) {
				return
			}
			continue
		}

		drained := func() bool {
			defer watcher.Stop()
			for {
				select {
				case event, ok := <-watcher.ResultChan():
					if !ok {
						return true // channel closed — reconnect
					}
					ns, ok := event.Object.(*corev1.Namespace)
					if !ok || event.Type == watch.Deleted {
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
					return false
				case <-ctx.Done():
					return false
				}
			}
		}()
		if !drained {
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
}

// kubeletSummary is the subset of the kubelet /stats/summary response we consume.
type kubeletSummary struct {
	Node struct {
		NodeName string `json:"nodeName"`
		CPU      struct {
			UsageNanoCores int64 `json:"usageNanoCores"`
		} `json:"cpu"`
		Memory struct {
			WorkingSetBytes int64 `json:"workingSetBytes"`
			UsageBytes      int64 `json:"usageBytes"`
		} `json:"memory"`
		Fs struct {
			UsedBytes     int64 `json:"usedBytes"`
			CapacityBytes int64 `json:"capacityBytes"`
		} `json:"fs"`
		Network struct {
			RxBytes int64 `json:"rxBytes"`
			TxBytes int64 `json:"txBytes"`
		} `json:"network"`
	} `json:"node"`
	Pods []json.RawMessage `json:"pods"`
}

// collectNodeMetrics samples each node's resource usage from the kubelet Summary
// API (proxied through the API server, so it also flows over the SSH tunnel) and
// appends a time-series row. Per-node failures are logged at debug level and do
// not abort the pass — some kubelets restrict the summary endpoint.
func (kc *KubernetesCollector) collectNodeMetrics(ctx context.Context) {
	if !kc.metrics.Enabled {
		return
	}

	nodes, err := kc.store.ListNodes(ctx, kc.clusterID)
	if err != nil {
		kc.logger.Error("list nodes for metrics", zap.Error(err))
		return
	}

	for _, node := range nodes {
		raw, err := kc.clientset.CoreV1().RESTClient().Get().
			AbsPath("/api/v1/nodes/" + node.Name + "/proxy/stats/summary").
			DoRaw(ctx)
		if err != nil {
			kc.logger.Debug("fetch node summary", zap.String("node", node.Name), zap.Error(err))
			continue
		}

		var summary kubeletSummary
		if err := json.Unmarshal(raw, &summary); err != nil {
			kc.logger.Debug("decode node summary", zap.String("node", node.Name), zap.Error(err))
			continue
		}

		m := kc.buildNodeMetric(&node, &summary)
		if err := kc.store.InsertNodeMetric(ctx, m); err != nil {
			kc.logger.Error("insert node metric", zap.String("node", node.Name), zap.Error(err))
		}
	}
}

// buildNodeMetric converts a kubelet summary into a NodeMetric, computing usage
// percentages against the node's capacity and network rates against the previous
// sample.
func (kc *KubernetesCollector) buildNodeMetric(node *models.Node, s *kubeletSummary) *models.NodeMetric {
	now := time.Now()

	m := &models.NodeMetric{
		ClusterID:             node.ClusterID,
		NodeID:                node.ID,
		NodeName:              node.Name,
		Timestamp:             now,
		CPUUsageNanoCores:     s.Node.CPU.UsageNanoCores,
		MemoryWorkingSetBytes: s.Node.Memory.WorkingSetBytes,
		MemoryUsageBytes:      s.Node.Memory.UsageBytes,
		FSUsedBytes:           s.Node.Fs.UsedBytes,
		FSCapacityBytes:       s.Node.Fs.CapacityBytes,
		NetworkRxBytes:        s.Node.Network.RxBytes,
		NetworkTxBytes:        s.Node.Network.TxBytes,
		PodsRunning:           len(s.Pods),
	}

	if capNano := parseCPUNanoCores(node.CapacityCPU); capNano > 0 {
		m.CPUUsagePercent = round2(float64(s.Node.CPU.UsageNanoCores) / float64(capNano) * 100)
	}
	if capBytes := parseMemoryBytes(node.CapacityMemory); capBytes > 0 {
		m.MemoryUsagePercent = round2(float64(s.Node.Memory.WorkingSetBytes) / float64(capBytes) * 100)
	}
	if s.Node.Fs.CapacityBytes > 0 {
		m.FSUsedPercent = round2(float64(s.Node.Fs.UsedBytes) / float64(s.Node.Fs.CapacityBytes) * 100)
	}

	kc.prevNetMu.Lock()
	if prev, ok := kc.prevNet[node.Name]; ok {
		if dt := now.Sub(prev.ts).Seconds(); dt > 0 {
			// Guard against counter resets (node/kubelet restart) producing negatives.
			if d := s.Node.Network.RxBytes - prev.rxBytes; d >= 0 {
				m.NetworkRxRate = round2(float64(d) / dt)
			}
			if d := s.Node.Network.TxBytes - prev.txBytes; d >= 0 {
				m.NetworkTxRate = round2(float64(d) / dt)
			}
		}
	}
	kc.prevNet[node.Name] = netSample{rxBytes: s.Node.Network.RxBytes, txBytes: s.Node.Network.TxBytes, ts: now}
	kc.prevNetMu.Unlock()

	return m
}

// parseCPUNanoCores converts a Kubernetes CPU quantity (e.g. "4", "500m") to nanocores.
func parseCPUNanoCores(q string) int64 {
	if q == "" {
		return 0
	}
	qty, err := resource.ParseQuantity(q)
	if err != nil {
		return 0
	}
	// MilliValue is millicores; 1 millicore = 1e6 nanocores.
	return qty.MilliValue() * 1e6
}

// parseMemoryBytes converts a Kubernetes memory quantity (e.g. "16331252Ki") to bytes.
func parseMemoryBytes(q string) int64 {
	if q == "" {
		return 0
	}
	qty, err := resource.ParseQuantity(q)
	if err != nil {
		return 0
	}
	return qty.Value()
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

// watchNodes watches node changes, reconnecting on channel close until stopped.
func (kc *KubernetesCollector) watchNodes(ctx context.Context) {
	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for {
		if kc.stopping(ctx) {
			return
		}
		watcher, err := kc.clientset.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{})
		if err != nil {
			kc.logger.Error("watch nodes", zap.Error(err))
			if !kc.waitBeforeRetry(ctx) {
				return
			}
			continue
		}

		drained := func() bool {
			defer watcher.Stop()
			for {
				select {
				case event, ok := <-watcher.ResultChan():
					if !ok {
						return true
					}
					node, ok := event.Object.(*corev1.Node)
					if !ok || event.Type == watch.Deleted {
						continue
					}
					model := nodeFromK8s(clusterUUID, node)
					_ = kc.store.UpsertNode(ctx, model)
				case <-kc.stopCh:
					return false
				case <-ctx.Done():
					return false
				}
			}
		}()
		if !drained {
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

// collectSecrets lists all secrets (excluding Helm storage secrets) and upserts metadata.
// Secret values are intentionally never read or stored.
func (kc *KubernetesCollector) collectSecrets(ctx context.Context) {
	list, err := kc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		kc.logger.Error("list secrets", zap.Error(err))
		return
	}

	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for _, s := range list.Items {
		s := s
		model := secretFromK8s(clusterUUID, &s)
		if model == nil {
			continue
		}
		if err := kc.store.UpsertSecret(ctx, model); err != nil {
			kc.logger.Error("upsert secret", zap.String("name", s.Name), zap.Error(err))
		}
	}
}

// watchSecrets watches secret changes, reconnecting on channel close until stopped.
func (kc *KubernetesCollector) watchSecrets(ctx context.Context) {
	clusterUUID, _ := uuid.Parse(kc.clusterID)

	for {
		if kc.stopping(ctx) {
			return
		}
		watcher, err := kc.clientset.CoreV1().Secrets("").Watch(ctx, metav1.ListOptions{})
		if err != nil {
			kc.logger.Error("watch secrets", zap.Error(err))
			if !kc.waitBeforeRetry(ctx) {
				return
			}
			continue
		}

		drained := func() bool {
			defer watcher.Stop()
			for {
				select {
				case event, ok := <-watcher.ResultChan():
					if !ok {
						return true
					}
					secret, ok := event.Object.(*corev1.Secret)
					if !ok || event.Type == watch.Deleted {
						continue
					}
					model := secretFromK8s(clusterUUID, secret)
					if model == nil {
						continue
					}
					_ = kc.store.UpsertSecret(ctx, model)
				case <-kc.stopCh:
					return false
				case <-ctx.Done():
					return false
				}
			}
		}()
		if !drained {
			return
		}
	}
}

// stopping reports whether the collector has been asked to stop.
func (kc *KubernetesCollector) stopping(ctx context.Context) bool {
	select {
	case <-kc.stopCh:
		return true
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// waitBeforeRetry sleeps briefly before a watch reconnect attempt.
// It returns false if the collector is stopped while waiting.
func (kc *KubernetesCollector) waitBeforeRetry(ctx context.Context) bool {
	select {
	case <-time.After(5 * time.Second):
		return true
	case <-kc.stopCh:
		return false
	case <-ctx.Done():
		return false
	}
}

// Stop signals the collector to shut down and waits for goroutines to finish.
func (kc *KubernetesCollector) Stop() {
	close(kc.stopCh)
	kc.wg.Wait()
	if kc.sshTunnel != nil {
		_ = kc.sshTunnel.Close()
	}
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
	desired := replicaCount(d.Spec.Replicas)
	health := healthFromReplicas(d.Status.ReadyReplicas, desired)
	labelsJSON, _ := json.Marshal(d.Labels)
	annotationsJSON, _ := json.Marshal(d.Annotations)

	return &models.Workload{
		ID:              uuid.New(),
		ClusterID:       clusterID,
		Name:            d.Name,
		NamespaceName:   d.Namespace,
		Kind:            models.WorkloadKindDeployment,
		ReplicasDesired: desired,
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
	desired := replicaCount(ss.Spec.Replicas)
	health := healthFromReplicas(ss.Status.ReadyReplicas, desired)
	labelsJSON, _ := json.Marshal(ss.Labels)
	annotationsJSON, _ := json.Marshal(ss.Annotations)

	return &models.Workload{
		ID:              uuid.New(),
		ClusterID:       clusterID,
		Name:            ss.Name,
		NamespaceName:   ss.Namespace,
		Kind:            models.WorkloadKindStatefulSet,
		ReplicasDesired: desired,
		ReplicasReady:   ss.Status.ReadyReplicas,
		HealthStatus:    health,
		Labels:          datatypes.JSON(labelsJSON),
		Annotations:     datatypes.JSON(annotationsJSON),
	}
}

// replicaCount safely dereferences a *int32 replica field. The Kubernetes API
// leaves Spec.Replicas nil to mean "default to 1", so a nil pointer must not panic.
func replicaCount(r *int32) int32 {
	if r == nil {
		return 1
	}
	return *r
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

// secretFromK8s builds a Secret model from a K8s secret.
// Returns nil for Helm storage secrets (owner=helm) to avoid duplication.
func secretFromK8s(clusterID uuid.UUID, s *corev1.Secret) *models.Secret {
	if s.Labels["owner"] == "helm" {
		return nil
	}

	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	keysJSON, _ := json.Marshal(keys)

	k8sCreated := s.CreationTimestamp.Time
	k8sUpdated := lastManagedFieldTime(s.ManagedFields)

	return &models.Secret{
		ID:            uuid.New(),
		ClusterID:     clusterID,
		NamespaceName: s.Namespace,
		Name:          s.Name,
		Type:          string(s.Type),
		Keys:          datatypes.JSON(keysJSON),
		K8sCreatedAt:  &k8sCreated,
		K8sUpdatedAt:  k8sUpdated,
	}
}

// lastManagedFieldTime returns the most recent time across all managedFields entries, or nil.
func lastManagedFieldTime(fields []metav1.ManagedFieldsEntry) *time.Time {
	var latest time.Time
	for _, mf := range fields {
		if mf.Time != nil && mf.Time.Time.After(latest) {
			latest = mf.Time.Time
		}
	}
	if latest.IsZero() {
		return nil
	}
	return &latest
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
