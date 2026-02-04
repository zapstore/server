package acl

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestManualHotReload is a manual test for hot-reloading. Run with:
//
// go test -run TestManualHotReload -v
//
// Then edit files in pkg/acl/testdata/ to see reload logs.
func TestManualHotReload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping manual test in short mode")
	}

	config := Config{
		Dir:                 "testdata",
		UnknownPubkeyPolicy: AllowAll,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	controller, err := New(config, logger)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}
	defer controller.Close()

	t.Logf("watching %s for 30 seconds...", config.Dir)
	t.Logf("allowed_pubkeys: %d", len(controller.AllowedPubkeys()))
	t.Logf("blocked_pubkeys: %d", len(controller.BlockedPubkeys()))
	t.Logf("blocked_events: %d", len(controller.BlockedEvents()))
	t.Logf("blocked_blobs: %d", len(controller.BlockedBlobs()))
	time.Sleep(30 * time.Second)
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		entities []string
		reasons  []string
	}{
		{
			name:    "empty file",
			content: "",
		},
		{
			name:    "only comments",
			content: "# comment 1\n# comment 2",
		},
		{
			name:     "valid",
			content:  "abc,some reason\ndef,another reason",
			entities: []string{"abc", "def"},
			reasons:  []string{"some reason", "another reason"},
		},
		{
			name:     "valid with empty line",
			content:  "abc,some reason\n\ndef,another reason",
			entities: []string{"abc", "def"},
			reasons:  []string{"some reason", "another reason"},
		},
		{
			name:     "valid with comment",
			content:  "abc,some reason\n# comment\ndef,another reason",
			entities: []string{"abc", "def"},
			reasons:  []string{"some reason", "another reason"},
		},
		{
			name:     "valid with quotes",
			content:  "abc,\"some reason with comma\"\ndef,another reason",
			entities: []string{"abc", "def"},
			reasons:  []string{"some reason with comma", "another reason"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeTempFile(t, test.content)

			entities, reasons, err := parseCSV(path)
			if err != nil {
				t.Fatalf("parseCSV() error = %v", err)
			}

			if !slices.Equal(entities, test.entities) {
				t.Errorf("parseCSV() expected entities %v, got %v", test.entities, entities)
			}
			if !slices.Equal(reasons, test.reasons) {
				t.Errorf("parseCSV() expected reasons %v, got %v", test.reasons, reasons)
			}
		})
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}
