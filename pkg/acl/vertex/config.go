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

type Algorithm struct {
	// The sorting algorithm to use. Default is "globalPagerank".
	Sort Sort `env:"VERTEX_SORT"`

	// The source of pubkey, used when the Sort is SortPersonalized. Default is "".
	Source string `env:"VERTEX_SOURCE"`

	// The threshold above which an unknown pubkey is allowed to publish to the relay.
	// Default is 0.0, which means that all pubkeys can publish to the relay.
	Threshold float64 `env:"VERTEX_THRESHOLD"`
}

func (a Algorithm) Validate() error {
	switch a.Sort {
	case SortGlobal:
		if a.Threshold < 0.0 || a.Threshold > 1.0 {
			return fmt.Errorf("threshold must be between 0.0 and 1.0 for %s", a.Sort)
		}
		return nil

	case SortPersonalized:
		if a.Source == "" {
			return fmt.Errorf("source is empty or not set for %s", a.Sort)
		}
		if !nostr.IsValid32ByteHex(a.Source) {
			return fmt.Errorf("source is not a valid 32 byte hex string for %s", a.Sort)
		}

		if a.Threshold < 0.0 || a.Threshold > 1.0 {
			return fmt.Errorf("threshold must be between 0.0 and 1.0 for %s", a.Sort)
		}
		return nil

	case SortFollowers:
		if a.Threshold < 0 {
			return fmt.Errorf("threshold must be greater than 0 for %s", a.Sort)
		}
		return nil

	default:
		return fmt.Errorf("invalid sort: %s", a.Sort)
	}
}

type Config struct {
	// The algorithm to use to decide whether to allow a pubkey not in the whitelist or blacklist.
	Algorithm Algorithm

	// The secret key to use for signing requests to the Vertex DVM.
	SecretKey string `env:"VERTEX_SECRET_KEY"`

	// CacheExpiration time for ranks in the cache. Default is 1 hour.
	CacheExpiration time.Duration `env:"VERTEX_CACHE_EXPIRATION"`

	// CacheSize is the maximum number of entries in the cache. Default is 10_000.
	CacheSize int `env:"VERTEX_CACHE_SIZE"`
}

func NewConfig() Config {
	return Config{
		Algorithm:       Algorithm{Sort: SortGlobal},
		CacheExpiration: 24 * time.Hour,
		CacheSize:       100_000,
	}
}

func (c Config) Validate() error {
	if err := c.Algorithm.Validate(); err != nil {
		return err
	}

	if c.SecretKey == "" {
		return errors.New("secret key is empty or not set")
	}
	if !nostr.IsValid32ByteHex(c.SecretKey) {
		return errors.New("secret key is not a valid 32 byte hex string")
	}

	if c.CacheExpiration < time.Second {
		return errors.New("cache expiration must be greater than 1 second")
	}
	if c.CacheSize <= 0 {
		return errors.New("cache size must be greater than 0")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Vertex:\n"+
		"\tSecret Key: %s\n"+
		"\tCache Expiration: %v\n"+
		"\tCache Size: %d\n"+
		"\tAlgorithm:\n"+
		"\t\tSource: %s\n"+
		"\t\tSort: %s\n"+
		"\t\tThreshold: %f\n",
		c.SecretKey[:4]+"___REDACTED___"+c.SecretKey[len(c.SecretKey)-4:],
		c.CacheExpiration, c.CacheSize,
		c.Algorithm.Source,
		c.Algorithm.Sort,
		c.Algorithm.Threshold,
	)
}
