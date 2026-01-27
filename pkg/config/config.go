// The package config is responsible for loading package specific configs from the
// environment variables, and validating them.
//
// Packages requiring configs should expose:
// - A Config struct with the package specific config parameters.
// - A NewConfig() function to create a new Config with default parameters.
// - A Validate() method to validate the config.
// - A String() method to return a string representation of the config.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	_ "github.com/joho/godotenv/autoload"
	"github.com/zapstore/server/pkg/bunny"
	"github.com/zapstore/server/pkg/rate"
	"github.com/zapstore/server/pkg/relay"
)

type Config struct {
	Relay relay.Config
	Rate  rate.Config
	Bunny bunny.Config
}

// Load creates a new [Config] with default parameters, that get overwritten by env variables when specified.
// It returns an error if the config is invalid.
func Load() (Config, error) {
	config := New()
	if err := env.Parse(&config); err != nil {
		return Config{}, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("failed to validate config: %w", err)
	}
	return config, nil
}

func New() Config {
	return Config{
		Relay: relay.NewConfig(),
		Rate:  rate.NewConfig(),
		Bunny: bunny.NewConfig(),
	}
}

func (c Config) Validate() error {
	if err := c.Relay.Validate(); err != nil {
		return fmt.Errorf("relay: %w", err)
	}
	if err := c.Rate.Validate(); err != nil {
		return fmt.Errorf("rate: %w", err)
	}
	if err := c.Bunny.Validate(); err != nil {
		return fmt.Errorf("bunny: %w", err)
	}
	return nil
}

func (c Config) Print() {
	fmt.Println(c.Relay)
	fmt.Println(c.Rate)
	fmt.Println(c.Bunny)
}
