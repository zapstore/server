// The vertex package exposes a configurable [Filter] struct that allows or rejects a pubkey
// based on its reputation.
package vertex

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/nbd-wtf/go-nostr"
)

const Relay = "wss://relay.vertexlab.io"

// Filter is responsible for allowing based on the reputation of a pubkey.
// It stores ranks in a LRU cache with size and time to live specified in the config.
type Filter struct {
	relay  *nostr.Relay
	cache  *expirable.LRU[string, float64]
	config Config
}

// NewFilter creates a new Filter with the given config.
// It returns an error if the connection to the relay fails.
func NewFilter(c Config) (Filter, error) {
	relay := nostr.NewRelay(context.Background(), Relay)

	// context only for the connection phase
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	if err := relay.Connect(ctx); err != nil {
		return Filter{}, err
	}

	return Filter{
		relay:  relay,
		cache:  expirable.NewLRU[string, float64](c.CacheSize, nil, c.CacheExpiration),
		config: c,
	}, nil
}

type Response struct {
	Pubkey    string  `json:"pubkey"`
	Rank      float64 `json:"rank"`
	Follows   int     `json:"follows"`
	Followers int     `json:"followers"`
}

// Reject returns whether the pubkey is below the threshold.
// It returns an error if the request to the relay fails.
func (f Filter) Reject(ctx context.Context, pubkey string) (bool, error) {
	allow, err := f.Allow(ctx, pubkey)
	return !allow, err
}

// Allow returns whether the pubkey is above the threshold.
// It returns an error if the request to the relay fails.
func (f Filter) Allow(ctx context.Context, pubkey string) (bool, error) {
	if slices.Contains(f.config.Blacklist, pubkey) {
		return false, nil
	}

	if slices.Contains(f.config.Whitelist, pubkey) {
		return true, nil
	}

	if f.config.Algorithm.Threshold == 0 {
		return true, nil
	}

	if rank, ok := f.cache.Get(pubkey); ok {
		return rank >= f.config.Algorithm.Threshold, nil
	}

	request := nostr.Event{
		Kind:      5312,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"param", "target", pubkey},
			{"param", "limit", "5"},
			{"param", "sort", string(f.config.Algorithm.Sort)},
			{"param", "source", f.config.Algorithm.Source},
		},
	}

	if err := request.Sign(f.config.SecretKey); err != nil {
		return false, fmt.Errorf("vertex.Filter: failed to sign the request: %w", err)
	}

	response, err := dvmResponse(ctx, request, f.relay)
	if err != nil {
		return false, fmt.Errorf("vertex.Filter: failed to verify the reputation of %s: %w", pubkey, err)
	}

	switch response.Kind {
	case 7000:
		msg := "unknown error"
		status := response.Tags.Find("status")
		if len(status) > 2 {
			msg = status[2]
		}
		return false, fmt.Errorf("vertex.Filter: received a DVM error: %s", msg)

	case 6312:
		var ranks []Response
		if err := json.Unmarshal([]byte(response.Content), &ranks); err != nil {
			return false, fmt.Errorf("vertex.Filter: failed to unmarshal the response: %w", err)
		}

		if len(ranks) == 0 {
			return false, fmt.Errorf("vertex.Filter: received an empty response")
		}

		target := ranks[0]
		f.cache.Add(target.Pubkey, target.Rank)
		return target.Rank >= f.config.Algorithm.Threshold, nil

	default:
		return false, fmt.Errorf("vertex.Filter: received an unexpected response kind: %d", response.Kind)
	}
}

// dvmResponse connects to the relay, send the request and fetches the response using the request ID.
func dvmResponse(ctx context.Context, request nostr.Event, relay *nostr.Relay) (nostr.Event, error) {
	if err := relay.Publish(ctx, request); err != nil {
		return nostr.Event{}, fmt.Errorf("failed to publish the dvm request to %s: %w", relay.URL, err)
	}

	filter := nostr.Filter{
		Kinds: []int{request.Kind + 1000, 7000},
		Tags:  nostr.TagMap{"e": {request.ID}},
	}

	ch, err := relay.QueryEvents(ctx, filter)
	if err != nil {
		return nostr.Event{}, fmt.Errorf("failed to query the dvm response: %w", err)
	}

	response := nostr.Event{}
	var counter int

	for event := range ch {
		response = *event
		counter++
	}

	if counter != 1 {
		return nostr.Event{}, fmt.Errorf("expected exactly one response, got %v", counter)
	}
	return response, nil
}
