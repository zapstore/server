package events

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

const KindApp = 32267

// App represents a parsed Software Application event (kind 32267).
type App struct {
	// Required fields
	D         string   // App identifier (reverse-domain recommended, e.g. com.example.app)
	Name      string   // Human-readable project name
	Platforms []string // Platform identifiers (at least one required)

	// Optional fields
	Content    string   // Full description, markdown allowed
	Summary    string   // Short description, no markdown
	Icon       string   // Icon URL
	Images     []string // Image URLs
	Tags       []string // Tags describing the application
	URL        string   // Website URL
	Repository string   // Source code repository URL
	License    string   // SPDX license ID
}

// ParseApp extracts a App from a nostr.Event.
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

		case "summary":
			app.Summary = tag[1]

		case "icon":
			app.Icon = tag[1]

		case "image":
			app.Images = append(app.Images, tag[1])

		case "t":
			app.Tags = append(app.Tags, tag[1])

		case "url":
			app.URL = tag[1]

		case "repository":
			app.Repository = tag[1]

		case "license":
			app.License = tag[1]
		}
	}
	return app, nil
}

func (app App) Validate() error {
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
		if p == "" {
			return fmt.Errorf("empty 'f' tag at index %d", i)
		}
	}
	return nil
}

// ValidateApp parses and validates a Software Application event.
func ValidateApp(event *nostr.Event) error {
	app, err := ParseApp(event)
	if err != nil {
		return err
	}
	return app.Validate()
}
