package blossom

import (
	"fmt"

	"github.com/zapstore/server/pkg/blossom/bunny"
)

type Config struct {
	// Hostname is the hostname of the blossom server, used to validate authorization
	// events and for the "url" field in blob descriptors.
	Hostname string `env:"BLOSSOM_HOSTNAME"`

	// Port is the port the blossom server will listen on. Default is "3335".
	Port string `env:"BLOSSOM_PORT"`

	// AllowedContentTypes is a list of content types that are allowed to be uploaded to the blossom server.
	// Default is "application/vnd.android.package-archive" and common image types.
	AllowedMedia []string `env:"BLOSSOM_ALLOWED_MEDIA"`

	Bunny bunny.Config
}

func NewConfig() Config {
	return Config{
		Port: "3335",
		AllowedMedia: []string{
			"application/vnd.android.package-archive",
			"image/jpeg",
			"image/png",
			"image/webp",
			"image/gif",
			"image/heic",
			"image/heif",
			"image/svg+xml",
		},
		Bunny: bunny.NewConfig(),
	}
}

func (c Config) Validate() error {
	if c.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if c.Port == "" {
		return fmt.Errorf("port is required")
	}

	for _, mime := range c.AllowedMedia {
		if mime == "" {
			return fmt.Errorf("allowed media type is empty")
		}
	}

	if err := c.Bunny.Validate(); err != nil {
		return fmt.Errorf("bunny: %w", err)
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Blossom:\n"+
		"\tHostname: %s\n"+
		"\tPort: %s\n"+
		"\tAllowed Media: %v\n"+
		c.Bunny.String(), c.Hostname, c.Port, c.AllowedMedia)
}
