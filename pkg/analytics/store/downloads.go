package store

import (
	"context"
	"fmt"
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

// DownloadCount is a Download paired with its occurrence count.
type DownloadCount struct {
	Download
	Count int
}

// DownloadSource returns the Source derived from the request headers.
func DownloadSource(h http.Header) Source {
	switch h.Get("X-Zapstore-Client") {
	case "app":
		return SourceApp
	case "web":
		return SourceWeb
	default:
		return SourceUnknown
	}
}

// NewDownload creates a new Download record from the given request headers and hash.
func NewDownload(country string, header http.Header, hash blossom.Hash) Download {
	return Download{
		Hash:        hash,
		Day:         Today(),
		Source:      DownloadSource(header),
		CountryCode: country,
	}
}

// SaveDownloads writes the given batch of counted downloads to the database.
// On conflict it increments the existing count. An empty batch is a no-op.
func (s *Store) SaveDownloads(ctx context.Context, batch []DownloadCount) error {
	if len(batch) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO downloads (hash, day, source, country_code, count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(hash, day, source, country_code)
		DO UPDATE SET count = downloads.count + excluded.count
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, download := range batch {
		if _, err := stmt.ExecContext(
			ctx,
			download.Hash,
			download.Day,
			string(download.Source),
			download.CountryCode,
			download.Count,
		); err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
