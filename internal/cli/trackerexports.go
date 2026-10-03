package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// Maintenance runs through reconcile, which the supervisor already schedules.
// Removal failures and record failures are reported without skipping the rest
// of reconciliation. Partial removals are recorded even when a later one fails.
func maintainTrackerExports(ctx context.Context, parts components) (beads.ExportCleanup, error) {
	store, err := runstate.NewTrackerExportCleanupStore(parts.stateRoot, parts.config.Product.ID)
	if err != nil {
		return beads.ExportCleanup{}, err
	}
	cleaned, cleanErr := beads.CleanExportTemporaries(ctx, parts.repository, parts.runner, time.Now())
	recordErr := store.Record(cleaned)
	if recordErr != nil {
		recordErr = fmt.Errorf("record removed tracker export temporaries: %w", recordErr)
	}
	refreshErr := parts.tracker().RefreshExportIfDue(ctx)
	if refreshErr != nil {
		refreshErr = fmt.Errorf("refresh the tracker export: %w", refreshErr)
	}
	return cleaned, errors.Join(cleanErr, recordErr, refreshErr)
}
