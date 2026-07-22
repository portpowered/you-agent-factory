package responseevents_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
)

var responseEventFixtures = []struct {
	name string
}{
	{name: "text_delta"},
	{name: "message_snapshot"},
	{name: "tool_lifecycle"},
	{name: "retry"},
	{name: "final_only_message"},
	{name: "usage"},
	{name: "stream_gap"},
	{name: "item_stream_gap"},
}

func TestFixtureRoundTrip_PreservesDeclaredFields(t *testing.T) {
	t.Parallel()

	for _, tc := range responseEventFixtures {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			event := loadFixtureEvent(t, tc.name)
			if err := responseevents.ValidateEvent(event); err != nil {
				t.Fatalf("ValidateEvent() error = %v", err)
			}

			remarshaled, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			var roundTripped responseevents.FactoryResponseEvent
			if err := json.Unmarshal(remarshaled, &roundTripped); err != nil {
				t.Fatalf("json.Unmarshal(remarshaled) error = %v", err)
			}

			assertEventsSemanticallyEqual(t, event, roundTripped)
		})
	}
}

func TestFixtureRoundTrip_MarshalIsDeterministic(t *testing.T) {
	t.Parallel()

	for _, tc := range responseEventFixtures {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			event := loadFixtureEvent(t, tc.name)

			first, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("first json.Marshal() error = %v", err)
			}
			second, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("second json.Marshal() error = %v", err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("marshal is not deterministic:\nfirst=%s\nsecond=%s", first, second)
			}
		})
	}
}

func TestFixtures_CoverPopulatedAndOmittedOptionalFields(t *testing.T) {
	t.Parallel()

	populated := loadFixtureEvent(t, "message_snapshot")
	for field, value := range map[string]string{
		"dispatchId":         populated.DispatchID,
		"turnId":             populated.TurnID,
		"itemId":             populated.ItemID,
		"parentItemId":       populated.ParentItemID,
		"providerSessionRef": populated.ProviderSessionRef,
		"nativeEventSubtype": populated.Provenance.NativeEventSubtype,
	} {
		if value == "" {
			t.Fatalf("message_snapshot optional field %s must be populated", field)
		}
	}

	omitted := loadFixtureEvent(t, "stream_gap")
	encoded, err := json.Marshal(omitted)
	if err != nil {
		t.Fatalf("json.Marshal(stream_gap) error = %v", err)
	}
	for _, field := range []string{"dispatchId", "turnId", "itemId", "parentItemId", "providerSessionRef", "nativeEventSubtype"} {
		if bytes.Contains(encoded, []byte(`"`+field+`"`)) {
			t.Fatalf("stream_gap optional field %s must be omitted: %s", field, encoded)
		}
	}
}

func TestFixtures_ContainNoProviderNativeProtocolFields(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"openai",
		"anthropic",
		"cursor",
		"compaction",
		"rawProvider",
		"providerPayload",
	}
	for _, tc := range responseEventFixtures {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := readFixtureBytes(t, tc.name)
			lower := strings.ToLower(string(raw))
			for _, needle := range forbidden {
				if strings.Contains(lower, needle) {
					t.Fatalf("fixture %q contains forbidden provider-native marker %q", tc.name, needle)
				}
			}
		})
	}
}

func loadFixtureEvent(t *testing.T, name string) responseevents.FactoryResponseEvent {
	t.Helper()

	raw := readFixtureBytes(t, name)
	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", name, err)
	}
	return event
}

func readFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()

	fixturePath := filepath.Join("testdata", "fixtures", name+".json")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %q: %v", fixturePath, err)
	}
	return raw
}

func assertEventsSemanticallyEqual(t *testing.T, want, got responseevents.FactoryResponseEvent) {
	t.Helper()

	if !jsonValuesEqual(want.Payload, got.Payload) {
		t.Fatalf("payload semantic mismatch:\nwant=%s\ngot=%s", want.Payload, got.Payload)
	}

	want.Payload = nil
	got.Payload = nil
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("envelope field mismatch:\nwant=%#v\ngot=%#v", want, got)
	}
}

func jsonValuesEqual(left, right json.RawMessage) bool {
	if len(left) == 0 && len(right) == 0 {
		return true
	}
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
