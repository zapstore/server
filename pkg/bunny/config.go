package bunny

import (
	"errors"
	"fmt"
	"time"
)

type Config struct {
	StorageZone StorageZone
	Timeout     time.Duration `env:"BUNNY_REQUEST_TIMEOUT"`
}

type StorageZone struct {
	// The username of the storage zone (e.g. "your-username").
	Name string `env:"BUNNY_STORAGE_ZONE_NAME"`

	// The endpoint of the primary region of the storage zone (e.g. "storage.bunnycdn.com").
	Endpoint string `env:"BUNNY_STORAGE_ZONE_ENDPOINT"`

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
		return errors.New("storage zone must be specified")
	}
	if c.StorageZone.Endpoint == "" {
		return errors.New("storage zone endpoint must be specified")
	}
	if len(c.StorageZone.Password) < 8 {
		return errors.New("storage zone password must be specified and at least 8 characters long")
	}
	if c.Timeout < time.Second {
		return errors.New("timeout must be greater than 1s to function reliably")
	}
	return nil
}

func (s StorageZone) String() string {
	return fmt.Sprintf("StorageZone:\n"+
		"\tName: %s\n"+
		"\tEndpoint: %s\n"+
		"\tPassword: %s\n",
		s.Name,
		s.Endpoint,
		s.Password[:4]+"___REDACTED___"+s.Password[len(s.Password)-4:],
	)
}

func (c Config) String() string {
	return fmt.Sprintf("Bunny:\n"+
		"\tRequest Timeout: %v\n"+
		"\tStorageZone:\n"+
		"\t\tName: %s\n"+
		"\t\tEndpoint: %s\n"+
		"\t\tPassword: %s\n",
		c.Timeout,
		c.StorageZone.Name,
		c.StorageZone.Endpoint,
		c.StorageZone.Password[:4]+"___REDACTED___"+c.StorageZone.Password[len(c.StorageZone.Password)-4:],
	)
}
