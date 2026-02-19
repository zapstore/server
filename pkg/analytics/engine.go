// Package analytics provides an analytics [Engine] for collecting privacy-preserving statistics
// useful for Zapstore developers to keep track of app usage.
package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pippellia-btc/blossom"
	"github.com/pippellia-btc/blossy"
	"github.com/pippellia-btc/rely"
	"github.com/zapstore/server/pkg/analytics/geo"
	"github.com/zapstore/server/pkg/analytics/store"
)

// Paths holds the file system locations for the analytics engine's files.
type Paths struct {
	Store string
	Geo   string
}

// Engine is the heart of the analytics system. It's responsible for processing data
// and saving it in the database on periodic and bounded batches.
type Engine struct {
	store *store.Store
	geo   *geo.Locator

	downloads          chan store.Download
	impressions        chan store.Impression
	pendingImpressions map[store.Impression]int // Impression --> count
	pendingDownloads   map[store.Download]int   // Download --> count

	config Config
	log    *slog.Logger
	wg     sync.WaitGroup
	done   chan struct{}
}

// NewEngine starts the background goroutine and returns the engine.
func NewEngine(c Config, paths Paths, logger *slog.Logger) (*Engine, error) {
	var err error
	engine := &Engine{
		impressions:        make(chan store.Impression, c.QueueSize),
		downloads:          make(chan store.Download, c.QueueSize),
		pendingImpressions: make(map[store.Impression]int),
		pendingDownloads:   make(map[store.Download]int),
		config:             c,
		log:                logger,
		done:               make(chan struct{}),
	}

	engine.store, err = store.New(paths.Store)
	if err != nil {
		return nil, fmt.Errorf("analytics: failed to open database at %q: %w", paths.Store, err)
	}

	if c.GeoEnabled {
		engine.geo, err = geo.NewLocator(c.Geo, paths.Geo)
		if err != nil {
			engine.store.Close()
			return nil, fmt.Errorf("analytics: failed to create geo locator: %w", err)
		}
	}

	engine.wg.Add(1)
	go func() {
		defer engine.wg.Done()
		engine.run()
	}()
	return engine, nil
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

// lookupCountry returns the ISO country code for the given IP.
// If geo-location is not enabled or the lookup fails, an empty string is returned.
func (e *Engine) lookupCountry(ip net.IP) string {
	if !e.config.GeoEnabled {
		return ""
	}

	country, err := e.geo.Country(ip)
	if err != nil {
		e.log.Warn("analytics: failed to lookup country", "error", err)
		return ""
	}
	return country
}

// RecordImpressions records the impressions derived from the given REQ (id, filters),
// and the nostr events served in response to it.
// The client IP address is only used to lookup the country of the client.
func (e *Engine) RecordImpressions(client rely.Client, id string, filters nostr.Filters, events []nostr.Event) {
	country := e.lookupCountry(client.IP().Raw)
	impressions := store.NewImpressions(country, id, filters, events)

	for i, impression := range impressions {
		select {
		case e.impressions <- impression:
		default:
			dropped := len(impressions) - i
			e.log.Warn("analytics: failed to record impressions", "error", "channel is full", "dropped", dropped)
			return
		}
	}
}

// RecordDownload records the download of the given hash by the given request.
// The client IP address is only used to lookup the country of the client.
func (e *Engine) RecordDownload(r blossy.Request, hash blossom.Hash) {
	country := e.lookupCountry(r.IP().Raw)
	download := store.NewDownload(country, r.Raw().Header, hash)

	select {
	case e.downloads <- download:
	default:
		e.log.Warn("analytics: failed to record download", "error", "channel is full")
		return
	}
}

func (e *Engine) run() {
	flushTicker := time.NewTicker(e.config.FlushInterval)
	defer flushTicker.Stop()

	var geoTicker <-chan time.Time
	if e.config.GeoEnabled {
		t := time.NewTicker(e.config.GeoRefreshInterval)
		defer t.Stop()
		geoTicker = t.C
	}

	for {
		select {
		case <-e.done:
			e.drain()
			if err := e.flushAll(); err != nil {
				e.log.Error("analytics: failed to flush", "err", err)
			}
			e.log.Info("analytics: flushed all pending data")

			if err := e.store.Close(); err != nil {
				e.log.Error("analytics: failed to close database", "err", err)
			}
			if e.config.GeoEnabled {
				if err := e.geo.Close(); err != nil {
					e.log.Error("analytics: failed to close geolocation db", "err", err)
				}
			}
			return

		case <-geoTicker:
			e.log.Info("analytics: refreshing geolocation database")
			if err := e.geo.Refresh(context.Background()); err != nil {
				e.log.Error("analytics: failed to refresh geolocation database", "err", err)
			}

		case <-flushTicker.C:
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

// pending returns the total number of pending impressions and downloads.
func (e *Engine) pending() int {
	return len(e.pendingImpressions) + len(e.pendingDownloads)
}

// flushAll commits any pending data to the database.
func (e *Engine) flushAll() error {
	for e.pending() > 0 {
		if err := e.flushImpressions(); err != nil {
			return fmt.Errorf("failed to flush impressions: %w", err)
		}

		if err := e.flushDownloads(); err != nil {
			return fmt.Errorf("failed to flush downloads: %w", err)
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

	flushed := make([]store.ImpressionCount, 0, e.config.FlushSize)
	for impression, count := range e.pendingImpressions {
		if len(flushed) >= e.config.FlushSize {
			break
		}

		if count <= 0 {
			continue
		}

		flushed = append(flushed, store.ImpressionCount{
			Impression: impression,
			Count:      count,
		})
	}

	if err := e.store.SaveImpressions(ctx, flushed); err != nil {
		return fmt.Errorf("failed to save impressions: %w", err)
	}

	for _, f := range flushed {
		delete(e.pendingImpressions, f.Impression)
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

	flushed := make([]store.DownloadCount, 0, e.config.FlushSize)
	for download, count := range e.pendingDownloads {
		if len(flushed) >= e.config.FlushSize {
			break
		}

		if count <= 0 {
			continue
		}

		flushed = append(flushed, store.DownloadCount{
			Download: download,
			Count:    count,
		})
	}

	if err := e.store.SaveDownloads(ctx, flushed); err != nil {
		return fmt.Errorf("failed to save downloads: %w", err)
	}

	for _, f := range flushed {
		delete(e.pendingDownloads, f.Download)
	}
	return nil
}
