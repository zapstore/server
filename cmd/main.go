package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	"github.com/zapstore/server/pkg/acl"
	"github.com/zapstore/server/pkg/blossom"
	blossomstore "github.com/zapstore/server/pkg/blossom/store"
	"github.com/zapstore/server/pkg/config"
	"github.com/zapstore/server/pkg/rate"
	"github.com/zapstore/server/pkg/relay"
	relaystore "github.com/zapstore/server/pkg/relay/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	config, err := config.Load()
	if err != nil {
		panic(err)
	}

	config.Print()
	return

	dataDir := filepath.Join(config.Sys.Dir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		panic(err)
	}

	rstore, err := relaystore.New(filepath.Join(dataDir, "relay.db"))
	if err != nil {
		panic(err)
	}
	defer rstore.Close()

	bstore, err := blossomstore.New(filepath.Join(dataDir, "blossom.db"))
	if err != nil {
		panic(err)
	}
	defer bstore.Close()

	limiter := rate.NewLimiter(config.Rate)
	logger := slog.Default()

	aclDir := filepath.Join(config.Sys.Dir, "acl")
	acl, err := acl.New(config.ACL, aclDir, logger)
	if err != nil {
		panic(err)
	}
	defer acl.Close()

	relay, err := relay.Setup(config.Relay, limiter, acl, rstore)
	if err != nil {
		panic(err)
	}

	blossom, err := blossom.Setup(config.Blossom, limiter, acl, bstore)
	if err != nil {
		panic(err)
	}

	exit := make(chan error, 2)
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		address := "localhost:" + config.Relay.Port
		if err := relay.StartAndServe(ctx, address); err != nil {
			exit <- err
		}
	}()

	go func() {
		defer wg.Done()
		address := "localhost:" + config.Blossom.Port
		if err := blossom.StartAndServe(ctx, address); err != nil {
			exit <- err
		}
	}()

	select {
	case <-ctx.Done():
		wg.Wait()
		return

	case err := <-exit:
		panic(err)
	}
}
