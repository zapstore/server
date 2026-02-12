// Package analytics provides an analytics [Engine] for collecting privacy-preserving statistics
// useful for Zapstore developers to keep track of app usage.
package analytics

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
)

//go:embed schema.sql
var schema string

// Engine is the heart of the analytics system. It's responsible for processing data
// and storing it in the database periodically.
type Engine struct {
	db          *sql.DB
	impressions chan Impression
	log         *slog.Logger

	config Config
	done   chan struct{}
}

// NewEngine starts the background goroutine and returns the engine.
func NewEngine(c Config, path string, logger *slog.Logger) (*Engine, error) {
	db, err := newDB(path)
	if err != nil {
		return nil, fmt.Errorf("analytics: failed to open database: %w", err)
	}

	e := &Engine{
		db:          db,
		config:      c,
		impressions: make(chan Impression, 1000),
		done:        make(chan struct{}),
	}
	//go e.run()
	return e, nil
}

func newDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to sqlite3 at %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to apply base schema: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to activate foreign keys: %w", err)
	}
	if _, err = db.Exec("PRAGMA optimize=0x10002;"); err != nil {
		return nil, fmt.Errorf("failed to PRAGMA optimize: %w", err)
	}
	return db, nil
}

// // run reads impressions from the channel, aggregates them, and flushes them to the database periodically.
// func (e *Engine) run() {
// 	ticker := time.NewTicker(e.config.FlushInterval)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-e.done:
// 			e.Flush()
// 			return

// 		case impression := <-e.impressions:
// 			key := impressionKey{
// 				appID:  ev.appID,
// 				date:   ev.date,
// 				source: ev.source,
// 				kind:   ev.kind,
// 			}
// 			agg[key] += ev.count
// 			pending++

// 			if e.cfg.MaxBatchSize > 0 && pending >= e.cfg.MaxBatchSize {
// 				flush()
// 			}

// 		case <-ticker.C:
// 			e.Flush()
// 		}
// 	}
// }

// func (e *Engine) RecordImpression(id string, filters nostr.Filters) {

// }

// func (e *Engine) flushImpressions(ctx context.Context, agg map[impressionKey]int) error {
// 	tx, err := e.db.BeginTx(ctx, nil)
// 	if err != nil {
// 		return err
// 	}

// 	stmt, err := tx.PrepareContext(ctx, `
// 		INSERT INTO impressions (app_id, day, source, type, count)
// 		VALUES (?, ?, ?, ?, ?)
// 		ON CONFLICT(app_id, day, source, type)
// 		DO UPDATE SET count = impressions.count + excluded.count
// 	`)
// 	if err != nil {
// 		_ = tx.Rollback()
// 		return err
// 	}
// 	defer stmt.Close()

// 	for k, v := range agg {
// 		if v <= 0 {
// 			continue
// 		}
// 		dateStr := k.date.Format("2006-01-02")
// 		if _, err := stmt.ExecContext(ctx, k.appID, dateStr, string(k.source), string(k.kind), v); err != nil {
// 			_ = tx.Rollback()
// 			return err
// 		}
// 	}

// 	return tx.Commit()
// }
