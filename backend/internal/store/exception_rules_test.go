package store

import (
	"context"
	"testing"
	"time"

	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm"
)

func TestExceptionRuleCRUD(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	rule := &models.ExceptionRule{
		Name: "freeze", RuleType: models.ExceptionRuleTypeReduceSeverity,
		ClusterID: &cluster.ID, Reason: "Q1 freeze",
	}
	if err := s.CreateExceptionRule(ctx, rule, true); err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("get preloads cluster", func(t *testing.T) {
		got, err := s.GetExceptionRule(ctx, rule.ID.String())
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got == nil {
			t.Fatal("expected a rule, got nil")
		}
		if got.Cluster == nil || got.Cluster.ID != cluster.ID {
			t.Errorf("expected Cluster preloaded, got %+v", got.Cluster)
		}
	})

	// GetExceptionRule's contract is (nil, nil) for "not found" — not an
	// error — the handler relies on this to return 404 vs 500 correctly.
	t.Run("get missing returns nil, nil (not an error)", func(t *testing.T) {
		got, err := s.GetExceptionRule(ctx, "00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("expected no error for a missing rule, got %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for a missing rule, got %+v", got)
		}
	})

	t.Run("update", func(t *testing.T) {
		rule.Reason = "extended freeze"
		rule.IsActive = false
		if err := s.UpdateExceptionRule(ctx, rule); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, _ := s.GetExceptionRule(ctx, rule.ID.String())
		if got.Reason != "extended freeze" || got.IsActive {
			t.Errorf("update did not persist: %+v", got)
		}
	})

	t.Run("list returns everything regardless of active state", func(t *testing.T) {
		rules, err := s.ListExceptionRules(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(rules) != 1 {
			t.Fatalf("expected 1 rule (even though inactive), got %d", len(rules))
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := s.DeleteExceptionRule(ctx, rule.ID.String()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		rules, _ := s.ListExceptionRules(ctx)
		if len(rules) != 0 {
			t.Errorf("expected 0 rules after delete, got %d", len(rules))
		}
	})

	// DeleteExceptionRule's contract is an explicit gorm.ErrRecordNotFound —
	// unlike GetExceptionRule — the handler relies on this to return 404.
	t.Run("delete missing returns ErrRecordNotFound", func(t *testing.T) {
		err := s.DeleteExceptionRule(ctx, "00000000-0000-0000-0000-000000000000")
		if err != gorm.ErrRecordNotFound {
			t.Errorf("err = %v, want gorm.ErrRecordNotFound", err)
		}
	})
}

func TestListActiveExceptionRules_FiltersExpiredAndInactive(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)

	active := &models.ExceptionRule{Name: "active", RuleType: models.ExceptionRuleTypeSuppress, Reason: "r"}
	activeWithFutureExpiry := &models.ExceptionRule{Name: "active-future", RuleType: models.ExceptionRuleTypeSuppress, Reason: "r", ExpiresAt: &future}
	expired := &models.ExceptionRule{Name: "expired", RuleType: models.ExceptionRuleTypeSuppress, Reason: "r", ExpiresAt: &past}
	paused := &models.ExceptionRule{Name: "paused", RuleType: models.ExceptionRuleTypeSuppress, Reason: "r"}

	for _, seed := range []struct {
		rule     *models.ExceptionRule
		isActive bool
	}{
		{active, true}, {activeWithFutureExpiry, true}, {expired, true}, {paused, false},
	} {
		if err := s.CreateExceptionRule(ctx, seed.rule, seed.isActive); err != nil {
			t.Fatalf("create %s: %v", seed.rule.Name, err)
		}
	}

	rules, err := s.ListActiveExceptionRules(ctx)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 active non-expired rules, got %d: %v", len(rules), rules)
	}
	names := map[string]bool{}
	for _, r := range rules {
		names[r.Name] = true
	}
	if !names["active"] || !names["active-future"] {
		t.Errorf("expected 'active' and 'active-future' present, got %v", names)
	}
	if names["expired"] || names["paused"] {
		t.Errorf("expired and paused rules must not be returned, got %v", names)
	}
}
