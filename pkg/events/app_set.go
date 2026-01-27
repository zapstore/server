package events

import (
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const KindAppSet = 30267

type AppIdentifier string // 32267:<event_id>:<app_id>

// AppSet represents a set of app identifiers.
// Learn more here: https://github.com/nostr-protocol/nips/blob/master/51.md#sets
type AppSet []AppIdentifier

func (s AppSet) Validate() error {
	for _, e := range s {
		if err := e.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (e AppIdentifier) Validate() error {
	parts := strings.Split(string(e), ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid app set element: %s", e)
	}
	if parts[0] != "32267" {
		return fmt.Errorf("invalid app set element: %s", e)
	}
	if err := ValidateHash(parts[1]); err != nil {
		return fmt.Errorf("invalid event ID in app set element: %w", err)
	}
	if parts[2] == "" {
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
			appSet = append(appSet, AppIdentifier(tag[1]))
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
