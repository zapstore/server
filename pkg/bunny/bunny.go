package bunny

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	ErrEmptyData        = errors.New("empty data")
	ErrEmptyPath        = errors.New("empty path")
	ErrInvalidChecksum  = errors.New("invalid sha256 checksum")
	ErrChecksumMismatch = errors.New("checksum mismatch")
)

type Client struct {
	http   http.Client
	config Config
}

// NewClient returns a client from the provided [Config], which is assumed to have been validated.
// Internally, it performs a call to checkavailability
func NewClient(c Config) (Client, error) {
	client := Client{
		http:   http.Client{Timeout: c.Timeout},
		config: c,
	}
	return client, nil
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
		if err := validateChecksum(sha256); err != nil {
			return fmt.Errorf("failed to upload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPut, c.getURL(path), data,
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
		if strings.Contains(string(body), "Checksum mismatch") {
			return fmt.Errorf("failed to upload: %w", ErrChecksumMismatch)
		}
		return fmt.Errorf("failed to upload: status %s: body %s", res.Status, string(body))

	default:
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to upload: status %s: body %s", res.Status, string(body))
	}
}

// Delete the file at the specified path.
// Returns nil if the file was deleted successfully, or if the file did not exists.
func (c Client) Delete(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("failed to delete: %w", ErrEmptyPath)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodDelete, c.getURL(path), nil,
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

func (c Client) setHeaders(r *http.Request) {
	r.Header.Add("accept", "application/json")
	r.Header.Add("content-type", "application/json")
	r.Header.Add("AccessKey", c.config.StorageZone.Password)
}

func (c Client) getURL(path string) string {
	return fmt.Sprintf("https://%s/%s/%s",
		c.config.StorageZone.Endpoint,
		c.config.StorageZone.Name,
		path,
	)
}

func validateChecksum(sha256 string) error {
	if len(sha256) != 64 {
		return fmt.Errorf("failed to upload 1): %w", ErrInvalidChecksum)
	}

	if _, err := hex.DecodeString(sha256); err != nil {
		return fmt.Errorf("failed to upload 2): %w", ErrInvalidChecksum)
	}
	return nil
}
