package store

import (
	"context"
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/kubepilot/backend/internal/models"
)

func TestUpsertSecret_OnConflict(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	sec := &models.Secret{
		ClusterID: cluster.ID, NamespaceName: "default", Name: "tls-cert",
		Type: "kubernetes.io/tls", Keys: datatypes.JSON(`["tls.crt","tls.key"]`),
	}
	if err := s.UpsertSecret(ctx, sec); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID := sec.ID
	firstSeen := sec.LastSeenAt

	time.Sleep(2 * time.Millisecond)

	sec2 := &models.Secret{
		ClusterID: cluster.ID, NamespaceName: "default", Name: "tls-cert",
		Type: "kubernetes.io/tls", Keys: datatypes.JSON(`["tls.crt","tls.key","ca.crt"]`),
	}
	if err := s.UpsertSecret(ctx, sec2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if sec2.ID != firstID {
		t.Errorf("expected the same secret row (id %s), got %s", firstID, sec2.ID)
	}

	var count int64
	s.DB.Model(&models.Secret{}).Where("cluster_id = ? AND namespace_name = ? AND name = ?", cluster.ID, "default", "tls-cert").Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 secret row, got %d", count)
	}

	var reloaded models.Secret
	s.DB.First(&reloaded, "id = ?", firstID)
	if string(reloaded.Keys) != `["tls.crt","tls.key","ca.crt"]` {
		t.Errorf("Keys = %s, want the second upsert's value", reloaded.Keys)
	}
	if !reloaded.LastSeenAt.After(firstSeen) {
		t.Errorf("expected LastSeenAt to advance on re-observation")
	}
}

func TestListSecrets_Filters(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	seed := []struct {
		ns, name, typ string
	}{
		{"default", "opaque-1", "Opaque"},
		{"default", "tls-1", "kubernetes.io/tls"},
		{"kube-system", "opaque-2", "Opaque"},
	}
	for _, sd := range seed {
		if err := s.UpsertSecret(ctx, &models.Secret{ClusterID: cluster.ID, NamespaceName: sd.ns, Name: sd.name, Type: sd.typ}); err != nil {
			t.Fatalf("seed %s: %v", sd.name, err)
		}
	}

	t.Run("by namespace", func(t *testing.T) {
		secrets, total, err := s.ListSecrets(ctx, SecretFilter{NamespaceName: "default"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 || len(secrets) != 2 {
			t.Fatalf("total=%d len=%d, want 2/2", total, len(secrets))
		}
	})

	t.Run("by type", func(t *testing.T) {
		_, total, err := s.ListSecrets(ctx, SecretFilter{Type: "Opaque"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 {
			t.Fatalf("total = %d, want 2", total)
		}
	})

	t.Run("by cluster", func(t *testing.T) {
		_, total, err := s.ListSecrets(ctx, SecretFilter{ClusterID: cluster.ID.String()})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
	})
}

func TestDeleteSecretsNotSeenSince(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	stale := &models.Secret{ClusterID: cluster.ID, NamespaceName: "default", Name: "stale"}
	if err := s.UpsertSecret(ctx, stale); err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := s.DB.Model(&models.Secret{}).Where("id = ?", stale.ID).Update("last_seen_at", old).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	fresh := &models.Secret{ClusterID: cluster.ID, NamespaceName: "default", Name: "fresh"}
	if err := s.UpsertSecret(ctx, fresh); err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}

	cutoff := time.Now().Add(-1 * time.Minute)
	if err := s.DeleteSecretsNotSeenSince(ctx, cluster.ID.String(), cutoff); err != nil {
		t.Fatalf("delete stale: %v", err)
	}

	secrets, total, _ := s.ListSecrets(ctx, SecretFilter{ClusterID: cluster.ID.String()})
	if total != 1 || secrets[0].Name != "fresh" {
		t.Errorf("expected only 'fresh' to survive, got %v", secrets)
	}
}
