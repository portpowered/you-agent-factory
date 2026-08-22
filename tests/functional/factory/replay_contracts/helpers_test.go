package replay_contracts_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func replayContractFactoryConfig() map[string]any {
	return map[string]any{
		"name": "recordings-functional-replay",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "ready", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "model-worker",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CODEX",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "gpt-5-codex",
		}},
		"workstations": []map[string]any{{
			"name":      "process-task",
			"worker":    "model-worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "ready"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func isolatedReplayEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
}

func selectedTickArtifactPayload(t *testing.T, includeTerminalEvents bool) []byte {
	t.Helper()
	const (
		workID    = "selected-tick-work"
		requestID = "selected-tick-request"
		traceID   = "selected-tick-trace"
	)
	snapshot, err := factorydefinitions.NewFactorySnapshot(replayContractFactoryConfig())
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	base := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	requestIDValue := requestID
	workIDValue := []string{workID}
	traceIDValue := []string{traceID}
	sourceValue := "functional-replay"
	events := []factorydefinitions.FactoryEvent{
		selectedTickEvent(t, "run-request", 0, 0, base, factorydefinitions.FactoryEventTypeRunRequest,
			factorydefinitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base}, nil, nil, nil, nil),
		selectedTickEvent(t, "work-request", 1, 1, base.Add(time.Second), factorydefinitions.FactoryEventTypeWorkRequest,
			work.WorkRequestEventPayload{
				Source: sourceValue,
				Type:   work.WorkRequestTypeFactoryRequestBatch,
				Works: []work.WorkRequestEventWork{{
					Name: "selected-tick-work", WorkID: workID, RequestID: requestID,
					WorkTypeID: "task", State: &work.WorkEventState{Name: "ready", Type: "INITIAL"}, TraceID: traceID,
				}},
			}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
	}
	if includeTerminalEvents {
		events = append(events,
			selectedTickEvent(t, "processing", 2, 2, base.Add(2*time.Second), factorydefinitions.FactoryEventTypeWorkStateChange,
				factorydefinitions.WorkStateChangeEventPayload{
					FromPlaceID: "task:ready", FromState: "ready", Source: work.WorkStateChangeSourceCLI,
					ToPlaceID: "task:processing", ToState: "processing", WorkID: workID, WorkTypeName: "task",
				}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
			selectedTickEvent(t, "complete", 3, 3, base.Add(3*time.Second), factorydefinitions.FactoryEventTypeWorkStateChange,
				factorydefinitions.WorkStateChangeEventPayload{
					FromPlaceID: "task:processing", FromState: "processing", Source: work.WorkStateChangeSourceCLI,
					ToPlaceID: "task:complete", ToState: "complete", WorkID: workID, WorkTypeName: "task",
				}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
			selectedTickEvent(t, "run-response", 4, 4, base.Add(4*time.Second), factorydefinitions.FactoryEventTypeRunResponse,
				factorydefinitions.RunResponseEventPayload{State: factoryStatePointer(factorydefinitions.FactoryStateCompleted)}, nil, nil, nil, nil),
		)
	}
	artifact := factorydefinitions.ReplayArtifact{
		SchemaVersion: factorydefinitions.ReplayV1SourceFormat,
		RecordedAt:    base,
		Events:        events,
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal selected-tick replay artifact: %v", err)
	}
	return payload
}

func selectedTickEvent(
	t *testing.T,
	id string,
	sequence, tick int,
	eventTime time.Time,
	eventType factorydefinitions.FactoryEventType,
	payload any,
	source, requestID *string,
	workIDs, traceIDs *[]string,
) factorydefinitions.FactoryEvent {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal replay event %q payload: %v", id, err)
	}
	return factorydefinitions.FactoryEvent{
		Id: id, Payload: payloadBytes, SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1, Type: eventType,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: eventTime, RequestID: requestID, Sequence: sequence, Source: source,
			Tick: tick, TraceIDs: traceIDs, WorkIDs: workIDs,
		},
	}
}

func factoryStatePointer(state factorydefinitions.FactoryState) *factorydefinitions.FactoryState {
	return &state
}
