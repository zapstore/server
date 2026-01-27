package bunny

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

var ctx = context.Background()

// Run this test with:
//
// PASSWORD=<your_password> go test
//
// Where <your_password> is the password of the Bunny storage zone.
func TestUpload(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		data   io.Reader
		sha256 string
		err    error
	}{
		{
			name: "empty data",
			path: "test.txt",
			data: nil,
			err:  ErrEmptyData,
		},
		{
			name: "invalid path (empty)",
			path: "",
			data: bytes.NewReader([]byte("This is a test")),
			err:  ErrEmptyPath,
		},
		{
			name:   "invalid sha256",
			path:   "test.txt",
			data:   bytes.NewReader([]byte("This is a test")),
			sha256: "invalid",
			err:    ErrInvalidChecksum,
		},
		{
			name: "valid test (no checksum)",
			path: "test.txt",
			data: bytes.NewReader([]byte("This is a test")),
		},
		{
			name:   "valid test (with checksum)",
			path:   "test_with_checksum.txt",
			data:   bytes.NewReader([]byte("This is a test with checksum")),
			sha256: "3323af1d54db3c1c940f90486d1816e9592636125f21b1b29ac927e7a9262ac9",
		},
	}

	config := Config{
		StorageZone: StorageZone{
			Name:     "zapstore-test",
			Endpoint: "storage.bunnycdn.com",
			Password: os.Getenv("PASSWORD"),
		},
		Timeout: 10 * time.Second,
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			err = client.Upload(ctx, test.data, test.path, test.sha256)
			if !errors.Is(err, test.err) {
				t.Fatalf("expected error %v, got %v", test.err, err)
			}
		})
	}
}
