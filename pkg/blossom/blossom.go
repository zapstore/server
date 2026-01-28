// The blossom package is responsible for setting up the blossom server.
// It exposes a [Setup] function to create a new relay with the given config.
package blossom

import (
	"fmt"

	"github.com/pippellia-btc/blossy"
)

func Setup(config Config) (*blossy.Server, error) {
	blossom, err := blossy.NewServer(
		blossy.WithBaseURL(config.Domain),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to setup blossom server: %w", err)
	}

	return blossom, nil
}
