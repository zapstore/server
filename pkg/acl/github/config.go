package github

import (
	"errors"
	"fmt"
	"time"
)

type Config struct {
	// Timeout is the maximum time to wait for a response from Github. Default is 10 seconds.
	Timeout time.Duration `env:"GITHUB_REQUEST_TIMEOUT"`
	// Token is an optional GitHub personal access token.
	// If set, it is added to all requests to increase the rate limit from 60 to 5,000 requests/hour.
	Token string `env:"GITHUB_TOKEN"`
}

func NewConfig() Config {
	return Config{
		Timeout: 10 * time.Second,
	}
}

func (c Config) Validate() error {
	if c.Timeout < time.Second {
		return errors.New("timeout must be at least 1 second to function reliably")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Github:\n"+
		"\tTimeout: %s\n"+
		"\tToken: %s",
		c.Timeout, c.Token[:4]+"___REDACTED___"+c.Token[len(c.Token)-4:],
	)
}
