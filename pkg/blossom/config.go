package blossom

import (
	"fmt"

	"github.com/zapstore/server/pkg/bunny"
)

type Config struct {
	// Domain is the domain of the blossom server, used to validate authorization
	// events and for the "url" field in blob descriptors.
	Domain string `env:"BLOSSOM_DOMAIN"`

	// Port is the port the blossom server will listen on. Default is "3335".
	Port string `env:"BLOSSOM_PORT"`

	Bunny bunny.Config
}

func NewConfig() Config {
	return Config{
		Port:  "3335",
		Bunny: bunny.NewConfig(),
	}
}

func (c Config) Validate() error {
	if c.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if c.Port == "" {
		return fmt.Errorf("port is required")
	}
	if err := c.Bunny.Validate(); err != nil {
		return fmt.Errorf("bunny: %w", err)
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Blossom:\n"+
		"\tDomain: %s\n"+
		"\tPort: %s\n"+
		c.Bunny.String(), c.Domain, c.Port)
}
