package store

import (
	_ "embed"
	"slices"

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
	if isAppSearch(filters...) {
		return appSearchQuery(filters[0])
	}
	return sqlite.DefaultQueryBuilder(filters...)
}

// isAppSearch returns true if there is exactly one filter and it is a search query for KindApp only.
//
// Note:
//  1. We don't support multiple filters because they will inevitably change the order of the results,
//     which will render useless our rank based search.
//  2. We only support KindApp because that's the only kind we index with FTS.
func isAppSearch(filters ...nostr.Filter) bool {
	return len(filters) == 1 && filters[0].Search != "" && slices.Equal(filters[0].Kinds, []int{events.KindApp})
}

// appSearchQuery builds an FTS query for searching apps.
// Results are ordered by FTS rank to preserve search relevance.
func appSearchQuery(filter nostr.Filter) ([]sqlite.Query, error) {
	query := `SELECT e.id, e.pubkey, e.created_at, e.kind, e.tags, e.content, e.sig
		FROM events e
		INNER JOIN apps_fts fts ON e.id = fts.id
		WHERE apps_fts MATCH ?
		ORDER BY rank`

	args := []any{filter.Search}

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	return []sqlite.Query{{SQL: query, Args: args}}, nil
}
