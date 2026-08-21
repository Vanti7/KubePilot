package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"github.com/robfig/cron/v3"
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
	// ExceptionApplied is set when an active ExceptionRule matched this
	// finding (docs/scoring.md §9) — nil otherwise.
	ExceptionApplied *ExceptionApplied `json:"exception_applied,omitempty"`
}

// ExceptionApplied records which ExceptionRule affected a score, for the
// "Exception Applied" / "Risk Accepted" badge in the UI.
type ExceptionApplied struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	RuleType string `json:"rule_type"`
	Reason   string `json:"reason"`
}

// cveEntry is one element of UpdateFinding.CVEs (JSONB array). Nothing in
// this codebase writes it yet — populating it is a future vulnerability
// scanner integration (Trivy/Grype/Snyk, see CLAUDE.md roadmap V2) — but the
// CVSS factor below reads it so the formula is correct once something does.
type cveEntry struct {
	ID   string  `json:"id"`
	CVSS float64 `json:"cvss"`
}

// cvssFactorFromCVEs implements docs/scoring.md §4 Factor 3: the maximum
// CVSS score across all CVEs associated with the finding, linearly scaled to
// a 0-20 raw factor. Returns 0 if there are no CVEs or the column doesn't
// parse (malformed data should not crash scoring).
func cvssFactorFromCVEs(raw datatypes.JSON) float64 {
	if len(raw) == 0 {
		return 0
	}
	var entries []cveEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return 0
	}
	var maxCVSS float64
	for _, e := range entries {
		if e.CVSS > maxCVSS {
			maxCVSS = e.CVSS
		}
	}
	if maxCVSS <= 0 {
		return 0
	}
	if maxCVSS > 10 {
		maxCVSS = 10 // defensive clamp against out-of-range scanner data
	}
	return (maxCVSS / 10.0) * 20
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
	factors.CVSSScore = cvssFactorFromCVEs(finding.CVEs)

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

	// --- Exception rules (docs/scoring.md §9) ---
	// Applied after the base score/severity, on top of them — never folded
	// into the weighted sum, since an exception is an override of the
	// outcome, not one more input to it.
	rule, err := se.selectExceptionRule(ctx, finding)
	if err != nil {
		se.logger.Warn("evaluate exception rules", zap.String("finding_id", finding.ID.String()), zap.Error(err))
	}
	if rule != nil {
		switch rule.RuleType {
		case models.ExceptionRuleTypeSuppress:
			score = 0
			severity = models.SeverityInfo
		case models.ExceptionRuleTypeReduceSeverity:
			severity = reduceSeverityOneLevel(severity)
		case models.ExceptionRuleTypeAcceptRisk:
			// Score/severity unchanged; the finding's status is forced to
			// ignored below, alongside the severity update.
		}
		factors.ExceptionApplied = &ExceptionApplied{
			RuleID:   rule.ID.String(),
			RuleName: rule.Name,
			RuleType: rule.RuleType,
			Reason:   rule.Reason,
		}
	}

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

	// Update finding severity to match the computed score. An accept_risk
	// rule additionally forces the finding to ignored, re-asserted on every
	// scoring pass for as long as the rule stays active/unexpired — that
	// re-assertion is what "the finding status is automatically set to
	// ignored" (docs/scoring.md §9) means in a system with no separate
	// enforcement job.
	updates := map[string]interface{}{"severity": severity}
	if rule != nil && rule.RuleType == models.ExceptionRuleTypeAcceptRisk {
		updates["status"] = models.FindingStatusIgnored
		updates["status_reason"] = fmt.Sprintf("Exception rule %q: %s", rule.Name, rule.Reason)
		updates["status_changed_at"] = time.Now()
	}
	se.store.DB.WithContext(ctx).
		Model(finding).
		Updates(updates)

	return rs, nil
}

// reduceSeverityOneLevel downgrades a severity by exactly one band
// (docs/scoring.md §9's reduce_severity effect), floored at info.
func reduceSeverityOneLevel(sev string) string {
	switch sev {
	case models.SeverityCritical:
		return models.SeverityHigh
	case models.SeverityHigh:
		return models.SeverityMedium
	case models.SeverityMedium:
		return models.SeverityLow
	default:
		return models.SeverityInfo
	}
}

// scopeSpecificity ranks an ExceptionRule's scope from most to least
// specific, so selectExceptionRule can prefer a workload-level rule over a
// cluster-wide one when both happen to match the same finding.
func scopeSpecificity(r models.ExceptionRule) int {
	switch {
	case r.WorkloadID != nil:
		return 4
	case r.ImagePattern != "":
		return 3
	case r.ClusterID != nil && r.NamespaceName != "":
		return 2
	case r.ClusterID != nil:
		return 1
	default:
		return 0 // global — ClusterID, WorkloadID and ImagePattern all unset
	}
}

// ruleMatchesScope reports whether rule applies to finding. containerImage is
// only consulted for image_pattern rules and may be nil (e.g. Helm findings,
// or when the lookup failed) — such rules simply never match in that case.
func ruleMatchesScope(rule models.ExceptionRule, finding *models.UpdateFinding, containerImage *models.ContainerImage) bool {
	if rule.FindingKind != "" && rule.FindingKind != finding.Kind {
		return false
	}
	switch {
	case rule.WorkloadID != nil:
		return finding.WorkloadID != nil && *finding.WorkloadID == *rule.WorkloadID
	case rule.ImagePattern != "":
		if containerImage == nil {
			return false
		}
		ref := fmt.Sprintf("%s/%s:%s", containerImage.Registry, containerImage.Repository, containerImage.Tag)
		matched, err := path.Match(rule.ImagePattern, ref)
		return err == nil && matched
	case rule.ClusterID != nil && rule.NamespaceName != "":
		return finding.ClusterID == *rule.ClusterID && finding.NamespaceName == rule.NamespaceName
	case rule.ClusterID != nil:
		return finding.ClusterID == *rule.ClusterID
	default:
		return true // global: no scope field set
	}
}

// selectExceptionRule returns the single most specific active, non-expired
// ExceptionRule that matches finding, or nil if none do. Only one rule's
// effect is ever applied per finding (docs/scoring.md §9 describes a single
// "Exception Applied" badge, not a stack of them).
func (se *ScoringEngine) selectExceptionRule(ctx context.Context, finding *models.UpdateFinding) (*models.ExceptionRule, error) {
	rules, err := se.store.ListActiveExceptionRules(ctx)
	if err != nil || len(rules) == 0 {
		return nil, err
	}

	sort.SliceStable(rules, func(i, j int) bool {
		return scopeSpecificity(rules[i]) > scopeSpecificity(rules[j])
	})

	var containerImage *models.ContainerImage
	var fetchedImage bool

	for i := range rules {
		r := &rules[i]
		if r.ImagePattern != "" && !fetchedImage {
			fetchedImage = true
			if finding.ContainerImageID != nil {
				containerImage, _ = se.store.GetContainerImage(ctx, finding.ContainerImageID.String())
			}
		}
		if ruleMatchesScope(*r, finding, containerImage) {
			return r, nil
		}
	}
	return nil, nil
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

// isWindowActive reports whether the given maintenance window covers now.
// A schedule only exposes Next(t) — the first activation strictly after t —
// so there's no direct "am I inside a firing?" query. The standard way to
// get one: probe Next(now - duration). The earliest activation that could
// still be covering now is the first one after (now - duration); if that
// activation is at or before now, we're inside it (it started somewhere in
// (now-duration, now]  and hasn't run out yet). If it's after now, nothing
// covers now.
func isWindowActive(w models.MaintenanceWindow, now time.Time) bool {
	duration := time.Duration(w.Duration) * time.Minute
	if duration <= 0 {
		return false
	}

	loc, err := time.LoadLocation(w.Timezone)
	if err != nil {
		loc = time.UTC
	}
	localNow := now.In(loc)

	schedule, err := cron.ParseStandard(w.CronExpr)
	if err != nil {
		return false // malformed expression: safe default, never active
	}

	candidateStart := schedule.Next(localNow.Add(-duration))
	return !candidateStart.After(localNow)
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
