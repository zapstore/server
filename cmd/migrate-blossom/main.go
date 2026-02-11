// Command migrate-blossom ensures that every blob referenced by the latest app
// events on the Zapstore relay is present on the Bunny CDN behind cdn.zapstore.dev.
//
// It works in two phases:
//
//  1. Collect: connects to the relay, fetches all apps (kind 32267), follows their
//     release pointers (kind 30063) and asset events, and builds a deduplicated
//     list of blob hashes from icon/image tags and x tags.
//
//  2. Upload: for each hash, verifies the full redirect chain
//     (cdn.zapstore.dev → 307 → Bunny CDN URL → 200). If the blob is missing,
//     downloads it from bcdn.zapstore.dev and uploads it to the Bunny storage zone,
//     saves metadata to the blossom database, then re-verifies the chain.
//
// Both phases are idempotent: re-running skips blobs already verified on Bunny.
//
// In dry-run mode only phase 1 runs and all collected URLs are printed.
//
// Bunny credentials are read from the .env file specified by -env (default: cmd/.env).
//
// Usage:
//
//	go run ./cmd/migrate-blossom \
//	  -blossom-db data/blossom.db \
//	  -env cmd/.env
//
//	go run ./cmd/migrate-blossom -dry-run
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
	blossomBunny "github.com/zapstore/server/pkg/blossom/bunny"
	blossomStore "github.com/zapstore/server/pkg/blossom/store"
)

const (
	cdnBase      = "https://cdn.zapstore.dev/"
	bcdnBase     = "https://bcdn.zapstore.dev/"
	defaultRelay = "wss://relay.zapstore.dev"
)

func main() {
	relayURL := flag.String("relay", defaultRelay, "relay WebSocket URL")
	blossomDBPath := flag.String("blossom-db", "", "path to blossom database")
	envFile := flag.String("env", "cmd/.env", "path to .env file with Bunny credentials")
	dryRun := flag.Bool("dry-run", false, "collect and print all URLs without uploading")
	flag.Parse()

	if !*dryRun && *blossomDBPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: migrate-blossom -blossom-db <path> [-env <.env>] [-relay <url>] [-dry-run]")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx := context.Background()
	start := time.Now()

	// ── Phase 1: Collect ─────────────────────────────────────────────────────

	log.Println("═══ Collecting blob hashes from relay ═══")

	hashes, err := collectHashes(ctx, *relayURL)
	if err != nil {
		log.Fatalf("collect: %v", err)
	}
	log.Printf("collected %d unique blob hashes\n", len(hashes))

	if len(hashes) == 0 {
		log.Println("nothing to migrate")
		return
	}

	if *dryRun {
		printDryRun(hashes, start)
		return
	}

	// ── Phase 2: Upload ──────────────────────────────────────────────────────

	log.Println("\n═══ Uploading missing blobs ═══")

	if err := godotenv.Load(*envFile); err != nil {
		log.Fatalf("env: %v", err)
	}

	bunnyConfig := blossomBunny.NewConfig()
	if err := env.Parse(&bunnyConfig); err != nil {
		log.Fatalf("bunny config parse: %v", err)
	}
	if err := bunnyConfig.Validate(); err != nil {
		log.Fatalf("bunny config validate: %v", err)
	}

	bunny, err := blossomBunny.NewClient(bunnyConfig)
	if err != nil {
		log.Fatalf("bunny client: %v", err)
	}

	store, err := blossomStore.New(*blossomDBPath)
	if err != nil {
		log.Fatalf("blossom store: %v", err)
	}
	defer store.Close()

	uploadMissing(ctx, hashes, bunny, store)

	log.Printf("\nall done in %s", time.Since(start).Round(time.Millisecond))
}

// ─── Collect ─────────────────────────────────────────────────────────────────

// blobInfo tracks a hash and the MIME type we know it should have (if any).
type blobInfo struct {
	hash string
	mime string // may be empty for icon/image hashes where we don't know yet
}

// collectHashes connects to the relay, walks apps → releases → assets,
// and returns a deduplicated list of blob hashes with optional MIME hints.
func collectHashes(ctx context.Context, relayURL string) ([]blobInfo, error) {
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", relayURL, err)
	}
	defer relay.Close()

	seen := make(map[string]bool)
	var blobs []blobInfo

	addHash := func(h, mime string) {
		h = strings.TrimSpace(h)
		if len(h) != 64 || seen[h] {
			return
		}
		if blossom.ValidateHash(h) != nil {
			return
		}
		seen[h] = true
		blobs = append(blobs, blobInfo{hash: h, mime: mime})
	}

	// Step 1: Fetch all apps (kind 32267).
	log.Printf("  fetching apps (kind 32267) from %s ...", relayURL)
	apps, err := fetchAll(ctx, relay, nostr.Filter{Kinds: []int{32267}})
	if err != nil {
		return nil, fmt.Errorf("fetch apps: %w", err)
	}
	log.Printf("  %d apps", len(apps))

	// Step 2: Extract CDN URLs from icon/image tags + collect release coordinates.
	type coord struct{ pubkey, dTag string }
	var coords []coord
	coordSet := make(map[string]bool)

	for _, app := range apps {
		for _, tag := range app.Tags {
			if len(tag) < 2 {
				continue
			}
			switch tag[0] {
			case "icon", "image":
				if strings.HasPrefix(tag[1], cdnBase) {
					addHash(strings.TrimPrefix(tag[1], cdnBase), "")
				}
			case "a":
				parts := strings.SplitN(tag[1], ":", 3)
				if len(parts) == 3 && parts[0] == "30063" {
					key := parts[1] + ":" + parts[2]
					if !coordSet[key] {
						coordSet[key] = true
						coords = append(coords, coord{parts[1], parts[2]})
					}
				}
			}
		}
	}

	iconImageCount := len(blobs)
	log.Printf("  %d icon/image hashes", iconImageCount)
	log.Printf("  %d release pointers (a tags)", len(coords))

	if len(coords) == 0 {
		return blobs, nil
	}

	// Step 3: Fetch releases (kind 30063) that apps point to.
	log.Println("  fetching releases (kind 30063) ...")
	authors := unique(coords, func(c coord) string { return c.pubkey })
	dTags := unique(coords, func(c coord) string { return c.dTag })

	releases, err := fetchAll(ctx, relay, nostr.Filter{
		Kinds:   []int{30063},
		Authors: authors,
		Tags:    nostr.TagMap{"d": dTags},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	log.Printf("  %d releases", len(releases))

	// Keep only releases that match an app's 'a' tag and collect their 'e' tag IDs.
	var assetIDs []string
	assetSet := make(map[string]bool)

	for _, rel := range releases {
		d := firstTag(rel.Tags, "d")
		key := rel.PubKey + ":" + d
		if !coordSet[key] {
			continue
		}
		for _, tag := range rel.Tags {
			if len(tag) >= 2 && tag[0] == "e" && !assetSet[tag[1]] {
				assetSet[tag[1]] = true
				assetIDs = append(assetIDs, tag[1])
			}
		}
	}
	log.Printf("  %d asset event references (e tags)", len(assetIDs))

	// Step 4: Fetch asset events in batches (max 100 IDs per request) and extract 'x' tags.
	if len(assetIDs) > 0 {
		log.Println("  fetching asset events ...")
		const batch = 100
		for i := 0; i < len(assetIDs); i += batch {
			end := min(i+batch, len(assetIDs))
			evts, err := relay.QuerySync(ctx, nostr.Filter{IDs: assetIDs[i:end]})
			if err != nil {
				log.Printf("    warning: batch [%d:%d] failed: %v", i, end, err)
				continue
			}
			for _, ev := range evts {
				x := firstTag(ev.Tags, "x")
				if x == "" {
					continue
				}
				mime := firstTag(ev.Tags, "m")
				if mime == "" {
					log.Printf("    skipping asset %s: no 'm' tag", ev.ID)
					continue
				}
				addHash(x, mime)
			}
		}
	}

	log.Printf("  %d asset hashes (x tags)", len(blobs)-iconImageCount)

	return blobs, nil
}

// ─── Upload ──────────────────────────────────────────────────────────────────

func uploadMissing(ctx context.Context, blobs []blobInfo, bunny blossomBunny.Client, store *blossomStore.Store) {
	// HTTP client that does NOT follow redirects (to capture 307 + Location).
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 30 * time.Second,
	}

	dlClient := &http.Client{Timeout: 5 * time.Minute}

	// Step 1: Fix MIME types in blossom DB.
	// Entries with wrong MIME cause 307 → .bin → 404 when the file is actually .apk, etc.
	var mimeFixed int
	for _, blob := range blobs {
		if blob.mime == "" {
			continue
		}
		res, err := store.DB.ExecContext(ctx,
			`UPDATE blobs SET type = ? WHERE hash = ? AND type != ?`,
			blob.mime, blob.hash, blob.mime,
		)
		if err != nil {
			log.Printf("  mime fix %s: %v", blob.hash, err)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			mimeFixed++
		}
	}
	if mimeFixed > 0 {
		log.Printf("  fixed MIME type for %d blobs in blossom DB", mimeFixed)
	}

	// Step 2: Verify and upload.
	var stats struct {
		total, existed, uploaded, notFound, failed int
	}

	for i, blob := range blobs {
		stats.total++
		tag := fmt.Sprintf("[%d/%d]", i+1, len(blobs))

		// Progress logging every 200 blobs.
		if i > 0 && i%200 == 0 {
			log.Printf("  progress: %d/%d checked (%d existed, %d uploaded, %d failed)",
				i, len(blobs), stats.existed, stats.uploaded, stats.failed)
		}

		// Check full chain: cdn → 307 → Location → 200.
		exists, err := verifyBlob(ctx, noRedirect, dlClient, blob.hash)
		if err != nil {
			log.Printf("%s %s: cdn check error: %v — will try upload", tag, blob.hash, err)
		}
		if exists {
			stats.existed++
			continue
		}

		// Download from bcdn.
		resp, err := dlClient.Get(bcdnBase + blob.hash)
		if err != nil {
			log.Printf("%s %s: bcdn download error: %v", tag, blob.hash, err)
			stats.failed++
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			log.Printf("%s %s: not on bcdn (404)", tag, blob.hash)
			stats.notFound++
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("%s %s: bcdn status %s", tag, blob.hash, resp.Status)
			stats.failed++
			continue
		}

		// Use the MIME hint from the event if available; fall back to the response header.
		mime := blob.mime
		if mime == "" {
			mime = resp.Header.Get("Content-Type")
		}
		size := resp.ContentLength

		hash, err := blossom.ParseHash(blob.hash)
		if err != nil {
			resp.Body.Close()
			log.Printf("%s %s: bad hash: %v", tag, blob.hash, err)
			stats.failed++
			continue
		}

		// Upload to Bunny storage zone.
		blobPath := "blobs/" + blob.hash + "." + blossom.ExtFromType(mime)
		if err := bunny.Upload(ctx, resp.Body, blobPath, blob.hash); err != nil {
			resp.Body.Close()
			log.Printf("%s %s: bunny upload error: %v", tag, blob.hash, err)
			stats.failed++
			continue
		}
		resp.Body.Close()

		// Get size from Bunny if Content-Length was missing.
		if size <= 0 {
			_, size, err = bunny.Check(ctx, blobPath)
			if err != nil {
				log.Printf("%s %s: bunny size check error: %v", tag, blob.hash, err)
				stats.failed++
				continue
			}
		}

		// Save metadata to blossom DB (idempotent via INSERT OR IGNORE).
		if _, err := store.Save(ctx, blossomStore.BlobMeta{
			Hash:      hash,
			Type:      mime,
			Size:      size,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			log.Printf("%s %s: db save error: %v", tag, blob.hash, err)
			stats.failed++
			continue
		}

		// Verify the full redirect chain after upload, with retries for CDN propagation.
		var ok bool
		for attempt := range 3 {
			if attempt > 0 {
				time.Sleep(2 * time.Second)
			}
			ok, err = verifyBlob(ctx, noRedirect, dlClient, blob.hash)
			if ok {
				break
			}
		}
		if err != nil || !ok {
			log.Printf("%s %s: VERIFY FAILED after upload (ok=%v err=%v)", tag, blob.hash, ok, err)
			stats.failed++
			continue
		}

		stats.uploaded++
		log.Printf("%s %s: uploaded (%s)", tag, blob.hash, mime)
	}

	log.Println("\n─── Summary ───")
	log.Printf("  total:     %d", stats.total)
	log.Printf("  existed:   %d (verified on Bunny CDN)", stats.existed)
	log.Printf("  uploaded:  %d", stats.uploaded)
	log.Printf("  not found: %d (missing from bcdn)", stats.notFound)
	log.Printf("  failed:    %d", stats.failed)
	log.Printf("  mime fixed:%d (corrected in blossom DB)", mimeFixed)
}

// ─── Verify ──────────────────────────────────────────────────────────────────

// verifyBlob checks the full redirect chain:
//
//	GET cdn.zapstore.dev/<hash>  →  expect 307
//	HEAD Location                →  expect 200
func verifyBlob(ctx context.Context, noRedirect, regular *http.Client, hashHex string) (bool, error) {
	// Step 1: GET cdn (no redirect follow).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnBase+hashHex, nil)
	if err != nil {
		return false, err
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return false, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusTemporaryRedirect {
		return false, fmt.Errorf("cdn: expected 307 or 404, got %s", resp.Status)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return false, fmt.Errorf("cdn: 307 with empty Location")
	}

	// Step 2: HEAD the Bunny CDN URL from the Location header.
	req2, err := http.NewRequestWithContext(ctx, http.MethodHead, location, nil)
	if err != nil {
		return false, err
	}
	resp2, err := regular.Do(req2)
	if err != nil {
		return false, err
	}
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return false, fmt.Errorf("bunny: expected 200, got %s (url: %s)", resp2.Status, location)
	}
	return true, nil
}

// ─── Dry run ─────────────────────────────────────────────────────────────────

func printDryRun(blobs []blobInfo, start time.Time) {
	fmt.Println()
	fmt.Println("═══ DRY RUN — Collected blob URLs ═══")
	fmt.Println()
	for i, b := range blobs {
		mimeLabel := b.mime
		if mimeLabel == "" {
			mimeLabel = "(detect from bcdn)"
		}
		fmt.Printf("  [%3d] cdn:  %s%s\n", i+1, cdnBase, b.hash)
		fmt.Printf("        bcdn: %s%s\n", bcdnBase, b.hash)
		fmt.Printf("        mime: %s\n", mimeLabel)
	}
	fmt.Println()
	log.Printf("%d blob hashes in %s", len(blobs), time.Since(start).Round(time.Millisecond))
}

// ─── Relay pagination ────────────────────────────────────────────────────────

// fetchAll paginates through a relay query using Until as a cursor.
// The relay enforces a maximum of 100 events per request.
func fetchAll(ctx context.Context, relay *nostr.Relay, filter nostr.Filter) ([]*nostr.Event, error) {
	const limit = 100
	filter.Limit = limit

	var all []*nostr.Event
	seen := make(map[string]bool)

	for {
		events, err := relay.QuerySync(ctx, filter)
		if err != nil {
			return all, err
		}
		if len(events) == 0 {
			break
		}

		var added int
		var oldest nostr.Timestamp
		for _, ev := range events {
			if !seen[ev.ID] {
				seen[ev.ID] = true
				all = append(all, ev)
				added++
			}
			if oldest == 0 || ev.CreatedAt < oldest {
				oldest = ev.CreatedAt
			}
		}

		// No new events — all duplicates from overlapping timestamps.
		if added == 0 {
			break
		}

		// Fewer than limit means we've exhausted the result set.
		if len(events) < limit {
			break
		}

		// Move the cursor back.
		filter.Until = &oldest
	}

	return all, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func firstTag(tags nostr.Tags, key string) string {
	for _, t := range tags {
		if len(t) >= 2 && t[0] == key {
			return t[1]
		}
	}
	return ""
}

func unique[T any](items []T, key func(T) string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, item := range items {
		k := key(item)
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
