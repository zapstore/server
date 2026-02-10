// Command fix-mime corrects MIME types in the blossom database by checking
// which files actually exist on Bunny Storage.
//
// The bug: after uploading a blob as blobs/{hash}.apk, the code sometimes
// stored "application/octet-stream" in the database (Bunny's detected MIME)
// instead of the original MIME. This causes download redirects to point to
// blobs/{hash}.bin (which doesn't exist) instead of blobs/{hash}.apk.
//
// This tool lists all files in the Bunny storage zone, builds a map of
// hash → actual extension, then updates any database rows whose stored MIME
// would produce the wrong extension.
//
// Usage:
//
//	CGO_ENABLED=1 go run ./cmd/fix-mime \
//	  -blossom-db data/blossom.db \
//	  -env cmd/.env \
//	  [-dry-run]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pippellia-btc/blossom"
	blossomBunny "github.com/zapstore/server/pkg/blossom/bunny"
	blossomStore "github.com/zapstore/server/pkg/blossom/store"
)

// bunnyFile represents a file entry returned by the Bunny Storage API.
type bunnyFile struct {
	ObjectName string `json:"ObjectName"`
	Length     int64  `json:"Length"`
	IsDirectory bool  `json:"IsDirectory"`
}

func main() {
	blossomDB := flag.String("blossom-db", "", "path to blossom database")
	envFile := flag.String("env", "cmd/.env", "path to .env file with Bunny credentials")
	dryRun := flag.Bool("dry-run", false, "show what would be changed without writing")
	flag.Parse()

	if *blossomDB == "" {
		fmt.Fprintf(os.Stderr, "Usage: fix-mime -blossom-db <blossom.db> [-env <.env>] [-dry-run]\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load Bunny config.
	if err := godotenv.Load(*envFile); err != nil {
		log.Fatalf("failed to load env file %s: %v", *envFile, err)
	}

	bunnyConfig := blossomBunny.NewConfig()
	if err := env.Parse(&bunnyConfig); err != nil {
		log.Fatalf("failed to parse Bunny config: %v", err)
	}
	if err := bunnyConfig.Validate(); err != nil {
		log.Fatalf("invalid Bunny config: %v", err)
	}

	// List actual files on Bunny Storage.
	log.Println("listing files on Bunny Storage...")
	storageFiles, err := listStorageFiles(bunnyConfig, "blobs/")
	if err != nil {
		log.Fatalf("failed to list storage files: %v", err)
	}
	log.Printf("found %d files on Bunny Storage", len(storageFiles))

	// Build a map: hash → actual extension on storage.
	hashToExt := make(map[string]string, len(storageFiles))
	for _, f := range storageFiles {
		if f.IsDirectory {
			continue
		}
		name := f.ObjectName // e.g. "abcdef1234.apk"
		dot := strings.LastIndex(name, ".")
		if dot < 0 {
			continue
		}
		hash := name[:dot]
		ext := name[dot+1:]
		if err := blossom.ValidateHash(hash); err != nil {
			log.Printf("skipping non-hash file: %s", name)
			continue
		}
		hashToExt[hash] = ext
	}
	log.Printf("indexed %d blob hashes from storage", len(hashToExt))

	// Open blossom DB.
	store, err := blossomStore.New(*blossomDB)
	if err != nil {
		log.Fatalf("failed to open blossom store: %v", err)
	}
	defer store.Close()

	// Query all blobs.
	rows, err := store.DB.Query("SELECT hash, type, size FROM blobs")
	if err != nil {
		log.Fatalf("failed to query blobs: %v", err)
	}

	type mismatch struct {
		hash   string
		oldMIME string
		newMIME string
	}

	var mismatches []mismatch
	var total, missing, matched int

	for rows.Next() {
		var hashHex, mime string
		var size int64
		if err := rows.Scan(&hashHex, &mime, &size); err != nil {
			log.Printf("scan error: %v", err)
			continue
		}
		total++

		actualExt, ok := hashToExt[hashHex]
		if !ok {
			missing++
			continue
		}

		// What extension would the DB's MIME produce?
		dbExt := blossom.ExtFromType(mime)
		if dbExt == actualExt {
			matched++
			continue
		}

		// Mismatch: derive the correct MIME from the actual extension on storage.
		correctMIME := blossom.TypeFromExt(actualExt)
		mismatches = append(mismatches, mismatch{
			hash:    hashHex,
			oldMIME: mime,
			newMIME: correctMIME,
		})
	}
	rows.Close()

	log.Printf("scanned %d DB entries: %d matched, %d not on storage, %d mismatched",
		total, matched, missing, len(mismatches))

	if len(mismatches) == 0 {
		log.Println("nothing to fix!")
		return
	}

	// Show mismatches.
	for _, m := range mismatches {
		log.Printf("  %s: %q (ext=%s) → %q (ext=%s)",
			m.hash,
			m.oldMIME, blossom.ExtFromType(m.oldMIME),
			m.newMIME, blossom.ExtFromType(m.newMIME),
		)
	}

	if *dryRun {
		log.Printf("[DRY RUN] would fix %d entries", len(mismatches))
		return
	}

	// Apply fixes.
	ctx := context.Background()
	var fixed, failed int
	for _, m := range mismatches {
		_, err := store.DB.ExecContext(ctx, "UPDATE blobs SET type = ? WHERE hash = ?", m.newMIME, m.hash)
		if err != nil {
			log.Printf("failed to update %s: %v", m.hash, err)
			failed++
			continue
		}
		fixed++
	}

	log.Printf("fixed %d entries (%d failed)", fixed, failed)
}

// listStorageFiles lists files in the given path on the Bunny Storage Zone.
func listStorageFiles(config blossomBunny.Config, path string) ([]bunnyFile, error) {
	path = strings.TrimPrefix(path, "/")
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	url := fmt.Sprintf("https://%s/%s/%s",
		config.StorageZone.Hostname,
		config.StorageZone.Name,
		path,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("AccessKey", config.StorageZone.Password)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	dir := filepath.Base(strings.TrimSuffix(path, "/"))

	var files []bunnyFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode response for dir %s: %w", dir, err)
	}
	return files, nil
}
