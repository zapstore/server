package geo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Locator represents a geo-locator, returning the country code for a given IP address.
type Locator struct {
	db   *maxminddb.Reader
	http *http.Client

	path   string
	config Config
	log    *slog.Logger
}

func NewLocator(c Config, path string, log *slog.Logger) (*Locator, error) {
	if log == nil {
		return nil, errors.New("logger is required")
	}

	locator := &Locator{
		http:   &http.Client{Timeout: c.DownloadTimeout},
		path:   path,
		config: c,
		log:    log,
	}

	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to stat geolocation database at %q: %w", path, err)
	}

	if errors.Is(err, os.ErrNotExist) {
		if err = locator.downloadDB(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to download geolocation database from %q: %w", c.DownloadEndpoint, err)
		}
	}

	locator.db, err = maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open geolocation database at %q: %w", path, err)
	}
	return locator, nil
}

// Close closes the geolocation database.
func (l *Locator) Close() error {
	return l.db.Close()
}

// LookupCountry looks up the country ISO code of the given IP address.
// If the Engine geolocation is not enabled or broken, the function returns an empty string.
// Any error will be logged and the ISO code will be returned as an empty string.
func (l *Locator) LookupCountry(ip net.IP) (string, error) {
	if ip == nil {
		return "", errors.New("failed to lookup country: ip is nil")
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return "", errors.New("failed to lookup country: failed to parse ip")
	}
	addr = addr.Unmap()

	var country string
	if err := l.db.Lookup(addr).DecodePath(&country, "country", "iso_code"); err != nil {
		return "", fmt.Errorf("failed to lookup country: %w", err)
	}
	return country, nil
}

// downloadDB downloads the .mmdb file from the specified endpoint and
// atomically stores it at the specified path. The download is written to a
// temporary file first, then renamed into place, so the existing file (if any)
// is never left in a partial state.
func (l *Locator) downloadDB(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.config.DownloadEndpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := l.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %s: %s", resp.Status, body)
	}

	cl := resp.ContentLength
	if cl > l.config.DownloadMaxSize {
		return fmt.Errorf("Content-Length %d exceeds max allowed size of %d bytes", cl, l.config.DownloadMaxSize)
	}

	tmp, err := os.CreateTemp(filepath.Dir(l.path), "geo.*.mmdb.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, l.config.DownloadMaxSize))
	if err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if n > cl {
		l.log.Warn("geo: bytes received exceed Content-Length", "content_length", cl, "received", n)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, l.path); err != nil {
		return fmt.Errorf("failed to move mmdb into place: %w", err)
	}
	return nil
}
