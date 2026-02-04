// The acl package provides access control list functionality for the relay and blossom servers.
// It supports hot-reloadable CSV files for allowed/blocked pubkeys, events, and blobs,
// with a configurable policy for unknown pubkeys (allow, block, or vertex-based reputation filtering).
package acl

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/zapstore/server/pkg/acl/vertex"
)

// Hardcoded file names within the ACL directory.
const (
	PubkeysAllowedFile = "pubkeys_allowed.csv"
	PubkeysBlockedFile = "pubkeys_blocked.csv"
	EventsBlockedFile  = "events_blocked.csv"
	BlobsBlockedFile   = "blobs_blocked.csv"
)

var RequiredFiles = []string{PubkeysAllowedFile, PubkeysBlockedFile, EventsBlockedFile, BlobsBlockedFile}

// PubkeyPolicy determines how to handle pubkeys that are not in the allowed or blocked lists.
type PubkeyPolicy string

const (
	// AllowAll allows unknown pubkeys.
	AllowAll PubkeyPolicy = "ALLOW"

	// BlockAll blocks unknown pubkeys.
	BlockAll PubkeyPolicy = "BLOCK"

	// UseVertex uses the Vertex DVM to determine whether to allow or block unknown pubkeys.
	UseVertex PubkeyPolicy = "VERTEX"
)

var PubkeyPolicies = []PubkeyPolicy{AllowAll, BlockAll, UseVertex}

type Config struct {
	// Path is the path to the directory containing the ACL CSV files.
	// The directory must contain:
	//   - allowed_pubkeys.csv
	//   - blocked_pubkeys.csv
	//   - blocked_events.csv
	//   - blocked_blobs.csv
	// Default is "acl".
	Dir string `env:"ACL_DIRECTORY_PATH"`

	// UnknownsPolicy is the policy to apply to pubkeys not in the allowed or blocked lists.
	// Possible values are "ALLOW", "BLOCK", "VERTEX". Default is "VERTEX".
	UnknownPubkeyPolicy PubkeyPolicy `env:"ACL_UNKNOWN_PUBKEY_POLICY"`

	// Vertex is the configuration for the Vertex DVM, used when PubkeyPolicy is "VERTEX".
	Vertex vertex.Config
}

// NewConfig creates a new Config with default values.
func NewConfig() Config {
	return Config{
		Dir:                 "acl",
		UnknownPubkeyPolicy: UseVertex,
		Vertex:              vertex.NewConfig(),
	}
}

// Validate checks that the configuration is valid.
func (c Config) Validate() error {
	if c.Dir == "" {
		return errors.New("path is empty or not set")
	}

	info, err := os.Stat(c.Dir)
	if err != nil {
		return fmt.Errorf("acl directory not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("acl path is not a directory: %s", c.Dir)
	}

	for _, file := range RequiredFiles {
		path := c.Dir + "/" + file
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("required file not found: %s", path)
		}
	}

	if !slices.Contains(PubkeyPolicies, c.UnknownPubkeyPolicy) {
		return fmt.Errorf("unknown pubkey policy: %s. Possible values are: %v", c.UnknownPubkeyPolicy, PubkeyPolicies)
	}

	if c.UnknownPubkeyPolicy == UseVertex {
		if err := c.Vertex.Validate(); err != nil {
			return fmt.Errorf("vertex: %w", err)
		}
	}
	return nil
}

// String returns a string representation of the configuration.
func (c Config) String() string {
	s := fmt.Sprintf("ACL:\n"+
		"\tDirectory Path: %s\n"+
		"\tUnknown Pubkey Policy: %s\n",
		c.Dir, c.UnknownPubkeyPolicy)

	if c.UnknownPubkeyPolicy == UseVertex {
		s += c.Vertex.String()
	}
	return s
}
