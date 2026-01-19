package legacy

import (
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const KindRelease = 30063

type Release struct {
	// Required fields
	ID      string   // Release identifier (from "d" tag, before '@')
	Version string   // Release version (from "d" tag, after '@')
	App     string   // Reference to app identifier "32267:<pubkey>:<packageID>" (from "a" tag)
	Files   []string // File event IDs (at least one required) (from "e" tag)

	// Optional fields
	URL     string // URL of the release (from "url" tag)
	R       string // Same as URL, duplicate refecence tag (from "r" tag)
	Commit  string // Git commit hash for reproducible builds (from "commit" tag)
	Content string // Full release notes, markdown allowed (from "content" tag)
}

func (r Release) Validate(pubkey string) error {
	if r.ID == "" {
		return fmt.Errorf("missing or invalid 'd' tag (identifier)")
	}
	if r.Version == "" {
		return fmt.Errorf("missing or invalid 'd' tag (version)")
	}

	if r.App == "" {
		return fmt.Errorf("missing or empty 'a' tag (app identifier)")
	}
	expectedApp := "32267:" + pubkey + ":" + r.ID
	if r.App != expectedApp {
		return fmt.Errorf("invalid 'a' tag: expected '%s', got '%s'", expectedApp, r.App)
	}

	if len(r.Files) == 0 {
		return fmt.Errorf("missing 'e' tag (file IDs)")
	}
	for i, file := range r.Files {
		if err := ValidateHash(file); err != nil {
			return fmt.Errorf("invalid file ID in 'e' tag at index %d: %w", i, err)
		}
	}
	return nil
}

func ParseRelease(event *nostr.Event) (Release, error) {
	if event.Kind != KindRelease {
		return Release{}, fmt.Errorf("invalid kind: expected %d, got %d", KindRelease, event.Kind)
	}

	r := Release{Content: event.Content}
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "d":
			if r.ID != "" {
				return Release{}, fmt.Errorf("duplicate 'd' tag")
			}

			parts := strings.Split(tag[1], "@")
			if len(parts) != 2 {
				return Release{}, fmt.Errorf("invalid 'd' tag: must contain '@'")
			}

			r.ID = parts[0]
			r.Version = parts[1]

		case "a":
			if r.App != "" {
				return Release{}, fmt.Errorf("duplicate 'a' tag")
			}
			r.App = tag[1]

		case "e":
			r.Files = append(r.Files, tag[1])

		case "url":
			if r.URL != "" {
				return Release{}, fmt.Errorf("duplicate 'url' tag")
			}
			r.URL = tag[1]

		case "r":
			if r.R != "" {
				return Release{}, fmt.Errorf("duplicate 'r' tag")
			}
			r.R = tag[1]

		case "commit":
			if r.Commit != "" {
				return Release{}, fmt.Errorf("duplicate 'commit' tag")
			}
			r.Commit = tag[1]
		}
	}
	return r, nil
}

func ValidateRelease(event *nostr.Event) error {
	release, err := ParseRelease(event)
	if err != nil {
		return err
	}
	return release.Validate(event.PubKey)
}
