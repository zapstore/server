package sqlite

import (
	"cmp"
	"context"
	"slices"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	sqlite "github.com/vertex-lab/nostr-sqlite"
)

func TestAppTagsIndexing(t *testing.T) {
	store, err := NewStore(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	event := &nostr.Event{
		ID:        "app123",
		PubKey:    "pubkey123",
		CreatedAt: nostr.Timestamp(1700000000),
		Kind:      32267, // KindApp
		Tags: nostr.Tags{
			{"d", "com.example.app"},
			{"name", "Example App"},
			{"f", "android-arm64-v8a"},
			{"f", "linux-x86_64"},
			{"summary", "A short description"},
			{"icon", "https://example.com/icon.png"},
			{"image", "https://example.com/screenshot1.png"},
			{"image", "https://example.com/screenshot2.png"},
			{"t", "productivity"},
			{"t", "tools"},
			{"url", "https://example.com"},
			{"repository", "https://github.com/example/app"},
			{"license", "MIT"},
			// Tags that should NOT be indexed
			{"unrelated", "should-not-index"},
		},
		Content: "Full app description",
		Sig:     "sig123",
	}

	saved, err := store.Save(ctx, event)
	if err != nil {
		t.Fatalf("failed to save event: %v", err)
	}
	if !saved {
		t.Fatal("event was not saved")
	}

	got := getIndexedTags(t, store, event.ID)
	want := expectedTags(event)

	if !equalTags(got, want) {
		t.Errorf("indexed tags mismatch\ngot:  %v\nwant: %v", got, want)
	}
}

func TestReleaseTagsIndexing(t *testing.T) {
	store, err := NewStore(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	event := &nostr.Event{
		ID:        "release123",
		PubKey:    "pubkey123",
		CreatedAt: nostr.Timestamp(1700000000),
		Kind:      30063, // KindRelease
		Tags: nostr.Tags{
			{"d", "com.example.app@1.0.0"},
			{"i", "com.example.app"},
			{"version", "1.0.0"},
			{"c", "stable"},
			{"e", "asset123"},
			{"e", "asset456"},
			// Tags that should NOT be indexed
			{"unrelated", "should-not-index"},
		},
		Content: "Release notes",
		Sig:     "sig123",
	}

	saved, err := store.Save(ctx, event)
	if err != nil {
		t.Fatalf("failed to save event: %v", err)
	}
	if !saved {
		t.Fatal("event was not saved")
	}

	got := getIndexedTags(t, store, event.ID)
	want := expectedTags(event)

	if !equalTags(got, want) {
		t.Errorf("indexed tags mismatch\ngot:  %v\nwant: %v", got, want)
	}
}

func TestAssetTagsIndexing(t *testing.T) {
	store, err := NewStore(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	event := &nostr.Event{
		ID:        "asset123",
		PubKey:    "pubkey123",
		CreatedAt: nostr.Timestamp(1700000000),
		Kind:      3063, // KindAsset
		Tags: nostr.Tags{
			{"i", "com.example.app"},
			{"x", "abc123def456"},
			{"version", "1.0.0"},
			{"f", "android-arm64-v8a"},
			{"f", "android-armeabi-v7a"},
			{"url", "https://cdn.example.com/app.apk"},
			{"m", "application/vnd.android.package-archive"},
			{"size", "12345678"},
			{"min_platform_version", "21"},
			{"target_platform_version", "34"},
			{"supported_nip", "01"},
			{"supported_nip", "04"},
			{"filename", "app-release.apk"},
			{"variant", ""},
			{"commit", "abc123"},
			{"min_allowed_version", "0.9.0"},
			{"version_code", "100"},
			{"min_allowed_version_code", "90"},
			{"apk_certificate_hash", "hash123"},
			{"apk_certificate_hash", "hash456"},
			{"executable", "bin/app"},
			{"executable", "bin/helper"},
			// Tags that should NOT be indexed
			{"unrelated", "should-not-index"},
		},
		Content: "",
		Sig:     "sig123",
	}

	saved, err := store.Save(ctx, event)
	if err != nil {
		t.Fatalf("failed to save event: %v", err)
	}
	if !saved {
		t.Fatal("event was not saved")
	}

	got := getIndexedTags(t, store, event.ID)
	want := expectedTags(event)

	if !equalTags(got, want) {
		t.Errorf("indexed tags mismatch\ngot:  %v\nwant: %v", got, want)
	}
}

func TestFileTagsIndexing(t *testing.T) {
	store, err := NewStore(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	event := &nostr.Event{
		ID:        "file123",
		PubKey:    "pubkey123",
		CreatedAt: nostr.Timestamp(1700000000),
		Kind:      1063, // KindFile (legacy)
		Tags: nostr.Tags{
			{"x", "abc123def456"},
			{"url", "https://cdn.example.com/app.apk"},
			{"fallback", "https://backup.example.com/app.apk"},
			{"m", "application/vnd.android.package-archive"},
			{"version", "1.0.0"},
			{"version_code", "100"},
			{"f", "android-arm64-v8a"},
			{"f", "android-armeabi-v7a"},
			{"apk_signature_hash", "sighash123"},
			{"min_sdk_version", "21"},
			{"target_sdk_version", "34"},
			// Tags that should NOT be indexed
			{"unrelated", "should-not-index"},
		},
		Content: "",
		Sig:     "sig123",
	}

	saved, err := store.Save(ctx, event)
	if err != nil {
		t.Fatalf("failed to save event: %v", err)
	}
	if !saved {
		t.Fatal("event was not saved")
	}

	got := getIndexedTags(t, store, event.ID)
	want := expectedTags(event)

	if !equalTags(got, want) {
		t.Errorf("indexed tags mismatch\ngot:  %v\nwant: %v", got, want)
	}
}

// getIndexedTags returns all tags indexed for an event from the tags table,
// sorted in lexicographic order by key then value.
func getIndexedTags(t *testing.T, store *sqlite.Store, eventID string) nostr.Tags {
	t.Helper()
	rows, err := store.DB.Query("SELECT key, value FROM tags WHERE event_id = ?", eventID)
	if err != nil {
		t.Fatalf("failed to query tags: %v", err)
	}
	defer rows.Close()

	var tags nostr.Tags
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatalf("failed to scan tag row: %v", err)
		}
		tags = append(tags, nostr.Tag{key, value})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("error iterating tag rows: %v", err)
	}

	sortTags(tags)
	return tags
}

// expectedTags extracts tags from an event, filtering out "unrelated" tags
// and keeping only the first two elements of each tag.
// Returns tags sorted in lexicographic order by key then value.
func expectedTags(event *nostr.Event) nostr.Tags {
	var tags nostr.Tags
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		if tag[0] == "unrelated" {
			continue
		}
		tags = append(tags, nostr.Tag{tag[0], tag[1]})
	}

	sortTags(tags)
	return tags
}

// sortTags sorts tags in lexicographic order by key then value.
func sortTags(tags nostr.Tags) {
	slices.SortFunc(tags, func(a, b nostr.Tag) int {
		if c := cmp.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return cmp.Compare(a[1], b[1])
	})
}

// equalTags compares two nostr.Tags for equality.
func equalTags(a, b nostr.Tags) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !slices.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
