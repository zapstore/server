package bunny

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	StorageZone StorageZone

	// The hostname of the CDN (e.g. "zapstore.b-cdn.net"). It must not include the scheme.
	CDN string `env:"BUNNY_CDN_HOSTNAME"`

	// The timeout for the requests to the Bunny storage zone. Default is 10 seconds.
	Timeout time.Duration `env:"BUNNY_REQUEST_TIMEOUT"`
}

type StorageZone struct {
	// The username of the storage zone (e.g. "your-username").
	Name string `env:"BUNNY_STORAGE_ZONE_NAME"`

	// The hostname of the primary region of the storage zone (e.g. "storage.bunnycdn.com").
	// It must not include the scheme.
	Hostname string `env:"BUNNY_STORAGE_ZONE_HOSTNAME"`

	// The password of the storage zone (e.g. "your-password").
	Password string `env:"BUNNY_STORAGE_ZONE_PASSWORD"`
}

func NewConfig() Config {
	return Config{
		Timeout: 10 * time.Second,
	}
}

func (c Config) Validate() error {
	if c.StorageZone.Name == "" {
		return errors.New("storage zone name must be specified")
	}
	if err := ValidateHostname(c.StorageZone.Hostname); err != nil {
		return fmt.Errorf("storage zone hostname: %w", err)
	}
	if len(c.StorageZone.Password) < 8 {
		return errors.New("storage zone password must be specified and at least 8 characters long")
	}
	if c.Timeout < time.Second {
		return errors.New("timeout must be greater than 1s to function reliably")
	}
	if err := ValidateHostname(c.CDN); err != nil {
		return fmt.Errorf("CDN hostname: %w", err)
	}
	return nil
}

// ValidateHostname validates that a hostname string is not empty, is a valid hostname,
// and does not contain a scheme (e.g., "example.com" is valid, "https://example.com" is not).
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return errors.New("hostname must not be empty")
	}
	if strings.Contains(hostname, "://") {
		return errors.New("hostname must not include a scheme (e.g., http:// or https://)")
	}
	if strings.HasPrefix(hostname, "/") || strings.HasSuffix(hostname, "/") {
		return errors.New("hostname must not include a leading or trailing slash")
	}
	parsed, err := url.Parse("https://" + hostname)
	if err != nil {
		return fmt.Errorf("invalid hostname: %w", err)
	}
	if parsed.Host == "" {
		return errors.New("hostname must contain a valid host")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Bunny:\n"+
		"\tRequest Timeout: %v\n"+
		"\tCDN Hostname: %s\n"+
		"\tStorageZone:\n"+
		"\t\tName: %s\n"+
		"\t\tHostname: %s\n"+
		"\t\tPassword: %s\n",
		c.Timeout,
		c.CDN,
		c.StorageZone.Name,
		c.StorageZone.Hostname,
		c.StorageZone.Password[:4]+"___REDACTED___"+c.StorageZone.Password[len(c.StorageZone.Password)-4:],
	)
}
