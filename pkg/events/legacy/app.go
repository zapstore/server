package legacy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const KindApp = 32267

var PlatformIdentifiers = []string{
	"android-arm64-v8a",
	"android-armeabi-v7a",
	"android-x86",
	"android-x86_64",
	"darwin-arm64",
	"darwin-x86_64",
	"linux-aarch64",
	"linux-x86_64",
	"windows-aarch64",
	"windows-x86_64",
	"ios-arm64",
	"web",
}

// App represents a parsed Software Application event (kind 32267).
// It's a legacy version of the new Software Application event (kind 32267).
type App struct {
	// Required fields
	D         string   // App identifier (reverse-domain recommended, e.g. com.example.app)
	Name      string   // Human-readable project name
	Platforms []string // Platform identifiers (at least one required)
	Release   string   // Reference to the release event "30063:<pubkey>:<packageID>@<version>"

	// Optional fields
	Repository string // Source code repository URL
	License    string // Software file (e.g. MIT)
	Content    string // Full description, markdown allowed
	Icon       string // Icon URL
}

func (app App) Validate(pubkey string) error {
	if app.D == "" {
		return fmt.Errorf("missing or empty 'd' tag (app identifier)")
	}
	if app.Name == "" {
		return fmt.Errorf("missing or empty 'name' tag")
	}

	if len(app.Platforms) == 0 {
		return fmt.Errorf("missing 'f' tag (platform identifier)")
	}
	for i, p := range app.Platforms {
		if !slices.Contains(PlatformIdentifiers, p) {
			return fmt.Errorf("invalid platform identifier in 'f' tag at index %d: %s", i, p)
		}
	}

	if app.Release == "" {
		return fmt.Errorf("missing 'a' tag")
	}
	expectedRelease := "30063:" + pubkey + ":" + app.D + "@"
	if !strings.HasPrefix(app.Release, expectedRelease) {
		return fmt.Errorf("invalid 'a' tag: expected '%s', got '%s'", expectedRelease, app.Release)
	}
	return nil
}

// ParseApp extracts an App from a nostr.Event.
// Returns an error if the event kind is wrong or if duplicate singular tags are found.
func ParseApp(event *nostr.Event) (App, error) {
	if event.Kind != KindApp {
		return App{}, fmt.Errorf("invalid kind: expected %d, got %d", KindApp, event.Kind)
	}

	app := App{Content: event.Content}
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "d":
			if app.D != "" {
				return App{}, fmt.Errorf("duplicate 'd' tag")
			}
			app.D = tag[1]

		case "name":
			if app.Name != "" {
				return App{}, fmt.Errorf("duplicate 'name' tag")
			}
			app.Name = tag[1]

		case "f":
			app.Platforms = append(app.Platforms, tag[1])

		case "a":
			if app.Release != "" {
				return App{}, fmt.Errorf("duplicate 'a' tag")
			}
			app.Release = tag[1]

		case "repository":
			if app.Repository != "" {
				return App{}, fmt.Errorf("duplicate 'repository' tag")
			}
			app.Repository = tag[1]

		case "license":
			if app.License != "" {
				return App{}, fmt.Errorf("duplicate 'license' tag")
			}
			app.License = tag[1]

		case "icon":
			if app.Icon != "" {
				return App{}, fmt.Errorf("duplicate 'icon' tag")
			}
			app.Icon = tag[1]
		}
	}
	return app, nil
}

func ValidateApp(event *nostr.Event) error {
	app, err := ParseApp(event)
	if err != nil {
		return err
	}
	return app.Validate(event.PubKey)
}
