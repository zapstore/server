package relay

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/zapstore/server/pkg/events"
	"github.com/zapstore/server/pkg/relay/store"
	"github.com/zapstore/server/pkg/vertex"
)

type PubkeyPolicy string

const (
	// PubkeyPolicyAllow allows unknown pubkeys to publish to the relay.
	PubkeyPolicyAllow PubkeyPolicy = "ALLOW"

	// PubkeyPolicyBlock blocks unknown pubkeys from publishing to the relay.
	PubkeyPolicyBlock PubkeyPolicy = "BLOCK"

	// PubkeyPolicyVertex uses the Vertex DVM to determine whether to allow or block unknown pubkeys.
	// It uses the Vertex configuration to determine the threshold and algorithm to use.
	PubkeyPolicyVertex PubkeyPolicy = "VERTEX"
)

var PubkeyPolicies = []PubkeyPolicy{PubkeyPolicyAllow, PubkeyPolicyBlock, PubkeyPolicyVertex}

type Config struct {
	// Domain is the domain of the relay, used to validate NIP42 authentication.
	// Default is "".
	Domain string `env:"RELAY_DOMAIN"`

	// Port is the port the relay will listen on. Default is "3334".
	Port string `env:"RELAY_PORT"`

	// MaxMessageBytes is the maximum size of a message that can be sent to the relay.
	// Default is 500_000 (0.5MB).
	MaxMessageBytes int64 `env:"RELAY_MAX_MESSAGE_BYTES"`

	// MaxFilters is the maximum number of filters that can be applied to a connection.
	// Default is 50.
	MaxFilters int `env:"RELAY_MAX_REQ_FILTERS"`

	// AllowedKinds is a list of event kinds that are allowed to be published to the relay.
	// Default is all kinds.
	AllowedKinds []int `env:"RELAY_ALLOWED_EVENT_KINDS"`

	// BlockedIDs is a list of event IDs that are blocked from being published to the relay.
	// Default is empty.
	BlockedIDs []string `env:"RELAY_BLOCKED_EVENT_IDS"`

	// Allowlist is a list of pubkeys that are considered trusted and can publish to the relay.
	Allowlist []string `env:"RELAY_PUBKEY_ALLOWLIST"`

	// Blocklist is a list of pubkeys that are considered distrusted and cannot publish to the relay.
	Blocklist []string `env:"RELAY_PUBKEY_BLOCKLIST"`

	// UnknownPubkeyPolicy is the policy to apply to unknown pubkeys,
	// which are the ones not in the allowlist or blocklist.
	// Possible values are "allow", "block", "vertex". Default is "vertex".
	UnknownPubkeyPolicy PubkeyPolicy `env:"RELAY_PUBKEY_UNKNOWN_POLICY"`

	Vertex vertex.Config

	Store store.Config

	Info Info
}

// NewConfig create a new config with default values.
func NewConfig() Config {
	return Config{
		Port:                "3334",
		MaxMessageBytes:     500_000,
		MaxFilters:          50,
		AllowedKinds:        events.WithValidation,
		UnknownPubkeyPolicy: PubkeyPolicyVertex,
		Vertex:              vertex.NewConfig(),
	}
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

func (c Config) Validate() error {
	if c.Domain == "" {
		return errors.New("domain is not set")
	}
	if c.Port == "" {
		return errors.New("port is not set")
	}
	if c.MaxMessageBytes <= 0 {
		return errors.New("max message bytes must be greater than 0")
	}
	if c.MaxFilters <= 0 {
		return errors.New("max filters must be greater than 0")
	}

	if len(c.AllowedKinds) == 0 {
		slog.Warn("relay allowed kinds is empty. No events will be accepted.")
	}

	for _, id := range c.BlockedIDs {
		if err := events.ValidateHash(id); err != nil {
			return fmt.Errorf("invalid blocked event ID: %w", err)
		}
	}
	for i, pubkey := range c.Blocklist {
		if !nostr.IsValid32ByteHex(pubkey) {
			return fmt.Errorf("invalid blacklisted pubkey at index %d: %s", i, pubkey)
		}
	}
	for i, pubkey := range c.Allowlist {
		if !nostr.IsValid32ByteHex(pubkey) {
			return fmt.Errorf("invalid whitelisted pubkey at index %d: %s", i, pubkey)
		}
	}
	if !slices.Contains(PubkeyPolicies, c.UnknownPubkeyPolicy) {
		return fmt.Errorf("unknown pubkey policy: %s. Possible values are: %v", c.UnknownPubkeyPolicy, PubkeyPolicies)
	}

	if err := c.Vertex.Validate(); err != nil {
		return fmt.Errorf("vertex: %w", err)
	}

	if err := c.Store.Validate(); err != nil {
		return fmt.Errorf("store: %w", err)
	}

	if err := c.Info.Validate(); err != nil {
		// info is not critical, so we log the error and continue
		slog.Error("relay info is invalid or incomplete", "error", err)
	}
	return nil
}

func (i Info) Validate() error {
	if i.Name == "" {
		return errors.New("relay name is not set")
	}
	if i.Pubkey == "" {
		return errors.New("relay pubkey is not set")
	}
	if !nostr.IsValid32ByteHex(i.Pubkey) {
		return errors.New("relay pubkey is not a valid 32 byte hex string")
	}
	if i.Description == "" {
		return errors.New("relay description is not set")
	}
	if i.URL == "" {
		return errors.New("relay URL is not set")
	}
	if i.Contact == "" {
		return errors.New("relay contact is not set")
	}
	if i.Icon == "" {
		return errors.New("relay icon is not set")
	}
	if i.Banner == "" {
		return errors.New("relay banner is not set")
	}
	if i.Software == "" {
		return errors.New("relay software is not set")
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
		"\tMax Message Bytes: %d\n"+
		"\tMax Filters: %d\n"+
		"\tAllowed Kinds: %v\n"+
		"\tBlocked IDs: %v\n"+
		"\tAllowlist: %v\n"+
		"\tBlocklist: %v\n"+
		"\tUnknown Pubkey Policy: %s\n"+
		c.Info.String()+
		c.Vertex.String()+
		c.Store.String(),
		c.Domain, c.Port, c.MaxMessageBytes, c.MaxFilters, c.AllowedKinds, c.BlockedIDs, c.Allowlist, c.Blocklist, c.UnknownPubkeyPolicy,
	)
}
