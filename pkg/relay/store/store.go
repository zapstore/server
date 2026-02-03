// The store package is responsible for storing nostr events in a sqlite database.
// It exposes a [New] function to create a new sqlite store with the given config.
package store

import (
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	sqlite "github.com/vertex-lab/nostr-sqlite"
	"github.com/zapstore/server/pkg/events"
)

//go:embed schema.sql
var schema string

func New(c Config) (*sqlite.Store, error) {
	return sqlite.New(
		c.Path,
		sqlite.WithQueryBuilder(queryBuilder),
		sqlite.WithAdditionalSchema(schema),
		sqlite.WithoutEventPolicy(),  // events have been validated by the relay
		sqlite.WithoutFilterPolicy(), // filters have been validated by the relay
	)
}

// queryBuilder handles FTS search for apps when there's exactly one app search filter.
// Otherwise, it delegates to the default query builder.
func queryBuilder(filters ...nostr.Filter) ([]sqlite.Query, error) {
	if searchesIn(filters) == 0 {
		return sqlite.DefaultQueryBuilder(filters...)
	}

	if len(filters) > 1 {
		// We don't support multiple filters when one is using NIP-50 search because the order
		// of the result events will inevitably be ambiguous.
		return nil, errors.New("we support only one filter per REQ when using NIP-50 search")
	}

	search := filters[0]
	if !slices.Equal(search.Kinds, []int{events.KindApp}) {
		return nil, fmt.Errorf("We only support NIP-50 search for kind %d", events.KindApp)
	}
	return appSearchQuery(search)
}

// searchesIn counts the number of filters with a non-empty search term.
func searchesIn(filters nostr.Filters) int {
	count := 0
	for _, filter := range filters {
		if filter.Search != "" {
			count++
		}
	}
	return count
}

// appSearchQuery builds an FTS query for searching apps.
// Results are ordered by BM25 relevance with custom weights.
func appSearchQuery(filter nostr.Filter) ([]sqlite.Query, error) {
	if len(filter.Search) < 3 {
		return nil, fmt.Errorf("search term must be at least 3 characters") // because of the trigram tokenizer

	}

	filter.Search = escapeFTS5(filter.Search)
	conditions, args := appSearchSql(filter)

	query := `SELECT e.id, e.pubkey, e.created_at, e.kind, e.tags, e.content, e.sig
		FROM events e
		JOIN apps_fts fts ON e.id = fts.id
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY bm25(apps_fts, 0, 20, 5, 1)
		LIMIT ?`

	args = append(args, filter.Limit)
	return []sqlite.Query{{SQL: query, Args: args}}, nil
}

// appSearchSql converts a nostr.Filter into SQL conditions and arguments.
// Tags are filtered using subqueries to avoid JOIN and GROUP BY,
// which would break bm25() ranking.
func appSearchSql(filter nostr.Filter) (conditions []string, args []any) {
	conditions = []string{"apps_fts MATCH ?"}
	args = []any{filter.Search}

	if len(filter.IDs) > 0 {
		conditions = append(conditions, "e.id"+inClause(len(filter.IDs)))
		for _, id := range filter.IDs {
			args = append(args, id)
		}
	}

	if len(filter.Authors) > 0 {
		conditions = append(conditions, "e.pubkey"+inClause(len(filter.Authors)))
		for _, pk := range filter.Authors {
			args = append(args, pk)
		}
	}

	if filter.Since != nil {
		conditions = append(conditions, "e.created_at >= ?")
		args = append(args, filter.Since.Time().Unix())
	}

	if filter.Until != nil {
		conditions = append(conditions, "e.created_at <= ?")
		args = append(args, filter.Until.Time().Unix())
	}

	for key, vals := range filter.Tags {
		if len(vals) == 0 {
			continue
		}
		conditions = append(conditions,
			"EXISTS (SELECT 1 FROM tags WHERE event_id = e.id AND key = ? AND value"+inClause(len(vals))+")")
		args = append(args, key)
		for _, v := range vals {
			args = append(args, v)
		}
	}
	return conditions, args
}

// escapeFTS5 prepares a search term for SQLite FTS5
func escapeFTS5(term string) string {
	term = strings.ReplaceAll(term, `"`, `""`)
	return `"` + term + `"`
}

// inClause returns " = ?" for a single value or " IN (?, ?, ...)" for multiple values.
func inClause(n int) string {
	if n == 1 {
		return " = ?"
	}
	return " IN (?" + strings.Repeat(",?", n-1) + ")"
}
