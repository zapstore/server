package analytics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const mmdbURL = "https://downloads.ip66.dev/db/ip66.mmdb"

// downloadMMDB downloads the ip66.mmdb file from the ip66.dev endpoint and
// atomically stores it at the specified path. The download is written to a
// temporary file first, then renamed into place, so the existing file (if any)
// is never left in a partial state.
func downloadMMDB(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mmdbURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %s: %s", resp.Status, body)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "ip66.*.mmdb.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to move mmdb into place: %w", err)
	}
	return nil
}
