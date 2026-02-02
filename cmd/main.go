package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/zapstore/server/pkg/blossom"
	"github.com/zapstore/server/pkg/config"
	"github.com/zapstore/server/pkg/rate"
	"github.com/zapstore/server/pkg/relay"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	config, err := config.Load()
	if err != nil {
		panic(err)
	}

	limiter := rate.NewLimiter(config.Rate)

	relay, err := relay.Setup(config.Relay, limiter)
	if err != nil {
		panic(err)
	}

	blossom, err := blossom.Setup(config.Blossom, limiter)
	if err != nil {
		panic(err)
	}

	exit := make(chan error, 2)

	go func() {
		address := "localhost:" + config.Relay.Port
		if err := relay.StartAndServe(ctx, address); err != nil {
			exit <- err
		}
	}()

	go func() {
		address := "localhost:" + config.Blossom.Port
		if err := blossom.StartAndServe(ctx, address); err != nil {
			exit <- err
		}
	}()

	select {
	case <-ctx.Done():
		return

	case err := <-exit:
		panic(err)
	}
}
