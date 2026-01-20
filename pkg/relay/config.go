package relay

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/zapstore/server/pkg/vertex"
)

type Config struct {
	// Domain is the domain of the relay, used to validate NIP42 authentication.
	// Default is "".
	Domain string `env:"RELAY_DOMAIN"`

	// Port is the port the relay will listen on. Default is "3334".
	Port string `env:"RELAY_PORT"`

	Info   Info
	Vertex vertex.Config
}

// Info stores information about the relay, used in the NIP11 relay information document.
type Info struct {
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
		Port:   "3334",
		Vertex: vertex.NewConfig(),
	}
}

func (c Config) Validate() error {
	if c.Domain == "" {
		return errors.New("domain is not set")
	}
	if c.Port == "" {
		return errors.New("port is not set")
	}
	if err := c.Info.Validate(); err != nil {
		return err
	}
	if err := c.Vertex.Validate(); err != nil {
		return err
	}
	return nil
}

func (i Info) Validate() error {
	if i.Name == "" {
		slog.Warn("relay name is not set")
	}
	if i.Pubkey == "" {
		slog.Warn("relay pubkey is not set")
	}
	if !nostr.IsValid32ByteHex(i.Pubkey) {
		slog.Warn("relay pubkey is not a valid 32 byte hex string")
	}
	if i.Description == "" {
		slog.Warn("relay description is not set")
	}
	if i.URL == "" {
		slog.Warn("relay URL is not set")
	}
	if i.Contact == "" {
		slog.Warn("relay contact is not set")
	}
	if i.Icon == "" {
		slog.Warn("relay icon is not set")
	}
	if i.Banner == "" {
		slog.Warn("relay banner is not set")
	}
	if i.Software == "" {
		slog.Warn("relay software is not set")
	}
	return nil
}

func (i Info) NIP11() nip11.RelayInformationDocument {
	return nip11.RelayInformationDocument{
		Name:        i.Name,
		PubKey:      i.Pubkey,
		Description: i.Description,
		URL:         i.URL,
		Contact:     i.Contact,
		Icon:        i.Icon,
		Banner:      i.Banner,
		Software:    i.Software,
	}
}

func (i Info) String() string {
	return fmt.Sprintf("Info:\n"+
		"\tName: %s\n"+
		"\tPubkey: %s\n"+
		"\tDescription: %s\n"+
		"\tURL: %s\n"+
		"\tContact: %s\n"+
		"\tIcon: %s\n"+
		"\tBanner: %s\n"+
		"\tSoftware: %s\n",
		i.Name, i.Pubkey, i.Description, i.URL, i.Contact, i.Icon, i.Banner, i.Software)
}

func (c Config) String() string {
	return fmt.Sprintf("Relay:\n"+
		"\tDomain: %s\n"+
		"\tPort: %s\n"+
		c.Info.String()+
		c.Vertex.String(),
		c.Domain, c.Port,
	)
}
