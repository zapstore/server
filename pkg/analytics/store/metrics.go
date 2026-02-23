package store

import (
	"context"
	"fmt"
)

// RelayMetrics holds aggregated relay counters for a single day.
type RelayMetrics struct {
	Day     string // formatted as "YYYY-MM-DD"
	Reqs    int64  // REQs fulfilled
	Filters int64  // filters fulfilled
	Events  int64  // events saved or replaced
}

// BlossomMetrics holds aggregated blossom counters for a single day.
type BlossomMetrics struct {
	Day       string // formatted as "YYYY-MM-DD"
	Uploads   int64  // uploads that hit bunny
	Downloads int64  // downloads that succeeded
	Checks    int64  // checks that succeeded
}

// SaveRelayMetrics writes the given relay metrics to the database for the given day.
// On conflict it increments the existing counters.
func (s *Store) SaveRelayMetrics(ctx context.Context, m RelayMetrics) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO relay_metrics (day, reqs, filters, events)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(day)
		DO UPDATE SET
			reqs    = relay_metrics.reqs    + excluded.reqs,
			filters = relay_metrics.filters + excluded.filters,
			events  = relay_metrics.events  + excluded.events
	`, m.Day, m.Reqs, m.Filters, m.Events)
	if err != nil {
		return fmt.Errorf("failed to save relay metrics: %w", err)
	}
	return nil
}

// SaveBlossomMetrics writes the given blossom metrics to the database for the given day.
// On conflict it increments the existing counters.
func (s *Store) SaveBlossomMetrics(ctx context.Context, m BlossomMetrics) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO blossom_metrics (day, uploads, downloads, checks)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(day)
		DO UPDATE SET
			uploads   = blossom_metrics.uploads   + excluded.uploads,
			downloads = blossom_metrics.downloads + excluded.downloads,
			checks    = blossom_metrics.checks    + excluded.checks
	`, m.Day, m.Uploads, m.Downloads, m.Checks)
	if err != nil {
		return fmt.Errorf("failed to save blossom metrics: %w", err)
	}
	return nil
}
