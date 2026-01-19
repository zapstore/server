package vertex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/nbd-wtf/go-nostr"
)

const Relay = "wss://relay.vertexlab.io"

// Checker is responsible for checking the reputation of a pubkey using the Vertex DVM.
// It stores ranks in a LRU cache with size and time to live specified in the config.
type Checker struct {
	relay  *nostr.Relay
	cache  *expirable.LRU[string, float64]
	config Config
}

// NewChecker creates a new Checker with the given config.
// It returns an error if the connection to the relay fails.
func NewChecker(c Config) (Checker, error) {
	relay := nostr.NewRelay(context.Background(), Relay)

	// context only for the connection phase
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	if err := relay.Connect(ctx); err != nil {
		return Checker{}, err
	}

	return Checker{
		relay:  relay,
		cache:  expirable.NewLRU[string, float64](c.CacheSize, nil, c.CacheExpiration),
		config: c,
	}, nil
}

type PubkeyRank struct {
	Pubkey string  `json:"pubkey"`
	Rank   float64 `json:"rank"`
}

// Check returns whether the pubkey is above the threshold.
// It returns an error if the request to the relay fails.
func (c Checker) Check(ctx context.Context, pubkey string) (bool, error) {
	if c.config.Threshold == 0 {
		return true, nil
	}

	if rank, ok := c.cache.Get(pubkey); ok {
		return rank >= c.config.Threshold, nil
	}

	request := nostr.Event{
		Kind:      5312,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"param", "target", pubkey},
			{"param", "limit", "1"},
		},
	}

	if err := request.Sign(c.config.SecretKey); err != nil {
		return false, fmt.Errorf("vertex.Check: failed to sign the request: %w", err)
	}

	response, err := dvmResponse(ctx, request, c.relay)
	if err != nil {
		return false, fmt.Errorf("vertex.Check: failed to get the reputation of %s: %w", pubkey, err)
	}

	switch response.Kind {
	case 7000:
		msg := "unknown error"
		status := response.Tags.Find("status")
		if len(status) > 2 {
			msg = status[2]
		}
		return false, fmt.Errorf("vertex.Check: received a dvm error: %s", msg)

	case 6312:
		decoder := json.NewDecoder(strings.NewReader(response.Content))
		if _, err := decoder.Token(); err != nil {
			return false, fmt.Errorf("vertex.Check: failed to read opening bracket: %w", err)
		}

		var rank PubkeyRank
		if decoder.More() {
			if err := decoder.Decode(&rank); err != nil {
				return false, fmt.Errorf("vertex.Check: failed to unmarshal the response: %w", err)
			}
		}

		c.cache.Add(pubkey, rank.Rank)
		return rank.Rank >= c.config.Threshold, nil

	default:
		return false, fmt.Errorf("vertex.Check: received an unexpected response kind: %d", response.Kind)
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
