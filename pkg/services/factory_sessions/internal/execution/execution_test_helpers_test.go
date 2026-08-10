package factorysessionexecution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
)

func contractFixturesPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "..", "transports", "http", "testdata", "durable-session-contract-fixtures.json")
}

func newContractFakeService(t *testing.T) *FakeService {
	t.Helper()
	service, err := NewFakeServiceFromContractFixtures(contractFixturesPath(t), fakeServiceTestClock(), fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func fakeServiceTestClock() durableFixedClock {
	return durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func mustNewFakeService(t *testing.T, scenarios ...FakeScenario) *FakeService {
	t.Helper()
	service, err := NewFakeService(fakeServiceTestClock(), scenarios...)
	if err != nil {
		t.Fatalf("NewFakeService: %v", err)
	}
	return service
}

func int64Ptr(value int64) *int64 {
	return &value
}

func assertCanonicalEventEnvelope(t *testing.T, raw json.RawMessage, eventType, id string) {
	t.Helper()
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		ID            string `json:"id"`
		Type          string `json:"type"`
		Context       struct {
			Sequence int `json:"sequence"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if envelope.SchemaVersion != canonicalFactoryEventSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", envelope.SchemaVersion, canonicalFactoryEventSchemaVersion)
	}
	if id != "" && envelope.ID != id {
		t.Fatalf("id = %q, want %q", envelope.ID, id)
	}
	if eventType != "" && envelope.Type != eventType {
		t.Fatalf("type = %q, want %q", envelope.Type, eventType)
	}
	if envelope.Context.Sequence <= 0 {
		t.Fatalf("sequence = %d, want positive", envelope.Context.Sequence)
	}
	if len(envelope.Payload) == 0 {
		t.Fatal("payload missing")
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil &&
		json.Unmarshal(right, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}

func canonicalTypedInternalEvent(t *testing.T, eventType, sessionID string, payload any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"schemaVersion": canonicalFactoryEventSchemaVersion,
		"id":            "internal/" + eventType,
		"type":          eventType,
		"context": map[string]any{
			"sequence":  99,
			"sessionId": sessionID,
		},
		"payload": payload,
	})
	if err != nil {
		t.Fatalf("marshal %s event: %v", eventType, err)
	}
	return raw
}
