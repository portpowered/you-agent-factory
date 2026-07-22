package responseeventstore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseeventstore"
)

var canonicalResponseEventFixtures = []string{
	"text_delta",
	"message_snapshot",
	"tool_lifecycle",
	"retry",
	"final_only_message",
	"usage",
	"stream_gap",
}

func TestSerializedEventSize_MatchesCanonicalFixtureEnvelopeBytes(t *testing.T) {
	t.Parallel()

	for _, name := range canonicalResponseEventFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			event := loadCanonicalResponseEventFixture(t, name)
			canonical, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("json.Marshal(%s): %v", name, err)
			}
			got, err := responseeventstore.SerializedEventSize(event)
			if err != nil {
				t.Fatalf("SerializedEventSize(%s): %v", name, err)
			}
			if got != len(canonical) {
				t.Fatalf("SerializedEventSize(%s) = %d, want canonical envelope length %d", name, got, len(canonical))
			}
		})
	}
}

func TestSessionResponseEventStore_OptionalEnvelopeUsesExactCanonicalByteBoundary(t *testing.T) {
	t.Parallel()

	input := loadCanonicalResponseEventFixture(t, "message_snapshot")
	input.FactorySessionID = ""
	store := newRetentionStore(t, responseeventstore.RetentionLimits{
		MaxEvents: 1,
		MaxBytes:  generousByteLimit,
	})
	published := publishRetentionEvents(t, store, input)[0]
	canonical, err := json.Marshal(published)
	if err != nil {
		t.Fatalf("json.Marshal(published): %v", err)
	}

	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: len(canonical)}); err != nil {
		t.Fatalf("SetRetentionLimits(exact): %v", err)
	}
	if accounting := store.RetentionAccounting(); accounting.TotalBytes != len(canonical) || accounting.EventCount != 1 {
		t.Fatalf("exact-boundary accounting = %#v, want one event and %d bytes", accounting, len(canonical))
	}

	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: len(canonical) - 1}); err != nil {
		t.Fatalf("SetRetentionLimits(below): %v", err)
	}
	if events := store.Events(); len(events) != 0 {
		t.Fatalf("below-boundary retained events = %#v, want none", events)
	}
	if accounting := store.RetentionAccounting(); accounting != (responseeventstore.RetentionAccounting{}) {
		t.Fatalf("below-boundary accounting = %#v, want zero", accounting)
	}
}

func loadCanonicalResponseEventFixture(t *testing.T, name string) responseevents.FactoryResponseEvent {
	t.Helper()

	path := filepath.Join("..", "responseevents", "testdata", "fixtures", name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}
	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", name, err)
	}
	return event
}
