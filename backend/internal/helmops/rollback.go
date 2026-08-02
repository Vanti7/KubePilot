package helmops

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
)

// Rollback restores releaseName to toRevision. Unlike UpgradeChart, this
// needs no chart download: Helm restores the manifest already stored for
// that revision in its own release history.
func Rollback(cfg *action.Configuration, releaseName string, toRevision int) error {
	rb := action.NewRollback(cfg)
	rb.Version = toRevision
	if err := rb.Run(releaseName); err != nil {
		return fmt.Errorf("helm rollback: %w", err)
	}
	return nil
}
