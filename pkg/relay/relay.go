// The relay package is responsible for setting up the relay.
// It exposes a [Setup] function to create a new relay with the given config.
package relay

import (
	"github.com/pippellia-btc/rate"
	"github.com/pippellia-btc/rely"
	"github.com/zapstore/server/pkg/vertex"
)

// Filter is responsible for enforcing rate limits and reputation checks for pubkeys.
type Filter struct {
	limiter *rate.Limiter[string]
	vertex  vertex.Filter
}

func NewFilter(c FilterConfig, limiter *rate.Limiter[string]) (Filter, error) {
	vertex, err := vertex.NewFilter(c.Vertex)
	if err != nil {
		return Filter{}, err
	}

	filter := Filter{
		limiter: limiter,
		vertex:  vertex,
	}
	return filter, nil
}

func Setup(c Config, limiter *rate.Limiter[string]) (*rely.Relay, error) {
	filter, err := NewFilter(c.Filter, limiter)
	if err != nil {
		return nil, err
	}

	_ = filter

	relay := rely.NewRelay(
		rely.WithDomain(c.Domain),
		rely.WithInfo(c.Info.NIP11()),
	)

	relay.Reject.Connection.Append()

	relay.On.Connect = func(c rely.Client) { c.SendAuth() }

	return relay, nil
}
