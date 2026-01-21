package main

import (
	"context"
	"os"
	"os/signal"

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

	if err := relay.StartAndServe(ctx, config.Relay.Port); err != nil {
		panic(err)
	}
}
