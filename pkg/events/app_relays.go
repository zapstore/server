package events

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

const KindAppRelays = 10067

type AppRelays []AppRelay

type AppRelay struct {
	URL   string
	Read  bool
	Write bool
}

func (a AppRelays) Validate() error {
	for _, relay := range a {
		if err := relay.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (a AppRelay) Validate() error {
	if !nostr.IsValidRelayURL(a.URL) {
		return fmt.Errorf("invalid relay URL: %s", a.URL)
	}
	if !a.Read && !a.Write {
		return fmt.Errorf("at least one of read or write must be true")
	}
	return nil
}

// ParseAppRelays extracts a AppRelays from a nostr.Event.
// Returns an error if the event kind is wrong.
func ParseAppRelays(event *nostr.Event) (AppRelays, error) {
	if event.Kind != KindAppRelays {
		return AppRelays{}, fmt.Errorf("invalid kind: expected %d, got %d", KindAppRelays, event.Kind)
	}

	relays := AppRelays{}
	for _, tag := range event.Tags {
		if len(tag) < 2 || tag[0] != "r" {
			continue
		}

		relay := AppRelay{URL: tag[1]}
		for _, flag := range tag[2:] {
			if flag == "read" {
				relay.Read = true
			}
			if flag == "write" {
				relay.Write = true
			}
		}

		relays = append(relays, relay)
	}
	return relays, nil
}

// ValidateAppRelays parses and validates a AppRelays event.
func ValidateAppRelays(event *nostr.Event) error {
	relays, err := ParseAppRelays(event)
	if err != nil {
		return err
	}
	return relays.Validate()
}
