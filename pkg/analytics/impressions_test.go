package analytics

import (
	"reflect"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	eventPkg "github.com/zapstore/server/pkg/events"
)

func TestDetermineType(t *testing.T) {
	tests := []struct {
		name   string
		filter nostr.Filter
		want   Type
	}{
		{
			name:   "stack filter",
			filter: nostr.Filter{Kinds: []int{eventPkg.KindAppSet}},
			want:   TypeStack,
		},
		{
			name:   "feed filter",
			filter: nostr.Filter{Kinds: []int{eventPkg.KindApp}},
			want:   TypeFeed,
		},
		{
			name:   "search filter",
			filter: nostr.Filter{Kinds: []int{eventPkg.KindApp}, Search: "signal"},
			want:   TypeSearch,
		},
		{
			name:   "detail filter",
			filter: nostr.Filter{Kinds: []int{eventPkg.KindApp}, Tags: nostr.TagMap{"d": {"com.example.app"}}},
			want:   TypeDetail,
		},
		{
			name:   "empty filter",
			filter: nostr.Filter{},
			want:   TypeUndetermined,
		},
		{
			name:   "undetermined filter",
			filter: nostr.Filter{Kinds: []int{0, 1}, Tags: nostr.TagMap{"p": {"5c50da132947fa3bf4759eb978d784db12baad1c3e5b6a575410aeb654639b4b"}}},
			want:   TypeUndetermined,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ImpressionType(test.filter); got != test.want {
				t.Errorf("determineType() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNewImpressions(t *testing.T) {
	day := today()

	tests := []struct {
		name    string
		id      string
		filters nostr.Filters
		events  []nostr.Event
		want    []Impression
	}{
		{
			name:    "feed impressions",
			id:      "app-req-1",
			filters: nostr.Filters{{Kinds: []int{eventPkg.KindApp}}},
			events: []nostr.Event{
				appEvent("com.example.app1", "pk1"),
				appEvent("com.example.app2", "pk2"),
				appEvent("com.example.app3", "pk3"),
			},
			want: []Impression{
				{AppID: "com.example.app1", AppPubkey: "pk1", Day: day, Source: SourceApp, Type: TypeFeed},
				{AppID: "com.example.app2", AppPubkey: "pk2", Day: day, Source: SourceApp, Type: TypeFeed},
				{AppID: "com.example.app3", AppPubkey: "pk3", Day: day, Source: SourceApp, Type: TypeFeed},
			},
		},
		{
			name:    "detail impressions for matching d tag",
			id:      "web-req-1",
			filters: nostr.Filters{{Kinds: []int{eventPkg.KindApp}, Tags: nostr.TagMap{"d": {"com.example.app1"}}}},
			events: []nostr.Event{
				appEvent("com.example.app1", "pubkey"),
				appEvent("com.example.app2", "pubkey"), // skipped since it doesn't match the filter
			},
			want: []Impression{
				{AppID: "com.example.app1", AppPubkey: "pubkey", Day: day, Source: SourceWeb, Type: TypeDetail},
			},
		},
		{
			name:    "search impressions skip empty d",
			id:      "app-req-2",
			filters: nostr.Filters{{Kinds: []int{eventPkg.KindApp}, Search: "signal"}},
			events: []nostr.Event{
				appEvent("com.example.app1", "pubkey"),
				appEvent("", "pubkey"),
			},
			want: []Impression{
				{AppID: "com.example.app1", AppPubkey: "pubkey", Day: day, Source: SourceApp, Type: TypeSearch},
			},
		},
		{
			name:    "stack impressions from app set",
			id:      "web-req-2",
			filters: nostr.Filters{{Kinds: []int{eventPkg.KindAppSet}}},
			events: []nostr.Event{
				appSetEvent("32267:pubkey:com.example.app1", "32267:pubkey:com.example.app2"),
				appEvent("com.example.app1", "pubkey"), // not considered because doesn't match the filter
			},
			want: []Impression{
				{AppID: "com.example.app1", AppPubkey: "pubkey", Day: day, Source: SourceWeb, Type: TypeStack},
				{AppID: "com.example.app2", AppPubkey: "pubkey", Day: day, Source: SourceWeb, Type: TypeStack},
			},
		},
		{
			name: "stack impressions from app set and app",
			id:   "web-req-2",
			filters: nostr.Filters{
				{Kinds: []int{eventPkg.KindAppSet}},
				{Kinds: []int{eventPkg.KindApp}, Tags: nostr.TagMap{"d": {"com.example.app3"}}},
			},
			events: []nostr.Event{
				appSetEvent("32267:pubkey:com.example.app1", "32267:pubkey:com.example.app2"),
				appEvent("com.example.app3", "PUBKEY"),
			},
			want: []Impression{
				{AppID: "com.example.app1", AppPubkey: "pubkey", Day: day, Source: SourceWeb, Type: TypeStack},
				{AppID: "com.example.app2", AppPubkey: "pubkey", Day: day, Source: SourceWeb, Type: TypeStack},
				{AppID: "com.example.app3", AppPubkey: "PUBKEY", Day: day, Source: SourceWeb, Type: TypeDetail},
			},
		},
		{
			name:    "undetermined filter skipped",
			id:      "app-req-3",
			filters: nostr.Filters{{}},
			events: []nostr.Event{
				appEvent("com.example.app1", "pubkey"),
			},
			want: []Impression{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NewImpressions("", test.id, test.filters, test.events)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("NewImpressions() = %v, want %v", got, test.want)
			}
		})
	}
}

func appEvent(appID string, pubkey string) nostr.Event {
	tags := nostr.Tags{}
	if appID != "" {
		tags = append(tags, nostr.Tag{"d", appID})
	}

	return nostr.Event{
		PubKey: pubkey,
		Kind:   eventPkg.KindApp,
		Tags:   tags,
	}
}

func appSetEvent(aTags ...string) nostr.Event {
	tags := nostr.Tags{}
	for _, aTag := range aTags {
		tags = append(tags, nostr.Tag{"a", aTag})
	}

	return nostr.Event{
		Kind: eventPkg.KindAppSet,
		Tags: tags,
	}
}
