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
	"github.com/zapstore/server/pkg/events"
	relayStore "github.com/zapstore/server/pkg/relay/store"
)

const (
	// cdnPattern is the URL prefix found in event tags (icon, image, url, etc.).
	cdnPattern = "https://cdn.zapstore.dev/"
	// downloadCDN is the base URL to actually download blobs from (the pre-migration CDN).
	downloadCDN = "https://bcdn.zapstore.dev/"
)

func main() {
	from := flag.String("from", "", "path to old relay SQLite database (read-only)")
	relayDB := flag.String("relay-db", "", "path to new relay database (created if missing)")
	blossomDB := flag.String("blossom-db", "", "path to new blossom database (created if missing)")
	envFile := flag.String("env", "cmd/.env", "path to .env file with Bunny credentials")
	skipInvalid := flag.Bool("skip-invalid", false, "skip events with invalid ID/signature instead of aborting")
	dryRun := flag.Bool("dry-run", false, "validate without writing anything")
	blossomFromOld := flag.Bool("blossom-from-old", false, "phase 2: extract blob hashes from the old DB (-from) instead of the relay DB; use when relay DB is missing events")
	flag.Parse()

	if *from == "" || *relayDB == "" || *blossomDB == "" {
		fmt.Fprintf(os.Stderr, "Usage: migrate -from <old.db> -relay-db <new-relay.db> -blossom-db <new-blossom.db> [-env <.env>] [-skip-invalid] [-dry-run] [-blossom-from-old]\n\n")
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
	// Extract hashes from relay DB, or from old DB if -blossom-from-old or dry-run.
	blobSource := *relayDB
	if *dryRun || *blossomFromOld {
		blobSource = *from
		if *blossomFromOld {
			log.Println("phase 2 will extract blob hashes from old DB (-blossom-from-old)")
		}
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
		total, saved, existed, skipped, invalidID, invalidSig, invalidStruct, saveErr int
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

		// Verify event structure
		if err := events.Validate(&ev); err != nil {
			msg := fmt.Sprintf("[%d/%d] %s kind=%d: invalid event structure (err=%v)", stats.total, totalCount, ev.ID, ev.Kind, err)
			if skipInvalid {
				log.Printf("%s — skipping", msg)
				stats.invalidStruct++
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
		resp, err := httpClient.Get(downloadCDN + hashHex)
		if err != nil {
			log.Printf("[%d/%d] %s: download error: %v", i+1, len(hashes), hashHex, err)
			stats.failed++
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			log.Printf("not on old CDN (404): %s", hashHex)
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
			_, size, err = bunny.Check(ctx, blobPath)
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

// extractBlobHashes collects blob hashes from LATEST releases only:
//  1. Get all App events (kind 32267) - these are replaceable, so only latest per app
//  2. Extract CDN URLs from app events (icons, images, etc.)
//  3. Follow the 'a' tag to find the referenced Release event (kind 30063)
//  4. Follow the 'e' tag(s) from the release to find File events (kind 1063/3063)
//  5. Extract the 'x' tag (blob hash) from those file events
func extractBlobHashes(db *sql.DB) ([]string, error) {
	type eventInfo struct {
		PubKey    string
		CreatedAt int64
		Kind      int
		Tags      nostr.Tags
	}

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

	// Build a map of all events by ID for quick lookup.
	eventMap := make(map[string]eventInfo)
	allRows, err := db.Query(`SELECT id, pubkey, created_at, kind, tags FROM events`)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	for allRows.Next() {
		var id, pubkey string
		var createdAt int64
		var kind int
		var tagsRaw []byte
		if err := allRows.Scan(&id, &pubkey, &createdAt, &kind, &tagsRaw); err != nil {
			continue
		}
		var tags nostr.Tags
		if err := json.Unmarshal(tagsRaw, &tags); err != nil {
			continue
		}
		eventMap[id] = eventInfo{
			PubKey:    pubkey,
			CreatedAt: createdAt,
			Kind:      kind,
			Tags:      tags,
		}
	}
	allRows.Close()
	log.Printf("loaded %d events into memory", len(eventMap))

	// Build a map of Release events by their 'd' tag for quick lookup
	// Key: "30063:<pubkey>:<d-tag>" -> event ID
	releaseMap := make(map[string]string)
	// Track latest release IDs by app key as fallback when app->release linkage is missing.
	// Key is usually release 'a' tag ("32267:<pubkey>:<app-id>"), with legacy fallback
	// to "<pubkey>:<app-id>" parsed from 'd' tag "<app-id>@<version>".
	latestReleaseByAppKey := make(map[string]string)
	findTag := func(tags nostr.Tags, key string) string {
		for _, t := range tags {
			if len(t) >= 2 && t[0] == key {
				return t[1]
			}
		}
		return ""
	}
	releaseRows, err := db.Query(`SELECT id, pubkey, tags FROM events WHERE kind = 30063`)
	if err != nil {
		return nil, fmt.Errorf("failed to query release events: %w", err)
	}
	for releaseRows.Next() {
		var id, pubkey string
		var tagsRaw []byte
		if err := releaseRows.Scan(&id, &pubkey, &tagsRaw); err != nil {
			continue
		}
		var tags nostr.Tags
		if err := json.Unmarshal(tagsRaw, &tags); err != nil {
			continue
		}
		for _, tag := range tags {
			if len(tag) >= 2 && tag[0] == "d" {
				key := fmt.Sprintf("30063:%s:%s", pubkey, tag[1])
				releaseMap[key] = id
				break
			}
		}

		// Track the latest release per app key for fallback extraction.
		appKey := findTag(tags, "a")
		if appKey == "" {
			d := findTag(tags, "d")
			if i := strings.LastIndex(d, "@"); i > 0 {
				appKey = pubkey + ":" + d[:i]
			}
		}
		if appKey != "" {
			current, ok := latestReleaseByAppKey[appKey]
			if !ok {
				latestReleaseByAppKey[appKey] = id
			} else if cur, curOK := eventMap[current]; !curOK {
				latestReleaseByAppKey[appKey] = id
			} else if next, nextOK := eventMap[id]; nextOK && next.CreatedAt > cur.CreatedAt {
				latestReleaseByAppKey[appKey] = id
			}
		}
	}
	releaseRows.Close()
	log.Printf("indexed %d release events", len(releaseMap))

	// --- Step 1: Process all App events (kind 32267) ---
	appRows, err := db.Query(`SELECT id, tags FROM events WHERE kind = 32267`)
	if err != nil {
		return nil, fmt.Errorf("failed to query app events: %w", err)
	}

	selectedReleaseIDs := make(map[string]bool)
	releaseKeyFromLatestApps := make(map[string]bool)
	var appCount int
	for appRows.Next() {
		var appID string
		var tagsRaw []byte
		if err := appRows.Scan(&appID, &tagsRaw); err != nil {
			continue
		}

		var tags nostr.Tags
		if err := json.Unmarshal(tagsRaw, &tags); err != nil {
			continue
		}
		appCount++

		var releaseRef string
		for _, tag := range tags {
			// Find the 'a' tag pointing to the release
			if len(tag) >= 2 && tag[0] == "a" {
				releaseRef = tag[1]
			}
		}

		if releaseRef == "" {
			continue
		}

		// Find the release event
		releaseID, ok := releaseMap[releaseRef]
		if !ok {
			continue
		}
		releaseKeyFromLatestApps[releaseRef] = true
		selectedReleaseIDs[releaseID] = true
	}
	appRows.Close()

	// Fallback: if app linkage is incomplete in the source DB, still include latest
	// releases discovered directly from kind 30063 events.
	var fallbackReleaseCount int
	for appKey, releaseID := range latestReleaseByAppKey {
		if releaseKeyFromLatestApps[appKey] {
			continue
		}
		if !selectedReleaseIDs[releaseID] {
			selectedReleaseIDs[releaseID] = true
			fallbackReleaseCount++
		}
	}

	var cdnCount, fileCount, selectedReleaseCount int
	for releaseID := range selectedReleaseIDs {
		releaseInfo, ok := eventMap[releaseID]
		if !ok {
			continue
		}
		releaseTags := releaseInfo.Tags
		selectedReleaseCount++

		// Extract CDN URLs from latest app event that references this release.
		// This keeps app assets (icons, images) in the migrated blob set.
		for _, ev := range eventMap {
			if ev.Kind != 32267 {
				continue
			}
			if findTag(ev.Tags, "a") != fmt.Sprintf("30063:%s:%s", releaseInfo.PubKey, findTag(releaseTags, "d")) {
				continue
			}
			for _, tag := range ev.Tags {
				for _, val := range tag[1:] {
					if strings.HasPrefix(val, cdnPattern) {
						before := len(hashes)
						addHash(strings.TrimPrefix(val, cdnPattern))
						if len(hashes) > before {
							cdnCount++
						}
					}
				}
			}
			break
		}

		// Extract CDN URLs from release event
		for _, tag := range releaseTags {
			for _, val := range tag[1:] {
				if strings.HasPrefix(val, cdnPattern) {
					before := len(hashes)
					addHash(strings.TrimPrefix(val, cdnPattern))
					if len(hashes) > before {
						cdnCount++
					}
				}
			}
		}

		// Find file events referenced by 'e' tags in the release
		for _, tag := range releaseTags {
			if len(tag) >= 2 && tag[0] == "e" {
				fileEventID := tag[1]
				fileInfo, ok := eventMap[fileEventID]
				if !ok {
					continue
				}
				fileTags := fileInfo.Tags

				// Extract CDN URLs from file event
				for _, ftag := range fileTags {
					for _, val := range ftag[1:] {
						if strings.HasPrefix(val, cdnPattern) {
							before := len(hashes)
							addHash(strings.TrimPrefix(val, cdnPattern))
							if len(hashes) > before {
								cdnCount++
							}
						}
					}
				}

				// Extract 'x' tag (blob hash) from file event
				for _, ftag := range fileTags {
					if len(ftag) >= 2 && ftag[0] == "x" {
						before := len(hashes)
						addHash(ftag[1])
						if len(hashes) > before {
							fileCount++
						}
						break
					}
				}
			}
		}
	}

	log.Printf("processed %d app events", appCount)
	log.Printf("selected %d latest release events (%d via app chain, %d via fallback)",
		selectedReleaseCount, selectedReleaseCount-fallbackReleaseCount, fallbackReleaseCount)
	log.Printf("found %d cdn URL hashes from latest releases", cdnCount)
	log.Printf("found %d file hashes (x tags) from latest releases", fileCount)

	return hashes, nil
}
