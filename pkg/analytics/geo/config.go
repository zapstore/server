package geo

import (
	"errors"
	"fmt"
	"time"
)

const ip66Endpoint = "https://downloads.ip66.dev/db/ip66.mmdb"

type Config struct {
	// DownloadEndpoint is the URL to download the .mmdb database from. Default is [ip66Endpoint].
	DownloadEndpoint string `env:"ANALYTICS_GEO_DOWNLOAD_ENDPOINT"`

	// DownloadTimeout is the timeout for downloading the .mmdb database. Default is 1 minute.
	DownloadTimeout time.Duration `env:"ANALYTICS_GEO_DOWNLOAD_TIMEOUT"`

	// DownloadMaxSize is the maximum size in bytes of the .mmdb database to download. Default is 1 GB.
	DownloadMaxSize int64 `env:"ANALYTICS_GEO_DOWNLOAD_MAX_SIZE"`
}

func NewConfig() Config {
	return Config{
		DownloadEndpoint: ip66Endpoint,
		DownloadTimeout:  time.Minute,
		DownloadMaxSize:  1_000_000_000,
	}
}

func (c Config) Validate() error {
	if c.DownloadEndpoint == "" {
		return errors.New("download endpoint is required")
	}
	if c.DownloadTimeout < time.Second {
		return errors.New("download timeout must be greater than 1 second to function reliably")
	}
	if c.DownloadMaxSize <= 0 {
		return errors.New("download max size must be positive")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Geo:\n"+
		"\tDownload Endpoint: %s\n"+
		"\tDownload Timeout: %s\n"+
		"\tDownload Max Size: %d bytes",
		c.DownloadEndpoint,
		c.DownloadTimeout,
		c.DownloadMaxSize)
}
