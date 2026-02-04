package acl

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/pippellia-btc/blossom"
	"github.com/pippellia-btc/smallset"
	"github.com/zapstore/server/pkg/acl/vertex"
)

// Controller is an access control list that manages allowed/blocked pubkeys, ids, and blobs.
// It supports hot-reloading of the CSV files when they are modified.
type Controller struct {
	mu             sync.RWMutex
	pubkeysAllowed *smallset.Ordered[string]
	pubkeysBlocked *smallset.Ordered[string]
	vertex         vertex.Filter

	eventsBlocked *smallset.Ordered[string]
	blobsBlocked  *smallset.Ordered[string]

	log     *slog.Logger
	watcher *fsnotify.Watcher
	done    chan struct{}

	config Config
}

// New creates a new Controller with the given configuration.
// It reloads all CSV files from the [Config.Dir] directory when they change, logging using the given logger.
func New(c Config, log *slog.Logger) (*Controller, error) {
	if log == nil {
		return nil, errors.New("logger is required")
	}

	// Resolve absolute path for reliable comparison with fsnotify ids
	absPath, err := filepath.Abs(c.Dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve acl directory path: %w", err)
	}
	c.Dir = absPath

	acl := &Controller{
		config:         c,
		pubkeysAllowed: smallset.New[string](100),
		pubkeysBlocked: smallset.New[string](100),
		eventsBlocked:  smallset.New[string](100),
		blobsBlocked:   smallset.New[string](100),
		log:            log,
		done:           make(chan struct{}),
	}

	if c.UnknownPubkeyPolicy == UseVertex {
		acl.vertex, err = vertex.NewFilter(c.Vertex)
		if err != nil {
			return nil, fmt.Errorf("failed to create vertex filter: %w", err)
		}
	}

	if _, err = acl.reloadAllowedPubkeys(); err != nil {
		return nil, fmt.Errorf("failed to load allowed pubkeys: %w", err)
	}
	if _, err = acl.reloadBlockedPubkeys(); err != nil {
		return nil, fmt.Errorf("failed to load blocked pubkeys: %w", err)
	}
	if _, err = acl.reloadBlockedEvents(); err != nil {
		return nil, fmt.Errorf("failed to load blocked ids: %w", err)
	}
	if _, err = acl.reloadBlockedBlobs(); err != nil {
		return nil, fmt.Errorf("failed to load blocked blobs: %w", err)
	}

	acl.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// We watch the directory instead of individual files because most editors
	// use atomic writes (write to temp file, then rename), which would cause us to lose the watcher.
	if err := acl.watcher.Add(c.Dir); err != nil {
		acl.watcher.Close()
		return nil, fmt.Errorf("failed to watch acl directory: %w", err)
	}

	go acl.watch()
	return acl, nil
}

// Close stops the file watcher and releases resources.
func (c *Controller) Close() error {
	close(c.done)
	return c.watcher.Close()
}

// AllowPubkey checks if a pubkey should be allowed.
// It checks the blocked list first, then the allowed list, and finally applies the unknown pubkey policy.
func (c *Controller) AllowPubkey(ctx context.Context, pubkey string) (bool, error) {
	c.mu.RLock()
	blocked := c.pubkeysBlocked.Contains(pubkey)
	allowed := c.pubkeysAllowed.Contains(pubkey)
	c.mu.RUnlock()

	if blocked {
		return false, nil
	}

	if allowed {
		return true, nil
	}

	switch c.config.UnknownPubkeyPolicy {
	case AllowAll:
		return true, nil

	case BlockAll:
		return false, nil

	case UseVertex:
		allow, err := c.vertex.Allow(ctx, pubkey)
		if err != nil {
			return false, fmt.Errorf("failed to verify reputation: %w", err)
		}
		return allow, nil

	default:
		return false, fmt.Errorf("internal error: unknown pubkey policy: %s", c.config.UnknownPubkeyPolicy)
	}
}

// IsEventBlocked checks if an event ID is in the blocked list.
func (c *Controller) IsEventBlocked(ID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.eventsBlocked.Contains(ID)
}

// IsBlobBlocked checks if a blob hash is in the blocked list.
func (c *Controller) IsBlobBlocked(hash blossom.Hash) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.blobsBlocked.Contains(hash.Hex())
}

// AllowedPubkeys returns a copy of the current allowed pubkeys list.
func (c *Controller) AllowedPubkeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pubkeysAllowed.Items()
}

// BlockedPubkeys returns a copy of the current blocked pubkeys list.
func (c *Controller) BlockedPubkeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pubkeysBlocked.Items()
}

// BlockedEvents returns a copy of the current blocked ids list.
func (c *Controller) BlockedEvents() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.eventsBlocked.Items()
}

// BlockedBlobs returns a copy of the current blocked blobs list.
func (c *Controller) BlockedBlobs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.blobsBlocked.Items()
}

// watch monitors the ACL directory for file changes and reloads the changed file.
func (c *Controller) watch() {
	const delay = 100 * time.Millisecond
	timer := map[string]*time.Timer{}

	for {
		select {
		case <-c.done:
			return

		case event, ok := <-c.watcher.Events:
			if !ok {
				return
			}

			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			file := filepath.Base(event.Name)
			if !slices.Contains(RequiredFiles, file) {
				continue
			}

			// debounce the reload by stopping the timer if it exists.
			if timer[file] != nil {
				timer[file].Stop()
			}

			timer[file] = time.AfterFunc(delay, func() {
				count, err := c.reload(file)
				if err != nil {
					c.log.Error("acl: reload failed, using old list", "file", file, "error", err)
					return
				}

				c.log.Info("acl: successful reload", "file", file, "items", count)
			})

		case err, ok := <-c.watcher.Errors:
			if !ok {
				return
			}
			c.log.Error("acl: watcher error", "error", err)
		}
	}
}

// reload reloads the given file.
// It returns the number of entities in the new list.
func (c *Controller) reload(file string) (int, error) {
	switch file {
	case PubkeysAllowedFile:
		return c.reloadAllowedPubkeys()
	case PubkeysBlockedFile:
		return c.reloadBlockedPubkeys()
	case EventsBlockedFile:
		return c.reloadBlockedEvents()
	case BlobsBlockedFile:
		return c.reloadBlockedBlobs()
	default:
		return 0, fmt.Errorf("unknown file: %s", file)
	}
}

// reloadAllowedPubkeys reloads the allowed pubkeys list from the allowed_pubkeys.csv file.
// It returns the number of pubkeys in the new list.
func (c *Controller) reloadAllowedPubkeys() (int, error) {
	path := filepath.Join(c.config.Dir, PubkeysAllowedFile)
	pubkeys, _, err := parseCSV(path)
	if err != nil {
		return 0, err
	}

	for i := range pubkeys {
		pk, err := toPubkey(pubkeys[i])
		if err != nil {
			return 0, fmt.Errorf("invalid pubkey at line %d: %w", i+1, err)
		}
		pubkeys[i] = pk
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pubkeysAllowed.Clear()
	c.pubkeysAllowed = smallset.NewFrom(pubkeys...)
	return c.pubkeysAllowed.Size(), nil
}

// reloadBlockedPubkeys reloads the blocked pubkeys list from the blocked_pubkeys.csv file.
// It returns the number of pubkeys in the new list.
func (c *Controller) reloadBlockedPubkeys() (int, error) {
	path := filepath.Join(c.config.Dir, PubkeysBlockedFile)
	pubkeys, _, err := parseCSV(path)
	if err != nil {
		return 0, err
	}

	for i := range pubkeys {
		pk, err := toPubkey(pubkeys[i])
		if err != nil {
			return 0, fmt.Errorf("invalid pubkey at line %d: %w", i+1, err)
		}
		pubkeys[i] = pk
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pubkeysBlocked.Clear()
	c.pubkeysBlocked = smallset.NewFrom(pubkeys...)
	return c.pubkeysBlocked.Size(), nil
}

// reloadBlockedEvents reloads the blocked events list from the blocked_events.csv file.
// It returns the number of events in the new list.
func (c *Controller) reloadBlockedEvents() (int, error) {
	path := filepath.Join(c.config.Dir, EventsBlockedFile)
	ids, _, err := parseCSV(path)
	if err != nil {
		return 0, err
	}

	for i, id := range ids {
		if err := blossom.ValidateHash(id); err != nil {
			return 0, fmt.Errorf("invalid event id at line %d: %w", i+1, err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.eventsBlocked.Clear()
	c.eventsBlocked = smallset.NewFrom(ids...)
	return c.eventsBlocked.Size(), nil
}

// reloadBlockedBlobs reloads the blocked blobs list from the blocked_blobs.csv file.
// It returns the number of blobs in the new list.
func (c *Controller) reloadBlockedBlobs() (int, error) {
	path := filepath.Join(c.config.Dir, BlobsBlockedFile)
	hashes, _, err := parseCSV(path)
	if err != nil {
		return 0, err
	}

	for i, hash := range hashes {
		if err := blossom.ValidateHash(hash); err != nil {
			return 0, fmt.Errorf("invalid blob hash at line %d: %w", i+1, err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.blobsBlocked.Clear()
	c.blobsBlocked = smallset.NewFrom(hashes...)
	return c.blobsBlocked.Size(), nil
}

// toPubkey tries to parse a string into a hex pubkey.
// It supports both hex and npub formats.
func toPubkey(pk string) (string, error) {
	if nostr.IsValid32ByteHex(pk) {
		return pk, nil
	}

	if strings.HasPrefix(pk, "npub1") {
		_, data, err := nip19.Decode(pk)
		if err != nil {
			return "", fmt.Errorf("invalid pubkey: %w", err)
		}
		return data.(string), nil
	}

	return "", fmt.Errorf("invalid pubkey: %s", pk)
}

// parseCSV parses a CSV file with exactly two columns.
// The only lines skipped are the ones starting with # (comments).
func parseCSV(path string) (col1 []string, col2 []string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comment = '#'
	reader.FieldsPerRecord = 2
	reader.TrimLeadingSpace = true

	line := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		line++
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read CSV at line %d: %w", line, err)
		}

		col1 = append(col1, strings.TrimSpace(record[0]))
		col2 = append(col2, strings.TrimSpace(record[1]))
	}
	return col1, col2, nil
}
