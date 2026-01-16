package relay

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip11"
)

type Config struct {
	Domain      string `env:"RELAY_DOMAIN"`
	Port        string `env:"RELAY_PORT"`
	Name        string `env:"RELAY_NAME"`
	Pubkey      string `env:"RELAY_PUBKEY"`
	Description string `env:"RELAY_DESCRIPTION"`
	URL         string `env:"RELAY_URL"`
	Contact     string `env:"RELAY_CONTACT"`
	Icon        string `env:"RELAY_ICON"`
	Banner      string `env:"RELAY_BANNER"`
	Software    string `env:"RELAY_SOFTWARE"`
}

// NewConfig create a new config with default values.
func NewConfig() Config {
	return Config{
		Domain: "zapstore.dev",
		Port:   "3334",
	}
}

func (c Config) Validate() error {
	if c.Domain == "" {
		return errors.New("relay domain is required")
	}
	if c.Port == "" {
		return errors.New("relay port is required")
	}
	if c.Name == "" {
		slog.Warn("relay name is not set")
	}
	if c.Pubkey == "" {
		slog.Warn("relay pubkey is not set")
	}
	if !nostr.IsValid32ByteHex(c.Pubkey) {
		slog.Warn("relay pubkey is not a valid 32 byte hex string")
	}
	if c.Description == "" {
		slog.Warn("relay description is not set")
	}
	if c.URL == "" {
		slog.Warn("relay URL is not set")
	}
	if c.Contact == "" {
		slog.Warn("relay contact is not set")
	}
	if c.Icon == "" {
		slog.Warn("relay icon is not set")
	}
	if c.Banner == "" {
		slog.Warn("relay banner is not set")
	}
	if c.Software == "" {
		slog.Warn("relay software is not set")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Relay Config:\n"+
		"\tDomain: %s\n"+
		"\tPort: %s\n"+
		"\tName: %s\n"+
		"\tPubkey: %s\n"+
		"\tDescription: %s\n"+
		"\tURL: %s\n"+
		"\tContact: %s\n"+
		"\tIcon: %s\n"+
		"\tBanner: %s\n"+
		"\tSoftware: %s\n",
		c.Domain, c.Port, c.Name, c.Pubkey, c.Description, c.URL, c.Contact, c.Icon, c.Banner, c.Software)
}

func (c Config) NIP11() nip11.RelayInformationDocument {
	return nip11.RelayInformationDocument{
		URL:         c.URL,
		Name:        c.Name,
		Description: c.Description,
		PubKey:      c.Pubkey,
		Contact:     c.Contact,
		Icon:        c.Icon,
		Banner:      c.Banner,
		Software:    c.Software,
	}
}
