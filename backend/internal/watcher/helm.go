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
	"github.com/kubepilot/backend/internal/netguard"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const helmIndexCacheTTL = 30 * time.Minute

// maxIndexBytes caps how large an index.yaml we are willing to download and hold
// in the cache. Aggregator repositories (Bitnami and friends) publish indexes of
// tens of megabytes, which is more than a desktop install should spend on
// resolving one chart.
const maxIndexBytes = 10 << 20

// HelmWatcher polls Helm chart repositories for newer chart versions.
type HelmWatcher struct {
	store          *store.Store
	httpClient     *http.Client
	insecureClient *http.Client
	autodiscover   bool
	logger         *zap.Logger
	stopCh         chan struct{}
}

// NewHelmWatcher creates a new HelmWatcher. autodiscover allows falling back to
// Artifact Hub when no configured repository carries an installed chart.
func NewHelmWatcher(s *store.Store, logger *zap.Logger, autodiscover bool) *HelmWatcher {
	return &HelmWatcher{
		store: s,
		// repoURL comes either from an operator-entered Helm repository or from
		// Artifact Hub search results (resolveRepoURL / discoverViaArtifactHub in
		// helm_resolve.go) — the latter is not data KubePilot controls, and this
		// whole watcher runs on a timer with no authenticated user in the loop —
		// so both clients dial through netguard to refuse loopback/link-local/
		// metadata addresses.
		httpClient: netguard.NewHTTPClient(30*time.Second, false),
		// Used only for repositories explicitly marked tls_insecure (self-hosted
		// ChartMuseum & co); never used for public repositories.
		insecureClient: netguard.NewHTTPClient(30*time.Second, true),
		autodiscover:   autodiscover,
		logger:         logger,
		stopCh:         make(chan struct{}),
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
		repoURL := hw.resolveRepoURL(ctx, release)
		if repoURL == "" {
			// No known repository publishes this chart — nothing to compare against.
			hw.logger.Debug("helm watcher: unresolved chart repository",
				zap.String("release", release.Name),
				zap.String("chart", release.ChartName),
			)
			return nil
		}
		release.RepoURL = repoURL
		if err := hw.store.SetHelmReleaseRepoURL(ctx, release.ID, repoURL); err != nil {
			hw.logger.Warn("persist resolved repo url",
				zap.String("release", release.Name), zap.Error(err))
		}
	}

	// A configured repository may carry credentials or a TLS exception; an
	// autodiscovered one is public and resolves to nil here.
	repo, _ := hw.store.GetHelmRepositoryByURL(ctx, release.RepoURL)

	index, err := hw.fetchIndex(ctx, release.RepoURL, repo)
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
		// Already up-to-date — resolve any finding that was previously open for this release.
		if err := hw.store.ResolveActiveFindingForHelm(ctx, release.ID); err != nil {
			hw.logger.Warn("resolve helm finding", zap.String("release", release.Name), zap.Error(err))
		}
		return nil
	}

	updateType := classifyUpdate(currentSV, latestSV)
	title := fmt.Sprintf("Helm chart %s: update available (%s → %s)",
		release.ChartName, release.ChartVersion, latestVersion)

	finding := &models.UpdateFinding{
		ID:             uuid.New(),
		ClusterID:      release.ClusterID,
		NamespaceName:  release.NamespaceName,
		HelmReleaseID:  &release.ID,
		Kind:           models.FindingKindHelm,
		UpdateType:     updateType,
		Severity:       updateTypeSeverity(updateType),
		Status:         models.FindingStatusOpen,
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

// helmChartEntry keeps only the fields we compare on. Descriptions and
// timestamps make up most of an index.yaml and every parsed index is held in the
// cache, so they are deliberately dropped.
type helmChartEntry struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	AppVersion string `yaml:"appVersion"`
}

// fetchIndex retrieves and parses index.yaml from a Helm repo, using the shared
// cache. repo carries optional credentials and TLS settings; it is nil for
// public repositories that are not registered in KubePilot.
func (hw *HelmWatcher) fetchIndex(ctx context.Context, repoURL string, repo *models.HelmRepository) (*helmIndex, error) {
	cacheKey := "helmidx:" + repoURL

	// Try cache.
	cached, err := hw.store.Cache.Get(ctx, cacheKey)
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

	client := hw.httpClient
	if repo != nil {
		if repo.TLSInsecure {
			client = hw.insecureClient
		}
		var authCfg map[string]string
		if err := json.Unmarshal(repo.AuthConfig, &authCfg); err == nil {
			if u, ok := authCfg["username"]; ok && u != "" {
				req.SetBasicAuth(u, authCfg["password"])
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repo returned %d for %s", resp.StatusCode, indexURL)
	}

	// Read one byte past the cap so a truncated index is reported as such
	// instead of surfacing as a confusing YAML syntax error.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxIndexBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxIndexBytes {
		return nil, fmt.Errorf("index.yaml from %s exceeds %d MB", repoURL, maxIndexBytes>>20)
	}

	var idx helmIndex
	if err := yaml.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("parse index.yaml: %w", err)
	}

	// Cache the index.
	if data, jsonErr := json.Marshal(&idx); jsonErr == nil {
		_ = hw.store.Cache.Set(ctx, cacheKey, data, helmIndexCacheTTL)
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
