package analytics

import (
	"cmp"
	"database/sql"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pippellia-btc/blossom"
)

type impressionRow struct {
	Impression
	Count int
}

func TestFlushImpressions(t *testing.T) {
	tests := []struct {
		name        string
		impressions []Impression
		want        []impressionRow
	}{
		{
			name: "single impression",
			impressions: []Impression{
				{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
			},
			want: []impressionRow{
				{
					Impression: Impression{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
					Count:      1,
				},
			},
		},
		{
			name: "duplicate impressions coalesced",
			impressions: []Impression{
				{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceWeb, Type: TypeDetail},
				{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceWeb, Type: TypeDetail},
				{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceWeb, Type: TypeDetail},
			},
			want: []impressionRow{
				{
					Impression: Impression{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceWeb, Type: TypeDetail},
					Count:      3,
				},
			},
		},
		{
			name: "multiple impressions across keys",
			impressions: []Impression{
				{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
				{AppID: "com.example.app2", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
				{AppID: "com.example.app1", Day: Day("2024-01-02"), Source: SourceApp, Type: TypeSearch}, // different
				{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
				{AppID: "com.example.app2", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
			},
			want: []impressionRow{
				{
					Impression: Impression{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
					Count:      2,
				},
				{
					Impression: Impression{AppID: "com.example.app2", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
					Count:      2,
				},
				{
					Impression: Impression{AppID: "com.example.app1", Day: Day("2024-01-02"), Source: SourceApp, Type: TypeSearch},
					Count:      1,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, err := NewEngine(
				NewConfig(),
				":memory:",
				slog.Default(),
			)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			defer engine.Close()

			for _, imp := range test.impressions {
				engine.impressions <- imp
			}
			engine.drain()

			if err := engine.flushImpressions(); err != nil {
				t.Fatalf("flushImpressions: %v", err)
			}

			got, err := fetchImpressions(engine.db)
			if err != nil {
				t.Fatalf("fetchImpressions: %v", err)
			}

			sortRows(got)
			sortRows(test.want)

			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("rows mismatch\n got: %#v\nwant: %#v", got, test.want)
			}
		})
	}
}

type downloadRow struct {
	Download
	Count int
}

func TestFlushDownloads(t *testing.T) {
	h1 := blossom.ComputeHash([]byte("anything"))
	h2 := blossom.ComputeHash([]byte("anywhere"))

	tests := []struct {
		name      string
		downloads []Download
		want      []downloadRow
	}{
		{
			name: "single download",
			downloads: []Download{
				{Hash: h1, Day: Day("2024-01-01"), Source: SourceApp},
			},
			want: []downloadRow{
				{
					Download: Download{Hash: h1, Day: Day("2024-01-01"), Source: SourceApp},
					Count:    1,
				},
			},
		},
		{
			name: "duplicate downloads coalesced",
			downloads: []Download{
				{Hash: h1, Day: Day("2024-01-01"), Source: SourceWeb},
				{Hash: h1, Day: Day("2024-01-01"), Source: SourceWeb},
				{Hash: h1, Day: Day("2024-01-01"), Source: SourceWeb},
			},
			want: []downloadRow{
				{
					Download: Download{Hash: h1, Day: Day("2024-01-01"), Source: SourceWeb},
					Count:    3,
				},
			},
		},
		{
			name: "multiple downloads across keys",
			downloads: []Download{
				{Hash: h1, Day: Day("2024-01-01"), Source: SourceApp},
				{Hash: h2, Day: Day("2024-01-01"), Source: SourceApp},
				{Hash: h1, Day: Day("2024-01-01"), Source: SourceApp},
				{Hash: h1, Day: Day("2024-01-01"), Source: SourceUnknown},
				{Hash: h2, Day: Day("2024-01-01"), Source: SourceApp},
			},
			want: []downloadRow{
				{
					Download: Download{Hash: h1, Day: Day("2024-01-01"), Source: SourceApp},
					Count:    2,
				},
				{
					Download: Download{Hash: h2, Day: Day("2024-01-01"), Source: SourceApp},
					Count:    2,
				},
				{
					Download: Download{Hash: h1, Day: Day("2024-01-01"), Source: SourceUnknown},
					Count:    1,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, err := NewEngine(
				NewConfig(),
				":memory:",
				slog.Default(),
			)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			defer engine.Close()

			for _, dl := range test.downloads {
				engine.downloads <- dl
			}
			engine.drain()

			if err := engine.flushDownloads(); err != nil {
				t.Fatalf("flushDownloads: %v", err)
			}

			got, err := fetchDownloads(engine.db)
			if err != nil {
				t.Fatalf("fetchDownloads: %v", err)
			}

			sortDownloadRows(got)
			sortDownloadRows(test.want)

			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("rows mismatch\n got: %v\nwant: %v", got, test.want)
			}
		})
	}
}

func fetchImpressions(db *sql.DB) ([]impressionRow, error) {
	rows, err := db.Query(`SELECT app_id, day, source, type, count FROM impressions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []impressionRow
	for rows.Next() {
		var (
			appID  string
			day    string
			source string
			typ    string
			count  int
		)
		if err := rows.Scan(&appID, &day, &source, &typ, &count); err != nil {
			return nil, fmt.Errorf("scan impressions: %v", err)
		}

		results = append(results, impressionRow{
			Impression: Impression{
				AppID:  appID,
				Day:    Day(normalizeDay(day)),
				Source: Source(source),
				Type:   Type(typ),
			},
			Count: count,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %v", err)
	}
	return results, nil
}

func fetchDownloads(db *sql.DB) ([]downloadRow, error) {
	rows, err := db.Query(`SELECT hash, day, source, count FROM downloads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []downloadRow
	for rows.Next() {
		var (
			hash   blossom.Hash
			day    string
			source string
			count  int
		)
		if err := rows.Scan(&hash, &day, &source, &count); err != nil {
			return nil, fmt.Errorf("scan downloads: %v", err)
		}

		results = append(results, downloadRow{
			Download: Download{
				Hash:   hash,
				Day:    Day(normalizeDay(day)),
				Source: Source(source),
			},
			Count: count,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %v", err)
	}
	return results, nil
}

func normalizeDay(day string) string {
	if len(day) >= 10 {
		return day[:10]
	}
	return strings.TrimSpace(day)
}

func sortRows(rows []impressionRow) {
	slices.SortFunc(rows, func(r1, r2 impressionRow) int {
		if c := cmp.Compare(r1.AppID, r2.AppID); c != 0 {
			return c
		}
		if c := cmp.Compare(string(r1.Day), string(r2.Day)); c != 0 {
			return c
		}
		if c := cmp.Compare(string(r1.Source), string(r2.Source)); c != 0 {
			return c
		}
		return cmp.Compare(string(r1.Type), string(r2.Type))
	})
}

func sortDownloadRows(rows []downloadRow) {
	slices.SortFunc(rows, func(r1, r2 downloadRow) int {
		if c := cmp.Compare(r1.Hash.Hex(), r2.Hash.Hex()); c != 0 {
			return c
		}
		if c := cmp.Compare(string(r1.Day), string(r2.Day)); c != 0 {
			return c
		}
		return cmp.Compare(string(r1.Source), string(r2.Source))
	})
}
