package sqlite

import (
	_ "embed"

	sqlite "github.com/vertex-lab/nostr-sqlite"
)

//go:embed schema.sql
var schema string

func NewStore(c Config) (*sqlite.Store, error) {
	return sqlite.New(
		c.Path,
		sqlite.WithAdditionalSchema(schema),
		sqlite.WithoutEventPolicy(),  // events have been validated by the relay
		sqlite.WithoutFilterPolicy(), // filters have been validated by the relay
	)
}
