package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/zapstore/server/pkg/acl"
	"github.com/zapstore/server/pkg/analytics"
	"github.com/zapstore/server/pkg/blossom"
	blobstore "github.com/zapstore/server/pkg/blossom/store"
	"github.com/zapstore/server/pkg/config"
	"github.com/zapstore/server/pkg/rate"
	"github.com/zapstore/server/pkg/relay"
	eventstore "github.com/zapstore/server/pkg/relay/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	config, err := config.Load()
	if err != nil {
		panic(err)
	}

	if err := config.Validate(); err != nil {
		panic(err)
	}

	logger := slog.Default()
	logger.Info("-------------------server startup-------------------")
	defer logger.Info("-------------------server shutdown-------------------")

	// Step 1.
	// Initialize directory and databases
	dataDir := filepath.Join(config.Sys.Dir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		panic(err)
	}

	rstore, err := eventstore.New(filepath.Join(dataDir, "relay.db"))
	if err != nil {
		panic(err)
	}
	defer rstore.Close()

	bstore, err := blobstore.New(filepath.Join(dataDir, "blossom.db"))
	if err != nil {
		panic(err)
	}
	defer bstore.Close()

	path := filepath.Join(dataDir, "analytics.db")
	analytics, err := analytics.NewEngine(config.Analytics, path, logger)
	if err != nil {
		panic(err)
	}
	defer analytics.Close()

	// Step 2.
	// Initialize rate limiter and ACL
	limiter := rate.NewLimiter(config.Limiter)
	aclDir := filepath.Join(config.Sys.Dir, "acl")

	acl, err := acl.New(config.ACL, aclDir, logger)
	if err != nil {
		panic(err)
	}
	defer acl.Close()

	// Step 3.
	// Setup relay and blossom by passing dependencies
	relay, err := relay.Setup(
		config.Relay,
		limiter,
		acl,
		rstore,
		analytics,
	)
	if err != nil {
		panic(err)
	}

	blossom, err := blossom.Setup(
		config.Blossom,
		limiter,
		acl,
		bstore,
		analytics,
	)
	if err != nil {
		panic(err)
	}

	// Step 4.
	// Run everything
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
