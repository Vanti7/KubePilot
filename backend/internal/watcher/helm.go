package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const helmIndexCacheTTL = 30 * time.Minute

// HelmWatcher polls Helm chart repositories for newer chart versions.
type HelmWatcher struct {
	store      *store.Store
	httpClient *http.Client
	logger     *zap.Logger
	stopCh     chan struct{}
}

// NewHelmWatcher creates a new HelmWatcher.
func NewHelmWatcher(s *store.Store, logger *zap.Logger) *HelmWatcher {
	return &HelmWatcher{
		store: s,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Run starts the Helm watcher loop.
func (hw *HelmWatcher) Run(ctx context.Context, interval time.Duration) {
	hw.logger.Info("helm watcher started")
	hw.check(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hw.check(ctx)
		case <-hw.stopCh:
			hw.logger.Info("helm watcher stopped")
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop signals the watcher to stop.
func (hw *HelmWatcher) Stop() {
	close(hw.stopCh)
}

func (hw *HelmWatcher) check(ctx context.Context) {
	releases, err := hw.store.ListAllHelmReleases(ctx)
	if err != nil {
		hw.logger.Error("helm watcher: list releases", zap.Error(err))
		return
	}

	for _, rel := range releases {
		rel := rel
		if err := hw.CheckRelease(ctx, &rel); err != nil {
			hw.logger.Warn("helm watcher: check release",
				zap.String("release", rel.Name),
				zap.String("chart", rel.ChartName),
				zap.Error(err),
			)
		}
	}
}

// CheckRelease fetches the repo index and creates an UpdateFinding if a newer chart version exists.
func (hw *HelmWatcher) CheckRelease(ctx context.Context, release *models.HelmRelease) error {
	if release.RepoURL == "" {
		return nil
	}

	index, err := hw.fetchIndex(ctx, release.RepoURL)
	if err != nil {
		return fmt.Errorf("fetch index from %s: %w", release.RepoURL, err)
	}

	latestVersion, err := findLatestChartVersion(index, release.ChartName)
	if err != nil || latestVersion == "" {
		return err
	}

	currentSV, err := semver.NewVersion(release.ChartVersion)
	if err != nil {
		return nil // Non-semver current version; skip.
	}
	latestSV, err := semver.NewVersion(latestVersion)
	if err != nil {
		return nil
	}

	if !latestSV.GreaterThan(currentSV) {
		return nil // Already up-to-date.
	}

	updateType := classifyUpdate(currentSV, latestSV)
	title := fmt.Sprintf("Helm chart %s: update available (%s → %s)",
		release.ChartName, release.ChartVersion, latestVersion)

	finding := &models.UpdateFinding{
		ID:            uuid.New(),
		ClusterID:     release.ClusterID,
		NamespaceName: release.NamespaceName,
		HelmReleaseID: &release.ID,
		Kind:          models.FindingKindHelm,
		UpdateType:    updateType,
		Severity:      updateTypeSeverity(updateType),
		Status:        models.FindingStatusOpen,
		CurrentVersion: release.ChartVersion,
		LatestVersion:  latestVersion,
		Title:          title,
		Description: fmt.Sprintf(
			"Helm chart %s/%s is at version %s; version %s is available in repository %s.",
			release.NamespaceName, release.ChartName,
			release.ChartVersion, latestVersion, release.RepoURL,
		),
		FirstDetectedAt: time.Now(),
		LastObservedAt:  time.Now(),
	}

	return hw.store.UpsertFinding(ctx, finding)
}

// helmIndex is a minimal representation of a Helm repository index.yaml.
type helmIndex struct {
	Entries map[string][]helmChartEntry `yaml:"entries"`
}

type helmChartEntry struct {
	Name        string    `yaml:"name"`
	Version     string    `yaml:"version"`
	AppVersion  string    `yaml:"appVersion"`
	Description string    `yaml:"description"`
	Created     time.Time `yaml:"created"`
}

// fetchIndex retrieves and parses index.yaml from a Helm repo, using Redis as cache.
func (hw *HelmWatcher) fetchIndex(ctx context.Context, repoURL string) (*helmIndex, error) {
	cacheKey := "helmidx:" + repoURL

	// Try cache.
	cached, err := hw.store.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var idx helmIndex
		if jsonErr := json.Unmarshal([]byte(cached), &idx); jsonErr == nil {
			return &idx, nil
		}
	}

	// Normalise URL.
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := hw.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repo returned %d for %s", resp.StatusCode, indexURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MB cap
	if err != nil {
		return nil, err
	}

	var idx helmIndex
	if err := yaml.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("parse index.yaml: %w", err)
	}

	// Cache the index.
	if data, jsonErr := json.Marshal(&idx); jsonErr == nil {
		hw.store.Redis.Set(ctx, cacheKey, data, helmIndexCacheTTL)
	}

	return &idx, nil
}

// findLatestChartVersion returns the highest stable semver for chartName in the index.
func findLatestChartVersion(index *helmIndex, chartName string) (string, error) {
	entries, ok := index.Entries[chartName]
	if !ok {
		return "", nil // Chart not in this repo.
	}

	var best *semver.Version
	for _, entry := range entries {
		sv, err := semver.NewVersion(entry.Version)
		if err != nil {
			continue
		}
		if sv.Prerelease() != "" {
			continue // Skip pre-release versions.
		}
		if best == nil || sv.GreaterThan(best) {
			best = sv
		}
	}

	if best == nil {
		return "", nil
	}
	return best.Original(), nil
}
