package analytics

import (
	"net/http"

	"github.com/pippellia-btc/blossom"
	"github.com/pippellia-btc/blossy"
)

// Download of a blossom blob.
type Download struct {
	Hash   blossom.Hash
	Day    Day
	Source Source
}

// DownloadSource returns the source of the download based on the request headers.
func DownloadSource(h http.Header) Source {
	zc := h.Get("X-Zapstore-Client")
	switch zc {
	case "app":
		return SourceApp
	case "web":
		return SourceWeb
	default:
		return SourceUnknown
	}
}

// NewDownload creates a new download record.
func NewDownload(r blossy.Request, hash blossom.Hash) Download {
	return Download{
		Hash:   hash,
		Day:    Today(),
		Source: DownloadSource(r.Raw().Header),
	}
}
