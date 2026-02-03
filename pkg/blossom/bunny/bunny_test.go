package bunny

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/caarlos0/env/v11"
	_ "github.com/joho/godotenv/autoload"
)

// =========================== TESTS ============================
// The tests require a .env file with the following variables:
// - BUNNY_STORAGE_ZONE_NAME
// - BUNNY_STORAGE_ZONE_ENDPOINT
// - BUNNY_STORAGE_ZONE_PASSWORD
// - BUNNY_CDN_URL
//
// Configure these by checking your Bunny dashboard.
//
// Note: these tests require the file "file_exists.txt" to be present in the Bunny storage zone.
// ================================================================

var (
	config = NewConfig()
	ctx    = context.Background()
)

func init() {
	if err := env.Parse(&config); err != nil {
		panic(fmt.Errorf("failed to parse config: %w", err))
	}
}

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

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			err = client.Upload(ctx, test.data, test.path, test.sha256)
			if !errors.Is(err, test.err) {
				t.Fatalf("expected error %v, got %v", test.err, err)
			}
		})
	}
}

func TestDownload(t *testing.T) {
	tests := []struct {
		name string
		path string
		err  error
	}{
		{
			name: "invalid path (empty)",
			path: "",
			err:  ErrEmptyPath,
		},
		{
			name: "file does not exists",
			path: "file_does_not_exist.txt",
			err:  ErrFileNotFound,
		},
		{
			name: "file exists",
			path: "file_exists.txt",
		},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	expected := []byte("This is a test")
	payload := bytes.NewReader(expected)

	if err = client.Upload(ctx, payload, "file_exists.txt", ""); err != nil {
		t.Fatalf("failed to upload file: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := client.Download(ctx, test.path)
			if !errors.Is(err, test.err) {
				t.Fatalf("expected error %v, got %v", test.err, err)
			}

			if err == nil {
				defer data.Close()
				actual, err := io.ReadAll(data)
				if err != nil {
					t.Fatalf("failed to read data: %v", err)
				}
				if !bytes.Equal(actual, expected) {
					t.Fatalf("expected %v, got %v", expected, actual)
				}
			}
		})
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name string
		path string
		mime string
		size int64
		err  error
	}{
		{
			name: "invalid path (empty)",
			path: "",
			err:  ErrEmptyPath,
		},
		{
			name: "file does not exists",
			path: "file_does_not_exist.txt",
			err:  ErrFileNotFound,
		},
		{
			name: "file exists",
			path: "file_exists.txt",
			mime: "text/plain",
			size: 14,
		},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mime, size, err := client.Check(ctx, test.path)
			if !errors.Is(err, test.err) {
				t.Fatalf("expected error %v, got %v", test.err, err)
			}

			if mime != test.mime {
				t.Fatalf("expected mime %v, got %v", test.mime, mime)
			}
			if size != test.size {
				t.Fatalf("expected size %v, got %v", test.size, size)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name string
		path string
		err  error
	}{
		{
			name: "invalid path (empty)",
			path: "",
			err:  ErrEmptyPath,
		},
		{
			name: "valid delete (file exists)",
			path: "test.txt",
		},
		{
			name: "valid delete (file does not exist)",
			path: "test.txt",
		},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err = client.Delete(ctx, test.path)
			if !errors.Is(err, test.err) {
				t.Fatalf("expected error %v, got %v", test.err, err)
			}
		})
	}
}
