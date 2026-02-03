package blossom

import (
	"fmt"

	"github.com/pippellia-btc/blossom"
	"github.com/zapstore/server/pkg/blossom/bunny"
)

type Config struct {
	// Domain is the domain of the blossom server, used to validate authorization
	// events and for the "url" field in blob descriptors.
	Domain string `env:"BLOSSOM_DOMAIN"`

	// Port is the port the blossom server will listen on. Default is "3335".
	Port string `env:"BLOSSOM_PORT"`

	// AllowedContentTypes is a list of content types that are allowed to be uploaded to the blossom server.
	// Default is "application/vnd.android.package-archive" and common image types.
	AllowedMedia []string `env:"BLOSSOM_ALLOWED_MEDIA"`

	// BlockedBlobs is a list of blob hashes that are blocked from being published to the blossom server.
	// Default is empty.
	BlockedBlobs []string `env:"BLOSSOM_BLOCKED_BLOBS"`

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
	if c.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if c.Port == "" {
		return fmt.Errorf("port is required")
	}

	for _, mime := range c.AllowedMedia {
		if mime == "" {
			return fmt.Errorf("allowed media type is empty")
		}
	}

	for i, hash := range c.BlockedBlobs {
		if _, err := blossom.ParseHash(hash); err != nil {
			return fmt.Errorf("blocked blob hash is invalid at index %d: %w", i, err)
		}
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
		"\tAllowed Media: %v\n"+
		"\tBlocked Blobs: %v\n"+
		c.Bunny.String(), c.Domain, c.Port, c.AllowedMedia, c.BlockedBlobs)
}
