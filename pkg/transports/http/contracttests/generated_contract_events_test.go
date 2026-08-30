package apicontract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	data, err := os.ReadFile(filepath.FromSlash("../testdata/canonical-event-vocabulary-stream.json"))
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

func TestCanonicalFactoryEventFixtureUsesMachineTimeContract(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("../testdata/canonical-event-vocabulary-stream.json"))
	if err != nil {
		t.Fatalf("read canonical event fixture: %v", err)
	}

	var events []map[string]any
	decodeRoundTripJSON(t, data, &events, "canonical event fixture")
	for i, event := range events {
		context, ok := event["context"].(map[string]any)
		if !ok {
			t.Fatalf("event[%d].context = %#v, want object", i, event["context"])
		}
		eventTime, ok := context["eventTime"].(string)
		if !ok || eventTime == "" {
			t.Fatalf("event[%d].context.eventTime = %#v, want non-empty string", i, context["eventTime"])
		}
		assertRFC3339TimestampWithTimezone(t, "context.eventTime", eventTime)
		assertMachineTimePayloadValues(t, event["payload"], "event.payload")
	}
}

func TestGeneratedArtifactsAndCanonicalFixturesOmitRetiredEventNames(t *testing.T) {
	paths := []string{
		filepath.FromSlash("../generated/server.gen.go"),
		filepath.FromSlash("../testdata/canonical-event-vocabulary-stream.json"),
		filepath.FromSlash("../../../services/recordings/internal/replay/testdata/inference-events.replay.json"),
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

func TestGeneratedFactoryInferenceResponseEvent_UsesCanonicalPublicFieldsOnly(t *testing.T) {
	eventTime := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	dispatchID := "dispatch-cursor-1"
	requestMetadata := factoryapi.StringMap{
		"request_id": "req-123",
	}
	responseMetadata := factoryapi.StringMap{
		"duration_ms": "25",
	}
	event := factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Id:            "event-inference-response-cursor",
		Type:          factoryapi.FactoryEventTypeInferenceResponse,
		Context: factoryapi.FactoryEventContext{
			Sequence:   10,
			Tick:       4,
			EventTime:  eventTime,
			DispatchId: &dispatchID,
		},
		Payload: factoryEventPayload(t, factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "inference-request-cursor",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			Response:           stringPtr("Plan done"),
			DurationMillis:     25,
			ProviderSession: &factoryapi.ProviderSessionMetadata{
				Provider: stringPtr("cursor"),
				Kind:     stringPtr("session_id"),
				Id:       stringPtr("cursor-session-123"),
			},
			Diagnostics: &factoryapi.SafeWorkDiagnostics{
				Provider: &factoryapi.ProviderDiagnostic{
					Provider:         stringPtr("cursor"),
					Model:            stringPtr("gpt-5"),
					RequestMetadata:  &requestMetadata,
					ResponseMetadata: &responseMetadata,
				},
			},
		}),
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal generated FactoryEvent: %v", err)
	}

	var roundTripped map[string]any
	decodeRoundTripJSON(t, encoded, &roundTripped, "generated cursor inference response event")
	payload, ok := roundTripped["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want object", roundTripped["payload"])
	}

	assertJSONKeysAbsent(t, payload, "generated inference response payload", "kind", "payload", "providerSessionRef", "timestamp_ms", "model_call_id")
	providerSession, ok := payload["providerSession"].(map[string]any)
	if !ok {
		t.Fatalf("payload.providerSession = %#v, want object", payload["providerSession"])
	}
	assertJSONKeysAbsent(t, providerSession, "generated provider session payload", "providerSessionRef", "session_id", "timestamp_ms", "model_call_id")

	diagnostics, ok := payload["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("payload.diagnostics = %#v, want object", payload["diagnostics"])
	}
	assertJSONKeysAbsent(t, diagnostics, "generated safe diagnostics payload", "kind", "payload", "rawEvent", "streamJson")

	providerDiagnostics, ok := diagnostics["provider"].(map[string]any)
	if !ok {
		t.Fatalf("payload.diagnostics.provider = %#v, want object", diagnostics["provider"])
	}
	assertJSONKeysAbsent(t, providerDiagnostics, "generated provider diagnostics payload", "providerSessionRef", "timestamp_ms", "model_call_id")
}

func TestGeneratedPublicEventArtifactsOmitInternalResponseStreamTerms(t *testing.T) {
	paths := []string{
		filepath.FromSlash("../testdata/canonical-event-vocabulary-stream.json"),
		filepath.FromSlash("../../../services/recordings/internal/replay/testdata/inference-events.replay.json"),
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			assertTextOmitsInternalResponseStreamTerms(t, string(data))
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
			"failureDetail": {"reason": "unknown", "message": "Provider request failed."}
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

	events := []factoryapi.FactoryEvent{
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
	events = append(events, generatedFactoryChangeEvents(t, eventTime)...)
	events = append(events, generatedFactorySessionLifecycleEvents(t, eventTime)...)
	events = append(events, generatedFactoryOrchestratorLifecycleEvents(t, eventTime)...)
	return events
}

func generatedFactoryOrchestratorLifecycleEvents(t *testing.T, eventTime time.Time) []factoryapi.FactoryEvent {
	t.Helper()
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-javascript-checkpoint-ref",
			Type:          factoryapi.FactoryEventTypeJavaScriptCheckpointRef,
			Context:       factoryapi.FactoryEventContext{Sequence: 10, Tick: 5, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.JavaScriptCheckpointRefEventPayload{
				CheckpointId: "ckpt-1",
				Label:        stringPtr("after-plan"),
				Summary:      stringPtr("Completed planning phase"),
				Timestamp:    &eventTime,
				ArtifactRef: factoryapi.FactoryArtifactRef{
					Id:         "artifact-ckpt-1",
					Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
					Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
				},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-javascript-phase-change",
			Type:          factoryapi.FactoryEventTypeJavaScriptPhaseChange,
			Context:       factoryapi.FactoryEventContext{Sequence: 11, Tick: 6, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.JavaScriptPhaseChangeEventPayload{
				Phase:        "execute",
				Phases:       []string{"plan", "execute"},
				ArgsDigest:   stringPtr("sha256:args"),
				ScriptStatus: factoryapi.FactorySessionJavaScriptScriptStatusRUNNING,
				ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{
					Queued:    1,
					Running:   2,
					Completed: 3,
				},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-artifact-created",
			Type:          factoryapi.FactoryEventTypeArtifactCreated,
			Context:       factoryapi.FactoryEventContext{Sequence: 12, Tick: 6, EventTime: eventTime},
			Payload: factoryEventPayload(t, factoryapi.ArtifactCreatedEventPayload{
				Artifact: factoryapi.FactoryArtifact{
					Id:         "artifact-result-1",
					Kind:       factoryapi.FactoryArtifactKindFINALRESULT,
					Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
					Label:      stringPtr("Review summary"),
					Summary:    stringPtr("Completed review findings"),
				},
				CapturedAt: &eventTime,
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
	events := make([]factoryapi.FactoryEvent, 0, 10)
	events = append(events, generatedFactoryDispatchEvents(t)...)
	events = append(events, generatedFactoryIgnoredResultEvents(t)...)
	events = append(events, generatedFactoryWorkStateChangeEvents(t)...)
	events = append(events, generatedFactoryModelEvents(t)...)
	events = append(events, generatedFactoryInferenceEvents(t)...)
	events = append(events, generatedFactoryScriptEvents(t)...)
	events = append(events, generatedFactoryAgentRunEvents(t)...)
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
			Id:            "event-human-approval-requested",
			Type:          factoryapi.FactoryEventTypeHumanApprovalRequested,
			Context: factoryapi.FactoryEventContext{
				Sequence:   3,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &scriptDispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.HumanApprovalRequestedEventPayload{
				ApprovalId:    "approval-dispatch-script-1",
				Decisions:     []factoryapi.HumanApprovalRequestedEventPayloadDecisions{factoryapi.HumanApprovalRequestedEventPayloadDecisionsAPPROVE, factoryapi.HumanApprovalRequestedEventPayloadDecisionsREJECT},
				Status:        factoryapi.HumanApprovalRequestedEventPayloadStatusPENDING,
				WorkstationId: "transition-1",
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-dispatch-worker-session-association",
			Type:          factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
			Context: factoryapi.FactoryEventContext{
				Sequence:   3,
				Tick:       2,
				EventTime:  eventTime,
				DispatchId: &scriptDispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.DispatchWorkerSessionAssociationEventPayload{
				WorkerSessionId: "worker-session-1",
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

func generatedFactoryWorkStateChangeEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	requestID := "move-request-1"
	workIDs := []string{"work-1"}
	source := factoryapi.WorkStateChangeSourceCLI
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-work-state-change-operator",
			Type:          factoryapi.FactoryEventTypeWorkStateChange,
			Context: factoryapi.FactoryEventContext{
				Sequence:  8,
				Tick:      3,
				EventTime: eventTime,
				RequestId: &requestID,
				WorkIds:   &workIDs,
			},
			Payload: factoryEventPayload(t, factoryapi.WorkStateChangeEventPayload{
				WorkId:       "work-1",
				WorkTypeName: "task",
				FromState:    "failed",
				ToState:      "in-progress",
				FromPlaceId:  "task:failed",
				ToPlaceId:    "task:in-progress",
				Source:       source,
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

func generatedFactoryModelEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	dispatchID := "dispatch-1"
	resourceType := factoryapi.ResourceTypeModel
	responseContent := factoryapi.WorkContent{
		mustGeneratedModelAudioPart(t, "/tmp/factory/out.wav"),
	}
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-model-request",
			Type:          factoryapi.FactoryEventTypeModelRequest,
			Context: factoryapi.FactoryEventContext{
				Sequence:   3,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &dispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.ModelRequestEventPayload{
				ModelRequestId:   "model-request-1",
				Attempt:          1,
				Operation:        "TTS",
				Worker:           "tts-worker",
				Model:            "OMNIVOICE_Q4_K_M",
				ProviderLocality: "LOCAL",
				Resources: &[]factoryapi.ModelResourceSummary{{
					Name:       "omnivoice-cache",
					Type:       resourceType,
					Capacity:   1,
					Model:      stringPtr("OMNIVOICE_Q4_K_M"),
					Backend:    stringPtr("LLAMACPP"),
					LoadPolicy: stringPtr("ON_DEMAND"),
				}},
				Bindings: &[]factoryapi.ResolvedModelOperationBinding{{
					Slot:   "text",
					Source: factoryapi.INPUT,
				}},
				WorkingDirectory: stringPtr("/tmp/factory/work"),
				Worktree:         stringPtr("/tmp/factory/worktree"),
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-model-response",
			Type:          factoryapi.FactoryEventTypeModelResponse,
			Context: factoryapi.FactoryEventContext{
				Sequence:   4,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &dispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.ModelResponseEventPayload{
				ModelRequestId:   "model-request-1",
				Attempt:          1,
				Operation:        "TTS",
				Worker:           "tts-worker",
				Model:            "OMNIVOICE_Q4_K_M",
				ProviderLocality: "LOCAL",
				Outcome:          factoryapi.InferenceOutcomeSucceeded,
				DurationMillis:   175,
				ResourceAcquired: boolPtr(true),
				LoadRequested:    boolPtr(true),
				LoadDurationMillis: func() *int64 {
					v := int64(40)
					return &v
				}(),
				OutputContent: &responseContent,
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

func assertRFC3339TimestampWithTimezone(t *testing.T, field string, value string) {
	t.Helper()
	if strings.HasPrefix(value, "0001-01-01") {
		t.Fatalf("%s = %q, want omitted optional time instead of Go zero-time string", field, value)
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		t.Fatalf("%s = %q, want RFC3339/RFC3339Nano timestamp: %v", field, value, err)
	}
	if !strings.HasSuffix(value, "Z") && !strings.Contains(value[10:], "+") && !strings.Contains(value[10:], "-") {
		t.Fatalf("%s = %q, want explicit timezone offset", field, value)
	}
}

func assertMachineTimePayloadValues(t *testing.T, value any, path string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "." + key
			switch key {
			case "durationNanos":
				t.Fatalf("%s must not be used; public elapsed durations use durationMillis", childPath)
			case "durationMillis":
				if _, ok := child.(float64); !ok {
					t.Fatalf("%s = %#v, want JSON number", childPath, child)
				}
			case "recordedAt", "finishedAt", "startedAt", "eventTime":
				if text, ok := child.(string); ok {
					assertRFC3339TimestampWithTimezone(t, childPath, text)
				}
			}
			assertMachineTimePayloadValues(t, child, childPath)
		}
	case []any:
		for _, child := range typed {
			assertMachineTimePayloadValues(t, child, path+"[]")
		}
	}
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
	assertTextOmitsInternalResponseStreamTerms(t, string(encoded))
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

func mustGeneratedModelAudioPart(t *testing.T, file string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkAudioContentPart(factoryapi.WorkAudioContentPart{
		Type: factoryapi.WorkContentPartTypeAudio,
		Url:  factoryapi.WorkContentURLProperty("file://" + file),
	}); err != nil {
		t.Fatalf("build generated audio part: %v", err)
	}
	return part
}
