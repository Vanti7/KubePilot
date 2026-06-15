package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ensureDemoData seeds a realistic, multi-cluster synthetic dataset so the whole
// UI (overview, nodes + metrics, inventory, helm, updates) is populated without a
// real cluster. Idempotent: it does nothing once any cluster exists.
func ensureDemoData(ctx context.Context, db *gorm.DB, logger *zap.Logger) error {
	var clusterCount int64
	db.WithContext(ctx).Model(&models.Cluster{}).Count(&clusterCount)
	if clusterCount > 0 {
		logger.Info("demo data already present, skipping seed")
		return nil
	}

	var envs []models.Environment
	if err := db.WithContext(ctx).Find(&envs).Error; err != nil {
		return err
	}
	envBySlug := make(map[string]uuid.UUID, len(envs))
	for _, e := range envs {
		envBySlug[e.Slug] = e.ID
	}
	envPtr := func(slug string) *uuid.UUID {
		if id, ok := envBySlug[slug]; ok {
			return &id
		}
		return nil
	}

	now := time.Now()
	rng := rand.New(rand.NewSource(42)) // deterministic across runs

	type nodeSpec struct {
		name    string
		role    string
		cores   int
		memGi   int
		fsCapGi int
		cpuPct  float64
		memPct  float64
		fsPct   float64
	}
	type clusterSpec struct {
		name, slug, provider, region, k8sVersion, osImage, kernel, runtime, env string
		nodes                                                                   []nodeSpec
		namespaces                                                              []string
	}

	specs := []clusterSpec{
		{
			name: "prod-eu-west", slug: "prod-eu-west", provider: "bare-metal", region: "eu-west-1",
			k8sVersion: "v1.29.6", osImage: "Ubuntu 22.04.4 LTS", kernel: "5.15.0-105-generic", runtime: "containerd://1.7.18",
			env: "prod", namespaces: []string{"kube-system", "ingress-nginx", "monitoring", "payments", "storefront"},
			nodes: []nodeSpec{
				{"prod-cp-1", "control-plane", 4, 8, 100, 18, 42, 31},
				{"prod-w-1", "worker", 16, 64, 400, 67, 71, 58},
				{"prod-w-2", "worker", 16, 64, 400, 54, 63, 49},
				{"prod-w-3", "worker", 16, 64, 400, 88, 79, 64},
			},
		},
		{
			name: "staging", slug: "staging", provider: "bare-metal", region: "eu-west-1",
			k8sVersion: "v1.30.2", osImage: "Debian GNU/Linux 12 (bookworm)", kernel: "6.1.0-21-amd64", runtime: "containerd://1.7.20",
			env: "staging", namespaces: []string{"kube-system", "ingress-nginx", "apps"},
			nodes: []nodeSpec{
				{"stg-cp-1", "control-plane", 4, 8, 80, 12, 35, 27},
				{"stg-w-1", "worker", 8, 32, 200, 41, 52, 44},
				{"stg-w-2", "worker", 8, 32, 200, 33, 47, 38},
			},
		},
		{
			name: "homelab", slug: "homelab", provider: "k3s", region: "home",
			k8sVersion: "v1.32.13", osImage: "Debian GNU/Linux 13 (trixie)", kernel: "6.12.74+deb13+1-amd64", runtime: "containerd://1.7.24",
			env: "dev", namespaces: []string{"kube-system", "media", "home-automation", "default"},
			nodes: []nodeSpec{
				{"fo-k8s-cp", "control-plane", 4, 8, 60, 5, 22, 5},
				{"fo-k8s-pp", "worker", 4, 8, 120, 16, 29, 13},
				{"fo-k8s-prd", "worker", 4, 8, 120, 14, 22, 38},
				{"emma-docker", "worker", 8, 16, 200, 9, 46, 33},
			},
		},
	}

	for _, cs := range specs {
		cl := &models.Cluster{
			ID:             uuid.New(),
			EnvironmentID:  envPtr(cs.env),
			Name:           cs.name,
			Slug:           cs.slug,
			Provider:       cs.provider,
			Region:         cs.region,
			K8sVersion:     cs.k8sVersion,
			Status:         models.ClusterStatusHealthy,
			LastSeenAt:     &now,
			ConnectionMode: models.ClusterConnKubeconfig,
		}
		if err := db.WithContext(ctx).Create(cl).Error; err != nil {
			return fmt.Errorf("create demo cluster %s: %w", cs.name, err)
		}

		for _, nsName := range cs.namespaces {
			ns := &models.Namespace{
				ID:        uuid.New(),
				ClusterID: cl.ID,
				Name:      nsName,
				Status:    "Active",
				Labels:    datatypes.JSON([]byte("{}")),
			}
			if err := db.WithContext(ctx).Create(ns).Error; err != nil {
				return fmt.Errorf("create demo namespace: %w", err)
			}
		}

		for _, ns := range cs.nodes {
			node := &models.Node{
				ID:                uuid.New(),
				ClusterID:         cl.ID,
				Name:              ns.name,
				Role:              ns.role,
				Status:            "Ready",
				K8sVersion:        cs.k8sVersion,
				OSImage:           cs.osImage,
				KernelVersion:     cs.kernel,
				ContainerRuntime:  cs.runtime,
				Arch:              "amd64",
				CapacityCPU:       fmt.Sprintf("%d", ns.cores),
				CapacityMemory:    fmt.Sprintf("%dGi", ns.memGi),
				AllocatableCPU:    fmt.Sprintf("%d", ns.cores),
				AllocatableMemory: fmt.Sprintf("%dGi", ns.memGi),
				Conditions:        nodeConditionsJSON(),
				Labels:            datatypes.JSON([]byte("{}")),
				Taints:            datatypes.JSON([]byte("[]")),
			}
			if err := db.WithContext(ctx).Create(node).Error; err != nil {
				return fmt.Errorf("create demo node %s: %w", ns.name, err)
			}

			samples := buildNodeMetricSeries(cl.ID, node, now, rng, ns.cores, int64(ns.memGi)*(1<<30), int64(ns.fsCapGi)*(1<<30), ns.cpuPct, ns.memPct, ns.fsPct)
			if err := db.WithContext(ctx).CreateInBatches(samples, 100).Error; err != nil {
				return fmt.Errorf("create demo node metrics: %w", err)
			}
		}

		if err := seedDemoFindings(ctx, db, cl, cs.namespaces, now, rng); err != nil {
			return err
		}
	}

	logger.Info("demo data seeded", zap.Int("clusters", len(specs)))
	return nil
}

// nodeConditionsJSON returns a healthy node's condition set.
func nodeConditionsJSON() datatypes.JSON {
	conds := []map[string]string{
		{"type": "Ready", "status": "True", "reason": "KubeletReady"},
		{"type": "MemoryPressure", "status": "False", "reason": "KubeletHasSufficientMemory"},
		{"type": "DiskPressure", "status": "False", "reason": "KubeletHasNoDiskPressure"},
		{"type": "PIDPressure", "status": "False", "reason": "KubeletHasSufficientPID"},
	}
	return jsonOf(conds)
}

// buildNodeMetricSeries generates ~6h of samples (every 10 min) oscillating
// around the target usage percentages, with absolute values consistent with the
// node's capacity.
func buildNodeMetricSeries(clusterID uuid.UUID, node *models.Node, now time.Time, rng *rand.Rand, cores int, memBytes, fsCapBytes int64, cpuPct, memPct, fsPct float64) []models.NodeMetric {
	const points = 37 // 6h at 10-min steps, inclusive
	capNano := float64(cores) * 1e9
	out := make([]models.NodeMetric, 0, points)

	var prevRx, prevTx int64
	for k := points - 1; k >= 0; k-- {
		ts := now.Add(-time.Duration(k) * 10 * time.Minute)

		cpu := clampPct(cpuPct + 6*math.Sin(float64(k)/3) + (rng.Float64()-0.5)*4)
		mem := clampPct(memPct + 3*math.Sin(float64(k)/5) + (rng.Float64()-0.5)*2)
		fs := clampPct(fsPct + float64(points-k)*0.02) // slow upward drift

		rxBytes := prevRx + int64((2_000_000+rng.Float64()*8_000_000))   // ~2-10 MB/10min
		txBytes := prevTx + int64((1_000_000+rng.Float64()*5_000_000))
		var rxRate, txRate float64
		if k < points-1 {
			rxRate = round2(float64(rxBytes-prevRx) / 600)
			txRate = round2(float64(txBytes-prevTx) / 600)
		}
		prevRx, prevTx = rxBytes, txBytes

		out = append(out, models.NodeMetric{
			ID:                    uuid.New(),
			ClusterID:             clusterID,
			NodeID:                node.ID,
			NodeName:              node.Name,
			Timestamp:             ts,
			CPUUsageNanoCores:     int64(cpu / 100 * capNano),
			CPUUsagePercent:       round2(cpu),
			MemoryWorkingSetBytes: int64(mem / 100 * float64(memBytes)),
			MemoryUsageBytes:      int64((mem + 4) / 100 * float64(memBytes)),
			MemoryUsagePercent:    round2(mem),
			FSUsedBytes:           int64(fs / 100 * float64(fsCapBytes)),
			FSCapacityBytes:       fsCapBytes,
			FSUsedPercent:         round2(fs),
			NetworkRxBytes:        rxBytes,
			NetworkTxBytes:        txBytes,
			NetworkRxRate:         rxRate,
			NetworkTxRate:         txRate,
			PodsRunning:           8 + rng.Intn(40),
		})
	}
	return out
}

// demoFinding is a compact spec for a seeded update finding.
type demoFinding struct {
	kind, namespace, title, current, latest, updateType, severity, status string
	score                                                                 float64
}

func seedDemoFindings(ctx context.Context, db *gorm.DB, cl *models.Cluster, namespaces []string, now time.Time, rng *rand.Rand) error {
	ns := func(i int) string {
		if len(namespaces) == 0 {
			return "default"
		}
		return namespaces[i%len(namespaces)]
	}

	findings := []demoFinding{
		{models.FindingKindImage, ns(3), "nginx 1.25.3 → 1.27.1", "1.25.3", "1.27.1", models.UpdateTypeMinor, models.SeverityHigh, models.FindingStatusOpen, 68},
		{models.FindingKindImage, ns(4), "postgres 15.4 → 16.3", "15.4", "16.3", models.UpdateTypeMajor, models.SeverityCritical, models.FindingStatusOpen, 86},
		{models.FindingKindImage, ns(1), "redis 7.2.3 → 7.2.5", "7.2.3", "7.2.5", models.UpdateTypePatch, models.SeverityLow, models.FindingStatusOpen, 24},
		{models.FindingKindHelm, ns(2), "kube-prometheus-stack 58.2.1 → 61.3.0", "58.2.1", "61.3.0", models.UpdateTypeMinor, models.SeverityMedium, models.FindingStatusPlanned, 47},
		{models.FindingKindImage, ns(0), "coredns 1.11.1 → 1.11.3", "1.11.1", "1.11.3", models.UpdateTypePatch, models.SeverityMedium, models.FindingStatusOpen, 41},
		{models.FindingKindHelm, ns(1), "ingress-nginx 4.10.0 → 4.11.2", "4.10.0", "4.11.2", models.UpdateTypeMinor, models.SeverityHigh, models.FindingStatusOpen, 63},
		{models.FindingKindImage, ns(3), "traefik 2.10.7 → 3.1.2", "2.10.7", "3.1.2", models.UpdateTypeMajor, models.SeverityCritical, models.FindingStatusOpen, 82},
		{models.FindingKindImage, ns(4), "grafana 10.4.2 → 11.1.0", "10.4.2", "11.1.0", models.UpdateTypeMajor, models.SeverityMedium, models.FindingStatusIgnored, 44},
	}

	for _, f := range findings {
		detected := now.Add(-time.Duration(rng.Intn(20)+1) * 24 * time.Hour)
		finding := &models.UpdateFinding{
			ID:              uuid.New(),
			ClusterID:       cl.ID,
			NamespaceName:   f.namespace,
			Kind:            f.kind,
			UpdateType:      f.updateType,
			Severity:        f.severity,
			Status:          f.status,
			CurrentVersion:  f.current,
			LatestVersion:   f.latest,
			Title:           f.title,
			Description:     "Seeded demo finding.",
			CVEs:            datatypes.JSON([]byte("[]")),
			Metadata:        datatypes.JSON([]byte("{}")),
			FirstDetectedAt: detected,
			LastObservedAt:  now,
		}
		if err := db.WithContext(ctx).Create(finding).Error; err != nil {
			return fmt.Errorf("create demo finding: %w", err)
		}

		score := &models.RiskScore{
			ID:                 uuid.New(),
			FindingID:          finding.ID,
			Score:              f.score,
			Severity:           f.severity,
			Factors:            jsonOf(map[string]float64{"version_gap": f.score * 0.4, "exposure": f.score * 0.3, "age": f.score * 0.3}),
			EnvMultiplier:      1.0,
			ExposureMultiplier: 1.0,
			ComputedAt:         now,
		}
		if err := db.WithContext(ctx).Create(score).Error; err != nil {
			return fmt.Errorf("create demo risk score: %w", err)
		}
	}
	return nil
}

func clampPct(v float64) float64 {
	if v < 1 {
		return 1
	}
	if v > 97 {
		return 97
	}
	return v
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

func jsonOf(v any) datatypes.JSON {
	b, _ := json.Marshal(v)
	return datatypes.JSON(b)
}
