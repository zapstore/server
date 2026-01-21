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
	"github.com/pippellia-btc/rate"
	"github.com/pippellia-btc/rely"
	"github.com/zapstore/server/pkg/vertex"
)

var (
	ErrEventKindNotAllowed = errors.New("event kind is not in the allowed list")
	ErrEventIDBlocked      = errors.New("event ID is blocked")
	ErrEventPubkeyBlocked  = errors.New("event pubkey has not enough reputation. Please contact the Zapstore team.")
	ErrInternal            = errors.New("internal error, please contact the Zapstore team.")
	ErrTooManyFilters      = errors.New("number of filters exceed the maximum allowed per REQ")
	ErrRateLimited         = errors.New("rate-limited: slow down there chief")
)

func Setup(config Config, limiter *rate.Limiter[string]) (*rely.Relay, error) {
	var err error
	vertexFilter := vertex.Filter{}

	if config.UnknownPubkeyPolicy == PubkeyPolicyVertex {
		vertexFilter, err = vertex.NewFilter(config.Vertex)
		if err != nil {
			return nil, fmt.Errorf("failed to create vertex filter: %w", err)
		}
	}

	relay := rely.NewRelay(
		rely.WithDomain(config.Domain),
		rely.WithInfo(config.Info.NIP11()),
		rely.WithMaxMessageSize(config.MaxMessageBytes),
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
		IDIsBlocked(config.BlockedIDs),
		rely.InvalidID,
		rely.InvalidSignature,
		AuthorNotTrusted(config, vertexFilter),
	)

	relay.Reject.Req.Clear()
	relay.Reject.Req.Append(
		RateReqIP(limiter),
		FiltersExceed(config.MaxFilters),
	)

	relay.On.Connect = func(c rely.Client) { c.SendAuth() }

	return relay, nil
}

func RateConnectionIP(limiter *rate.Limiter[string]) func(_ rely.Stats, request *http.Request) error {
	return func(_ rely.Stats, request *http.Request) error {
		cost := 1.0
		return RateLimitIP(limiter, rely.GetIP(request), cost)
	}
}

func RateEventIP(limiter *rate.Limiter[string]) func(client rely.Client, _ *nostr.Event) error {
	return func(client rely.Client, _ *nostr.Event) error {
		cost := 1.0
		return RateLimitIP(limiter, client.IP(), cost)
	}
}

func RateReqIP(limiter *rate.Limiter[string]) func(client rely.Client, filters nostr.Filters) error {
	return func(client rely.Client, filters nostr.Filters) error {
		cost := float64(len(filters))
		return RateLimitIP(limiter, client.IP(), cost)
	}
}

// RateLimitIP rejects an IP if it's exceeding the rate limit.
func RateLimitIP(limiter *rate.Limiter[string], ip rely.IP, cost float64) error {
	reject, err := limiter.Reject(ip.Group(), cost)
	if err != nil {
		// fail open policy; if the rate limiter fails, we allow the request
		slog.Error("rate limiter failed", "error", err)
		return nil
	}
	if reject {
		return ErrRateLimited
	}
	return nil
}

func FiltersExceed(n int) func(rely.Client, nostr.Filters) error {
	return func(_ rely.Client, filters nostr.Filters) error {
		if len(filters) > n {
			return ErrTooManyFilters
		}
		return nil
	}
}

func KindNotAllowed(kinds []int) func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {
		if !slices.Contains(kinds, e.Kind) {
			return fmt.Errorf("%w: %v", ErrEventKindNotAllowed, kinds)
		}
		return nil
	}
}

func IDIsBlocked(ids []string) func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {
		if slices.Contains(ids, e.ID) {
			return fmt.Errorf("%w: %v", ErrEventIDBlocked, ids)
		}
		return nil
	}
}

func AuthorNotTrusted(config Config, vertex vertex.Filter) func(_ rely.Client, e *nostr.Event) error {
	return func(_ rely.Client, e *nostr.Event) error {

		if slices.Contains(config.Blocklist, e.PubKey) {
			return ErrEventPubkeyBlocked
		}

		if slices.Contains(config.Allowlist, e.PubKey) {
			return nil
		}

		switch config.UnknownPubkeyPolicy {
		case PubkeyPolicyAllow:
			return nil

		case PubkeyPolicyBlock:
			return ErrEventPubkeyBlocked

		case PubkeyPolicyVertex:
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			reject, err := vertex.Reject(ctx, e.PubKey)
			if err != nil {
				slog.Error("relay failed to use vertex filter", "pubkey", e.PubKey, "error", err)
				return ErrInternal
			}
			if reject {
				return ErrEventPubkeyBlocked
			}
			return nil

		default:
			slog.Error("unknown pubkey policy", "policy", config.UnknownPubkeyPolicy)
			return ErrInternal
		}
	}
}
