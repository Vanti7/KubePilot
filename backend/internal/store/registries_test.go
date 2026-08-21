package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

func mustCreateImageOnHost(t *testing.T, s *Store, host string) *models.ContainerImage {
	t.Helper()
	cluster := mustCreateCluster(t, s, "c-"+host)
	w := &models.Workload{ID: uuid.New(), ClusterID: cluster.ID, NamespaceName: "default", Name: "app", Kind: "Deployment"}
	if err := s.DB.Create(w).Error; err != nil {
		t.Fatalf("create workload: %v", err)
	}
	img := &models.ContainerImage{
		ID: uuid.New(), WorkloadID: w.ID, ContainerName: "app",
		Image: host + "/app:1.0", Registry: host, Repository: "app", Tag: "1.0",
	}
	if err := s.DB.Create(img).Error; err != nil {
		t.Fatalf("create container image: %v", err)
	}
	return img
}

func TestCreateRegistry_LinksExistingImages(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	img := mustCreateImageOnHost(t, s, "harbor.internal")

	registry := &models.ImageRegistry{Name: "Harbor", Host: "harbor.internal", Type: "harbor"}
	if err := s.CreateRegistry(ctx, registry); err != nil {
		t.Fatalf("create registry: %v", err)
	}

	var reloaded models.ContainerImage
	if err := s.DB.First(&reloaded, "id = ?", img.ID).Error; err != nil {
		t.Fatalf("reload image: %v", err)
	}
	if reloaded.RegistryID == nil || *reloaded.RegistryID != registry.ID {
		t.Errorf("expected image linked to registry %s, got %v", registry.ID, reloaded.RegistryID)
	}
}

func TestUpdateRegistry_RelinksOnHostChange(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	registry := &models.ImageRegistry{Name: "Registry", Host: "old.host", Type: "generic"}
	if err := s.CreateRegistry(ctx, registry); err != nil {
		t.Fatalf("create registry: %v", err)
	}

	newHostImage := mustCreateImageOnHost(t, s, "new.host")

	registry.Host = "new.host"
	if err := s.UpdateRegistry(ctx, registry); err != nil {
		t.Fatalf("update registry: %v", err)
	}

	var reloaded models.ContainerImage
	if err := s.DB.First(&reloaded, "id = ?", newHostImage.ID).Error; err != nil {
		t.Fatalf("reload image: %v", err)
	}
	if reloaded.RegistryID == nil || *reloaded.RegistryID != registry.ID {
		t.Errorf("expected the new.host image linked after the host change, got %v", reloaded.RegistryID)
	}
}

func TestDeleteRegistry_UnlinksImagesWithoutDeletingThem(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	img := mustCreateImageOnHost(t, s, "harbor.internal")
	registry := &models.ImageRegistry{Name: "Harbor", Host: "harbor.internal", Type: "harbor"}
	if err := s.CreateRegistry(ctx, registry); err != nil {
		t.Fatalf("create registry: %v", err)
	}

	if err := s.DeleteRegistry(ctx, registry.ID.String()); err != nil {
		t.Fatalf("delete registry: %v", err)
	}

	if _, err := s.GetRegistry(ctx, registry.ID.String()); err == nil {
		t.Error("expected an error fetching the deleted registry")
	}

	var reloaded models.ContainerImage
	if err := s.DB.First(&reloaded, "id = ?", img.ID).Error; err != nil {
		t.Fatalf("image row should still exist after registry deletion: %v", err)
	}
	if reloaded.RegistryID != nil {
		t.Errorf("expected RegistryID cleared, got %v", reloaded.RegistryID)
	}
}

func TestListRegistries(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	if err := s.CreateRegistry(ctx, &models.ImageRegistry{Name: "z-registry", Host: "z.host"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.CreateRegistry(ctx, &models.ImageRegistry{Name: "a-registry", Host: "a.host"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	registries, err := s.ListRegistries(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(registries) != 2 || registries[0].Name != "a-registry" {
		t.Fatalf("expected alphabetical order, got %v", registries)
	}
}
