package events

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

const KindSoftwareRelease = 30063

// SoftwareRelease represents a parsed Software Release event (kind 30063).
type SoftwareRelease struct {
	// Required fields
	I        string   // App identifier (must match 'd' tag from application)
	Version  string   // Release version (e.g., "1.2.3", "v0.1.2-rc1")
	D        string   // Identifier@version
	Channel  string   // Channel ID
	AssetIDs []string // Asset event IDs (at least one required)

	// Content
	Content string // Full release notes, markdown allowed
}

// ParseSoftwareRelease extracts a SoftwareRelease from a nostr.Event.
// Returns an error if the event kind is wrong or if duplicate singular tags are found.
func ParseSoftwareRelease(event *nostr.Event) (SoftwareRelease, error) {
	if event.Kind != KindSoftwareRelease {
		return SoftwareRelease{}, fmt.Errorf("invalid kind: expected %d, got %d", KindSoftwareRelease, event.Kind)
	}

	release := SoftwareRelease{Content: event.Content}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "i":
			if release.I != "" {
				return SoftwareRelease{}, fmt.Errorf("duplicate 'i' tag")
			}
			release.I = tag[1]

		case "version":
			if release.Version != "" {
				return SoftwareRelease{}, fmt.Errorf("duplicate 'version' tag")
			}
			release.Version = tag[1]

		case "d":
			if release.D != "" {
				return SoftwareRelease{}, fmt.Errorf("duplicate 'd' tag")
			}
			release.D = tag[1]

		case "c":
			if release.Channel != "" {
				return SoftwareRelease{}, fmt.Errorf("duplicate 'c' tag")
			}
			release.Channel = tag[1]

		case "e":
			release.AssetIDs = append(release.AssetIDs, tag[1])
		}
	}
	return release, nil
}

// Validate checks that all required fields are present and valid.
func (r SoftwareRelease) Validate() error {
	if r.I == "" {
		return fmt.Errorf("missing or empty 'i' tag (app identifier)")
	}
	if r.Version == "" {
		return fmt.Errorf("missing or empty 'version' tag")
	}
	if r.D == "" {
		return fmt.Errorf("missing or empty 'd' tag")
	}
	if r.Channel == "" {
		return fmt.Errorf("missing or empty 'c' tag (channel ID)")
	}
	if len(r.AssetIDs) == 0 {
		return fmt.Errorf("missing 'e' tag (asset ID)")
	}
	for i, id := range r.AssetIDs {
		if id == "" {
			return fmt.Errorf("empty 'e' tag at index %d", i)
		}
	}

	// Validate that 'd' tag equals 'i' + '@' + 'version'
	expectedD := r.I + "@" + r.Version
	if r.D != expectedD {
		return fmt.Errorf("invalid 'd' tag: expected '%s', got '%s'", expectedD, r.D)
	}
	return nil
}

// ValidateSoftwareRelease parses and validates a Software Release event.
func ValidateSoftwareRelease(event *nostr.Event) error {
	release, err := ParseSoftwareRelease(event)
	if err != nil {
		return err
	}
	return release.Validate()
}
