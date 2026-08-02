package collector

import (
	"context"
	"sync"
	"time"

	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// MetricsConfig controls node-metrics collection behaviour, shared by every
// collector the manager spawns.
type MetricsConfig struct {
	Enabled   bool
	Retention time.Duration
}

// CollectorManager starts and stops KubernetesCollectors as clusters are added/removed.
type CollectorManager struct {
	store      *store.Store
	logger     *zap.Logger
	metrics    MetricsConfig
	collectors map[string]*KubernetesCollector
	mu         sync.Mutex
	stopCh     chan struct{}
}

// NewCollectorManager creates a new CollectorManager.
func NewCollectorManager(s *store.Store, logger *zap.Logger, metrics MetricsConfig) *CollectorManager {
	return &CollectorManager{
		store:      s,
		logger:     logger,
		metrics:    metrics,
		collectors: make(map[string]*KubernetesCollector),
		stopCh:     make(chan struct{}),
	}
}

// SyncClusters reconciles running collectors against the clusters currently in the database.
// New clusters get a new collector; removed clusters have their collector stopped.
func (m *CollectorManager) SyncClusters(ctx context.Context) {
	clusters, err := m.store.ListClusters(ctx)
	if err != nil {
		m.logger.Error("collector manager: list clusters", zap.Error(err))
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	desired := make(map[string]struct{}, len(clusters))
	for _, cl := range clusters {
		idStr := cl.ID.String()
		desired[idStr] = struct{}{}

		if _, running := m.collectors[idStr]; running {
			continue
		}

		// Start a new collector for this cluster.
		cl := cl // capture loop var
		kc, err := NewKubernetesCollector(&cl, m.store, m.logger, m.metrics)
		if err != nil {
			m.logger.Error("create collector",
				zap.String("cluster_id", idStr),
				zap.String("cluster_name", cl.Name),
				zap.Error(err),
			)
			// Mark cluster as unreachable so the UI can surface it.
			_ = m.store.UpdateClusterStatus(ctx, idStr, "unreachable", time.Now())
			continue
		}

		m.collectors[idStr] = kc
		kc.Start(ctx)
		m.logger.Info("collector started", zap.String("cluster_id", idStr), zap.String("cluster_name", cl.Name))
	}

	// Stop collectors for clusters that no longer exist in the DB.
	for idStr, kc := range m.collectors {
		if _, ok := desired[idStr]; !ok {
			kc.Stop()
			delete(m.collectors, idStr)
			m.logger.Info("collector stopped (cluster removed)", zap.String("cluster_id", idStr))
		}
	}
}

// TriggerSync runs an immediate collection pass for a single cluster, if a collector
// is currently active for it. The collection runs in the background so callers (e.g. the
// HTTP sync endpoint) do not block. Returns false if no collector is running for the cluster.
func (m *CollectorManager) TriggerSync(clusterID string) bool {
	m.mu.Lock()
	kc, ok := m.collectors[clusterID]
	m.mu.Unlock()
	if !ok {
		return false
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		kc.Collect(ctx)
	}()
	return true
}

// GetClientset returns the live Kubernetes client for a cluster with an
// active collector — the same long-lived clientset (and, for ssh-mode
// clusters, the same already-open tunnel) the collector itself watches with.
// Returns false if no collector is currently running for that cluster.
func (m *CollectorManager) GetClientset(clusterID string) (kubernetes.Interface, bool) {
	m.mu.Lock()
	kc, ok := m.collectors[clusterID]
	m.mu.Unlock()
	if !ok {
		return nil, false
	}
	return kc.clientset, true
}

// GetRESTConfig returns the live *rest.Config for a cluster with an active
// collector — the same config the clientset was built from (same SSH tunnel
// dialer for ssh-mode clusters). Callers that need more than the typed
// clientset (e.g. helmops, which needs a discovery client and RESTMapper)
// use this instead of GetClientset. Returns false if no collector is
// currently running for that cluster.
func (m *CollectorManager) GetRESTConfig(clusterID string) (*rest.Config, bool) {
	m.mu.Lock()
	kc, ok := m.collectors[clusterID]
	m.mu.Unlock()
	if !ok {
		return nil, false
	}
	return kc.restConfig, true
}

// RunCron periodically calls SyncClusters and blocks until ctx is cancelled or Stop is called.
func (m *CollectorManager) RunCron(ctx context.Context) {
	// Perform an immediate sync on start.
	m.SyncClusters(ctx)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.SyncClusters(ctx)
		case <-m.stopCh:
			m.logger.Info("collector manager stopping")
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop halts all managed collectors and the cron loop.
func (m *CollectorManager) Stop() {
	close(m.stopCh)

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, kc := range m.collectors {
		kc.Stop()
		m.logger.Info("collector stopped", zap.String("cluster_id", id))
	}
	m.collectors = make(map[string]*KubernetesCollector)
}
