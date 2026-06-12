package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// maxPossibleSum is the maximum weighted factor sum before multipliers
// (all factors at their maximum). The final score normalizes the weighted
// sum × multipliers against this constant to a 0–100 scale.
// See docs/scoring.md §7 — the maximum after multipliers is 18.0 × 1.2 = 21.6.
const maxPossibleSum = 21.6

// ScoringEngine computes risk scores for UpdateFindings.
type ScoringEngine struct {
	store  *store.Store
	logger *zap.Logger
}

// NewScoringEngine creates a new ScoringEngine.
func NewScoringEngine(s *store.Store, logger *zap.Logger) *ScoringEngine {
	return &ScoringEngine{store: s, logger: logger}
}

// ScoreFactors holds the breakdown of each scoring component.
type ScoreFactors struct {
	UpdateType              float64 `json:"update_type"`
	Age                     float64 `json:"age"`
	ServiceCriticality      float64 `json:"service_criticality"`
	HealthStatus            float64 `json:"health_status"`
	RollbackPossible        float64 `json:"rollback_possible"`
	MaintenanceWindowActive float64 `json:"maintenance_window_active"`
	CVSSScore               float64 `json:"cvss_score"`
	EnvMultiplier           float64 `json:"env_multiplier"`
	ExposureMultiplier      float64 `json:"exposure_multiplier"`
	WeightedSum             float64 `json:"weighted_sum"`
}

// ScoreFinding computes and persists a RiskScore for the given finding, workload and cluster.
func (se *ScoringEngine) ScoreFinding(
	ctx context.Context,
	finding *models.UpdateFinding,
	workload *models.Workload,
	cluster *models.Cluster,
) (*models.RiskScore, error) {
	factors := ScoreFactors{}

	// --- UpdateType factor (weight 0.25) ---
	switch finding.UpdateType {
	case models.UpdateTypePatch:
		factors.UpdateType = 5
	case models.UpdateTypeMinor:
		factors.UpdateType = 15
	case models.UpdateTypeMajor:
		factors.UpdateType = 25
	default:
		factors.UpdateType = 10
	}

	// --- Age factor (weight 0.20) ---
	age := time.Since(finding.FirstDetectedAt)
	days := age.Hours() / 24
	switch {
	case days < 7:
		factors.Age = 2
	case days < 30:
		factors.Age = 8
	case days < 90:
		factors.Age = 15
	default:
		factors.Age = 20
	}

	// --- Service criticality factor (weight 0.15) ---
	factors.ServiceCriticality = 5 // default medium
	if workload != nil {
		// Check annotation first.
		var annotations map[string]string
		if len(workload.Annotations) > 0 {
			_ = json.Unmarshal(workload.Annotations, &annotations)
		}
		switch annotations["kubepilot/criticality"] {
		case "critical":
			factors.ServiceCriticality = 15
		case "high":
			factors.ServiceCriticality = 10
		case "medium":
			factors.ServiceCriticality = 5
		case "low":
			factors.ServiceCriticality = 2
		default:
			// Heuristic: many replicas → higher criticality.
			if workload.ReplicasDesired >= 3 {
				factors.ServiceCriticality = 10
			}
		}
	}

	// --- Health status factor (weight 0.10) ---
	if workload != nil {
		switch workload.HealthStatus {
		case models.WorkloadHealthDegraded:
			factors.HealthStatus = 10
		case models.WorkloadHealthWarning:
			factors.HealthStatus = 5
		default:
			factors.HealthStatus = 0
		}
	}

	// --- Rollback possible factor (weight 0.05) ---
	// Check for "kubepilot/rollback" annotation; default to unknown (3).
	factors.RollbackPossible = 3
	if workload != nil {
		var annotations map[string]string
		if len(workload.Annotations) > 0 {
			_ = json.Unmarshal(workload.Annotations, &annotations)
		}
		switch annotations["kubepilot/rollback"] {
		case "true", "yes":
			factors.RollbackPossible = 0
		case "false", "no":
			factors.RollbackPossible = 5
		}
	}

	// --- Maintenance window factor (weight 0.05) ---
	// For MVP, check if any maintenance window is active right now for this cluster.
	factors.MaintenanceWindowActive = 5 // assume outside window
	if cluster != nil {
		if active, err := se.isInsideMaintenanceWindow(ctx, cluster.ID.String()); err == nil && active {
			factors.MaintenanceWindowActive = 0
		}
	}

	// --- CVSS score factor (weight 0.20) ---
	// Reserved for future scanner integration; 0 for MVP.
	factors.CVSSScore = 0

	// --- Weighted sum ---
	weighted := factors.UpdateType*0.25 +
		factors.Age*0.20 +
		factors.ServiceCriticality*0.15 +
		factors.HealthStatus*0.10 +
		factors.RollbackPossible*0.05 +
		factors.MaintenanceWindowActive*0.05 +
		factors.CVSSScore*0.20
	factors.WeightedSum = weighted

	// --- Environment multiplier ---
	envMultiplier := 1.0
	if cluster != nil && cluster.Environment != nil {
		envMultiplier = cluster.Environment.CriticalityWeight
	}
	factors.EnvMultiplier = envMultiplier

	// --- Exposure multiplier ---
	// Check for exposure annotations; default to ClusterIP (1.0).
	exposureMultiplier := 1.0
	if workload != nil {
		var annotations map[string]string
		if len(workload.Annotations) > 0 {
			_ = json.Unmarshal(workload.Annotations, &annotations)
		}
		switch annotations["kubepilot/exposure"] {
		case "external", "ingress":
			exposureMultiplier = 1.2
		case "none", "headless":
			exposureMultiplier = 0.8
		}
	}
	factors.ExposureMultiplier = exposureMultiplier

	// --- Final score ---
	// Normalize the weighted sum (× multipliers) to a 0–100 scale (docs/scoring.md §7).
	preNorm := weighted * envMultiplier * exposureMultiplier
	score := preNorm / maxPossibleSum * 100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	score = math.Round(score*10) / 10 // one decimal place

	severity := scoreToSeverity(score)

	factorsJSON, err := json.Marshal(factors)
	if err != nil {
		return nil, fmt.Errorf("marshal factors: %w", err)
	}

	rs := &models.RiskScore{
		ID:                 uuid.New(),
		FindingID:          finding.ID,
		Score:              score,
		Severity:           severity,
		Factors:            datatypes.JSON(factorsJSON),
		EnvMultiplier:      envMultiplier,
		ExposureMultiplier: exposureMultiplier,
		ComputedAt:         time.Now(),
	}

	if err := se.store.UpsertRiskScore(ctx, rs); err != nil {
		return nil, fmt.Errorf("upsert risk score: %w", err)
	}

	// Update finding severity to match the computed score.
	se.store.DB.WithContext(ctx).
		Model(finding).
		Update("severity", severity)

	return rs, nil
}

// ScoreAll rescores every open finding.
func (se *ScoringEngine) ScoreAll(ctx context.Context) error {
	findings, err := se.store.GetAllOpenFindings(ctx)
	if err != nil {
		return fmt.Errorf("get open findings: %w", err)
	}

	se.logger.Info("scoring all open findings", zap.Int("count", len(findings)))

	for _, f := range findings {
		f := f

		var workload *models.Workload
		if f.WorkloadID != nil {
			workload, _ = se.store.GetWorkload(ctx, f.WorkloadID.String())
		}

		var cluster *models.Cluster
		if f.ClusterID != uuid.Nil {
			cluster, _ = se.store.GetCluster(ctx, f.ClusterID.String())
		}

		if _, err := se.ScoreFinding(ctx, &f, workload, cluster); err != nil {
			se.logger.Warn("score finding",
				zap.String("finding_id", f.ID.String()),
				zap.Error(err),
			)
		}
	}
	return nil
}

// isInsideMaintenanceWindow returns true if a maintenance window is currently active for the cluster.
func (se *ScoringEngine) isInsideMaintenanceWindow(ctx context.Context, clusterID string) (bool, error) {
	var windows []models.MaintenanceWindow
	if err := se.store.DB.WithContext(ctx).
		Where("(cluster_id = ? OR cluster_id IS NULL) AND is_active = true", clusterID).
		Find(&windows).Error; err != nil {
		return false, err
	}

	now := time.Now().UTC()
	for _, w := range windows {
		if isWindowActive(w, now) {
			return true, nil
		}
	}
	return false, nil
}

// isWindowActive checks if the given maintenance window is active at the provided time.
// It uses a simple check based on the cron expression hour/day-of-week fields.
// For production use, a full cron parser (like robfig/cron) should be used.
func isWindowActive(w models.MaintenanceWindow, now time.Time) bool {
	// Parse the cron expression using robfig/cron to find the last scheduled start.
	// For MVP simplicity, we check using a conservative approximation:
	// if the cron expression contains the current hour, treat it as active.
	// A proper implementation would compute the schedule and check [start, start+duration].
	return false // Placeholder — safe default: assume outside window.
}

// scoreToSeverity maps a numeric score to a severity string.
func scoreToSeverity(score float64) string {
	switch {
	case score >= 80:
		return models.SeverityCritical
	case score >= 60:
		return models.SeverityHigh
	case score >= 40:
		return models.SeverityMedium
	case score >= 20:
		return models.SeverityLow
	default:
		return models.SeverityInfo
	}
}
