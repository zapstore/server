// The blossom package is responsible for setting up the blossom server.
// It exposes a [Setup] function to create a new relay with the given config.
package blossom

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/pippellia-btc/blossom"
	"github.com/pippellia-btc/blossy"
	"github.com/zapstore/server/pkg/acl"
	"github.com/zapstore/server/pkg/blossom/bunny"
	"github.com/zapstore/server/pkg/rate"
)

func Setup(config Config, limiter rate.Limiter, acl *acl.Controller) (*blossy.Server, error) {
	bunny, err := bunny.NewClient(config.Bunny)
	if err != nil {
		return nil, fmt.Errorf("failed to setup bunny client: %w", err)
	}

	blossom, err := blossy.NewServer(
		blossy.WithBaseURL("https://" + config.Domain),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to setup blossom server: %w", err)
	}

	blossom.Reject.Check.Append(
		RateCheckIP(limiter),
	)

	blossom.Reject.Download.Append(
		RateDownloadIP(limiter),
	)

	blossom.Reject.Upload.Append(
		RateUploadIP(limiter),
		MissingHints(),
		MediaNotAllowed(config.AllowedMedia),
		BlobIsBlocked(acl),
	)

	blossom.On.Check = Check(bunny)
	blossom.On.Download = Download(bunny)
	blossom.On.Upload = Upload(bunny)
	return blossom, nil
}

func Check(client bunny.Client) func(r blossy.Request, hash blossom.Hash, ext string) (blossy.MetaDelivery, *blossom.Error) {
	return func(r blossy.Request, hash blossom.Hash, ext string) (blossy.MetaDelivery, *blossom.Error) {
		if ext == "" {
			// TODO: find the extention from the hash using the stats database
			return nil, blossom.ErrBadRequest("extension is required")
		}

		path := client.CDN() + "/" + hash.Hex() + "." + ext
		return blossy.Redirect(path, http.StatusTemporaryRedirect), nil
	}
}

func Download(client bunny.Client) func(r blossy.Request, hash blossom.Hash, ext string) (blossy.BlobDelivery, *blossom.Error) {
	return func(r blossy.Request, hash blossom.Hash, ext string) (blossy.BlobDelivery, *blossom.Error) {
		if ext == "" {
			// TODO: find the extention from the hash using the stats database
			return nil, blossom.ErrBadRequest("extension is required")
		}

		path := client.CDN() + "/" + hash.Hex() + "." + ext
		return blossy.Redirect(path, http.StatusTemporaryRedirect), nil
	}
}

func Upload(client bunny.Client) func(r blossy.Request, hints blossy.UploadHints, data io.Reader) (blossom.BlobDescriptor, *blossom.Error) {
	return func(r blossy.Request, hints blossy.UploadHints, data io.Reader) (blossom.BlobDescriptor, *blossom.Error) {

		name := hints.Hash.Hex() + "." + blossom.ExtFromType(hints.Type)
		sha256 := hints.Hash.Hex()

		if err := client.Upload(r.Context(), data, name, sha256); err != nil {
			if errors.Is(err, bunny.ErrChecksumMismatch) {
				// TODO: punish the client for providing a bad hash
				return blossom.BlobDescriptor{}, blossom.ErrBadRequest("checksum mismatch")
			}
			slog.Error("blossom: failed to upload blob", "error", err, "name", name)
			return blossom.BlobDescriptor{}, blossom.ErrInternal("internal error, please contact the Zapstore team.")
		}

		mime, size, err := client.Check(r.Context(), name)
		if err != nil {
			return blossom.BlobDescriptor{}, blossom.ErrInternal("internal error, please contact the Zapstore team.")
		}

		if size != hints.Size {
			// TODO: punish the client for providing a bad size
		}
		if mime != hints.Type {
			// TODO: punish the client for providing a bad mime
		}

		return blossom.BlobDescriptor{
			Hash:     hints.Hash,
			Type:     mime,
			Size:     size,
			Uploaded: time.Now().UTC().Unix(),
		}, nil
	}
}

// UploadCost estimates the cost in tokens for an upload based on the clients hints.
func UploadCost(hints blossy.UploadHints) float64 {
	if hints.Size <= 0 {
		// default cost is very high to punish clients that don't provide the size (-1)
		// or provided a clearly false size of 0.
		return 100
	}
	// The cost is roughly 1 token per 10 MB.
	return float64(hints.Size) / 10_000_000
}

func MissingHints() func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
	return func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
		if hints.Hash.Hex() == "" {
			return blossom.ErrBadRequest("'Content-Digest' header is required")
		}
		if hints.Type == "" {
			return blossom.ErrBadRequest("'Content-Type' header is required")
		}
		if hints.Size == -1 {
			return blossom.ErrBadRequest("'Content-Length' header is required")
		}
		return nil
	}
}

func MediaNotAllowed(allowed []string) func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
	return func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
		if !slices.Contains(allowed, hints.Type) {
			reason := fmt.Sprintf("content type is not in the allowed list: %s", strings.Join(allowed, ", "))
			return blossom.ErrUnsupportedMedia(reason)
		}
		return nil
	}
}

func BlobIsBlocked(acl *acl.Controller) func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
	return func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
		if acl.IsBlobBlocked(hints.Hash) {
			return blossom.ErrForbidden("this blob is blocked")
		}
		return nil
	}
}

func RateUploadIP(limiter rate.Limiter) func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
	return func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
		cost := UploadCost(hints)
		return RateLimitIP(limiter, r.IP(), cost)
	}
}

func RateDownloadIP(limiter rate.Limiter) func(r blossy.Request, hash blossom.Hash, ext string) *blossom.Error {
	return func(r blossy.Request, hash blossom.Hash, ext string) *blossom.Error {
		cost := 10.0
		return RateLimitIP(limiter, r.IP(), cost)
	}
}

func RateCheckIP(limiter rate.Limiter) func(r blossy.Request, hash blossom.Hash, ext string) *blossom.Error {
	return func(r blossy.Request, hash blossom.Hash, ext string) *blossom.Error {
		cost := 1.0
		return RateLimitIP(limiter, r.IP(), cost)
	}
}

// RateLimitIP rejects an IP if it's exceeding the rate limit.
func RateLimitIP(limiter rate.Limiter, ip blossy.IP, cost float64) *blossom.Error {
	reject, err := limiter.Reject(ip.Group(), cost)
	if err != nil {
		// fail open policy; if the rate limiter fails, we allow the request
		slog.Error("blossom: rate limiter failed", "error", err)
		return nil
	}
	if reject {
		return blossom.ErrTooMany("rate-limited: slow down chief")
	}
	return nil
}
