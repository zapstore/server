package analytics

import (
	"slices"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	eventPkg "github.com/zapstore/server/pkg/events"
)

// Impression of an app
type Impression struct {
	AppID  string
	Day    Day // formatted as "YYYY-MM-DD"
	Source Source
	Type   Type
}

// Day represents the day the impression was made, formatted as "YYYY-MM-DD".
type Day string

// Today returns the current day formatted as "YYYY-MM-DD".
func Today() Day {
	return Day(time.Now().UTC().Format("2006-01-02"))
}

// Source represents where the impression was made.
type Source string

const (
	SourceApp     Source = "app"
	SourceWeb     Source = "web"
	SourceUnknown Source = "unknown"
)

// ImpressionSource from the REQ id.
func ImpressionSource(id string) Source {
	switch {
	case strings.HasPrefix(id, "app-"):
		return SourceApp
	case strings.HasPrefix(id, "web-"):
		return SourceWeb
	default:
		return SourceUnknown
	}
}

// Type represents the type of impression, which is determined by the REQ.
// For example, a "detail" impression is made when the client requests kind = 32267 (app), 'd' = <app_id>.
type Type string

const (
	TypeFeed         Type = "feed"
	TypeDetail       Type = "detail"
	TypeSearch       Type = "search"
	TypeStack        Type = "stack"
	TypeUndetermined Type = "undetermined"
)

// ImpressionType returns the filter Type.
func ImpressionType(filter nostr.Filter) Type {
	hasApp := slices.Contains(filter.Kinds, eventPkg.KindApp)
	hasStack := slices.Contains(filter.Kinds, eventPkg.KindAppSet)

	if hasStack && !hasApp {
		return TypeStack
	}

	if hasApp {
		dTags := len(filter.Tags["d"])
		switch {
		case dTags == 0 && filter.Search == "":
			return TypeFeed

		case dTags == 0 && filter.Search != "":
			return TypeSearch

		case dTags > 0:
			return TypeDetail
		}
	}
	return TypeUndetermined
}

// NewImpressions creates the impressions from the REQ id, filters and returned events.
func NewImpressions(id string, filters nostr.Filters, events []nostr.Event) []Impression {
	day := Today()
	source := ImpressionSource(id)
	impressions := make([]Impression, 0, len(events))

	for _, f := range filters {
		typ := ImpressionType(f)
		if typ == TypeUndetermined {
			continue
		}

		for _, event := range matchingEvents(f, events) {
			switch {
			case typ == TypeStack && event.Kind == eventPkg.KindAppSet:
				// One impression for all apps inside the app set
				for _, appID := range eventPkg.ExtractAppsFromSet(event) {
					impressions = append(impressions, Impression{
						AppID:  appID,
						Day:    day,
						Source: source,
						Type:   typ,
					})
				}

			default:
				// One impression for the app
				appID := event.Tags.GetD()
				if appID == "" {
					continue
				}

				impressions = append(impressions, Impression{
					AppID:  appID,
					Day:    day,
					Source: source,
					Type:   typ,
				})
			}
		}
	}
	return impressions
}

// MatchingEvents returns the events that match the given filter.
func matchingEvents(f nostr.Filter, events []nostr.Event) []nostr.Event {
	var matched []nostr.Event
	for _, e := range events {
		if f.Matches(&e) {
			matched = append(matched, e)
		}
	}
	return matched
}
