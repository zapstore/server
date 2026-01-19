package events

import (
	"encoding/hex"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

// IsZapstoreEvent returns true if the event is a supported Zapstore event type.
func IsZapstoreEvent(e *nostr.Event) bool {
	return e.Kind == KindApp || e.Kind == KindRelease || e.Kind == KindAsset
}

// ValidateZapstore validates a Zapstore event by routing to the appropriate
// validation function based on the event kind. Returns an error if the event
// kind is not a supported Zapstore event type.
func ValidateZapstore(event *nostr.Event) error {
	switch event.Kind {
	case KindApp:
		return ValidateApp(event)
	case KindRelease:
		return ValidateRelease(event)
	case KindAsset:
		return ValidateAsset(event)
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
