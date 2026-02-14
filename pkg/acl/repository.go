package acl

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// zapstoreConfig represents the relevant fields of a zapstore.yaml file.
type zapstoreConfig struct {
	Pubkey string `yaml:"pubkey"`
}

// VerifyByRepository checks if a pubkey is authorized to publish by fetching the
// zapstore.yaml from the given repository URL and checking if it contains a matching pubkey.
//
// If the pubkey matches, it is appended to the allowed pubkeys CSV file so that
// subsequent events from this pubkey are allowed without another fetch.
//
// Only GitHub repositories are supported.
func (c *Controller) VerifyByRepository(ctx context.Context, pubkey, repoURL string) (bool, error) {
	rawURL, err := rawZapstoreYAMLURL(repoURL)
	if err != nil {
		return false, fmt.Errorf("acl: invalid repository URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, fmt.Errorf("acl: failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("acl: failed to fetch zapstore.yaml: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("acl: unexpected status fetching zapstore.yaml: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false, fmt.Errorf("acl: failed to read zapstore.yaml: %w", err)
	}

	var config zapstoreConfig
	if err := yaml.Unmarshal(body, &config); err != nil {
		return false, fmt.Errorf("acl: failed to parse zapstore.yaml: %w", err)
	}

	if config.Pubkey == "" {
		return false, nil
	}

	configPubkey, err := toPubkey(config.Pubkey)
	if err != nil {
		return false, fmt.Errorf("acl: invalid pubkey in zapstore.yaml: %w", err)
	}

	if configPubkey != pubkey {
		return false, nil
	}

	// Pubkey verified: append to allowed list so subsequent events are allowed immediately.
	if err := c.appendAllowedPubkey(pubkey, repoURL); err != nil {
		c.log.Error("acl: failed to append verified pubkey to allowed list", "error", err, "pubkey", pubkey)
		// Still return true: the pubkey is verified even if we failed to persist it.
	}

	c.log.Info("acl: pubkey verified via repository",
		slog.String("pubkey", pubkey),
		slog.String("repository", repoURL),
	)
	return true, nil
}

// appendAllowedPubkey appends a pubkey to the allowed pubkeys CSV file.
// The file watcher will pick up the change and reload the list automatically.
func (c *Controller) appendAllowedPubkey(pubkey, reason string) error {
	csvPath := filepath.Join(c.dir, PubkeysAllowedFile)

	f, err := os.OpenFile(csvPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", PubkeysAllowedFile, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s,\"%s\"\n", pubkey, reason); err != nil {
		return fmt.Errorf("failed to write to %s: %w", PubkeysAllowedFile, err)
	}
	return nil
}

// rawZapstoreYAMLURL converts a GitHub repository URL to a raw content URL for zapstore.yaml.
// For example:
//
//	https://github.com/user/repo -> https://raw.githubusercontent.com/user/repo/HEAD/zapstore.yaml
func rawZapstoreYAMLURL(repoURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if u.Host != "github.com" {
		return "", fmt.Errorf("unsupported host: %s (only github.com is supported)", u.Host)
	}

	// Clean the path: remove trailing slashes and .git suffix.
	p := strings.TrimSuffix(path.Clean(u.Path), ".git")

	// Validate that the path has at least two segments: /user/repo
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid repository path: %s (expected github.com/user/repo)", u.Path)
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/zapstore.yaml", parts[0], parts[1]), nil
}
