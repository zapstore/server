package vertex

import (
	"errors"
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

type Sort string

const (
	SortGlobal       Sort = "globalPagerank"
	SortPersonalized Sort = "personalizedPagerank"
	SortFollowers    Sort = "followerCount"
)

type Config struct {
	// The sorting algorithm to use. Default is "globalPagerank".
	// Learn more here: https://vertexlab.io/docs/endpoints/verify-reputation/
	Sort Sort `env:"VERTEX_SORT"`

	// The threshold above which an unknown pubkey is allowed to publish to the relay.
	// Default is 0.0, which means that all pubkeys can publish to the relay.
	Threshold float64 `env:"VERTEX_THRESHOLD"`

	// The secret key to use for signing requests to the Vertex DVM.
	SecretKey string `env:"VERTEX_SECRET_KEY"`

	// Timeout for requests to the Vertex DVM. Default is 10 seconds.
	Timeout time.Duration `env:"VERTEX_TIMEOUT"`

	// CacheExpiration time for ranks in the cache. Default is 1 hour.
	CacheExpiration time.Duration `env:"VERTEX_CACHE_EXPIRATION"`

	// CacheSize is the maximum number of entries in the cache. Default is 10_000.
	CacheSize int `env:"VERTEX_CACHE_SIZE"`
}

func NewConfig() Config {
	return Config{
		Sort:            SortGlobal,
		Threshold:       0.0,
		Timeout:         10 * time.Second,
		CacheExpiration: 1 * time.Hour,
		CacheSize:       10_000,
	}
}

func (c Config) Validate() error {
	if c.Timeout < time.Second {
		return errors.New("timeout must be greater than 1s to function reliably")
	}

	if c.SecretKey == "" {
		return errors.New("secret key is empty or not set")
	}
	if !nostr.IsValid32ByteHex(c.SecretKey) {
		return errors.New("secret key is not a valid 32 byte hex string")
	}

	if c.Sort == SortGlobal || c.Sort == SortPersonalized {
		if c.Threshold < 0.0 || c.Threshold > 1.0 {
			return errors.New("threshold must be between 0.0 and 1.0")
		}
		return nil
	}

	if c.Sort == SortFollowers {
		if c.Threshold < 0 {
			return errors.New("threshold must be greater than 0")
		}
		return nil
	}

	return errors.New("invalid sort")
}

func (c Config) String() string {
	return fmt.Sprintf("Vertex Config:\n"+
		"\tSort: %s\n"+
		"\tThreshold: %f\n"+
		"\tSecretKey: %s\n"+
		"\tTimeout: %s\n",
		c.Sort, c.Threshold, c.SecretKey, c.Timeout)
}
