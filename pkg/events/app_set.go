package events

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const KindAppSet = 30267

type AppIdentifier string // 32267:<pubkey>:<app_id>

// AppSet represents a set of app identifiers with associated platform identifiers.
// Learn more here: https://github.com/nostr-protocol/nips/blob/master/51.md#sets
type AppSet struct {
	Apps      []AppIdentifier
	Platforms []string
}

func (s AppSet) Validate() error {
	for _, e := range s.Apps {
		if err := e.Validate(); err != nil {
			return err
		}
	}

	if len(s.Platforms) == 0 {
		return fmt.Errorf("missing 'f' tag (platform identifier)")
	}
	for i, p := range s.Platforms {
		if !slices.Contains(PlatformIdentifiers, p) {
			return fmt.Errorf("invalid platform identifier in 'f' tag at index %d: %s", i, p)
		}
	}
	return nil
}

func (e AppIdentifier) Validate() error {
	parts := strings.Split(string(e), ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid app set element: %s", e)
	}
	kind, pk, appID := parts[0], parts[1], parts[2]

	if kind != "32267" {
		return fmt.Errorf("invalid app set element: %s", e)
	}
	if !nostr.IsValidPublicKey(pk) {
		return errors.New("invalid pubkey in app set element")
	}
	if appID == "" {
		return fmt.Errorf("invalid app ID in app set element: %s", e)
	}
	return nil
}

// ParseAppSet extracts a AppSet from a nostr.Event.
// Returns an error if the event kind is wrong.
func ParseAppSet(event *nostr.Event) (AppSet, error) {
	if event.Kind != KindAppSet {
		return AppSet{}, fmt.Errorf("invalid kind: expected %d, got %d", KindAppSet, event.Kind)
	}

	appSet := AppSet{}
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "a":
			appSet.Apps = append(appSet.Apps, AppIdentifier(tag[1]))
		case "f":
			appSet.Platforms = append(appSet.Platforms, tag[1])
		}
	}
	return appSet, nil
}

// ValidateAppSet parses and validates a AppSet event.
func ValidateAppSet(event *nostr.Event) error {
	appSet, err := ParseAppSet(event)
	if err != nil {
		return err
	}
	return appSet.Validate()
}

// ExtractAppsFromSet returns all app IDs (kind 32267 identifiers)
// referenced by an AppSet (kind 30267) event via "a" tags.
//
// Expected "a" tag format:
//
//	["a", "<32267>:<pubkey>:<identifier>"]
func ExtractAppsFromSet(e nostr.Event) []string {
	if e.Kind != KindAppSet {
		return nil
	}

	var appIDs []string

	for _, tag := range e.Tags {
		if len(tag) < 2 || tag[0] != "a" {
			continue
		}

		parts := strings.Split(tag[1], ":")
		if len(parts) != 3 {
			continue
		}

		kind, appID := parts[0], parts[2]
		if kind != "32267" {
			continue
		}
		if appID == "" {
			continue
		}

		appIDs = append(appIDs, appID)
	}
	return appIDs
}
