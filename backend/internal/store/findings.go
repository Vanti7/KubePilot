package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FindingFilter holds optional filters for listing findings.
type FindingFilter struct {
	ClusterID   string
	NamespaceID string
	Severity    string
	Status      string
	Kind        string
	Limit       int
	Offset      int
}

// FindingWithScore joins an UpdateFinding with its RiskScore and related names.
type FindingWithScore struct {
	models.UpdateFinding
	Score        *float64 `json:"score,omitempty"`
	ScoreSeverity string  `json:"score_severity,omitempty"`
	WorkloadName string   `json:"workload_name,omitempty"`
	ClusterName  string   `json:"cluster_name,omitempty"`
}

// FindingSummary holds counts of findings by severity.
type FindingSummary struct {
	Total    int64 `json:"total"`
	Critical int64 `json:"critical"`
	High     int64 `json:"high"`
	Medium   int64 `json:"medium"`
	Low      int64 `json:"low"`
	Info     int64 `json:"info"`
}

// ListFindings returns findings matching the provided filter, with total count.
func (s *Store) ListFindings(ctx context.Context, filter FindingFilter) ([]FindingWithScore, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	baseQ := s.DB.WithContext(ctx).Model(&models.UpdateFinding{})

	if filter.ClusterID != "" {
		baseQ = baseQ.Where("update_findings.cluster_id = ?", filter.ClusterID)
	}
	if filter.NamespaceID != "" {
		baseQ = baseQ.Where("update_findings.namespace_name IN (SELECT name FROM namespaces WHERE id = ?)", filter.NamespaceID)
	}
	if filter.Severity != "" {
		baseQ = baseQ.Where("update_findings.severity = ?", filter.Severity)
	}
	if filter.Status != "" {
		baseQ = baseQ.Where("update_findings.status = ?", filter.Status)
	}
	if filter.Kind != "" {
		baseQ = baseQ.Where("update_findings.kind = ?", filter.Kind)
	}

	var total int64
	if err := baseQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	type row struct {
		models.UpdateFinding
		Score         *float64 `gorm:"column:score"`
		ScoreSeverity string   `gorm:"column:score_severity"`
		WorkloadName  string   `gorm:"column:workload_name"`
		ClusterName   string   `gorm:"column:cluster_name"`
	}

	var rows []row
	err := s.DB.WithContext(ctx).
		Table("update_findings uf").
		Select(`uf.*,
			rs.score,
			rs.severity AS score_severity,
			w.name  AS workload_name,
			c.name  AS cluster_name`).
		Joins("LEFT JOIN risk_scores rs ON rs.finding_id = uf.id").
		Joins("LEFT JOIN workloads w  ON w.id  = uf.workload_id").
		Joins("LEFT JOIN clusters c  ON c.id  = uf.cluster_id").
		Where(buildFindingWhere(filter)).
		Order("COALESCE(rs.score, 0) DESC").
		Limit(limit).
		Offset(filter.Offset).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	out := make([]FindingWithScore, len(rows))
	for i, r := range rows {
		out[i] = FindingWithScore{
			UpdateFinding: r.UpdateFinding,
			Score:         r.Score,
			ScoreSeverity: r.ScoreSeverity,
			WorkloadName:  r.WorkloadName,
			ClusterName:   r.ClusterName,
		}
	}
	return out, total, nil
}

// buildFindingWhere converts a FindingFilter into a GORM condition map.
func buildFindingWhere(f FindingFilter) map[string]interface{} {
	cond := map[string]interface{}{}
	if f.ClusterID != "" {
		cond["uf.cluster_id"] = f.ClusterID
	}
	if f.Severity != "" {
		cond["uf.severity"] = f.Severity
	}
	if f.Status != "" {
		cond["uf.status"] = f.Status
	}
	if f.Kind != "" {
		cond["uf.kind"] = f.Kind
	}
	return cond
}

// GetFinding retrieves a single finding by ID with all associations.
func (s *Store) GetFinding(ctx context.Context, id string) (*models.UpdateFinding, error) {
	var finding models.UpdateFinding
	result := s.DB.WithContext(ctx).
		Preload("Cluster").
		Preload("Workload").
		Preload("HelmRelease").
		Preload("ContainerImage").
		Where("id = ?", id).
		First(&finding)
	if result.Error != nil {
		return nil, result.Error
	}
	return &finding, nil
}

// UpsertFinding inserts or updates a finding.
// The uniqueness key is (cluster_id, kind, current_version, workload_id OR helm_release_id).
func (s *Store) UpsertFinding(ctx context.Context, finding *models.UpdateFinding) error {
	if finding.ID == uuid.Nil {
		finding.ID = uuid.New()
	}
	if finding.FirstDetectedAt.IsZero() {
		finding.FirstDetectedAt = time.Now()
	}
	finding.LastObservedAt = time.Now()

	return s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "cluster_id"},
				{Name: "kind"},
				{Name: "current_version"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"latest_version",
				"title",
				"description",
				"update_type",
				"severity",
				"last_observed_at",
				"cves",
				"metadata",
				"updated_at",
			}),
		}).
		Create(finding).Error
}

// UpdateFindingStatus changes the status of a finding and records who changed it.
func (s *Store) UpdateFindingStatus(ctx context.Context, id, status string, userID string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	now := time.Now()
	updates["status_changed_at"] = now
	if userID != "" {
		if uid, err := uuid.Parse(userID); err == nil {
			updates["status_changed_by_id"] = uid
		}
	}
	switch status {
	case models.FindingStatusResolved:
		updates["resolved_at"] = now
	}

	return s.DB.WithContext(ctx).
		Model(&models.UpdateFinding{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// GetFindingSummary returns counts of open findings grouped by severity for the given clusters.
func (s *Store) GetFindingSummary(ctx context.Context, clusterIDs []string) (FindingSummary, error) {
	type result struct {
		Severity string
		Count    int64
	}

	q := s.DB.WithContext(ctx).
		Model(&models.UpdateFinding{}).
		Select("severity, COUNT(*) AS count").
		Where("status = ?", models.FindingStatusOpen).
		Group("severity")

	if len(clusterIDs) > 0 {
		q = q.Where("cluster_id IN ?", clusterIDs)
	}

	var rows []result
	if err := q.Scan(&rows).Error; err != nil {
		return FindingSummary{}, err
	}

	var summary FindingSummary
	for _, r := range rows {
		summary.Total += r.Count
		switch r.Severity {
		case models.SeverityCritical:
			summary.Critical = r.Count
		case models.SeverityHigh:
			summary.High = r.Count
		case models.SeverityMedium:
			summary.Medium = r.Count
		case models.SeverityLow:
			summary.Low = r.Count
		case models.SeverityInfo:
			summary.Info = r.Count
		}
	}
	return summary, nil
}

// GetOpenFindingsForCluster returns all open findings for a cluster (used by scoring).
func (s *Store) GetOpenFindingsForCluster(ctx context.Context, clusterID string) ([]models.UpdateFinding, error) {
	var findings []models.UpdateFinding
	result := s.DB.WithContext(ctx).
		Where("cluster_id = ? AND status IN ?", clusterID, []string{
			models.FindingStatusOpen,
			models.FindingStatusPlanned,
			models.FindingStatusApproved,
		}).
		Find(&findings)
	return findings, result.Error
}

// GetAllOpenFindings returns every open finding (used by scoring engine).
func (s *Store) GetAllOpenFindings(ctx context.Context) ([]models.UpdateFinding, error) {
	var findings []models.UpdateFinding
	result := s.DB.WithContext(ctx).
		Where("status IN ?", []string{
			models.FindingStatusOpen,
			models.FindingStatusPlanned,
			models.FindingStatusApproved,
		}).
		Find(&findings)
	return findings, result.Error
}

// UpsertRiskScore inserts or replaces the risk score for a finding.
func (s *Store) UpsertRiskScore(ctx context.Context, rs *models.RiskScore) error {
	if rs.ID == uuid.Nil {
		rs.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "finding_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"score", "severity", "factors", "env_multiplier", "exposure_multiplier", "computed_at", "updated_at"}),
		}).
		Create(rs).Error
}

// GetRiskScore returns the risk score for a finding.
func (s *Store) GetRiskScore(ctx context.Context, findingID string) (*models.RiskScore, error) {
	var rs models.RiskScore
	result := s.DB.WithContext(ctx).
		Where("finding_id = ?", findingID).
		First(&rs)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &rs, nil
}

// TopCriticalFindings returns the N highest-scored open findings.
func (s *Store) TopCriticalFindings(ctx context.Context, n int) ([]FindingWithScore, error) {
	filter := FindingFilter{
		Status: models.FindingStatusOpen,
		Limit:  n,
	}
	findings, _, err := s.ListFindings(ctx, filter)
	return findings, err
}
