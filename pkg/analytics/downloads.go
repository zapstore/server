package analytics

import (
	"net/http"

	"github.com/pippellia-btc/blossom"
)

// Download of a blossom blob.
type Download struct {
	Hash        blossom.Hash
	Day         string // formatted as "YYYY-MM-DD"
	Source      Source
	CountryCode string // ISO 2 letter code
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
func NewDownload(country string, header http.Header, hash blossom.Hash) Download {
	return Download{
		Hash:        hash,
		Day:         today(),
		Source:      DownloadSource(header),
		CountryCode: country,
	}
}
