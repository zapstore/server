// Package analytics provides an analytics [Engine] for collecting privacy-preserving statistics
// useful for Zapstore developers to keep track of app usage.
package analytics

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/pippellia-btc/blossom"
	"github.com/pippellia-btc/blossy"
	"github.com/pippellia-btc/rely"
)

//go:embed schema.sql
var schema string

// Paths holds the file system locations for the analytics engine's files.
// DB is the path to the SQLite database, MMDB is the path to the MaxMind IP geolocation database.
type Paths struct {
	DB   string
	MMDB string
}

// Engine is the heart of the analytics system. It's responsible for processing data
// and saving it in the database on periodic and bounded batches.
type Engine struct {
	db  *sql.DB
	geo *maxminddb.Reader

	downloads          chan Download
	impressions        chan Impression
	pendingImpressions map[Impression]int // Impression --> count
	pendingDownloads   map[Download]int   // Download --> count

	config Config
	log    *slog.Logger
	wg     sync.WaitGroup
	done   chan struct{}
}

// NewEngine starts the background goroutine and returns the engine.
func NewEngine(c Config, paths Paths, logger *slog.Logger) (*Engine, error) {
	var err error
	engine := &Engine{
		impressions:        make(chan Impression, c.QueueSize),
		downloads:          make(chan Download, c.QueueSize),
		pendingImpressions: make(map[Impression]int),
		pendingDownloads:   make(map[Download]int),
		config:             c,
		log:                logger,
		done:               make(chan struct{}),
	}

	engine.db, err = newDB(paths.DB)
	if err != nil {
		return nil, fmt.Errorf("analytics: failed to open database at %q: %w", paths.DB, err)
	}

	if c.GeoEnabled {
		engine.geo, err = maxminddb.Open(paths.MMDB)
		if errors.Is(err, os.ErrNotExist) {
			logger.Warn("analytics: ip geolocation database not found, geolocation disabled", "path", paths.MMDB)
		} else if err != nil {
			return nil, fmt.Errorf("analytics: failed to open ip geolocation database at %q: %w", paths.MMDB, err)
		}
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
// It will force [Engine.run] to flush any pending data, close the database connections and return.
func (e *Engine) Close() {
	close(e.done)
	e.wg.Wait()
}

// Drain drains all the Engine's channels on a best effort basis, meaning the first time
// all channels are empty, the function returns.
func (e *Engine) drain() {
	for {
		select {
		case impression := <-e.impressions:
			e.pendingImpressions[impression]++

		case download := <-e.downloads:
			e.pendingDownloads[download]++

		default:
			return
		}
	}
}

// LookupCountry looks up the country ISO code of the given IP address.
// If the Engine geolocation is not enabled or broken, the function returns an empty string.
// Any error will be logged and the ISO code will be returned as an empty string.
func (e *Engine) LookupCountry(ip net.IP) string {
	if ip == nil {
		return ""
	}

	if e.geo == nil {
		return ""
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		e.log.Error("failed to lookup country of ip", "error", "failed to parse ip")
		return ""
	}

	var country string
	if err := e.geo.Lookup(addr).DecodePath(&country, "country", "iso_code"); err != nil {
		e.log.Warn("failed to lookup country of ip", "error", err)
		return ""
	}
	return country
}

// RecordImpressions records the impressions derived from the given REQ (id, filters),
// and the nostr events served in response to it.
// The client IP address is only used to lookup the country of the client.
func (e *Engine) RecordImpressions(client rely.Client, id string, filters nostr.Filters, events []nostr.Event) {
	country := e.LookupCountry(client.IP().Raw)
	impressions := NewImpressions(country, id, filters, events)

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

// RecordDownload records the download of the given hash by the given request.
// The client IP address is only used to lookup the country of the client.
func (e *Engine) RecordDownload(r blossy.Request, hash blossom.Hash) {
	country := e.LookupCountry(r.IP().Raw)
	download := NewDownload(country, r.Raw().Header, hash)

	select {
	case e.downloads <- download:
	default:
		e.log.Warn("failed to record download", "error", "channel is full")
		return
	}
}

func (e *Engine) run() {
	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.done:
			e.drain()
			if err := e.flushAll(); err != nil {
				e.log.Error("analytics: failed to flush", "err", err)
			}
			e.log.Info("analytics: flushed all pending data")

			if err := e.db.Close(); err != nil {
				e.log.Error("analytics: failed to close database", "err", err)
			}
			if e.geo != nil {
				if err := e.geo.Close(); err != nil {
					e.log.Error("analytics: failed to close geolocation db", "err", err)
				}
			}
			return

		case <-ticker.C:
			e.log.Debug("analytics: flushing on interval")
			e.drain()

			if err := e.flushAll(); err != nil {
				e.log.Error("analytics: failed to flush", "err", err)
			}

		case impression := <-e.impressions:
			e.log.Debug("analytics: received impression")
			e.pendingImpressions[impression]++

			if len(e.pendingImpressions) >= e.config.FlushSize {
				if err := e.flushImpressions(); err != nil {
					e.log.Error("analytics: failed to flush impressions", "err", err)
				}
			}

		case download := <-e.downloads:
			e.log.Debug("analytics: received download")
			e.pendingDownloads[download]++

			if len(e.pendingDownloads) >= e.config.FlushSize {
				if err := e.flushDownloads(); err != nil {
					e.log.Error("analytics: failed to flush downloads", "err", err)
				}
			}
		}
	}
}

// flushAll commits any pending data to the database.
func (e *Engine) flushAll() error {
	for {
		if len(e.pendingImpressions)+len(e.pendingDownloads) == 0 {
			break
		}

		if len(e.pendingImpressions) > 0 {
			if err := e.flushImpressions(); err != nil {
				return fmt.Errorf("failed to flush impressions: %w", err)
			}
		}

		if len(e.pendingDownloads) > 0 {
			if err := e.flushDownloads(); err != nil {
				return fmt.Errorf("failed to flush downloads: %w", err)
			}
		}
	}
	return nil
}

// flushImpressions commits up to [Config.FlushSize] impressions to the database.
// The operation is guaranteed to terminate within [Config.FlushTimeout].
func (e *Engine) flushImpressions() error {
	if len(e.pendingImpressions) == 0 {
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
		INSERT INTO impressions (app_id, app_pubkey, day, source, type, country_code, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_id, app_pubkey, day, source, type, country_code)
		DO UPDATE SET count = impressions.count + excluded.count
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	flushed := make([]Impression, 0, e.config.FlushSize)
	for impression, count := range e.pendingImpressions {
		if count <= 0 {
			continue
		}

		if _, err := stmt.ExecContext(
			ctx,
			impression.AppID,
			impression.AppPubkey,
			impression.Day,
			string(impression.Source),
			string(impression.Type),
			impression.CountryCode,
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
		delete(e.pendingImpressions, f)
	}
	return nil
}

// flushDownloads commits up to [Config.FlushSize] downloads to the database.
// The operation is guaranteed to terminate within [Config.FlushTimeout].
func (e *Engine) flushDownloads() error {
	if len(e.pendingDownloads) == 0 {
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
		INSERT INTO downloads (hash, day, source, country_code, count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(hash, day, source, country_code)
		DO UPDATE SET count = downloads.count + excluded.count
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	flushed := make([]Download, 0, e.config.FlushSize)
	for download, count := range e.pendingDownloads {
		if count <= 0 {
			continue
		}

		if _, err := stmt.ExecContext(
			ctx,
			download.Hash,
			string(download.Day),
			string(download.Source),
			download.CountryCode,
			count,
		); err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}

		flushed = append(flushed, download)
		if len(flushed) >= e.config.FlushSize {
			break
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	for _, f := range flushed {
		delete(e.pendingDownloads, f)
	}
	return nil
}
