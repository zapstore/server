package analytics

import (
	"fmt"
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
			if got := determineType(test.filter); got != test.want {
				t.Errorf("determineType() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNewImpressions(t *testing.T) {
	day := Today()

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
				appEvent("com.example.app1"),
				appEvent("com.example.app2"),
				appEvent("com.example.app3"),
			},
			want: []Impression{
				{AppID: "com.example.app1", Day: day, Source: SourceApp, Type: TypeFeed},
				{AppID: "com.example.app2", Day: day, Source: SourceApp, Type: TypeFeed},
				{AppID: "com.example.app3", Day: day, Source: SourceApp, Type: TypeFeed},
			},
		},
		{
			name:    "detail impressions for matching d tag",
			id:      "web-req-1",
			filters: nostr.Filters{{Kinds: []int{eventPkg.KindApp}, Tags: nostr.TagMap{"d": {"com.example.app1"}}}},
			events: []nostr.Event{
				appEvent("com.example.app1"),
				appEvent("com.example.app2"), // skipped since it doesn't match the filter
			},
			want: []Impression{
				{AppID: "com.example.app1", Day: day, Source: SourceWeb, Type: TypeDetail},
			},
		},
		{
			name:    "search impressions skip empty d",
			id:      "app-req-2",
			filters: nostr.Filters{{Kinds: []int{eventPkg.KindApp}, Search: "signal"}},
			events: []nostr.Event{
				appEvent("com.example.app1"),
				appEvent(""),
			},
			want: []Impression{
				{AppID: "com.example.app1", Day: day, Source: SourceApp, Type: TypeSearch},
			},
		},
		{
			name:    "stack impressions from app set",
			id:      "web-req-2",
			filters: nostr.Filters{{Kinds: []int{eventPkg.KindAppSet}}},
			events: []nostr.Event{
				appSetEvent("com.example.app1", "com.example.app2"),
				appEvent("com.example.app1"),
			},
			want: []Impression{
				{AppID: "com.example.app1", Day: day, Source: SourceWeb, Type: TypeStack},
				{AppID: "com.example.app2", Day: day, Source: SourceWeb, Type: TypeStack},
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
				appSetEvent("com.example.app1", "com.example.app2"),
				appEvent("com.example.app3"),
			},
			want: []Impression{
				{AppID: "com.example.app1", Day: day, Source: SourceWeb, Type: TypeStack},
				{AppID: "com.example.app2", Day: day, Source: SourceWeb, Type: TypeStack},
				{AppID: "com.example.app3", Day: day, Source: SourceWeb, Type: TypeDetail},
			},
		},
		{
			name:    "undetermined filter skipped",
			id:      "app-req-3",
			filters: nostr.Filters{{}},
			events: []nostr.Event{
				appEvent("com.example.app1"),
			},
			want: []Impression{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NewImpressions(test.id, test.filters, test.events)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("NewImpressions() = %v, want %v", got, test.want)
			}
		})
	}
}

func appEvent(appID string) nostr.Event {
	tags := nostr.Tags{}
	if appID != "" {
		tags = append(tags, nostr.Tag{"d", appID})
	}

	return nostr.Event{
		Kind: eventPkg.KindApp,
		Tags: tags,
	}
}

func appSetEvent(appIDs ...string) nostr.Event {
	tags := nostr.Tags{}
	for _, appID := range appIDs {
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("32267:pubkey:%s", appID)})
	}

	return nostr.Event{
		Kind: eventPkg.KindAppSet,
		Tags: tags,
	}
}
