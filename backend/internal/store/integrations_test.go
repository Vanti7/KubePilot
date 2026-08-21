package store

import (
	"context"
	"testing"

	"github.com/kubepilot/backend/internal/models"
)

func TestIntegrationCRUD(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	integration := &models.IntegrationAccount{Name: "Slack Alerts", Type: "slack"}
	if err := s.CreateIntegration(ctx, integration); err != nil {
		t.Fatalf("create: %v", err)
	}
	if integration.ID.String() == "" {
		t.Fatal("expected an assigned ID")
	}

	t.Run("get", func(t *testing.T) {
		got, err := s.GetIntegration(ctx, integration.ID.String())
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Type != "slack" {
			t.Errorf("Type = %q, want slack", got.Type)
		}
	})

	t.Run("list orders by name", func(t *testing.T) {
		if err := s.CreateIntegration(ctx, &models.IntegrationAccount{Name: "Alpha", Type: "webhook"}); err != nil {
			t.Fatalf("create second: %v", err)
		}
		integrations, err := s.ListIntegrations(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(integrations) != 2 || integrations[0].Name != "Alpha" {
			t.Fatalf("expected alphabetical order, got %v", integrations)
		}
	})

	t.Run("update sync time", func(t *testing.T) {
		if err := s.UpdateIntegrationSyncTime(ctx, integration.ID.String()); err != nil {
			t.Fatalf("update sync time: %v", err)
		}
		got, _ := s.GetIntegration(ctx, integration.ID.String())
		if got.LastSyncAt == nil {
			t.Error("expected LastSyncAt to be set")
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := s.DeleteIntegration(ctx, integration.ID.String()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		integrations, _ := s.ListIntegrations(ctx)
		if len(integrations) != 1 {
			t.Fatalf("expected 1 integration remaining, got %d", len(integrations))
		}
	})
}
