// The rate is a wrapper around the [github.com/pippellia-btc/rate] package,
// exposing a [Config] struct for configuring the limiter,
// and a [NewLimiter] function to create a new ip-rate limiter.
package rate

import (
	"fmt"
	"time"

	"github.com/pippellia-btc/rate"
)

type Config struct {
	// InitialTokens is the initial number of tokens for a new bucket. Default is 100.
	InitialTokens int `env:"RATE_INITIAL_TOKENS"`

	// MaxTokens is the maximum number of tokens for a bucket. Default is 100.
	MaxTokens int `env:"RATE_MAX_TOKENS"`

	// TokensPerInterval is the number of tokens added to a bucket per interval. Default is 10.
	TokensPerInterval int `env:"RATE_TOKENS_PER_INTERVAL"`

	// Interval is the duration of the interval. Default is 1 second.
	Interval time.Duration `env:"RATE_INTERVAL"`
}

func NewConfig() Config {
	return Config{
		InitialTokens:     100,
		MaxTokens:         100,
		TokensPerInterval: 10,
		Interval:          time.Second,
	}
}

// NewLimiter creates a new rate limiter with a [rate.FlatRefiller] from the given config.
func NewLimiter(c Config) *rate.Limiter[string] {
	refiller := rate.FlatRefiller[string]{
		InitialTokens:     float64(c.InitialTokens),
		MaxTokens:         float64(c.MaxTokens),
		TokensPerInterval: float64(c.TokensPerInterval),
		Interval:          c.Interval,
	}
	return rate.NewLimiter(refiller)
}

func (c Config) Validate() error {
	if c.InitialTokens < 0 {
		return fmt.Errorf("initial tokens must be greater than 0")
	}
	if c.MaxTokens < 0 {
		return fmt.Errorf("max tokens must be greater than 0")
	}
	if c.TokensPerInterval < 0 {
		return fmt.Errorf("tokens per interval must be greater than 0")
	}
	if c.Interval < time.Second {
		return fmt.Errorf("interval must be greater than 1 second")
	}
	if c.InitialTokens > c.MaxTokens {
		return fmt.Errorf("initial tokens must be less than or equal to max tokens")
	}
	if c.TokensPerInterval > c.MaxTokens {
		return fmt.Errorf("tokens per interval must be less than or equal to max tokens")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Rate Limiter:\n"+
		"\tInitialTokens: %d\n"+
		"\tMaxTokens: %d\n"+
		"\tTokensPerInterval: %d\n"+
		"\tInterval: %v",
		c.InitialTokens, c.MaxTokens, c.TokensPerInterval, c.Interval)
}
