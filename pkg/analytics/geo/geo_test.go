package geo

import (
	"net"
	"os"
	"testing"
)

// TestNewLocator_E2E is an end-to-end test that downloads the real geolocatio database,
// looks up a known IP address, and asserts the expected country code.
//
// Run with:
//
//	go test -run TestNewLocator_E2E -v
func TestLocator_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	dir := t.TempDir()
	path := dir + "/ip66.mmdb"

	locator, err := NewLocator(NewConfig(), path)
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}
	defer locator.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected database file to exist at %q: %v", path, err)
	}

	tests := []struct {
		name     string
		ip       string
		expected string
	}{
		{
			name:     "Google DNS (US)",
			ip:       "8.8.8.8",
			expected: "US",
		},
		{
			name:     "Cloudflare DNS (AU)",
			ip:       "1.1.1.1",
			expected: "AU",
		},
		{
			name:     "Deutsche Telekom (DE)",
			ip:       "80.148.0.1",
			expected: "DE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ip := net.ParseIP(test.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", test.ip)
			}

			country, err := locator.Country(ip)
			if err != nil {
				t.Fatalf("Country(%q) returned error: %v", test.ip, err)
			}

			if country != test.expected {
				t.Errorf("Country(%q) = %q, want %q", test.ip, country, test.expected)
			}
		})
	}
}
