package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kubepilot/backend/internal/models"
	"go.uber.org/zap"
)

const artifactHubSearchURL = "https://artifacthub.io/api/v1/packages/search"

// resolveRepoURL determines which chart repository an installed release came
// from. Helm 3 stores the chart name and version in the release secret but not
// its origin, so the link has to be rebuilt by looking the chart up.
//
// A repository whose index carries the exact installed version is a confirmed
// match — that is what separates goharbor's "harbor" 1.19.1 from bitnami's
// unrelated "harbor" 27.0.3. A repository that merely carries a chart of the
// same name is only used as a last resort.
func (hw *HelmWatcher) resolveRepoURL(ctx context.Context, release *models.HelmRelease) string {
	if release.ChartName == "" {
		return ""
	}

	repos, err := hw.store.ListHelmRepositories(ctx)
	if err != nil {
		hw.logger.Warn("helm watcher: list repositories", zap.Error(err))
		repos = nil
	}

	var nameOnlyMatch string
	for i := range repos {
		repo := &repos[i]
		index, err := hw.fetchIndex(ctx, repo.URL, repo)
		if err != nil {
			hw.logger.Debug("helm watcher: repository index unavailable",
				zap.String("repo", repo.URL), zap.Error(err))
			continue
		}
		entries, ok := index.Entries[release.ChartName]
		if !ok || len(entries) == 0 {
			continue
		}
		if hasChartVersion(entries, release.ChartVersion) {
			return repo.URL
		}
		if nameOnlyMatch == "" {
			nameOnlyMatch = repo.URL
		}
	}

	if hw.autodiscover {
		if found := hw.discoverViaArtifactHub(ctx, release); found != "" {
			return found
		}
	}

	return nameOnlyMatch
}

func hasChartVersion(entries []helmChartEntry, version string) bool {
	for _, e := range entries {
		if e.Version == version {
			return true
		}
	}
	return false
}

// discoverViaArtifactHub asks Artifact Hub which repositories publish the chart,
// then keeps the first whose index actually contains the installed version.
// Unconfirmed candidates are discarded: a chart name alone is far too common to
// pin a repository on.
func (hw *HelmWatcher) discoverViaArtifactHub(ctx context.Context, release *models.HelmRelease) string {
	candidates, err := hw.searchArtifactHub(ctx, release.ChartName)
	if err != nil {
		hw.logger.Debug("helm watcher: artifact hub search failed",
			zap.String("chart", release.ChartName), zap.Error(err))
		return ""
	}

	for _, repoURL := range candidates {
		index, err := hw.fetchIndex(ctx, repoURL, nil)
		if err != nil {
			continue
		}
		entries, ok := index.Entries[release.ChartName]
		if !ok {
			continue
		}
		if hasChartVersion(entries, release.ChartVersion) {
			hw.logger.Info("helm watcher: chart repository discovered",
				zap.String("chart", release.ChartName),
				zap.String("repo", repoURL),
			)
			return repoURL
		}
	}
	return ""
}

// searchArtifactHub returns candidate repository URLs publishing a chart with
// exactly this name, in Artifact Hub's own relevance order.
func (hw *HelmWatcher) searchArtifactHub(ctx context.Context, chartName string) ([]string, error) {
	q := url.Values{}
	q.Set("kind", "0") // Helm charts
	q.Set("ts_query_web", chartName)
	q.Set("limit", "20")
	q.Set("facets", "false")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		artifactHubSearchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := hw.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifact hub returned %d", resp.StatusCode)
	}

	var result struct {
		Packages []struct {
			Name       string `json:"name"`
			Repository struct {
				URL string `json:"url"`
			} `json:"repository"`
		} `json:"packages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	urls := make([]string, 0, len(result.Packages))
	for _, p := range result.Packages {
		if !strings.EqualFold(p.Name, chartName) || p.Repository.URL == "" {
			continue
		}
		if seen[p.Repository.URL] {
			continue
		}
		seen[p.Repository.URL] = true
		urls = append(urls, p.Repository.URL)
	}
	return urls, nil
}
