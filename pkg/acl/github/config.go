package github

import (
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Config struct {
	// Timeout is the maximum time to wait for a response from Github. Default is 10 seconds.
	Timeout time.Duration `env:"GITHUB_REQUEST_TIMEOUT"`

	// Token is an optional GitHub API token.
	// If set, it is added to all requests to increase the rate limit from 60 to 5,000 requests/hour.
	Token string `env:"GITHUB_API_TOKEN"`
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
	if c.Token == "" {
		slog.Warn("Github API token not set, which might cause stricter rate-limiting.")
	}
	return nil
}

func (c Config) String() string {
	token := "[not set]"
	if c.Token != "" {
		token = c.Token[:4] + "___REDACTED___" + c.Token[len(c.Token)-4:]
	}

	return fmt.Sprintf("Github:\n"+
		"\tTimeout: %s\n"+
		"\tToken: %s\n",
		c.Timeout, token,
	)
}
