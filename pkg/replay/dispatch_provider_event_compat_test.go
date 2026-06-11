package replay

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestNormalizeDispatchProviderEvent_MapsLegacyDispatchRequestMetadata(t *testing.T) {
	event := legacyDispatchRequestEvent(t, map[string]any{
		"runnerId":              "cursor-cli",
		"runnerSelectionSource": "factory",
		"replayKey":             "replay-legacy",
	})

	if err := normalizeDispatchProviderEvent(&event); err != nil {
		t.Fatalf("normalizeDispatchProviderEvent: %v", err)
	}

	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch request payload: %v", err)
	}
	if payload.Metadata == nil {
		t.Fatal("metadata = nil, want normalized provider metadata")
	}
	if got := stringValue(payload.Metadata.ModelProvider); got != string(factoryapi.WorkerModelProviderCursor) {
		t.Fatalf("metadata.modelProvider = %q, want %q", got, factoryapi.WorkerModelProviderCursor)
	}
	if got := stringValue(payload.Metadata.ModelProviderSelectionSource); got != string(factoryapi.ModelProviderSelectionSourceFactory) {
		t.Fatalf("metadata.modelProviderSelectionSource = %q, want %q", got, factoryapi.ModelProviderSelectionSourceFactory)
	}
	if got := stringValue(payload.Metadata.ReplayKey); got != "replay-legacy" {
		t.Fatalf("metadata.replayKey = %q, want replay-legacy", got)
	}
}

func TestNormalizeDispatchProviderEvent_MapsLegacyDispatchQueuedPayload(t *testing.T) {
	event := legacyDispatchQueuedEvent(t, map[string]any{
		"dispatchKind": string(factoryapi.FactoryDispatchKindJAVASCRIPTAGENT),
		"runnerId":     "gemini",
	})

	if err := normalizeDispatchProviderEvent(&event); err != nil {
		t.Fatalf("normalizeDispatchProviderEvent: %v", err)
	}

	payload, err := event.Payload.AsDispatchQueuedEventPayload()
	if err != nil {
		t.Fatalf("dispatch queued payload: %v", err)
	}
	if got := stringValue(payload.ModelProvider); got != string(factoryapi.WorkerModelProviderGemini) {
		t.Fatalf("payload.modelProvider = %q, want %q", got, factoryapi.WorkerModelProviderGemini)
	}
}

func TestNormalizeDispatchProviderEvent_RejectsUnknownLegacyRunnerID(t *testing.T) {
	event := legacyDispatchQueuedEvent(t, map[string]any{
		"dispatchKind": string(factoryapi.FactoryDispatchKindJAVASCRIPTAGENT),
		"runnerId":     "mystery-runner",
	})

	err := normalizeDispatchProviderEvent(&event)
	if err == nil {
		t.Fatal("expected error for unknown legacy runnerId")
	}
	if !strings.Contains(err.Error(), `unknown legacy runnerId "mystery-runner"`) {
		t.Fatalf("error = %q, want legacy runnerId naming", err)
	}
}

func legacyDispatchRequestEvent(t *testing.T, metadata map[string]any) factoryapi.FactoryEvent {
	t.Helper()
	payload := map[string]any{
		"transitionId": "review",
		"metadata":     metadata,
	}
	return legacyFactoryEvent(t, factoryapi.FactoryEventTypeDispatchRequest, payload)
}

func legacyDispatchQueuedEvent(t *testing.T, payload map[string]any) factoryapi.FactoryEvent {
	t.Helper()
	return legacyFactoryEvent(t, factoryapi.FactoryEventTypeDispatchQueued, payload)
}

func legacyFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, payload map[string]any) factoryapi.FactoryEvent {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.UnmarshalJSON(encoded); err != nil {
		t.Fatalf("decode payload union: %v", err)
	}
	dispatchID := "dispatch-legacy"
	return factoryapi.FactoryEvent{
		Id:   "factory-event/" + string(eventType),
		Type: eventType,
		Context: factoryapi.FactoryEventContext{
			DispatchId: &dispatchID,
			EventTime:  time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
			Tick:       1,
		},
		Payload: union,
	}
}
