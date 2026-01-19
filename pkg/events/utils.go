package events

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/zapstore/server/pkg/events/legacy"
)

// IsZapstoreEvent returns true if the event is a supported Zapstore event type.
func IsZapstoreEvent(e *nostr.Event) bool {
	return e.Kind == KindApp || e.Kind == KindRelease || e.Kind == KindAsset || legacy.IsZapstoreEvent(e)
}

// ValidateZapstore validates a Zapstore event by routing to the appropriate
// validation function based on the event kind. Returns an error if the event
// kind is not a supported Zapstore event type.
func ValidateZapstore(event *nostr.Event) error {
	switch event.Kind {
	case KindApp:
		a, ok := Find(event.Tags, "a")
		if ok && strings.HasPrefix(a, "30063:") {
			return legacy.ValidateApp(event)
		}
		return ValidateApp(event)

	case KindRelease:
		a, ok := Find(event.Tags, "a")
		if ok && strings.HasPrefix(a, "32267:") {
			return legacy.ValidateRelease(event)
		}
		return ValidateRelease(event)

	case KindAsset:
		return ValidateAsset(event)

	case legacy.KindFile:
		return legacy.ValidateFile(event)

	default:
		return fmt.Errorf("event kind %d not supported in Zapstore", event.Kind)
	}
}

// ValidateHash validates a sha256 hash, reporting an error if it is invalid.
func ValidateHash(hash string) error {
	if len(hash) != 64 {
		return fmt.Errorf("invalid sha256 length: %d", len(hash))
	}

	if _, err := hex.DecodeString(hash); err != nil {
		return fmt.Errorf("invalid sha256 hex: %w", err)
	}
	return nil
}

// Find the value of the first tag with the given key.
func Find(tags nostr.Tags, key string) (string, bool) {
	for _, tag := range tags {
		if len(tag) > 1 && tag[0] == key {
			return tag[1], true
		}
	}
	return "", false
}
