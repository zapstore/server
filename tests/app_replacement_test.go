package tests

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

var ctx = context.Background()

var AppTemplate = nostr.Event{
	Kind: 32267,
	Tags: nostr.Tags{
		{"name", "test-app-replacement"},
		{"d", "test.app.replacement"},
		{"url", "https://zapstore.dev"},
		{"f", "android-arm64-v8a"},
		{"t", "android"},
		{"t", "apk"},
	},
}

func TestAppReplacement(t *testing.T) {
	t.Skip("it's a manual test")
	sk := nostr.GeneratePrivateKey()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		t.Fatal(err)
	}

	app := AppTemplate
	app.CreatedAt = nostr.Timestamp(1000)
	app.Tags = append(app.Tags, nostr.Tag{"a", "30063:" + pk + ":test.app.replacement@1.0.0"})
	if err := app.Sign(sk); err != nil {
		t.Fatal(err)
	}

	relay := nostr.NewRelay(ctx, "http://localhost:3334")
	if err := relay.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	if err := relay.Publish(ctx, app); err != nil {
		t.Fatal(err)
	}

	replacement := app
	replacement.CreatedAt = nostr.Timestamp(1001)
	if err := replacement.Sign(sk); err != nil {
		t.Fatal(err)
	}

	if err := relay.Publish(ctx, replacement); err != nil {
		t.Fatal(err)
	}

	// manually check if the replacement was successful
	// SELECT * FROM apps WHERE kind = 32267 AND created_at = 1000;
	// SELECT * FROM apps WHERE kind = 32267 AND created_at = 1001;
}
