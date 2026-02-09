// Command migrate performs a full migration from an old zapstore relay to the new one.
// It runs two phases sequentially:
//
//  1. Relay: reads events from the old SQLite database, validates each event's ID and
//     Schnorr signature, and inserts them into the new relay database.
//  2. Blossom: extracts all cdn.zapstore.dev blob URLs from the migrated events,
//     downloads each blob from the old CDN, uploads it to the new Bunny storage zone,
//     and records metadata in the new blossom database.
//
// Both phases are idempotent: re-running the tool skips already-migrated events and blobs.
//
// Bunny credentials are read from the .env file specified by -env (default: cmd/.env).
//
// Usage:
//
//	CGO_ENABLED=1 go run -tags fts5 ./cmd/migrate \
//	  -from relay_backup.db \
//	  -relay-db data/relay.db \
//	  -blossom-db data/blossom.db \
//	  -env cmd/.env
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pippellia-btc/blossom"
	sqlite "github.com/vertex-lab/nostr-sqlite"
	blossomBunny "github.com/zapstore/server/pkg/blossom/bunny"
	blossomStore "github.com/zapstore/server/pkg/blossom/store"
	relayStore "github.com/zapstore/server/pkg/relay/store"
)

const oldCDN = "https://cdn.zapstore.dev/"

func main() {
	from := flag.String("from", "", "path to old relay SQLite database (read-only)")
	relayDB := flag.String("relay-db", "", "path to new relay database (created if missing)")
	blossomDB := flag.String("blossom-db", "", "path to new blossom database (created if missing)")
	envFile := flag.String("env", "cmd/.env", "path to .env file with Bunny credentials")
	skipInvalid := flag.Bool("skip-invalid", false, "skip events with invalid ID/signature instead of aborting")
	dryRun := flag.Bool("dry-run", false, "validate without writing anything")
	flag.Parse()

	if *from == "" || *relayDB == "" || *blossomDB == "" {
		fmt.Fprintf(os.Stderr, "Usage: migrate -from <old.db> -relay-db <new-relay.db> -blossom-db <new-blossom.db> [-env <.env>] [-skip-invalid] [-dry-run]\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load Bunny config from .env
	if err := godotenv.Load(*envFile); err != nil {
		log.Fatalf("failed to load env file %s: %v", *envFile, err)
	}

	bunnyConfig := blossomBunny.NewConfig()
	if err := env.Parse(&bunnyConfig); err != nil {
		log.Fatalf("failed to parse Bunny config from env: %v", err)
	}
	if !*dryRun {
		if err := bunnyConfig.Validate(); err != nil {
			log.Fatalf("invalid Bunny config: %v", err)
		}
	}

	start := time.Now()

	// Open old database (read-only).
	oldDB, err := sql.Open("sqlite3", "file:"+*from+"?mode=ro")
	if err != nil {
		log.Fatalf("failed to open old database: %v", err)
	}
	defer oldDB.Close()

	// Phase 1: Relay
	log.Println("═══ Phase 1: Relay migration ═══")
	migrateRelay(oldDB, *relayDB, *skipInvalid, *dryRun)

	// Phase 2: Blossom
	// In dry-run mode the new relay DB doesn't exist, so extract hashes from the old DB.
	blobSource := *relayDB
	if *dryRun {
		blobSource = *from
	}
	log.Println("")
	log.Println("═══ Phase 2: Blossom migration ═══")
	migrateBlossom(blobSource, *blossomDB, bunnyConfig, *dryRun)

	log.Printf("\nall done in %s", time.Since(start).Round(time.Millisecond))
}

// migrateRelay reads events from the old database, validates ID + signature,
// and inserts into the new relay store. Idempotent via INSERT OR IGNORE.
func migrateRelay(oldDB *sql.DB, relayDBPath string, skipInvalid, dryRun bool) {
	var totalCount int
	if err := oldDB.QueryRow("SELECT COUNT(*) FROM events").Scan(&totalCount); err != nil {
		log.Fatalf("failed to count events: %v", err)
	}
	log.Printf("found %d events in old database", totalCount)

	var newStore *sqlite.Store
	if !dryRun {
		var err error
		newStore, err = relayStore.New(relayDBPath)
		if err != nil {
			log.Fatalf("failed to open new relay store: %v", err)
		}
		defer newStore.Close()
	}

	rows, err := oldDB.Query(`SELECT id, pubkey, created_at, kind, tags, content, sig FROM events ORDER BY created_at ASC`)
	if err != nil {
		log.Fatalf("failed to query events: %v", err)
	}
	defer rows.Close()

	ctx := context.Background()
	var stats struct {
		total, saved, existed, skipped, invalidID, invalidSig, saveErr int
	}

	for rows.Next() {
		stats.total++

		var (
			ev        nostr.Event
			createdAt int64
			tagsRaw   []byte
		)

		if err := rows.Scan(&ev.ID, &ev.PubKey, &createdAt, &ev.Kind, &tagsRaw, &ev.Content, &ev.Sig); err != nil {
			log.Printf("[%d/%d] scan error: %v", stats.total, totalCount, err)
			stats.skipped++
			continue
		}

		ev.CreatedAt = nostr.Timestamp(createdAt)

		if err := json.Unmarshal(tagsRaw, &ev.Tags); err != nil {
			log.Printf("[%d/%d] %s kind=%d: failed to parse tags: %v", stats.total, totalCount, ev.ID, ev.Kind, err)
			stats.skipped++
			continue
		}

		// Verify event ID.
		if !ev.CheckID() {
			msg := fmt.Sprintf("[%d/%d] %s kind=%d: invalid event ID (computed: %s)", stats.total, totalCount, ev.ID, ev.Kind, ev.GetID())
			if skipInvalid {
				log.Printf("%s — skipping", msg)
				stats.invalidID++
				continue
			}
			log.Fatalf("%s — aborting (use -skip-invalid to continue)", msg)
		}

		// Verify Schnorr signature.
		ok, err := ev.CheckSignature()
		if err != nil || !ok {
			msg := fmt.Sprintf("[%d/%d] %s kind=%d: invalid signature (err=%v)", stats.total, totalCount, ev.ID, ev.Kind, err)
			if skipInvalid {
				log.Printf("%s — skipping", msg)
				stats.invalidSig++
				continue
			}
			log.Fatalf("%s — aborting (use -skip-invalid to continue)", msg)
		}

		if dryRun {
			stats.saved++
			if stats.saved%500 == 0 {
				log.Printf("[dry-run] validated %d/%d events", stats.saved, totalCount)
			}
			continue
		}

		var inserted bool
		switch {
		case nostr.IsRegularKind(ev.Kind):
			inserted, err = newStore.Save(ctx, &ev)

		case nostr.IsReplaceableKind(ev.Kind) || nostr.IsAddressableKind(ev.Kind):
			inserted, err = newStore.Replace(ctx, &ev)

		default:
			log.Printf("[%d/%d] %s: unknown kind category %d — skipping", stats.total, totalCount, ev.ID, ev.Kind)
			stats.skipped++
			continue
		}

		if err != nil {
			log.Printf("[%d/%d] %s kind=%d: save error: %v", stats.total, totalCount, ev.ID, ev.Kind, err)
			stats.saveErr++
			continue
		}

		if inserted {
			stats.saved++
		} else {
			stats.existed++
		}

		if (stats.saved+stats.existed)%500 == 0 {
			log.Printf("progress: %d/%d processed (%d new, %d existed)", stats.saved+stats.existed, totalCount, stats.saved, stats.existed)
		}
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("row iteration error: %v", err)
	}

	prefix := ""
	if dryRun {
		prefix = "[DRY RUN] "
	}
	log.Printf("%srelay migration complete", prefix)
	log.Printf("  total:       %d", stats.total)
	log.Printf("  new:         %d", stats.saved)
	log.Printf("  existed:     %d", stats.existed)
	log.Printf("  invalid ID:  %d", stats.invalidID)
	log.Printf("  invalid sig: %d", stats.invalidSig)
	log.Printf("  save errors: %d", stats.saveErr)
	log.Printf("  skipped:     %d", stats.skipped)
}

// migrateBlossom extracts cdn.zapstore.dev blob hashes from the migrated relay DB,
// downloads each blob from the old CDN, uploads to Bunny, and saves metadata.
// Idempotent: blobs already in the blossom store are skipped entirely.
func migrateBlossom(relayDBPath, blossomDBPath string, bunnyConfig blossomBunny.Config, dryRun bool) {
	// Open the migrated relay DB to extract blob URLs from indexed tags.
	relayDB, err := sql.Open("sqlite3", "file:"+relayDBPath+"?mode=ro")
	if err != nil {
		log.Fatalf("failed to open relay database for blob extraction: %v", err)
	}
	defer relayDB.Close()

	// Extract unique blob hashes from cdn.zapstore.dev URLs in tag values.
	hashes, err := extractBlobHashes(relayDB)
	if err != nil {
		log.Fatalf("failed to extract blob hashes: %v", err)
	}
	log.Printf("found %d unique blob hashes in relay events", len(hashes))

	if len(hashes) == 0 {
		log.Println("blossom migration complete: nothing to migrate")
		return
	}

	var store *blossomStore.Store
	var bunny blossomBunny.Client
	if !dryRun {
		store, err = blossomStore.New(blossomDBPath)
		if err != nil {
			log.Fatalf("failed to open blossom store: %v", err)
		}
		defer store.Close()

		bunny, err = blossomBunny.NewClient(bunnyConfig)
		if err != nil {
			log.Fatalf("failed to create bunny client: %v", err)
		}
	}

	httpClient := &http.Client{Timeout: 5 * time.Minute}
	ctx := context.Background()
	var stats struct {
		total, uploaded, existed, notFound, failed int
	}

	for i, hashHex := range hashes {
		stats.total++

		hash, err := blossom.ParseHash(hashHex)
		if err != nil {
			log.Printf("[%d/%d] invalid hash %s: %v — skipping", i+1, len(hashes), hashHex, err)
			stats.failed++
			continue
		}

		if dryRun {
			if (i+1)%100 == 0 {
				log.Printf("[dry-run] would migrate %d/%d blobs", i+1, len(hashes))
			}
			continue
		}

		// Idempotent: skip blobs already in the store.
		exists, err := store.Contains(ctx, hash)
		if err != nil {
			log.Printf("[%d/%d] %s: store check error: %v — skipping", i+1, len(hashes), hashHex, err)
			stats.failed++
			continue
		}
		if exists {
			stats.existed++
			continue
		}

		// Download from old CDN.
		resp, err := httpClient.Get(oldCDN + hashHex)
		if err != nil {
			log.Printf("[%d/%d] %s: download error: %v", i+1, len(hashes), hashHex, err)
			stats.failed++
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			stats.notFound++
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("[%d/%d] %s: download returned %s", i+1, len(hashes), hashHex, resp.Status)
			stats.failed++
			continue
		}

		mime := resp.Header.Get("Content-Type")
		size := resp.ContentLength

		// Upload to Bunny. The hash is passed for server-side checksum verification.
		blobPath := "blobs/" + hashHex + "." + blossom.ExtFromType(mime)
		err = bunny.Upload(ctx, resp.Body, blobPath, hashHex)
		resp.Body.Close()

		if err != nil {
			log.Printf("[%d/%d] %s: bunny upload error: %v", i+1, len(hashes), hashHex, err)
			stats.failed++
			continue
		}

		// If Content-Length was missing (-1), do a HEAD to get the actual size.
		if size <= 0 {
			mime, size, err = bunny.Check(ctx, blobPath)
			if err != nil {
				log.Printf("[%d/%d] %s: bunny check error after upload: %v", i+1, len(hashes), hashHex, err)
				stats.failed++
				continue
			}
		}

		// Save metadata to blossom store.
		_, err = store.Save(ctx, blossomStore.BlobMeta{
			Hash:      hash,
			Type:      mime,
			Size:      size,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			log.Printf("[%d/%d] %s: metadata save error: %v", i+1, len(hashes), hashHex, err)
			stats.failed++
			continue
		}

		stats.uploaded++
		if stats.uploaded%50 == 0 {
			log.Printf("progress: %d uploaded, %d existed, %d not found, %d failed out of %d",
				stats.uploaded, stats.existed, stats.notFound, stats.failed, stats.total)
		}
	}

	prefix := ""
	if dryRun {
		prefix = "[DRY RUN] "
		log.Printf("%sblossom migration complete: would migrate %d blobs", prefix, len(hashes))
		return
	}

	log.Printf("%sblossom migration complete", prefix)
	log.Printf("  total:     %d", stats.total)
	log.Printf("  uploaded:  %d", stats.uploaded)
	log.Printf("  existed:   %d", stats.existed)
	log.Printf("  not found: %d (not on old CDN — skipped)", stats.notFound)
	log.Printf("  failed:    %d", stats.failed)
}

// extractBlobHashes queries the relay DB for all unique sha256 hashes
// referenced in cdn.zapstore.dev URLs or "x" tags within events.
func extractBlobHashes(db *sql.DB) ([]string, error) {
	seen := make(map[string]bool)
	var hashes []string

	addHash := func(hashHex string) {
		if len(hashHex) != 64 {
			return
		}
		if err := blossom.ValidateHash(hashHex); err != nil {
			return
		}
		if seen[hashHex] {
			return
		}
		seen[hashHex] = true
		hashes = append(hashes, hashHex)
	}

	// Try the indexed tags table first (available after relay migration).
	rows, err := db.Query(`SELECT DISTINCT value FROM tags WHERE value LIKE ?`, oldCDN+"%")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var url string
			if err := rows.Scan(&url); err != nil {
				continue
			}
			addHash(strings.TrimPrefix(url, oldCDN))
		}
	}

	// Scan event tags JSON directly. This catches:
	// - cdn.zapstore.dev URLs in tags not covered by indexing (icon, image, etc.)
	// - bare hashes in "x" tags (file/asset sha256)
	evRows, err := db.Query(`SELECT tags FROM events`)
	if err != nil {
		return nil, fmt.Errorf("failed to query events for blob URLs: %w", err)
	}
	defer evRows.Close()

	for evRows.Next() {
		var tagsRaw []byte
		if err := evRows.Scan(&tagsRaw); err != nil {
			continue
		}

		var tags nostr.Tags
		if err := json.Unmarshal(tagsRaw, &tags); err != nil {
			continue
		}

		for _, tag := range tags {
			if len(tag) < 2 {
				continue
			}

			// "x" tags contain the bare sha256 hash of the blob.
			if tag[0] == "x" {
				addHash(tag[1])
				continue
			}

			// All other tags: look for cdn.zapstore.dev URLs.
			for _, val := range tag[1:] {
				if strings.HasPrefix(val, oldCDN) {
					addHash(strings.TrimPrefix(val, oldCDN))
				}
			}
		}
	}

	return hashes, evRows.Err()
}
