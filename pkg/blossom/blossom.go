// The blossom package is responsible for setting up the blossom server.
// It exposes a [Setup] function to create a new relay with the given config.
package blossom

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/pippellia-btc/blossom"
	"github.com/pippellia-btc/blossy"
	"github.com/pippellia-btc/rate"
)

var (
	ErrTypeNotAllowed = &blossom.Error{Code: http.StatusUnsupportedMediaType, Reason: "content type not allowed"}
	ErrRateLimited    = &blossom.Error{Code: http.StatusTooManyRequests, Reason: "rate-limited: slow down chief"}
	ErrInternal       = &blossom.Error{Code: http.StatusInternalServerError, Reason: "internal error, please contact the Zapstore team."}
)

func Setup(config Config, limiter *rate.Limiter[string]) (*blossy.Server, error) {
	blossom, err := blossy.NewServer(
		blossy.WithBaseURL(config.Domain),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to setup blossom server: %w", err)
	}

	blossom.Reject.Upload.Append(
		RateUploadIP(limiter),
		TypeNotAllowed(config.AllowedContentTypes),
		BlobIsBlocked(config.BlockedBlobs),
	)

	return blossom, nil
}

func RateUploadIP(limiter *rate.Limiter[string]) func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
	return func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
		cost := UploadCost(hints)
		return RateLimitIP(limiter, r.IP(), cost)
	}
}

// UploadCost estimates the cost in tokens for an upload based on the clients hints.
func UploadCost(hints blossy.UploadHints) float64 {
	if hints.Size == -1 {
		return 100
	}
	// The cost is roughly 1 token per 10 MB.
	return float64(hints.Size) / 10_000_000
}

func TypeNotAllowed(allowed []string) func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
	return func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
		if !slices.Contains(allowed, hints.Type) {
			reason := fmt.Sprintf("content type is not in the allowed list: %v", allowed)
			return &blossom.Error{Code: http.StatusUnsupportedMediaType, Reason: reason}
		}
		return nil
	}
}

func BlobIsBlocked(blocked []string) func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
	return func(r blossy.Request, hints blossy.UploadHints) *blossom.Error {
		if slices.Contains(blocked, hints.Hash.Hex()) {
			return &blossom.Error{Code: http.StatusForbidden, Reason: "this blob is blocked"}
		}
		return nil
	}
}

// RateLimitIP rejects an IP if it's exceeding the rate limit.
func RateLimitIP(limiter *rate.Limiter[string], ip blossy.IP, cost float64) *blossom.Error {
	reject, err := limiter.Reject(ip.Group(), cost)
	if err != nil {
		// fail open policy; if the rate limiter fails, we allow the request
		slog.Error("blossom: rate limiter failed", "error", err)
		return nil
	}
	if reject {
		return ErrRateLimited
	}
	return nil
}
