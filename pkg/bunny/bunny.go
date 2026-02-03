// The bunny package is responsible for interacting with the Bunny Storage Zone and CDN.
// It exposes a [NewClient] function to create a new client with the given config.
package bunny

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/pippellia-btc/blossom"
)

var (
	ErrEmptyData        = errors.New("empty data")
	ErrEmptyPath        = errors.New("empty path")
	ErrInvalidChecksum  = errors.New("invalid sha256 checksum")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrFileNotFound     = errors.New("file not found")
)

type Client struct {
	http   http.Client
	config Config
}

// NewClient returns a client from the provided [Config], which is assumed to have been validated.
func NewClient(c Config) (Client, error) {
	client := Client{
		http:   http.Client{Timeout: c.Timeout},
		config: c,
	}
	return client, nil
}

// CDN returns the CDN endpoint.
func (c Client) CDN() string {
	return c.config.CDN
}

// storageURL returns the request URL for the provided path.
func (c Client) storageURL(path string) string {
	return fmt.Sprintf("https://%s/%s/%s",
		c.config.StorageZone.Endpoint,
		c.config.StorageZone.Name,
		path,
	)
}

// cdnURL returns the request URL for the provided path on the CDN.
func (c Client) cdnURL(path string) string {
	return fmt.Sprintf("%s/%s", c.config.CDN, path)
}

// Download the file at the specified path.
// Returns the reader for the file, or an error if the file does not exist.
// The caller is responsible for closing the reader.
func (c Client) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	if path == "" {
		return nil, fmt.Errorf("failed to download: %w", ErrEmptyPath)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.storageURL(path), nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}

	if res.StatusCode == http.StatusOK {
		return res.Body, nil
	}

	res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("failed to download: %w", ErrFileNotFound)
	}
	return nil, fmt.Errorf("failed to download: status %s", res.Status)
}

// Check returns the metadata of the file at the specified path on the CDN.
func (c Client) Check(ctx context.Context, path string) (mime string, size int64, err error) {
	if path == "" {
		return "", 0, fmt.Errorf("failed to check: %w", ErrEmptyPath)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodHead, c.cdnURL(path), nil,
	)

	if err != nil {
		return "", 0, fmt.Errorf("failed to check: failed to create request: %w", err)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to check: %w", err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		mime = res.Header.Get("Content-Type")
		size, err = strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
		if err != nil {
			return "", 0, fmt.Errorf("failed to check: failed to parse content length: %w", err)
		}
		return mime, size, nil

	case http.StatusNotFound:
		return "", 0, fmt.Errorf("failed to check: %w", ErrFileNotFound)

	default:
		return "", 0, fmt.Errorf("failed to check: status %s", res.Status)
	}
}

// Upload the data to the Bunny storage zone with the specified path.
//
// If the hex-encoded sha256 is not empty (not ""), it will be used to set the "Checksum" header.
// If the sha256(data) != sha256, Bunny will reject the upload and [ErrChecksumMismatch] will be returned.
func (c Client) Upload(ctx context.Context, data io.Reader, path string, sha256 string) error {
	if data == nil {
		return fmt.Errorf("failed to upload: %w", ErrEmptyData)
	}
	if path == "" {
		return fmt.Errorf("failed to upload: %w", ErrEmptyPath)
	}
	if sha256 != "" {
		if err := blossom.ValidateHash(sha256); err != nil {
			return fmt.Errorf("failed to upload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPut, c.storageURL(path), data,
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	if sha256 != "" {
		req.Header.Add("Checksum", strings.ToUpper(sha256))
	}

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusCreated:
		return nil

	case http.StatusBadRequest:
		body, _ := io.ReadAll(res.Body)
		if strings.Contains(string(body), "Checksum") {
			return fmt.Errorf("failed to upload: %w", ErrChecksumMismatch)
		}
		return fmt.Errorf("failed to upload: status %s: body %s", res.Status, string(body))

	default:
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to upload: status %s: body %s", res.Status, string(body))
	}
}

// Delete the file at the specified path.
// Returns nil if the file was deleted successfully, or if the file did not exist.
func (c Client) Delete(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("failed to delete: %w", ErrEmptyPath)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodDelete, c.storageURL(path), nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound ||
		res.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("failed to delete: %s", res.Status)
}

// setHeaders sets the common headers for the request.
func (c Client) setHeaders(r *http.Request) {
	r.Header.Add("accept", "application/json")
	r.Header.Add("AccessKey", c.config.StorageZone.Password)
}
