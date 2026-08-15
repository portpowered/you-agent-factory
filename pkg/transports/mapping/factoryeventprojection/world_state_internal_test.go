package factoryeventprojection

import (
	"bytes"
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCanonicalExecutionPayloadProjectsProviderSession(t *testing.T) {
	input := json.RawMessage(`{"outcome":"FAILED","providerSession":{"provider":"antigravity","kind":"session_id","id":"session-1"}}`)
	got, err := canonicalExecutionPayload(input)
	if err != nil {
		t.Fatalf("canonicalExecutionPayload() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatalf("decode canonical payload: %v", err)
	}
	if _, ok := fields["providerSession"]; ok {
		t.Fatal("canonical payload retained providerSession after projecting continuation")
	}
	var continuation struct {
		Provider          string `json:"Provider"`
		ProviderSessionID string `json:"ProviderSessionID"`
		ExternalRef       string `json:"ExternalRef"`
	}
	if err := json.Unmarshal(fields["continuation"], &continuation); err != nil {
		t.Fatalf("decode projected continuation: %v", err)
	}
	if continuation.Provider != "antigravity" || continuation.ProviderSessionID != "session-1" || continuation.ExternalRef != "session-1" {
		t.Fatalf("continuation = %#v, want provider/session identity", continuation)
	}
}

func TestCanonicalExecutionPayloadPreservesExistingAndNonSessionPayloads(t *testing.T) {
	tests := []struct {
		name  string
		input json.RawMessage
	}{
		{name: "empty", input: nil},
		{name: "existing continuation", input: json.RawMessage(`{"continuation":{"Provider":"codex","ProviderSessionID":"session-1"}}`)},
		{name: "no provider session", input: json.RawMessage(`{"outcome":"SUCCEEDED"}`)},
		{name: "provider without session id", input: json.RawMessage(`{"providerSession":{"provider":"antigravity"}}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := canonicalExecutionPayload(test.input)
			if err != nil {
				t.Fatalf("canonicalExecutionPayload() error = %v", err)
			}
			if test.input == nil {
				if got != nil {
					t.Fatalf("empty payload = %s, want nil", got)
				}
				return
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(got, &fields); err != nil {
				t.Fatalf("decode preserved payload: %v", err)
			}
			if test.name == "provider without session id" {
				if _, ok := fields["providerSession"]; ok {
					t.Fatal("provider-only session was not removed")
				}
			} else if _, ok := fields["outcome"]; test.name == "no provider session" && !ok {
				t.Fatal("non-session payload changed unexpectedly")
			}
		})
	}
}

func TestCanonicalExecutionPayloadRejectsMalformedPayloads(t *testing.T) {
	for _, input := range []json.RawMessage{
		json.RawMessage("{"),
		json.RawMessage(`{"providerSession":"invalid"}`),
	} {
		if _, err := canonicalExecutionPayload(input); err == nil {
			t.Fatalf("canonicalExecutionPayload(%s) error = nil, want malformed payload error", input)
		}
	}
}

func TestCanonicalFactoryEventRejectsMalformedExecutionPayload(t *testing.T) {
	event := factoryapi.FactoryEvent{Type: factoryapi.FactoryEventTypeModelResponse}
	encoded, err := json.Marshal(map[string]any{
		"context":       event.Context,
		"id":            "malformed",
		"payload":       "not an object",
		"schemaVersion": event.SchemaVersion,
		"type":          event.Type,
	})
	if err != nil {
		t.Fatalf("marshal malformed event: %v", err)
	}
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatalf("decode malformed event: %v", err)
	}
	if _, err := CanonicalFactoryEvent(event); err == nil {
		t.Fatal("CanonicalFactoryEvent() error = nil, want malformed payload error")
	}
}

func TestCanonicalExecutionPayloadPreservesBytesWithContinuation(t *testing.T) {
	input := json.RawMessage(`{"continuation":{"Provider":"codex","ProviderSessionID":"session-1"}}`)
	got, err := canonicalExecutionPayload(input)
	if err != nil {
		t.Fatalf("canonicalExecutionPayload() error = %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("preserved payload = %s, want %s", got, input)
	}
}
