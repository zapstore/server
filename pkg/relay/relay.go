// The relay package is responsible for setting up the relay.
// It exposes a [Setup] function to create a new relay with the given config.
package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pippellia-btc/rely"
	sqlite "github.com/vertex-lab/nostr-sqlite"
	"github.com/zapstore/server/pkg/acl"
	"github.com/zapstore/server/pkg/analytics"
	"github.com/zapstore/server/pkg/events"
	"github.com/zapstore/server/pkg/rate"
)

var (
	ErrEventKindNotAllowed = errors.New("event kind is not in the allowed list")
	ErrEventIDBlocked      = errors.New("event ID is blocked")
	ErrEventPubkeyBlocked  = errors.New("event pubkey is not allowed. Please contact the Zapstore team.")

	ErrAppAlreadyExists = errors.New(`failed to publish app: another pubkey has already published an app with the same 'd' tag identifier.
		This is a precautionary measure because Android doesn't allow apps with the same identifier to be installed side by side.
		Please use a different identifier or contact the Zapstore team for more information.`)

	ErrTooManyFilters  = errors.New("number of filters exceed the maximum allowed per REQ")
	ErrFiltersTooVague = errors.New("filters are too vague")

	ErrInternal    = errors.New("internal error, please contact the Zapstore team.")
	ErrRateLimited = errors.New("rate-limited: slow down chief")
)

func Setup(
	config Config,
	limiter rate.Limiter,
	acl *acl.Controller,
	store *sqlite.Store,
	analytics *analytics.Engine,
) (*rely.Relay, error) {

	relay := rely.NewRelay(
		rely.WithAuthURL(config.Hostname),
		rely.WithInfo(config.Info.NIP11()),
		rely.WithQueueCapacity(config.QueueCapacity),
		rely.WithMaxMessageSize(config.MaxMessageBytes),
		rely.WithClientResponseLimit(config.ResponseLimit),
	)

	relay.Reject.Connection.Clear()
	relay.Reject.Connection.Append(
		RateConnectionIP(limiter),
		rely.RegistrationFailWithin(3*time.Second),
	)

	relay.Reject.Event.Clear()
	relay.Reject.Event.Append(
		RateEventIP(limiter),
		KindNotAllowed(config.AllowedKinds),
		IDIsBlocked(acl),
		rely.InvalidID,
		rely.InvalidSignature,
		InvalidStructure(),
		AuthorNotAllowed(acl),
		AppAlreadyExists(store),
	)

	relay.Reject.Req.Clear()
	relay.Reject.Req.Append(
		RateReqIP(limiter),
		FiltersExceed(config.MaxReqFilters),
		// VagueFilters(),
	)

	relay.On.Event = Save(store, analytics)
	relay.On.Req = Query(store, analytics)
	return relay, nil
}

func Save(store *sqlite.Store, analytics *analytics.Engine) func(c rely.Client, event *nostr.Event) error {
	return func(c rely.Client, event *nostr.Event) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		switch {
		case event.Kind == nostr.KindDeletion:
			if _, err := store.DeleteRequest(ctx, event); err != nil {
				slog.Error("relay: failed to fulfill the delete request", "error", err, "event", event.ID)
				return err
			}

			if _, err := store.Save(ctx, event); err != nil {
				slog.Error("relay: failed to save the delete request", "error", err, "event", event.ID)
				return err
			}

		case nostr.IsRegularKind(event.Kind):
			if _, err := store.Save(ctx, event); err != nil {
				slog.Error("relay: failed to save the event", "error", err, "event", event.ID)
				return err
			}

		case nostr.IsReplaceableKind(event.Kind) || nostr.IsAddressableKind(event.Kind):
			if _, err := store.Replace(ctx, event); err != nil {
				slog.Error("relay: failed to replace the event", "error", err, "event", event.ID)
				return err
			}
		}

		analytics.RecordEvent(c, event)
		return nil
	}
}

func Query(store *sqlite.Store, analytics *analytics.Engine) func(ctx context.Context, c rely.Client, id string, filters nostr.Filters) ([]nostr.Event, error) {
	return func(ctx context.Context, client rely.Client, id string, filters nostr.Filters) ([]nostr.Event, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		events, err := store.Query(ctx, filters...)
		if err != nil {
			slog.Error("relay: failed to query events", "error", err, "filters", filters)
			return nil, err
		}

		analytics.RecordReq(client, id, filters, events)
		return events, nil
	}
}

func RateConnectionIP(limiter rate.Limiter) func(_ rely.Stats, request *http.Request) error {
	return func(_ rely.Stats, request *http.Request) error {
		cost := 1.0
		if !limiter.Allow(rely.GetIP(request).Group(), cost) {
			return ErrRateLimited
		}
		return nil
	}
}

func RateEventIP(limiter rate.Limiter) func(client rely.Client, _ *nostr.Event) error {
	return func(client rely.Client, _ *nostr.Event) error {
		cost := 5.0
		if !limiter.Allow(client.IP().Group(), cost) {
			client.Disconnect()
			return ErrRateLimited
		}
		return nil
	}
}

func RateReqIP(limiter rate.Limiter) func(client rely.Client, id string, filters nostr.Filters) error {
	return func(client rely.Client, id string, filters nostr.Filters) error {
		cost := 1.0
		if len(filters) > 10 {
			cost = 5.0
		}

		if !limiter.Allow(client.IP().Group(), cost) {
			client.Disconnect()
			return ErrRateLimited
		}
		return nil
	}
}

func FiltersExceed(n int) func(_ rely.Client, _ string, filters nostr.Filters) error {
	return func(_ rely.Client, _ string, filters nostr.Filters) error {
		if len(filters) > n {
			return ErrTooManyFilters
		}
		return nil
	}
}

// VagueFilters rejects filters that are too vague, as determined by the specificity scoring mechanism.
func VagueFilters() func(rely.Client, nostr.Filters) error {
	return func(_ rely.Client, filters nostr.Filters) error {
		for _, f := range filters {
			if specificity(f) < 2 {
				return ErrFiltersTooVague
			}
		}
		return nil
	}
}

// specificity estimates how specific a filter is, based on the presence of conditions.
// TODO: make it more accurate by considering what the conditions are (e.g. 1 kind vs 10 kinds).
func specificity(filter nostr.Filter) int {
	points := 0
	if len(filter.IDs) > 0 {
		points += 10
	}
	if filter.Search != "" {
		points += 3
	}
	if len(filter.Authors) > 0 {
		points += 2
	}
	if len(filter.Tags) > 0 {
		points += 2
	}
	if len(filter.Kinds) > 0 {
		points += 1
	}
	if filter.Since != nil {
		points += 1
	}
	if filter.Until != nil {
		points += 1
	}
	if filter.LimitZero {
		points += 3
	}
	if filter.Limit != 0 && filter.Limit < 100 {
		points += 1
	}
	return points
}

func KindNotAllowed(kinds []int) func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {
		if !slices.Contains(kinds, e.Kind) {
			return fmt.Errorf("%w: %v", ErrEventKindNotAllowed, kinds)
		}
		return nil
	}
}

func IDIsBlocked(acl *acl.Controller) func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {
		if acl.IsEventBlocked(e.ID) {
			return fmt.Errorf("%w: %v", ErrEventIDBlocked, e.ID)
		}
		return nil
	}
}

func InvalidStructure() func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {
		return events.Validate(e)
	}
}

// AppAlreadyExists checks if an app (kind 32267) with the same identifier ("d" tag) has already been published by another pubkey.
// This is to avoid duplicate apps with the same identifier, as that causes issues on Android.
//
// TODO: we should not need this check at all.
func AppAlreadyExists(store *sqlite.Store) func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {
		if e.Kind != events.KindApp {
			return nil
		}

		appID, ok := events.Find(e.Tags, "d")
		if !ok {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		query := `SELECT COUNT(e.id)
					FROM events AS e JOIN tags AS t ON t.event_id = e.id
					WHERE e.kind = ?
					AND e.pubkey != ?
					AND t.key = 'd' AND t.value = ?`
		args := []any{events.KindApp, e.PubKey, appID}

		var count int
		err := store.DB.QueryRowContext(ctx, query, args...).Scan(&count)
		if err != nil {
			slog.Error("failed to check if app event with the same id already exists", "error", err)
			return ErrInternal
		}

		if count > 0 {
			return ErrAppAlreadyExists
		}
		return nil
	}
}

func AuthorNotAllowed(acl *acl.Controller) func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {
		if e.Kind == events.KindAppSet {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		allow, err := acl.AllowPubkey(ctx, e.PubKey)
		if err != nil {
			// fail closed policy;
			slog.Error("relay: failed to check if pubkey is allowed", "error", err)
			return ErrEventPubkeyBlocked
		}
		if !allow {
			return ErrEventPubkeyBlocked
		}
		return nil
	}
}
