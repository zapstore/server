package rate

import (
	"time"

	"github.com/pippellia-btc/rate"
)

// Limiter is a wrapper around the [rate.Limiter] that adds a [Config] to the limiter.
type Limiter struct {
	*rate.Limiter[string]
	config Config
}

// NewLimiter creates a new rate limiter with a [rate.FlatRefiller] from the given config.
func NewLimiter(c Config) Limiter {
	refiller := rate.FlatRefiller[string]{
		InitialTokens:     float64(c.InitialTokens),
		MaxTokens:         float64(c.MaxTokens),
		TokensPerInterval: float64(c.TokensPerInterval),
		Interval:          c.Interval,
	}

	return Limiter{
		Limiter: rate.NewLimiter(refiller),
		config:  c,
	}
}

func (l Limiter) InitialTokens() int      { return l.config.InitialTokens }
func (l Limiter) MaxTokens() int          { return l.config.MaxTokens }
func (l Limiter) TokensPerInterval() int  { return l.config.TokensPerInterval }
func (l Limiter) Interval() time.Duration { return l.config.Interval }
