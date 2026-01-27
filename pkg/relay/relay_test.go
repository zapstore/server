package relay

import (
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/zapstore/server/pkg/rate"
)

func TestRateLimiting(t *testing.T) {
	limiter := rate.NewLimiter(rate.NewConfig())
	config := NewConfig()

	relay, err := Setup(config, limiter)
	if err != nil {
		t.Fatalf("failed to setup relay: %v", err)
	}

	exitErr := make(chan error)
	go func() {
		if err := relay.StartAndServe(t.Context(), ":"+config.Port); err != nil {
			exitErr <- err
		}
	}()

	// connect to the relay to start spamming it
	conn, err := nostr.RelayConnect(t.Context(), "localhost:3334")
	if err != nil {
		t.Fatalf("failed to connect to relay: %v", err)
	}
	defer conn.Close()

	sent := 0
	blocked := false

spamming:
	for range 1000 {
		select {
		case err := <-exitErr:
			t.Fatalf("failed to start and serve the relay: %v", err)
		default:
			// proceed to spam

			_, err := conn.QueryEvents(t.Context(), nostr.Filter{Kinds: []int{1}})
			if err != nil {
				if strings.Contains(err.Error(), "failed to write") {
					blocked = true
					break spamming
				}

				t.Fatalf("unexpected error occurred: %v", err)
			}

			sent++
			t.Logf("sent %d filters", sent)
		}
	}

	if !blocked {
		t.Fatalf("expected to be rate limited, but was not")
	}
}
