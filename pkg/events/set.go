package events

import (
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const KindAppSet = 30267

type AppIdentifier string // 32267:<event_id>:<app_id>

type AppSet []AppIdentifier // a set of app identifiers

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

func ValidateAppSet(event *nostr.Event) error {
	appSet, err := ParseAppSet(event)
	if err != nil {
		return err
	}
	return appSet.Validate()
}
