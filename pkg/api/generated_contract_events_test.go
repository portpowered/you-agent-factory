package api_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestGeneratedFactoryEventContractsCompile(t *testing.T) {
	events := generatedFactoryEventFixtures(t)

	if len(events) != len(canonicalFactoryEventTypes) {
		t.Fatalf("generated FactoryEvent contract coverage = %d, want %d", len(events), len(canonicalFactoryEventTypes))
	}

	seen := make(map[factoryapi.FactoryEventType]int, len(events))
	for _, event := range events {
		seen[event.Type]++
	}
	for _, eventType := range canonicalFactoryEventTypes {
		if seen[eventType] != 1 {
			t.Fatalf("generated FactoryEvent contract coverage for %s = %d, want 1", eventType, seen[eventType])
		}
	}
}

func TestGeneratedFactoryEventContractsRoundTripCanonicalFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("testdata/canonical-event-vocabulary-stream.json"))
	if err != nil {
		t.Fatalf("read canonical event fixture: %v", err)
	}
	assertTextOmitsRetiredEventNames(t, string(data))

	var events []factoryapi.FactoryEvent
	decodeRoundTripJSON(t, data, &events, "canonical event fixture")
	if len(events) != len(canonicalFactoryEventTypes) {
		t.Fatalf("canonical event fixture count = %d, want %d", len(events), len(canonicalFactoryEventTypes))
	}

	seen := make(map[factoryapi.FactoryEventType]int, len(events))
	for _, event := range events {
		seen[event.Type]++
		assertGeneratedFactoryEventRoundTrip(t, event)
	}
	for _, eventType := range canonicalFactoryEventTypes {
		if seen[eventType] != 1 {
			t.Fatalf("canonical fixture coverage for %s = %d, want 1", eventType, seen[eventType])
		}
	}
}

func TestGeneratedArtifactsAndCanonicalFixturesOmitRetiredEventNames(t *testing.T) {
	paths := []string{
		filepath.FromSlash("generated/server.gen.go"),
		filepath.FromSlash("testdata/canonical-event-vocabulary-stream.json"),
		filepath.FromSlash("../replay/testdata/inference-events.replay.json"),
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			assertTextOmitsRetiredEventNames(t, string(data))
		})
	}
}

func TestGeneratedInferenceEventJSONRoundTripPreservesAttemptCorrelation(t *testing.T) {
	event := decodeFactoryEventJSON(t, `{
		"schemaVersion": "agent-factory.event.v1",
		"id": "event-inference-response",
		"type": "INFERENCE_RESPONSE",
		"context": {
			"sequence": 9,
			"tick": 4,
			"eventTime": "2026-04-18T12:30:00Z",
			"dispatchId": "dispatch-1"
		},
		"payload": {
			"inferenceRequestId": "inference-request-1",
			"attempt": 2,
			"outcome": "FAILED",
			"durationMillis": 251,
			"exitCode": 1,
			"errorClass": "provider_error"
		}
	}`)

	if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
		t.Fatalf("event type = %q, want INFERENCE_RESPONSE", event.Type)
	}
	payload, err := event.Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("decode inference response payload: %v", err)
	}
	if payload.InferenceRequestId != "inference-request-1" || payload.Attempt != 2 {
		t.Fatalf("inference correlation = %s attempt %d, want inference-request-1 attempt 2", payload.InferenceRequestId, payload.Attempt)
	}
	if payload.Outcome != factoryapi.InferenceOutcomeFailed || payload.ExitCode == nil || *payload.ExitCode != 1 {
		t.Fatalf("inference outcome = %q exitCode %#v, want FAILED exitCode 1", payload.Outcome, payload.ExitCode)
	}

	assertEventPayloadJSONOmitsKeys(t, event, "dispatchId", "transitionId")
}

func TestGeneratedScriptEventJSONRoundTripPreservesRequestCorrelationAndFailureShape(t *testing.T) {
	event := decodeFactoryEventJSON(t, `{
		"schemaVersion": "agent-factory.event.v1",
		"id": "event-script-response",
		"type": "SCRIPT_RESPONSE",
		"context": {
			"sequence": 10,
			"tick": 4,
			"eventTime": "2026-04-18T12:30:00Z",
			"dispatchId": "dispatch-script-1"
		},
		"payload": {
			"scriptRequestId": "script-request-1",
			"dispatchId": "dispatch-script-1",
			"transitionId": "transition-script-1",
			"attempt": 2,
			"outcome": "PROCESS_ERROR",
			"stdout": "",
			"stderr": "exec: file not found",
			"durationMillis": 17,
			"failureType": "PROCESS_ERROR"
		}
	}`)

	if event.Type != factoryapi.FactoryEventTypeScriptResponse {
		t.Fatalf("event type = %q, want SCRIPT_RESPONSE", event.Type)
	}
	payload, err := event.Payload.AsScriptResponseEventPayload()
	if err != nil {
		t.Fatalf("decode script response payload: %v", err)
	}
	if payload.ScriptRequestId != "script-request-1" || payload.Attempt != 2 {
		t.Fatalf("script correlation = %s attempt %d, want script-request-1 attempt 2", payload.ScriptRequestId, payload.Attempt)
	}
	if payload.Outcome != factoryapi.ScriptExecutionOutcomeProcessError {
		t.Fatalf("script outcome = %q, want PROCESS_ERROR", payload.Outcome)
	}
	if payload.FailureType == nil || *payload.FailureType != factoryapi.ScriptFailureTypeProcessError {
		t.Fatalf("script failureType = %#v, want PROCESS_ERROR", payload.FailureType)
	}
}

func TestGeneratedFactoryEventJSONRoundTripPreservesWorkRequestContextAndWorks(t *testing.T) {
	event := decodeFactoryEventJSON(t, `{
		"schemaVersion": "agent-factory.event.v1",
		"id": "event-work-request",
		"type": "WORK_REQUEST",
		"context": {
			"sequence": 7,
			"tick": 3,
			"eventTime": "2026-04-18T12:30:00Z",
			"requestId": "request-1",
			"traceIds": ["trace-1", "trace-2"],
			"workIds": ["work-1"],
			"source": "api"
		},
		"payload": {
			"type": "FACTORY_REQUEST_BATCH",
			"works": [
				{
					"name": "draft release notes",
					"workId": "work-1",
					"requestId": "request-1",
					"workTypeName": "task",
					"traceId": "trace-1",
					"payload": {"title": "event log"},
					"tags": {"priority": "high"}
				}
			],
			"relations": [
				{
					"type": "DEPENDS_ON",
					"sourceWorkName": "publish",
					"targetWorkName": "draft release notes"
				}
			],
			"source": "api",
			"parentLineage": ["parent-work-1"]
		}
	}`)

	assertGeneratedWorkRequestEventContext(t, event)
	assertGeneratedWorkRequestEventRoundTrip(t, event)
}

func TestGeneratedFactoryEventJSONRoundTripPreservesRunRequestFactoryConfig(t *testing.T) {
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	metadata := factoryapi.StringMap{"factory_hash": "sha256:test"}
	event := factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Id:            "event-run-started",
		Type:          factoryapi.FactoryEventTypeRunRequest,
		Context:       factoryapi.FactoryEventContext{Sequence: 0, Tick: 0, EventTime: eventTime},
		Payload: factoryEventPayload(t, factoryapi.RunRequestEventPayload{
			RecordedAt: eventTime,
			Factory: factoryapi.Factory{
				Name:     "factory",
				Metadata: &metadata,
				WorkTypes: &[]factoryapi.WorkType{{
					Name: "task",
					States: []factoryapi.WorkState{
						{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
						{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
					},
				}},
				Workers: &[]factoryapi.Worker{{Name: "agent"}},
			},
		}),
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal run-request factory event: %v", err)
	}
	if strings.Contains(string(encoded), "effectiveConfig") {
		t.Fatalf("run-request event JSON contains legacy effectiveConfig: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"factory"`) {
		t.Fatalf("run-request event JSON missing factory payload: %s", encoded)
	}

	var roundTripped factoryapi.FactoryEvent
	decodeRoundTripJSON(t, encoded, &roundTripped, "run-request factory event")
	payload, err := roundTripped.Payload.AsRunRequestEventPayload()
	if err != nil {
		t.Fatalf("decode run-request payload: %v", err)
	}
	if payload.Factory.WorkTypes == nil || len(*payload.Factory.WorkTypes) != 1 || (*payload.Factory.WorkTypes)[0].Name != "task" {
		t.Fatalf("round-tripped run-request factory = %#v, want task work type", payload.Factory)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 || (*payload.Factory.Workers)[0].Name != "agent" {
		t.Fatalf("round-tripped run-request workers = %#v, want agent worker", payload.Factory.Workers)
	}
}

func generatedFactoryEventFixtures(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()

	events := make([]factoryapi.FactoryEvent, 0, len(canonicalFactoryEventTypes))
	events = append(events, generatedFactoryLifecycleEvents(t)...)
	events = append(events, generatedFactoryWorkEvents(t)...)
	events = append(events, generatedFactoryExecutionEvents(t)...)
	return events
}

func generatedFactoryLifecycleEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	runningState := factoryapi.FactoryState("RUNNING")
	pausedState := factoryapi.FactoryState("PAUSED")
	completedState := factoryapi.FactoryState("COMPLETED")

	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-run-started",
			Type:          factoryapi.FactoryEventTypeRunRequest,
			Context:       factoryapi.FactoryEventContext{Sequence: 0, Tick: 0, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.RunRequestEventPayload{
				RecordedAt: eventTime,
				Factory:    factoryapi.Factory{Name: "factory"},
				WallClock:  &factoryapi.WallClock{StartedAt: &eventTime},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-initial-structure-request",
			Type:          factoryapi.FactoryEventTypeInitialStructureRequest,
			Context:       factoryapi.FactoryEventContext{Sequence: 1, Tick: 0, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.InitialStructureRequestEventPayload{
				Factory: factoryapi.Factory{Name: "factory"},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-factory-change",
			Type:          factoryapi.FactoryEventTypeFactoryChange,
			Context:       factoryapi.FactoryEventContext{Sequence: 2, Tick: 1, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.FactoryChangeEventPayload{
				Factory: factoryapi.Factory{Name: "factory-next"},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-factory-state-response",
			Type:          factoryapi.FactoryEventTypeFactoryStateResponse,
			Context:       factoryapi.FactoryEventContext{Sequence: 8, Tick: 3, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.FactoryStateResponseEventPayload{
				PreviousState: &runningState,
				Reason:        stringPtr("progress update"),
				State:         pausedState,
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-run-response",
			Type:          factoryapi.FactoryEventTypeRunResponse,
			Context:       factoryapi.FactoryEventContext{Sequence: 9, Tick: 4, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.RunResponseEventPayload{
				Reason: stringPtr("all work finished"),
				State:  &completedState,
			}),
		},
	}
}

func generatedFactoryWorkEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	requestID := "request-1"
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	source := "api"
	workTypeID := "task"
	work := factoryapi.Work{
		Name:         "draft release notes",
		WorkId:       &workIDs[0],
		RequestId:    &requestID,
		WorkTypeName: &workTypeID,
		TraceId:      &traceIDs[0],
		Payload:      map[string]any{"title": "event log"},
	}
	relation := factoryapi.Relation{
		Type:           factoryapi.RelationTypeDependsOn,
		SourceWorkName: "publish",
		TargetWorkName: "draft release notes",
	}
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-work-request",
			Type:          factoryapi.FactoryEventTypeWorkRequest,
			Context: factoryapi.FactoryEventContext{
				Sequence:  1,
				Tick:      1,
				EventTime: eventTime,
				RequestId: &requestID,
				TraceIds:  &traceIDs,
				WorkIds:   &workIDs,
				Source:    &source,
			},
			Payload: factoryEventPayload(t, factoryapi.WorkRequestEventPayload{
				Type:          factoryapi.WorkRequestTypeFactoryRequestBatch,
				Works:         &[]factoryapi.Work{work},
				Relations:     &[]factoryapi.Relation{relation},
				Source:        &source,
				ParentLineage: &[]string{"parent-work-1"},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-relationship-change-request",
			Type:          factoryapi.FactoryEventTypeRelationshipChangeRequest,
			Context: factoryapi.FactoryEventContext{
				Sequence:  2,
				Tick:      1,
				EventTime: eventTime,
				RequestId: &requestID,
				WorkIds:   &[]string{"work-1", "work-2"},
			},
			Payload: factoryEventPayload(t, factoryapi.RelationshipChangeRequestEventPayload{
				Relation: relation,
			}),
		},
	}
}

func generatedFactoryExecutionEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	events := make([]factoryapi.FactoryEvent, 0, 6)
	events = append(events, generatedFactoryDispatchEvents(t)...)
	events = append(events, generatedFactoryInferenceEvents(t)...)
	events = append(events, generatedFactoryScriptEvents(t)...)
	return events
}

func generatedFactoryDispatchEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	scriptDispatchID := "dispatch-script-1"
	workTypeID := "task"
	work := factoryapi.Work{
		Name:         "draft release notes",
		WorkId:       &workIDs[0],
		WorkTypeName: &workTypeID,
		TraceId:      &traceIDs[0],
		Payload:      map[string]any{"title": "event log"},
	}
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-dispatch-created",
			Type:          factoryapi.FactoryEventTypeDispatchRequest,
			Context: factoryapi.FactoryEventContext{
				Sequence:                 2,
				Tick:                     2,
				EventTime:                eventTime,
				TraceIds:                 &traceIDs,
				WorkIds:                  &workIDs,
				DispatchId:               &scriptDispatchID,
				CurrentChainingTraceId:   stringPtr("chain-current-1"),
				PreviousChainingTraceIds: &[]string{"chain-a", "chain-z"},
			},
			Payload: factoryEventPayload(t, factoryapi.DispatchRequestEventPayload{
				TransitionId:             "transition-1",
				CurrentChainingTraceId:   stringPtr("chain-current-1"),
				PreviousChainingTraceIds: &[]string{"chain-a", "chain-z"},
				Inputs:                   []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-1"}},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-dispatch-completed",
			Type:          factoryapi.FactoryEventTypeDispatchResponse,
			Context: factoryapi.FactoryEventContext{
				Sequence:                 7,
				Tick:                     3,
				EventTime:                eventTime,
				TraceIds:                 &traceIDs,
				WorkIds:                  &workIDs,
				DispatchId:               &scriptDispatchID,
				CurrentChainingTraceId:   stringPtr("chain-current-1"),
				PreviousChainingTraceIds: &[]string{"chain-a", "chain-z"},
			},
			Payload: factoryEventPayload(t, factoryapi.DispatchResponseEventPayload{
				TransitionId:             "transition-1",
				CurrentChainingTraceId:   stringPtr("chain-current-1"),
				PreviousChainingTraceIds: &[]string{"chain-a", "chain-z"},
				Outcome:                  factoryapi.WorkOutcomeAccepted,
				OutputWork:               &[]factoryapi.Work{work},
			}),
		},
	}
}

func generatedFactoryInferenceEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	dispatchID := "dispatch-1"
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-inference-request",
			Type:          factoryapi.FactoryEventTypeInferenceRequest,
			Context: factoryapi.FactoryEventContext{
				Sequence:   3,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &dispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.InferenceRequestEventPayload{
				InferenceRequestId: "inference-request-1",
				Attempt:            1,
				WorkingDirectory:   "/tmp/factory/work",
				Worktree:           "/tmp/factory/worktree",
				Prompt:             "Draft release notes for the event log.",
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-inference-response",
			Type:          factoryapi.FactoryEventTypeInferenceResponse,
			Context: factoryapi.FactoryEventContext{
				Sequence:   4,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &dispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.InferenceResponseEventPayload{
				InferenceRequestId: "inference-request-1",
				Attempt:            1,
				Outcome:            factoryapi.InferenceOutcomeSucceeded,
				Response:           stringPtr("Release notes drafted."),
				DurationMillis:     124,
			}),
		},
	}
}

func generatedFactoryScriptEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	scriptDispatchID := "dispatch-script-1"
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-script-request",
			Type:          factoryapi.FactoryEventTypeScriptRequest,
			Context: factoryapi.FactoryEventContext{
				Sequence:   5,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &scriptDispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.ScriptRequestEventPayload{
				ScriptRequestId: "script-request-1",
				DispatchId:      scriptDispatchID,
				TransitionId:    "transition-script-1",
				Attempt:         1,
				Command:         "script-tool",
				Args:            []string{"--work", "work-1", "--project", "docs"},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-script-response",
			Type:          factoryapi.FactoryEventTypeScriptResponse,
			Context: factoryapi.FactoryEventContext{
				Sequence:   6,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &scriptDispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.ScriptResponseEventPayload{
				ScriptRequestId: "script-request-1",
				DispatchId:      scriptDispatchID,
				TransitionId:    "transition-script-1",
				Attempt:         1,
				Outcome:         factoryapi.ScriptExecutionOutcomeFailedExitCode,
				Stdout:          "script stdout\n",
				Stderr:          "script stderr\n",
				DurationMillis:  238,
				ExitCode:        intPtr(3),
			}),
		},
	}
}

func assertGeneratedFactoryEventRoundTrip(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	requireGeneratedFactoryEventPayloadRoundTrip(t, event)

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal canonical event %s: %v", event.Id, err)
	}
	assertTextOmitsRetiredEventNames(t, string(encoded))

	var roundTripped factoryapi.FactoryEvent
	decodeRoundTripJSON(t, encoded, &roundTripped, "round-tripped canonical event")
	if roundTripped.Type != event.Type {
		t.Fatalf("round-tripped event %s type = %q, want %q", event.Id, roundTripped.Type, event.Type)
	}
	requireGeneratedFactoryEventPayloadRoundTrip(t, roundTripped)
}

func decodeFactoryEventJSON(t *testing.T, input string) factoryapi.FactoryEvent {
	t.Helper()

	var event factoryapi.FactoryEvent
	decodeRoundTripJSON(t, []byte(input), &event, "generated FactoryEvent")
	return event
}

func assertEventPayloadJSONOmitsKeys(t *testing.T, event factoryapi.FactoryEvent, keys ...string) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal generated FactoryEvent: %v", err)
	}
	var roundTripped map[string]any
	decodeRoundTripJSON(t, encoded, &roundTripped, "generated FactoryEvent JSON")
	payloadJSON, _ := roundTripped["payload"].(map[string]any)
	for _, key := range keys {
		if _, ok := payloadJSON[key]; ok {
			t.Fatalf("generated event payload must not reintroduce payload.%s: %#v", key, payloadJSON)
		}
	}
}

func assertGeneratedWorkRequestEventContext(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	if event.Type != factoryapi.FactoryEventTypeWorkRequest {
		t.Fatalf("event type = %q, want WORK_REQUEST", event.Type)
	}
	if event.Context.RequestId == nil || *event.Context.RequestId != "request-1" {
		t.Fatalf("context.requestId = %#v, want request-1", event.Context.RequestId)
	}
	if event.Context.TraceIds == nil || len(*event.Context.TraceIds) != 2 || (*event.Context.TraceIds)[1] != "trace-2" {
		t.Fatalf("context.traceIds = %#v, want trace-1 and trace-2", event.Context.TraceIds)
	}
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode work request payload: %v", err)
	}
	if payload.Works == nil || len(*payload.Works) != 1 || (*payload.Works)[0].Name != "draft release notes" {
		t.Fatalf("payload.works = %#v, want one preserved work item", payload.Works)
	}
}

func assertGeneratedWorkRequestEventRoundTrip(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal generated FactoryEvent: %v", err)
	}
	var roundTripped factoryapi.FactoryEvent
	decodeRoundTripJSON(t, encoded, &roundTripped, "round-tripped FactoryEvent")
	roundTrippedPayload, err := roundTripped.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode round-tripped work request payload: %v", err)
	}
	if roundTripped.Context.RequestId == nil || *roundTripped.Context.RequestId != "request-1" {
		t.Fatalf("round-tripped context.requestId = %#v, want request-1", roundTripped.Context.RequestId)
	}
	if roundTripped.Context.TraceIds == nil || len(*roundTripped.Context.TraceIds) != 2 {
		t.Fatalf("round-tripped context.traceIds = %#v, want two trace ids", roundTripped.Context.TraceIds)
	}
	if roundTrippedPayload.Works == nil || len(*roundTrippedPayload.Works) != 1 || (*roundTrippedPayload.Works)[0].WorkId == nil || *(*roundTrippedPayload.Works)[0].WorkId != "work-1" {
		t.Fatalf("round-tripped payload.works = %#v, want work-1 preserved", roundTrippedPayload.Works)
	}
}
