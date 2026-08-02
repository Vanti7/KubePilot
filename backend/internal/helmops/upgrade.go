package helmops

import (
	"context"
	"fmt"

	"github.com/kubepilot/backend/internal/models"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

// UpgradeChart resolves and downloads chartName at version from repoURL (repo
// carries optional credentials/TLS settings, nil for a public unregistered
// repository), then runs a Helm upgrade of releaseName with it. values
// replaces whatever values the release currently has — the same semantics as
// `helm upgrade -f values.yaml` (not `--reuse-values`); callers that want to
// keep the existing values must pass them back in themselves.
func UpgradeChart(ctx context.Context, cfg *action.Configuration, repoURL string, repo *models.HelmRepository, releaseName, chartName, version string, values map[string]interface{}) (*release.Release, error) {
	idx, err := fetchChartIndex(ctx, repoURL, repo)
	if err != nil {
		return nil, fmt.Errorf("fetch chart index: %w", err)
	}
	tarballURL, err := resolveChartTarballURL(idx, repoURL, chartName, version)
	if err != nil {
		return nil, err
	}
	chartArchive, err := downloadChart(ctx, tarballURL, repo)
	if err != nil {
		return nil, fmt.Errorf("download chart: %w", err)
	}
	return runUpgrade(ctx, cfg, releaseName, chartArchive, values)
}

func runUpgrade(ctx context.Context, cfg *action.Configuration, releaseName string, chartArchive *chart.Chart, values map[string]interface{}) (*release.Release, error) {
	up := action.NewUpgrade(cfg)
	rel, err := up.RunWithContext(ctx, releaseName, chartArchive, values)
	if err != nil {
		return nil, fmt.Errorf("helm upgrade: %w", err)
	}
	return rel, nil
}
