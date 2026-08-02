package helmops

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kubepilot/backend/internal/models"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// maxIndexBytes/maxChartBytes cap what we're willing to download. This runs
// once per user-triggered upgrade (not on a poll loop like the Helm watcher),
// so no cache is needed here — a fresh fetch each time is cheap enough.
const (
	maxIndexBytes = 10 << 20
	maxChartBytes = 50 << 20
)

// chartIndex/chartEntry mirror watcher.helmIndex/helmChartEntry but also keep
// Urls, which that package deliberately drops (it only ever compares
// name/version, never downloads). Kept as its own copy rather than reusing
// watcher's unexported fetchIndex: this runs far less often, needs no cache,
// and importing into watcher's internals would couple two otherwise
// independent packages for a handful of lines.
type chartIndex struct {
	Entries map[string][]chartEntry `yaml:"entries"`
}

type chartEntry struct {
	Name    string   `yaml:"name"`
	Version string   `yaml:"version"`
	Urls    []string `yaml:"urls"`
}

func httpClientFor(repo *models.HelmRepository) *http.Client {
	client := &http.Client{Timeout: 30 * time.Second}
	if repo != nil && repo.TLSInsecure {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	return client
}

func applyRepoAuth(req *http.Request, repo *models.HelmRepository) {
	if repo == nil {
		return
	}
	var authCfg map[string]string
	if err := json.Unmarshal(repo.AuthConfig, &authCfg); err == nil {
		if u, ok := authCfg["username"]; ok && u != "" {
			req.SetBasicAuth(u, authCfg["password"])
		}
	}
}

// fetchChartIndex downloads and parses a repository's index.yaml. repo
// carries optional credentials/TLS settings; nil for a public,
// unregistered repository.
func fetchChartIndex(ctx context.Context, repoURL string, repo *models.HelmRepository) (*chartIndex, error) {
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, err
	}
	applyRepoAuth(req, repo)

	resp, err := httpClientFor(repo).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repo returned %d for %s", resp.StatusCode, indexURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxIndexBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxIndexBytes {
		return nil, fmt.Errorf("index.yaml from %s exceeds %d MB", repoURL, maxIndexBytes>>20)
	}

	var idx chartIndex
	if err := yaml.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("parse index.yaml: %w", err)
	}
	return &idx, nil
}

// resolveChartTarballURL finds the download URL for chartName at exactly
// version within an already-fetched index. Helm's index.yaml allows each
// entry's urls to be absolute or relative to the repo's own base URL.
func resolveChartTarballURL(idx *chartIndex, repoURL, chartName, version string) (string, error) {
	entries, ok := idx.Entries[chartName]
	if !ok {
		return "", fmt.Errorf("chart %q not found in repository index", chartName)
	}
	for _, e := range entries {
		if e.Version != version || len(e.Urls) == 0 {
			continue
		}
		raw := e.Urls[0]
		resolved, err := resolveURL(repoURL, raw)
		if err != nil {
			return "", fmt.Errorf("resolve chart url %q: %w", raw, err)
		}
		return resolved, nil
	}
	return "", fmt.Errorf("chart %q version %q not found in repository index", chartName, version)
}

func resolveURL(repoURL, raw string) (string, error) {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw, nil
	}
	base, err := url.Parse(strings.TrimRight(repoURL, "/") + "/")
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

// downloadChart fetches a chart tarball and loads it into memory. repo
// carries the same optional credentials/TLS settings as fetchChartIndex.
func downloadChart(ctx context.Context, tarballURL string, repo *models.HelmRepository) (*chart.Chart, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tarballURL, nil)
	if err != nil {
		return nil, err
	}
	applyRepoAuth(req, repo)

	resp, err := httpClientFor(repo).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chart download returned %d for %s", resp.StatusCode, tarballURL)
	}

	body := io.LimitReader(resp.Body, maxChartBytes+1)
	chartArchive, err := loader.LoadArchive(body)
	if err != nil {
		return nil, fmt.Errorf("load chart archive: %w", err)
	}
	return chartArchive, nil
}
