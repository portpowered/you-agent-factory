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

var canonicalFactoryEventTypes = []factoryapi.FactoryEventType{
	factoryapi.FactoryEventTypeRunRequest,
	factoryapi.FactoryEventTypeInitialStructureRequest,
	factoryapi.FactoryEventTypeWorkRequest,
	factoryapi.FactoryEventTypeRelationshipChangeRequest,
	factoryapi.FactoryEventTypeDispatchRequest,
	factoryapi.FactoryEventTypeInferenceRequest,
	factoryapi.FactoryEventTypeInferenceResponse,
	factoryapi.FactoryEventTypeScriptRequest,
	factoryapi.FactoryEventTypeScriptResponse,
	factoryapi.FactoryEventTypeDispatchResponse,
	factoryapi.FactoryEventTypeFactoryStateResponse,
	factoryapi.FactoryEventTypeRunResponse,
}

var retiredFactoryEventTypeStrings = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
}

func TestGeneratedOpenAPIContractsCompile(t *testing.T) {
	var submitRequest factoryapi.SubmitWorkRequest
	submitRequest.WorkTypeName = "task"
	submitRequest.CurrentChainingTraceId = stringPtr("chain-submit-1")
	submitRelationState := "complete"
	submitRequest.Relations = &[]factoryapi.SubmitRelation{{
		Type:          factoryapi.RelationTypeDependsOn,
		TargetWorkId:  "work-1",
		RequiredState: &submitRelationState,
	}}

	workID := "work-1"
	requestID := "request-1"
	traceID := "trace-1"
	currentChainingTraceID := "chain-work-1"
	previousChainingTraceIDs := []string{"chain-a", "chain-z"}
	initialState := "queued"
	tags := factoryapi.StringMap{"priority": "high"}
	batchWork := factoryapi.Work{
		Name:                     "draft",
		WorkId:                   &workID,
		RequestId:                &requestID,
		WorkTypeName:             stringPtr("task"),
		State:                    &initialState,
		CurrentChainingTraceId:   &currentChainingTraceID,
		PreviousChainingTraceIds: &previousChainingTraceIDs,
		TraceId:                  &traceID,
		Payload:                  map[string]any{"title": "first draft"},
		Tags:                     &tags,
	}
	relation := factoryapi.Relation{
		Type:           factoryapi.RelationTypeDependsOn,
		SourceWorkName: "publish",
		TargetWorkName: "draft",
		RequiredState:  stringPtr("complete"),
	}
	parentChildRelation := factoryapi.Relation{
		Type:           factoryapi.RelationTypeParentChild,
		SourceWorkName: "draft",
		TargetWorkName: "epic",
	}
	workstationKind := factoryapi.WorkstationKindCron
	workstationRuntimeType := factoryapi.WorkstationTypeModelWorkstation
	workstation := factoryapi.Workstation{
		Name:     "daily-refresh",
		Behavior: &workstationKind,
		Type:     &workstationRuntimeType,
		Worker:   "agent",
		Inputs:   []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
		Outputs:  []factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
		OnContinue: &[]factoryapi.WorkstationIO{
			{WorkType: "task", State: "init"},
			{WorkType: "task", State: "retry"},
		},
		OnRejection: &[]factoryapi.WorkstationIO{
			{WorkType: "task", State: "rejected"},
		},
		OnFailure: &[]factoryapi.WorkstationIO{
			{WorkType: "task", State: "failed"},
		},
	}
	workRequest := factoryapi.WorkRequest{
		RequestId:              requestID,
		CurrentChainingTraceId: stringPtr("chain-request-1"),
		Type:                   factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:                  &[]factoryapi.Work{batchWork},
		Relations:              &[]factoryapi.Relation{relation, parentChildRelation},
	}
	submitResponse := factoryapi.SubmitWorkResponse{TraceId: "trace-1"}
	upsertResponse := factoryapi.UpsertWorkRequestResponse{RequestId: requestID, TraceId: "trace-1"}
	namedFactory := factoryapi.Factory{
		Name:         factoryapi.FactoryName("customer-support-triage"),
		Workstations: &[]factoryapi.Workstation{workstation},
	}
	triggerAtStart := true
	cron := factoryapi.WorkstationCron{
		Schedule:       "*/5 * * * *",
		TriggerAtStart: &triggerAtStart,
	}
	if submitRequest.WorkTypeName == "" || submitResponse.TraceId == "" || workRequest.RequestId == "" || upsertResponse.RequestId == "" || namedFactory.Name == "" || namedFactory.Workstations == nil || workstation.Behavior == nil || workstation.Type == nil || cron.Schedule == "" || cron.TriggerAtStart == nil {
		t.Fatal("generated OpenAPI request and response types should be usable")
	}
	if submitRequest.CurrentChainingTraceId == nil || *submitRequest.CurrentChainingTraceId != "chain-submit-1" {
		t.Fatal("generated submit request should expose current chaining trace ID")
	}
	submitRequestJSON, err := json.Marshal(submitRequest)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	if !strings.Contains(string(submitRequestJSON), `"relations"`) || !strings.Contains(string(submitRequestJSON), `"targetWorkId":"work-1"`) {
		t.Fatalf("generated submit request JSON must preserve token-level relations: %s", submitRequestJSON)
	}
	if workRequest.Relations == nil || len(*workRequest.Relations) != 2 || (*workRequest.Relations)[1].Type != factoryapi.RelationTypeParentChild {
		t.Fatal("generated work request relations should advertise parent-child support")
	}
	if workRequest.Works == nil || len(*workRequest.Works) != 1 || (*workRequest.Works)[0].State == nil || *(*workRequest.Works)[0].State != initialState {
		t.Fatal("generated work request works should advertise explicit state support")
	}
	if workRequest.CurrentChainingTraceId == nil || *workRequest.CurrentChainingTraceId != "chain-request-1" || (*workRequest.Works)[0].CurrentChainingTraceId == nil || *(*workRequest.Works)[0].CurrentChainingTraceId != currentChainingTraceID {
		t.Fatal("generated work request contracts should expose current chaining trace IDs")
	}
	if (*workRequest.Works)[0].PreviousChainingTraceIds == nil || len(*(*workRequest.Works)[0].PreviousChainingTraceIds) != 2 {
		t.Fatal("generated work request contracts should expose predecessor chaining trace IDs")
	}
}

func TestGeneratedFactoryContractsCompileAndRoundTrip(t *testing.T) {
	namedFactory := generatedNamedFactoryFixture()

	assertGeneratedNamedFactoryContracts(t, namedFactory)
	assertGeneratedNamedFactoryJSONRoundTrip(t, namedFactory)
	assertGeneratedReservedCurrentFactoryJSONRoundTrip(t, namedFactory)
	assertGeneratedCurrentFactoryNotFoundJSON(t)
}

func generatedNamedFactoryFixture() factoryapi.Factory {
	return factoryapi.Factory{
		Name: "customer-support-triage",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name:             "planner",
			Type:             workerTypePtr(factoryapi.WorkerTypeModelWorker),
			ModelProvider:    workerModelProviderPtr(factoryapi.WorkerModelProviderClaude),
			ExecutorProvider: workerProviderPtr(factoryapi.WorkerProviderScriptWrap),
			Model:            stringPtr("claude-sonnet-4-20250514"),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Inputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
			Outputs: []factoryapi.WorkstationIO{{
				WorkType: "task",
				State:    "done",
			}},
			OnContinue: &[]factoryapi.WorkstationIO{
				{WorkType: "task", State: "init"},
				{WorkType: "task", State: "queued"},
			},
			OnRejection: &[]factoryapi.WorkstationIO{
				{WorkType: "task", State: "review"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "task", State: "failed"},
			},
		}},
	}
}

func assertGeneratedNamedFactoryContracts(t *testing.T, namedFactory factoryapi.Factory) {
	t.Helper()

	createRequest := factoryapi.CreateFactoryJSONRequestBody(namedFactory)
	current := namedFactory
	badRequest := factoryapi.CreateFactoryBadRequest{
		Code:    factoryapi.INVALIDFACTORYNAME,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Message: "factory name must use lowercase letters, numbers, and hyphens",
	}
	conflict := factoryapi.CreateFactoryConflict{
		Code:    factoryapi.FACTORYALREADYEXISTS,
		Family:  factoryapi.ErrorFamilyConflict,
		Message: "factory already exists",
	}

	if createRequest.Name == "" || createRequest.WorkTypes == nil || createRequest.Workers == nil || createRequest.Workstations == nil {
		t.Fatal("generated named-factory request and response types should be usable")
	}
	if current.Name == "" || current.Workstations == nil {
		t.Fatal("generated current named-factory response type should be usable")
	}
	if badRequest.Code != factoryapi.INVALIDFACTORYNAME || badRequest.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("generated bad-request contract = %#v, want code %q and family %q", badRequest, factoryapi.INVALIDFACTORYNAME, factoryapi.ErrorFamilyBadRequest)
	}
	if conflict.Code != factoryapi.FACTORYALREADYEXISTS || conflict.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("generated conflict contract = %#v, want code %q and family %q", conflict, factoryapi.FACTORYALREADYEXISTS, factoryapi.ErrorFamilyConflict)
	}
}

func assertGeneratedNamedFactoryJSONRoundTrip(t *testing.T, namedFactory factoryapi.Factory) {
	t.Helper()

	encoded, err := json.Marshal(namedFactory)
	if err != nil {
		t.Fatalf("marshal generated NamedFactory: %v", err)
	}
	if !strings.Contains(string(encoded), `"name":"customer-support-triage"`) {
		t.Fatalf("generated NamedFactory JSON missing canonical name field: %s", encoded)
	}
	if strings.Contains(string(encoded), `"factory_name"`) {
		t.Fatalf("generated NamedFactory JSON contains unexpected legacy field: %s", encoded)
	}

	var roundTripped factoryapi.Factory
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal generated NamedFactory: %v", err)
	}
	if roundTripped.Name != namedFactory.Name {
		t.Fatalf("round-tripped named factory name = %q, want %q", roundTripped.Name, namedFactory.Name)
	}
	if roundTripped.Workstations == nil || len(*roundTripped.Workstations) != 1 || (*roundTripped.Workstations)[0].Worker != "planner" {
		t.Fatalf("round-tripped named factory workstations = %#v, want planner workstation", roundTripped.Workstations)
	}
	workstation := (*roundTripped.Workstations)[0]
	if workstation.OnContinue == nil || len(*workstation.OnContinue) != 2 {
		t.Fatalf("round-tripped workstation onContinue = %#v, want two array routes", workstation.OnContinue)
	}
	if workstation.OnRejection == nil || len(*workstation.OnRejection) != 1 || (*workstation.OnRejection)[0].State != "review" {
		t.Fatalf("round-tripped workstation onRejection = %#v, want review route", workstation.OnRejection)
	}
	if workstation.OnFailure == nil || len(*workstation.OnFailure) != 1 || (*workstation.OnFailure)[0].State != "failed" {
		t.Fatalf("round-tripped workstation onFailure = %#v, want failed route", workstation.OnFailure)
	}
}

func assertGeneratedReservedCurrentFactoryJSONRoundTrip(t *testing.T, namedFactory factoryapi.Factory) {
	t.Helper()

	namedFactory.Name = "UNDEFINED"
	encoded, err := json.Marshal(namedFactory)
	if err != nil {
		t.Fatalf("marshal generated current Factory: %v", err)
	}
	if !strings.Contains(string(encoded), `"name":"UNDEFINED"`) {
		t.Fatalf("generated current Factory JSON missing reserved current-factory name: %s", encoded)
	}

	var roundTripped factoryapi.Factory
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal generated current Factory: %v", err)
	}
	if roundTripped.Name != "UNDEFINED" {
		t.Fatalf("round-tripped current factory name = %q, want %q", roundTripped.Name, "UNDEFINED")
	}
}

func assertGeneratedCurrentFactoryNotFoundJSON(t *testing.T) {
	t.Helper()

	notFound := factoryapi.CurrentFactoryNotFound{
		Code:    factoryapi.NOTFOUND,
		Family:  factoryapi.ErrorFamilyNotFound,
		Message: "current factory not found",
	}
	if notFound.Code != factoryapi.NOTFOUND || notFound.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("generated not-found contract = %#v, want code %q and family %q", notFound, factoryapi.NOTFOUND, factoryapi.ErrorFamilyNotFound)
	}

	encoded, err := json.Marshal(notFound)
	if err != nil {
		t.Fatalf("marshal generated CurrentFactoryNotFound: %v", err)
	}
	if !strings.Contains(string(encoded), `"code":"NOT_FOUND"`) {
		t.Fatalf("generated CurrentFactoryNotFound JSON missing NOT_FOUND code: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"family":"NOT_FOUND"`) {
		t.Fatalf("generated CurrentFactoryNotFound JSON missing NOT_FOUND family: %s", encoded)
	}
}

// portos:func-length-exception owner=agent-factory reason=generated-event-contract-fixture review=2026-07-18 removal=split-payload-fixtures-before-next-event-contract-expansion
func TestGeneratedFactoryEventContractsCompile(t *testing.T) {
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	requestID := "request-1"
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	dispatchID := "dispatch-1"
	scriptDispatchID := "dispatch-script-1"
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
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("unmarshal canonical event fixture: %v", err)
	}
	if len(events) != len(canonicalFactoryEventTypes) {
		t.Fatalf("canonical event fixture count = %d, want %d", len(events), len(canonicalFactoryEventTypes))
	}

	seen := make(map[factoryapi.FactoryEventType]int, len(events))
	for _, event := range events {
		seen[event.Type]++
		requireGeneratedFactoryEventPayloadRoundTrip(t, event)

		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal canonical event %s: %v", event.Id, err)
		}
		assertTextOmitsRetiredEventNames(t, string(encoded))

		var roundTripped factoryapi.FactoryEvent
		if err := json.Unmarshal(encoded, &roundTripped); err != nil {
			t.Fatalf("round-trip unmarshal canonical event %s: %v", event.Id, err)
		}
		if roundTripped.Type != event.Type {
			t.Fatalf("round-tripped event %s type = %q, want %q", event.Id, roundTripped.Type, event.Type)
		}
		requireGeneratedFactoryEventPayloadRoundTrip(t, roundTripped)
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
	input := []byte(`{
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

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(input, &event); err != nil {
		t.Fatalf("unmarshal generated FactoryEvent: %v", err)
	}
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

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal generated FactoryEvent: %v", err)
	}
	var roundTripped map[string]any
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal generated FactoryEvent JSON: %v", err)
	}
	payloadJSON, _ := roundTripped["payload"].(map[string]any)
	if _, ok := payloadJSON["dispatchId"]; ok {
		t.Fatalf("generated inference response payload must not reintroduce payload.dispatchId: %#v", payloadJSON)
	}
	if _, ok := payloadJSON["transitionId"]; ok {
		t.Fatalf("generated inference response payload must not reintroduce payload.transitionId: %#v", payloadJSON)
	}
}

func TestGeneratedScriptEventJSONRoundTripPreservesRequestCorrelationAndFailureShape(t *testing.T) {
	input := []byte(`{
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

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(input, &event); err != nil {
		t.Fatalf("unmarshal generated FactoryEvent: %v", err)
	}
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

// portos:func-length-exception owner=agent-factory reason=generated-event-json-fixture review=2026-07-18 removal=split-input-fixture-and-roundtrip-assertions-before-next-event-contract-expansion
func TestGeneratedFactoryEventJSONRoundTripPreservesWorkRequestContextAndWorks(t *testing.T) {
	input := []byte(`{
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

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(input, &event); err != nil {
		t.Fatalf("unmarshal generated FactoryEvent: %v", err)
	}
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

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal generated FactoryEvent: %v", err)
	}
	var roundTripped factoryapi.FactoryEvent
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped FactoryEvent: %v", err)
	}
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
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal run-request factory event: %v", err)
	}
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

func requireGeneratedFactoryEventPayloadRoundTrip(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	switch event.Type {
	case factoryapi.FactoryEventTypeRunRequest:
		if _, err := event.Payload.AsRunRequestEventPayload(); err != nil {
			t.Fatalf("decode %s run-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeInitialStructureRequest:
		if _, err := event.Payload.AsInitialStructureRequestEventPayload(); err != nil {
			t.Fatalf("decode %s initial-structure payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeWorkRequest:
		if _, err := event.Payload.AsWorkRequestEventPayload(); err != nil {
			t.Fatalf("decode %s work-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeRelationshipChangeRequest:
		if _, err := event.Payload.AsRelationshipChangeRequestEventPayload(); err != nil {
			t.Fatalf("decode %s relationship-change payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeDispatchRequest:
		if _, err := event.Payload.AsDispatchRequestEventPayload(); err != nil {
			t.Fatalf("decode %s dispatch-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeInferenceRequest:
		if _, err := event.Payload.AsInferenceRequestEventPayload(); err != nil {
			t.Fatalf("decode %s inference-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeInferenceResponse:
		if _, err := event.Payload.AsInferenceResponseEventPayload(); err != nil {
			t.Fatalf("decode %s inference-response payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeScriptRequest:
		if _, err := event.Payload.AsScriptRequestEventPayload(); err != nil {
			t.Fatalf("decode %s script-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeScriptResponse:
		if _, err := event.Payload.AsScriptResponseEventPayload(); err != nil {
			t.Fatalf("decode %s script-response payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeDispatchResponse:
		if _, err := event.Payload.AsDispatchResponseEventPayload(); err != nil {
			t.Fatalf("decode %s dispatch-response payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeFactoryStateResponse:
		if _, err := event.Payload.AsFactoryStateResponseEventPayload(); err != nil {
			t.Fatalf("decode %s factory-state payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeRunResponse:
		if _, err := event.Payload.AsRunResponseEventPayload(); err != nil {
			t.Fatalf("decode %s run-response payload: %v", event.Id, err)
		}
	default:
		t.Fatalf("unexpected canonical event type %q", event.Type)
	}
}

func assertTextOmitsRetiredEventNames(t *testing.T, text string) {
	t.Helper()

	for _, retired := range retiredFactoryEventTypeStrings {
		if strings.Contains(text, `"`+retired+`"`) {
			t.Fatalf("unexpected retired public event name %q in artifact text", retired)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}

func workerModelProviderPtr(value factoryapi.WorkerModelProvider) *factoryapi.WorkerModelProvider {
	return &value
}

func workerProviderPtr(value factoryapi.WorkerProvider) *factoryapi.WorkerProvider {
	return &value
}

func workerTypePtr(value factoryapi.WorkerType) *factoryapi.WorkerType {
	return &value
}
func intPtr(value int) *int {
	return &value
}

func factoryEventPayload(t *testing.T, payload any) factoryapi.FactoryEvent_Payload {
	t.Helper()

	var eventPayload factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		err = eventPayload.FromRunRequestEventPayload(typed)
	case factoryapi.InitialStructureRequestEventPayload:
		err = eventPayload.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = eventPayload.FromWorkRequestEventPayload(typed)
	case factoryapi.RelationshipChangeRequestEventPayload:
		err = eventPayload.FromRelationshipChangeRequestEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	case factoryapi.InferenceRequestEventPayload:
		err = eventPayload.FromInferenceRequestEventPayload(typed)
	case factoryapi.InferenceResponseEventPayload:
		err = eventPayload.FromInferenceResponseEventPayload(typed)
	case factoryapi.ScriptRequestEventPayload:
		err = eventPayload.FromScriptRequestEventPayload(typed)
	case factoryapi.ScriptResponseEventPayload:
		err = eventPayload.FromScriptResponseEventPayload(typed)
	case factoryapi.DispatchResponseEventPayload:
		err = eventPayload.FromDispatchResponseEventPayload(typed)
	case factoryapi.FactoryStateResponseEventPayload:
		err = eventPayload.FromFactoryStateResponseEventPayload(typed)
	case factoryapi.RunResponseEventPayload:
		err = eventPayload.FromRunResponseEventPayload(typed)
	default:
		t.Fatalf("unsupported event payload type %T", payload)
	}
	if err != nil {
		t.Fatalf("encode generated FactoryEvent payload: %v", err)
	}
	return eventPayload
}
