package scoring

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.OpenSQLite(dbPath, false, zap.NewNop())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Environment{},
		&models.Cluster{},
		&models.Workload{},
		&models.ContainerImage{},
		&models.UpdateFinding{},
		&models.RiskScore{},
		&models.MaintenanceWindow{},
		&models.ExceptionRule{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() {
		// Windows can't remove the TempDir's db file on cleanup while the
		// pool still holds it open.
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return store.NewStore(db, nil, zap.NewNop())
}

func newFinding(t *testing.T, s *store.Store, clusterID uuid.UUID, updateType string, firstDetected time.Time, status string) *models.UpdateFinding {
	t.Helper()
	f := &models.UpdateFinding{
		ID:              uuid.New(),
		ClusterID:       clusterID,
		Kind:            models.FindingKindImage,
		UpdateType:      updateType,
		Severity:        models.SeverityInfo,
		Status:          status,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.0.0",
		Title:           "test finding",
		FirstDetectedAt: firstDetected,
		LastObservedAt:  time.Now(),
	}
	if err := s.DB.WithContext(context.Background()).Create(f).Error; err != nil {
		t.Fatalf("create finding: %v", err)
	}
	return f
}

func TestScoreToSeverity(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, models.SeverityInfo},
		{19.9, models.SeverityInfo},
		{20, models.SeverityLow},
		{39.9, models.SeverityLow},
		{40, models.SeverityMedium},
		{59.9, models.SeverityMedium},
		{60, models.SeverityHigh},
		{79.9, models.SeverityHigh},
		{80, models.SeverityCritical},
		{100, models.SeverityCritical},
	}
	for _, tc := range cases {
		if got := scoreToSeverity(tc.score); got != tc.want {
			t.Errorf("scoreToSeverity(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

// TestScoreFinding_Defaults pins the behavior with no workload and no cluster:
// every factor that reads from workload/cluster must fall back to its
// documented default (docs/scoring.md §4) rather than zero out silently.
func TestScoreFinding_Defaults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	clusterID := uuid.New()
	finding := newFinding(t, s, clusterID, models.UpdateTypeMajor, time.Now().Add(-10*24*time.Hour), models.FindingStatusOpen)

	rs, err := se.ScoreFinding(ctx, finding, nil, nil)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}

	if factors.UpdateType != 25 {
		t.Errorf("UpdateType factor = %v, want 25 (major)", factors.UpdateType)
	}
	if factors.Age != 8 {
		t.Errorf("Age factor = %v, want 8 (7-30 days)", factors.Age)
	}
	if factors.ServiceCriticality != 5 {
		t.Errorf("ServiceCriticality factor = %v, want 5 (default medium, no workload)", factors.ServiceCriticality)
	}
	if factors.HealthStatus != 0 {
		t.Errorf("HealthStatus factor = %v, want 0 (no workload)", factors.HealthStatus)
	}
	if factors.RollbackPossible != 3 {
		t.Errorf("RollbackPossible factor = %v, want 3 (default unknown, no workload)", factors.RollbackPossible)
	}
	if factors.MaintenanceWindowActive != 5 {
		t.Errorf("MaintenanceWindowActive factor = %v, want 5 (default outside, no cluster)", factors.MaintenanceWindowActive)
	}
	if factors.EnvMultiplier != 1.0 {
		t.Errorf("EnvMultiplier = %v, want 1.0 (no cluster)", factors.EnvMultiplier)
	}
	if factors.ExposureMultiplier != 1.0 {
		t.Errorf("ExposureMultiplier = %v, want 1.0 (no workload)", factors.ExposureMultiplier)
	}

	// weighted = 25*.25 + 8*.20 + 5*.15 + 0*.10 + 3*.05 + 5*.05 + 0*.20 = 9.0
	// score = 9.0 / 21.6 * 100 = 41.7 (medium)
	wantScore := math.Round((9.0/maxPossibleSum)*100*10) / 10
	if math.Abs(rs.Score-wantScore) > 0.01 {
		t.Errorf("Score = %v, want %v", rs.Score, wantScore)
	}
	if rs.Severity != models.SeverityMedium {
		t.Errorf("Severity = %q, want %q", rs.Severity, models.SeverityMedium)
	}

	// The finding row itself must be updated to match, not just the RiskScore.
	var reloaded models.UpdateFinding
	if err := s.DB.WithContext(ctx).First(&reloaded, "id = ?", finding.ID).Error; err != nil {
		t.Fatalf("reload finding: %v", err)
	}
	if reloaded.Severity != models.SeverityMedium {
		t.Errorf("finding.Severity in DB = %q, want %q", reloaded.Severity, models.SeverityMedium)
	}
}

// TestScoreFinding_WorkloadAnnotationsAndEnvironment exercises the
// annotation-driven overrides and the environment/exposure multipliers
// together — this is the combination the "production, externally-exposed,
// critical service" case in docs/scoring.md §8 relies on.
func TestScoreFinding_WorkloadAnnotationsAndEnvironment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	env := &models.Environment{ID: uuid.New(), Name: "Development", Slug: "dev", CriticalityWeight: 0.3}
	if err := s.DB.Create(env).Error; err != nil {
		t.Fatalf("create environment: %v", err)
	}
	cluster := &models.Cluster{ID: uuid.New(), Name: "c1", Slug: "c1", EnvironmentID: &env.ID, Environment: env}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	annotations, _ := json.Marshal(map[string]string{
		"kubepilot/criticality": "critical",
		"kubepilot/rollback":    "true",
		"kubepilot/exposure":    "external",
	})
	workload := &models.Workload{
		ID:            uuid.New(),
		ClusterID:     cluster.ID,
		NamespaceName: "default",
		Name:          "api",
		Kind:          "Deployment",
		HealthStatus:  models.WorkloadHealthDegraded,
		Annotations:   datatypes.JSON(annotations),
	}
	if err := s.DB.Create(workload).Error; err != nil {
		t.Fatalf("create workload: %v", err)
	}

	finding := newFinding(t, s, cluster.ID, models.UpdateTypePatch, time.Now().Add(-100*24*time.Hour), models.FindingStatusOpen)

	rs, err := se.ScoreFinding(ctx, finding, workload, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}

	if factors.UpdateType != 5 {
		t.Errorf("UpdateType factor = %v, want 5 (patch)", factors.UpdateType)
	}
	if factors.Age != 20 {
		t.Errorf("Age factor = %v, want 20 (>90 days)", factors.Age)
	}
	if factors.ServiceCriticality != 15 {
		t.Errorf("ServiceCriticality factor = %v, want 15 (critical annotation)", factors.ServiceCriticality)
	}
	if factors.HealthStatus != 10 {
		t.Errorf("HealthStatus factor = %v, want 10 (degraded)", factors.HealthStatus)
	}
	if factors.RollbackPossible != 0 {
		t.Errorf("RollbackPossible factor = %v, want 0 (rollback=true annotation)", factors.RollbackPossible)
	}
	if factors.EnvMultiplier != 0.3 {
		t.Errorf("EnvMultiplier = %v, want 0.3 (dev environment)", factors.EnvMultiplier)
	}
	if factors.ExposureMultiplier != 1.2 {
		t.Errorf("ExposureMultiplier = %v, want 1.2 (external annotation)", factors.ExposureMultiplier)
	}

	// weighted = 5*.25 + 20*.20 + 15*.15 + 10*.10 + 0*.05 + 5*.05 + 0*.20 = 8.75
	// preNorm = 8.75 * 0.3 * 1.2 = 3.15 ; score = 3.15/21.6*100 = 14.6 (info)
	wantScore := math.Round((3.15/maxPossibleSum)*100*10) / 10
	if math.Abs(rs.Score-wantScore) > 0.01 {
		t.Errorf("Score = %v, want %v", rs.Score, wantScore)
	}
	if rs.Severity != models.SeverityInfo {
		t.Errorf("Severity = %q, want %q (dev multiplier should suppress urgency despite degraded health)", rs.Severity, models.SeverityInfo)
	}
}

// TestScoreFinding_MaintenanceWindow_Active/_Inactive replace the old
// placeholder-pinning test now that isWindowActive is a real cron-based
// evaluation (engine.go) instead of a hardcoded false.
func TestScoreFinding_MaintenanceWindow_Active(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	cluster := &models.Cluster{ID: uuid.New(), Name: "c1", Slug: "c1"}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	window := &models.MaintenanceWindow{
		ID: uuid.New(), ClusterID: &cluster.ID, Name: "every minute, 60min window",
		CronExpr: "* * * * *", Duration: 60, IsActive: true,
	}
	if err := s.DB.Create(window).Error; err != nil {
		t.Fatalf("create maintenance window: %v", err)
	}

	finding := newFinding(t, s, cluster.ID, models.UpdateTypeMajor, time.Now(), models.FindingStatusOpen)
	rs, err := se.ScoreFinding(ctx, finding, nil, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}
	if factors.MaintenanceWindowActive != 0 {
		t.Errorf("MaintenanceWindowActive = %v, want 0 (fires every minute with a 60min window — always covers now)", factors.MaintenanceWindowActive)
	}
}

func TestScoreFinding_MaintenanceWindow_Inactive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	cluster := &models.Cluster{ID: uuid.New(), Name: "c1", Slug: "c1"}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	// Fires only at 00:00 on Feb 29th — deterministically inactive for any
	// real test run, without racing against the actual wall clock.
	window := &models.MaintenanceWindow{
		ID: uuid.New(), ClusterID: &cluster.ID, Name: "leap day only",
		CronExpr: "0 0 29 2 *", Duration: 60, IsActive: true,
	}
	if err := s.DB.Create(window).Error; err != nil {
		t.Fatalf("create maintenance window: %v", err)
	}

	finding := newFinding(t, s, cluster.ID, models.UpdateTypeMajor, time.Now(), models.FindingStatusOpen)
	rs, err := se.ScoreFinding(ctx, finding, nil, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}
	if factors.MaintenanceWindowActive != 5 {
		t.Errorf("MaintenanceWindowActive = %v, want 5 (leap-day-only window should not cover now)", factors.MaintenanceWindowActive)
	}
}

// TestScoreFinding_CVSSFactor wires docs/scoring.md §4 Factor 3: the max
// CVSS across all CVEs on the finding, not the first entry or their sum.
func TestScoreFinding_CVSSFactor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	finding := newFinding(t, s, uuid.New(), models.UpdateTypeMinor, time.Now(), models.FindingStatusOpen)

	cves, _ := json.Marshal([]map[string]interface{}{
		{"id": "CVE-2026-0001", "cvss": 4.0},
		{"id": "CVE-2026-0002", "cvss": 7.8},
	})
	finding.CVEs = datatypes.JSON(cves)
	if err := s.DB.Model(finding).Update("cves", finding.CVEs).Error; err != nil {
		t.Fatalf("set cves: %v", err)
	}

	rs, err := se.ScoreFinding(ctx, finding, nil, nil)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}
	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}

	want := (7.8 / 10.0) * 20 // 15.6
	if math.Abs(factors.CVSSScore-want) > 0.001 {
		t.Errorf("CVSSScore = %v, want %v (max of the two CVEs)", factors.CVSSScore, want)
	}
}

// TestScoreFinding_DocsExample1 reproduces docs/scoring.md §8 Example 1
// exactly (payment-api: major update, 45 days old, CVSS 7.8, critical
// service, healthy, Helm rollback, outside a maintenance window, prod,
// external ingress) end to end, tying the implementation directly to the
// documented worked example now that CVSS and maintenance windows are both
// real rather than stubbed.
func TestScoreFinding_DocsExample1(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	env := &models.Environment{ID: uuid.New(), Name: "Production", Slug: "production", CriticalityWeight: 1.0}
	if err := s.DB.Create(env).Error; err != nil {
		t.Fatalf("create environment: %v", err)
	}
	cluster := &models.Cluster{ID: uuid.New(), Name: "prod", Slug: "prod", EnvironmentID: &env.ID, Environment: env}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	// No maintenance window rows at all for this cluster -> outside (raw 5).
	annotations, _ := json.Marshal(map[string]string{
		"kubepilot/criticality": "critical",
		"kubepilot/rollback":    "true",
		"kubepilot/exposure":    "external",
	})
	workload := &models.Workload{
		ID: uuid.New(), ClusterID: cluster.ID, NamespaceName: "payments", Name: "payment-api", Kind: "Deployment",
		HealthStatus: models.WorkloadHealthHealthy, Annotations: datatypes.JSON(annotations),
	}
	if err := s.DB.Create(workload).Error; err != nil {
		t.Fatalf("create workload: %v", err)
	}

	finding := newFinding(t, s, cluster.ID, models.UpdateTypeMajor, time.Now().Add(-45*24*time.Hour), models.FindingStatusOpen)
	cves, _ := json.Marshal([]map[string]interface{}{{"id": "CVE-2026-1234", "cvss": 7.8}})
	finding.CVEs = datatypes.JSON(cves)
	if err := s.DB.Model(finding).Update("cves", finding.CVEs).Error; err != nil {
		t.Fatalf("set cves: %v", err)
	}

	rs, err := se.ScoreFinding(ctx, finding, workload, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	if math.Abs(rs.Score-82.6) > 0.05 {
		t.Errorf("Score = %v, want 82.6 (docs/scoring.md §8 Example 1)", rs.Score)
	}
	if rs.Severity != models.SeverityCritical {
		t.Errorf("Severity = %q, want %q", rs.Severity, models.SeverityCritical)
	}
}

func TestScoreFinding_ExceptionRule_Suppress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	clusterID := uuid.New()
	rule := &models.ExceptionRule{
		Name: "known false positive", RuleType: models.ExceptionRuleTypeSuppress,
		Reason: "accepted permanently",
	}
	if err := s.CreateExceptionRule(ctx, rule, true); err != nil {
		t.Fatalf("create exception rule: %v", err)
	}

	finding := newFinding(t, s, clusterID, models.UpdateTypeMajor, time.Now().Add(-100*24*time.Hour), models.FindingStatusOpen)
	rs, err := se.ScoreFinding(ctx, finding, nil, nil)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	if rs.Score != 0 {
		t.Errorf("Score = %v, want 0 (suppressed)", rs.Score)
	}
	if rs.Severity != models.SeverityInfo {
		t.Errorf("Severity = %q, want %q (suppressed)", rs.Severity, models.SeverityInfo)
	}
	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}
	if factors.ExceptionApplied == nil || factors.ExceptionApplied.RuleID != rule.ID.String() {
		t.Errorf("ExceptionApplied = %+v, want it to reference rule %s", factors.ExceptionApplied, rule.ID)
	}
}

func TestScoreFinding_ExceptionRule_ReduceSeverity(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	env := &models.Environment{ID: uuid.New(), Name: "Production", Slug: "production", CriticalityWeight: 1.0}
	if err := s.DB.Create(env).Error; err != nil {
		t.Fatalf("create environment: %v", err)
	}
	cluster := &models.Cluster{ID: uuid.New(), Name: "prod", Slug: "prod", EnvironmentID: &env.ID, Environment: env}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	rule := &models.ExceptionRule{
		Name: "freeze window Q1", RuleType: models.ExceptionRuleTypeReduceSeverity,
		ClusterID: &cluster.ID, Reason: "regulatory freeze period",
	}
	if err := s.CreateExceptionRule(ctx, rule, true); err != nil {
		t.Fatalf("create exception rule: %v", err)
	}

	// Old major update with no workload context: weighted sum 9.0, score
	// 41.7 (medium) without the rule — see TestScoreFinding_Defaults.
	finding := newFinding(t, s, cluster.ID, models.UpdateTypeMajor, time.Now().Add(-10*24*time.Hour), models.FindingStatusOpen)
	rs, err := se.ScoreFinding(ctx, finding, nil, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	wantScore := math.Round((9.0/maxPossibleSum)*100*10) / 10
	if math.Abs(rs.Score-wantScore) > 0.01 {
		t.Errorf("Score = %v, want %v (reduce_severity does not change the numeric score)", rs.Score, wantScore)
	}
	if rs.Severity != models.SeverityLow {
		t.Errorf("Severity = %q, want %q (medium reduced by one band)", rs.Severity, models.SeverityLow)
	}
}

func TestScoreFinding_ExceptionRule_AcceptRisk(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	clusterID := uuid.New()
	rule := &models.ExceptionRule{
		Name: "accepted for Q1", RuleType: models.ExceptionRuleTypeAcceptRisk,
		Reason: "business accepted the risk until next review",
	}
	if err := s.CreateExceptionRule(ctx, rule, true); err != nil {
		t.Fatalf("create exception rule: %v", err)
	}

	finding := newFinding(t, s, clusterID, models.UpdateTypeMinor, time.Now(), models.FindingStatusOpen)
	if _, err := se.ScoreFinding(ctx, finding, nil, nil); err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var reloaded models.UpdateFinding
	if err := s.DB.First(&reloaded, "id = ?", finding.ID).Error; err != nil {
		t.Fatalf("reload finding: %v", err)
	}
	if reloaded.Status != models.FindingStatusIgnored {
		t.Errorf("Status = %q, want %q (accept_risk forces the finding to ignored)", reloaded.Status, models.FindingStatusIgnored)
	}
	if reloaded.StatusReason == "" {
		t.Error("StatusReason is empty, want it to reference the exception rule")
	}
}

// TestScoreFinding_ExceptionRule_ScopePrecedence verifies a workload-scoped
// rule wins over a global one matching the same finding — scopeSpecificity's
// whole reason for existing.
func TestScoreFinding_ExceptionRule_ScopePrecedence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	cluster := &models.Cluster{ID: uuid.New(), Name: "c1", Slug: "c1"}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	workload := &models.Workload{ID: uuid.New(), ClusterID: cluster.ID, NamespaceName: "default", Name: "api", Kind: "Deployment"}
	if err := s.DB.Create(workload).Error; err != nil {
		t.Fatalf("create workload: %v", err)
	}

	global := &models.ExceptionRule{Name: "global suppress", RuleType: models.ExceptionRuleTypeSuppress, Reason: "r1"}
	if err := s.CreateExceptionRule(ctx, global, true); err != nil {
		t.Fatalf("create global rule: %v", err)
	}
	scoped := &models.ExceptionRule{
		Name: "workload reduce", RuleType: models.ExceptionRuleTypeReduceSeverity,
		WorkloadID: &workload.ID, Reason: "r2",
	}
	if err := s.CreateExceptionRule(ctx, scoped, true); err != nil {
		t.Fatalf("create workload-scoped rule: %v", err)
	}

	finding := newFinding(t, s, cluster.ID, models.UpdateTypeMajor, time.Now().Add(-10*24*time.Hour), models.FindingStatusOpen)
	finding.WorkloadID = &workload.ID
	if err := s.DB.Model(finding).Update("workload_id", workload.ID).Error; err != nil {
		t.Fatalf("set workload_id: %v", err)
	}

	rs, err := se.ScoreFinding(ctx, finding, workload, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}
	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}
	if factors.ExceptionApplied == nil || factors.ExceptionApplied.RuleID != scoped.ID.String() {
		t.Errorf("ExceptionApplied = %+v, want the more specific workload-scoped rule (%s) to win over the global one", factors.ExceptionApplied, scoped.ID)
	}
	if rs.Score == 0 {
		t.Error("Score = 0, want the reduce_severity rule to have won (suppress would have zeroed it)")
	}
}

func TestScoreFinding_ExceptionRule_ExpiredIsIgnored(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	expired := time.Now().Add(-24 * time.Hour)
	rule := &models.ExceptionRule{
		Name: "expired suppress", RuleType: models.ExceptionRuleTypeSuppress,
		Reason: "was temporary", ExpiresAt: &expired,
	}
	if err := s.CreateExceptionRule(ctx, rule, true); err != nil {
		t.Fatalf("create exception rule: %v", err)
	}

	finding := newFinding(t, s, uuid.New(), models.UpdateTypeMajor, time.Now().Add(-10*24*time.Hour), models.FindingStatusOpen)
	rs, err := se.ScoreFinding(ctx, finding, nil, nil)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}
	if rs.Score == 0 {
		t.Error("Score = 0, want the expired rule to have been ignored (not suppressed)")
	}
}

func TestScoreFinding_ExceptionRule_ImagePattern(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	cluster := &models.Cluster{ID: uuid.New(), Name: "c1", Slug: "c1"}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	workload := &models.Workload{ID: uuid.New(), ClusterID: cluster.ID, NamespaceName: "default", Name: "app", Kind: "Deployment"}
	if err := s.DB.Create(workload).Error; err != nil {
		t.Fatalf("create workload: %v", err)
	}
	image := &models.ContainerImage{
		ID: uuid.New(), WorkloadID: workload.ID, ContainerName: "app",
		Image: "myregistry.io/team/app:latest", Registry: "myregistry.io", Repository: "team/app", Tag: "latest",
	}
	if err := s.DB.Create(image).Error; err != nil {
		t.Fatalf("create container image: %v", err)
	}

	rule := &models.ExceptionRule{
		Name: "pinned latest tag", RuleType: models.ExceptionRuleTypeSuppress,
		ImagePattern: "myregistry.io/team/*:latest", Reason: "mutable tag, tracked elsewhere",
	}
	if err := s.CreateExceptionRule(ctx, rule, true); err != nil {
		t.Fatalf("create exception rule: %v", err)
	}

	finding := &models.UpdateFinding{
		ID: uuid.New(), ClusterID: cluster.ID, WorkloadID: &workload.ID, ContainerImageID: &image.ID,
		Kind: models.FindingKindImage, UpdateType: models.UpdateTypeMajor, Severity: models.SeverityInfo,
		Status: models.FindingStatusOpen, CurrentVersion: "latest", LatestVersion: "latest", Title: "t",
		FirstDetectedAt: time.Now(),
	}
	if err := s.DB.Create(finding).Error; err != nil {
		t.Fatalf("create finding: %v", err)
	}

	rs, err := se.ScoreFinding(ctx, finding, workload, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}
	if rs.Score != 0 {
		t.Errorf("Score = %v, want 0 (image_pattern %q should match %s/%s:%s)",
			rs.Score, rule.ImagePattern, image.Registry, image.Repository, image.Tag)
	}

	// A pattern that does not match must not suppress.
	nonMatching := &models.ExceptionRule{
		Name: "different repo", RuleType: models.ExceptionRuleTypeSuppress,
		ImagePattern: "otherregistry.io/*:latest", Reason: "r",
	}
	if err := s.CreateExceptionRule(ctx, nonMatching, true); err != nil {
		t.Fatalf("create non-matching rule: %v", err)
	}
	// Deactivate the first rule so only the non-matching one is in play.
	if err := s.DB.Model(rule).Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate first rule: %v", err)
	}
	rs2, err := se.ScoreFinding(ctx, finding, workload, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding (second): %v", err)
	}
	if rs2.Score == 0 {
		t.Error("Score = 0, want the non-matching image_pattern rule to have no effect")
	}
}

// TestScoreAll_SkipsInactiveFindings verifies ScoreAll only scores findings
// still in an active status (open/planned/approved) — a resolved or ignored
// finding must not get a fresh RiskScore row.
func TestScoreAll_SkipsInactiveFindings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	clusterID := uuid.New()
	open := newFinding(t, s, clusterID, models.UpdateTypeMinor, time.Now(), models.FindingStatusOpen)
	planned := newFinding(t, s, clusterID, models.UpdateTypeMinor, time.Now(), models.FindingStatusPlanned)
	resolved := newFinding(t, s, clusterID, models.UpdateTypeMinor, time.Now(), models.FindingStatusResolved)

	if err := se.ScoreAll(ctx); err != nil {
		t.Fatalf("ScoreAll: %v", err)
	}

	for _, f := range []*models.UpdateFinding{open, planned} {
		rs, err := s.GetRiskScore(ctx, f.ID.String())
		if err != nil {
			t.Fatalf("get risk score for %s: %v", f.Status, err)
		}
		if rs == nil {
			t.Errorf("expected a risk score for %s finding, got none", f.Status)
		}
	}

	rs, err := s.GetRiskScore(ctx, resolved.ID.String())
	if err != nil {
		t.Fatalf("get risk score for resolved: %v", err)
	}
	if rs != nil {
		t.Errorf("expected no risk score for a resolved finding, got %+v", rs)
	}
}
