// Package analytics provides an analytics [Engine] for collecting privacy-preserving statistics
// useful for Zapstore developers to keep track of app usage.
package analytics

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"
)

//go:embed schema.sql
var schema string

// Engine is the heart of the analytics system. It's responsible for processing data
// and saving it in the database on periodic and bounded batches.
type Engine struct {
	db *sql.DB

	impressions chan Impression
	pending     map[Impression]int // Impression --> count

	config Config
	log    *slog.Logger
	wg     sync.WaitGroup
	done   chan struct{}
}

// NewEngine starts the background goroutine and returns the engine.
func NewEngine(c Config, path string, logger *slog.Logger) (*Engine, error) {
	db, err := newDB(path)
	if err != nil {
		return nil, fmt.Errorf("analytics: failed to open database: %w", err)
	}

	engine := &Engine{
		db:          db,
		impressions: make(chan Impression, c.QueueSize),
		pending:     make(map[Impression]int),
		config:      c,
		log:         logger,
		done:        make(chan struct{}),
	}

	engine.wg.Add(1)
	go func() {
		defer engine.wg.Done()
		engine.run()
	}()
	return engine, nil
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

// Close closes the engine.
// It will force [Engine.run] to flush any pending data, close the database connection and return.
func (e *Engine) Close() {
	close(e.done)
	e.wg.Wait()
}

// RecordImpressions records the impressions derived from the given REQ (id, filters),
// and the nostr events served in response to it.
func (e *Engine) RecordImpressions(id string, filters nostr.Filters, events []nostr.Event) {
	impressions := NewImpressions(id, filters, events)
	for i, impression := range impressions {
		select {
		case e.impressions <- impression:
		default:
			dropped := len(impressions) - i
			e.log.Warn("failed to record impressions", "error", "channel is full", "dropped", dropped)
			return
		}
	}
}

func (e *Engine) run() {
	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.done:
			e.Drain()
			if err := e.flushAll(); err != nil {
				e.log.Error("analytics: failed to flush", "err", err)
			}

			if err := e.db.Close(); err != nil {
				e.log.Error("analytics: failed to close database", "err", err)
			}

			e.log.Info("analytics: flushed all pending data")
			return

		case <-ticker.C:
			e.log.Debug("analytics: flushing on interval")
			if err := e.flushAll(); err != nil {
				e.log.Error("analytics: failed to flush", "err", err)
			}

		case impression := <-e.impressions:
			e.log.Debug("analytics: received impression")
			e.pending[impression]++
			if len(e.pending) >= e.config.FlushSize {
				if err := e.flushImpressions(); err != nil {
					e.log.Error("analytics: failed to flush", "err", err)
				}
			}
		}
	}
}

// Drain drains all the Engine's channels on a best effort basis, meaning the first time
// the channel is empty, the function returns.
func (e *Engine) Drain() {
	for {
		select {
		case impression := <-e.impressions:
			e.pending[impression]++

		default:
			return
		}
	}
}

// flushAll commits any pending data to the database.
func (e *Engine) flushAll() error {
	for {
		if len(e.pending) == 0 {
			break
		}

		if err := e.flushImpressions(); err != nil {
			return fmt.Errorf("failed to flush impressions: %w", err)
		}
	}
	return nil
}

// flushImpressions commits up to [Config.FlushSize] impressions to the database.
// The operation is guaranteed to terminate within [Config.FlushTimeout].
func (e *Engine) flushImpressions() error {
	if len(e.pending) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.config.FlushTimeout)
	defer cancel()

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO impressions (app_id, day, source, type, count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(app_id, day, source, type)
		DO UPDATE SET count = impressions.count + excluded.count
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	flushed := make([]Impression, 0, e.config.FlushSize)
	for impression, count := range e.pending {
		if count <= 0 {
			continue
		}

		if _, err := stmt.ExecContext(
			ctx,
			impression.AppID,
			string(impression.Day),
			string(impression.Source),
			string(impression.Type),
			count,
		); err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}

		flushed = append(flushed, impression)
		if len(flushed) >= e.config.FlushSize {
			break
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	for _, f := range flushed {
		delete(e.pending, f)
	}
	return nil
}
