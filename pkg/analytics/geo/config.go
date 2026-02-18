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

	// DownloadTimeout is the timeout for downloading the .mmdb database. Default is 30 seconds.
	DownloadTimeout time.Duration `env:"ANALYTICS_GEO_DOWNLOAD_TIMEOUT"`

	// DownloadMaxSize is the maximum size in bytes of the .mmdb database to download. Default is 1 GiB.
	DownloadMaxSize int64 `env:"ANALYTICS_GEO_DOWNLOAD_MAX_SIZE"`

	// RefreshInterval is the interval at which the .mmdb database should be refreshed. Default is 24 hours.
	RefreshInterval time.Duration `env:"ANALYTICS_GEO_REFRESH_INTERVAL"`
}

func NewConfig() Config {
	return Config{
		DownloadEndpoint: ip66Endpoint,
		DownloadTimeout:  30 * time.Second,
		DownloadMaxSize:  1 << 30, // 1 GiB
		RefreshInterval:  24 * time.Hour,
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
	if c.RefreshInterval < time.Hour {
		return errors.New("refresh interval must be greater than 1 hour to avoid rate-limits")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Geo:\n"+
		"\tDownload Endpoint: %s\n"+
		"\tDownload Timeout: %s\n"+
		"\tDownload Max Size: %d bytes\n"+
		"\tRefresh Interval: %s",
		c.DownloadEndpoint,
		c.DownloadTimeout,
		c.DownloadMaxSize,
		c.RefreshInterval)
}
