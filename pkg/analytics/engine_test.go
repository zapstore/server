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
)

type dbRow struct {
	Impression
	Count int
}

func TestFlushImpressions(t *testing.T) {
	tests := []struct {
		name        string
		impressions []Impression
		want        []dbRow
	}{
		{
			name: "single impression",
			impressions: []Impression{
				{AppID: "com.example.app1", Day: Day("2024-01-01"), Source: SourceApp, Type: TypeFeed},
			},
			want: []dbRow{
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
			want: []dbRow{
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
			want: []dbRow{
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

			for _, imp := range test.impressions {
				engine.impressions <- imp
			}
			engine.Drain()

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

func fetchImpressions(db *sql.DB) ([]dbRow, error) {
	rows, err := db.Query(`SELECT app_id, day, source, type, count FROM impressions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []dbRow
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

		results = append(results, dbRow{
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

func normalizeDay(day string) string {
	if len(day) >= 10 {
		return day[:10]
	}
	return strings.TrimSpace(day)
}

func sortRows(rows []dbRow) {
	slices.SortFunc(rows, func(r1, r2 dbRow) int {
		return cmp.Compare(r1.AppID, r2.AppID)
	})
}
