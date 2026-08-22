package http

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldStateMapsGeneratedEventsToOwnerReducer(t *testing.T) {
	eventTime := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(`{
		"context":{"eventTime":"2026-07-16T03:00:00Z","sequence":1,"tick":1},
		"id":"evt-run-response","payload":{"status":"COMPLETED"},
		"schemaVersion":"agent-factory.event.v1","type":"RUN_RESPONSE"
	}`), &event); err != nil {
		t.Fatalf("decode generated event fixture: %v", err)
	}

	var reduced []interfaces.FactoryEvent
	state, err := ReconstructFactoryWorldState(func(events []interfaces.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
		reduced = append(reduced, events...)
		return interfaces.FactoryWorldState{Tick: selectedTick, EventTime: events[0].Context.EventTime}, nil
	}, []factoryapi.FactoryEvent{event}, 1)
	if err != nil {
		t.Fatalf("reconstruct mapped world state: %v", err)
	}
	if len(reduced) != 1 || reduced[0].Type != interfaces.FactoryEventTypeRunResponse || !state.EventTime.Equal(eventTime) || state.Tick != 1 {
		t.Fatalf("reduced=%#v state=%#v, want one RUN_RESPONSE at tick 1", reduced, state)
	}
}

func TestReconstructFactoryWorldStatePreservesEmptyInput(t *testing.T) {
	state, err := ReconstructFactoryWorldState(func(events []interfaces.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
		if len(events) != 0 {
			t.Fatalf("canonical reducer events = %#v, want empty", events)
		}
		return interfaces.FactoryWorldState{Tick: selectedTick}, nil
	}, nil, 4)
	if err != nil || state.Tick != 4 || !state.EventTime.IsZero() {
		t.Fatalf("state=%#v err=%v, want selected tick without event time", state, err)
	}
}

func TestCanonicalFactoryEventProjectsProviderSessionToContinuation(t *testing.T) {
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(`{
		"context":{"eventTime":"2026-07-16T03:00:00Z","sequence":1,"tick":1},
		"id":"evt-model-response","payload":{"outcome":"FAILED","providerSession":{"provider":"antigravity","kind":"session_id","id":"session-1"}},
		"schemaVersion":"agent-factory.event.v1","type":"MODEL_RESPONSE"
	}`), &event); err != nil {
		t.Fatalf("decode provider-session event: %v", err)
	}

	canonical, err := CanonicalFactoryEvent(event)
	if err != nil {
		t.Fatalf("canonical event: %v", err)
	}
	var payload workerexecution.ModelResponseEventPayload
	if err := canonical.DecodePayload(&payload); err != nil {
		t.Fatalf("decode canonical model response: %v", err)
	}
	if payload.ProviderSession != nil || payload.Continuation == nil || payload.Continuation.Provider != "antigravity" || payload.Continuation.ProviderSessionID != "session-1" {
		t.Fatalf("canonical payload = %#v, want provider continuation", payload)
	}
}

func TestCanonicalFactoryEventRejectsMalformedExecutionPayload(t *testing.T) {
	event := factoryapi.FactoryEvent{Type: factoryapi.FactoryEventTypeModelResponse}
	encoded, err := json.Marshal(map[string]any{
		"context": event.Context, "id": "malformed", "payload": "not an object",
		"schemaVersion": event.SchemaVersion, "type": event.Type,
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

func TestGeneratedWorkstationProjectionMapsOptionalRequestAndResponseFields(t *testing.T) {
	workstationName, traceID, workTypeID := "reviewer", "trace-1", "review"
	logicalWorkID, lineageParent, state := "logical-1", "parent-1", "READY"
	displayName, command, scriptRequestID := "Review task", "verify.sh", "script-request-1"
	feedback, outcome, classification := "looks good", "SUCCEEDED", "accepted"
	currentTrace := "trace-current"
	workDepth := 2
	attempt := 1
	duration := int64(42)
	exitCode := 0
	startedAt := time.Date(2026, time.August, 16, 1, 2, 3, 0, time.UTC)
	endedAt := startedAt.Add(time.Second)
	content := []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "review this"},
		{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"answer":42}`)},
	}
	inputTypes := []string{"review"}
	traceIDs := []string{"trace-1"}
	previousTraces := []string{"trace-previous"}
	lineageContinuity := recordings.WorkstationFactoryWorldWorkItemRefLineageContinuity("NEW_DOWNSTREAM_WORK")
	lineageSource := recordings.WorkstationFactoryWorldWorkItemRefLineageSourceKind("USER")
	payloadStatus := recordings.WorkstationFactoryWorldWorkItemRefPayloadStatus("RESOLVED")
	workItems := []recordings.WorkstationFactoryWorldWorkItemRef{{
		ChainingTraceDepth: &workDepth, Content: &content, CurrentChainingTraceId: &currentTrace,
		DisplayName: &displayName, LineageContinuity: &lineageContinuity, LineageLogicalWorkId: &logicalWorkID,
		LineageParentWorkIds: &[]string{lineageParent}, LineageSourceKind: &lineageSource,
		PayloadStatus: &payloadStatus, PayloadUnavailableReason: &feedback, PreviousChainingTraceIds: &previousTraces,
		State: &state, TraceId: &traceID, WorkId: "work-1", WorkTypeId: &workTypeID,
	}}
	tags := recordings.WorkstationStringMap{"priority": "high"}
	token := recordings.WorkstationFactoryWorldTokenView{
		ChainingTraceDepth: &workDepth, CurrentChainingTraceId: &currentTrace, Name: &displayName,
		PlaceId: "review.ready", PreviousChainingTraceIds: &previousTraces, Tags: &tags,
		TokenId: "token-1", TraceId: &traceID, WorkId: &logicalWorkID, WorkTypeId: &workTypeID,
	}
	tokens := []recordings.WorkstationFactoryWorldTokenView{token}
	mutationReason := "completed"
	fromPlace := "review.ready"
	toPlace := "review.done"
	mutations := []recordings.WorkstationFactoryWorldMutationView{{
		FromPlace: &fromPlace, Reason: &mutationReason, ToPlace: &toPlace, Token: &token,
		TokenId: "token-1", Type: "MOVE",
	}}
	runnerID := recordings.WorkstationRunnerID("runner-1")
	selectionSource := recordings.WorkstationRunnerSelectionSource("configured")
	capabilityDetail := "supports review"
	runner := &recordings.WorkstationFactoryWorldSelectedRunnerView{
		Capabilities: &recordings.WorkstationFactoryWorldRunnerCapabilitiesView{
			BaselineCapabilities: []recordings.WorkstationFactoryWorldRunnerBaselineCapability{"execute"},
			OptionalCapabilities: []recordings.WorkstationFactoryWorldRunnerOptionalCapabilitySupportView{{
				Capability: "review", Detail: &capabilityDetail, Status: "SUPPORTED",
			}},
		},
		DisplayName: &displayName, RunnerId: &runnerID, SelectionSource: &selectionSource,
	}
	requestScript := &recordings.WorkstationFactoryWorldScriptRequestView{
		Args: &[]string{"--strict"}, Attempt: &attempt, Command: &command, ScriptRequestId: &scriptRequestID,
	}
	responseScript := &recordings.WorkstationFactoryWorldScriptResponseView{
		Attempt: &attempt, DurationMillis: &duration, ExitCode: &exitCode, FailureType: &classification,
		Outcome: &outcome, ScriptRequestId: &scriptRequestID, Stderr: &feedback, Stdout: &command,
	}
	response := &recordings.WorkstationFactoryWorldWorkstationRequestResponseView{
		AgentRunInspection: &workerexecution.SafeAgentRunDiagnostic{ExecutionBehavior: "tool_use", ToolCallCount: 2},
		DurationMillis:     &duration, EndTime: &endedAt,
		FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeTimeout, Message: "retryable"},
		Feedback:      &feedback, Outcome: &outcome, OutputMutations: &mutations, OutputWorkItems: &workItems,
		Runner: runner, ScriptResponse: responseScript, SelectedClassificationLabel: &classification,
	}
	emptyStrings := []string{}
	emptyItems := []recordings.WorkstationFactoryWorldWorkItemRef{}
	emptyTokens := []recordings.WorkstationFactoryWorldTokenView{}
	emptyMutations := []recordings.WorkstationFactoryWorldMutationView{}
	projection := map[string]recordings.WorkstationFactoryWorldWorkstationRequestView{
		"dispatch-rich": {
			Counts:     recordings.WorkstationFactoryWorldWorkstationRequestCountView{DispatchedCount: 1, ErroredCount: 2, RespondedCount: 3},
			DispatchId: "dispatch-rich", Request: recordings.WorkstationFactoryWorldWorkstationRequestRequestView{
				ConsumedTokens: &tokens, CurrentChainingTraceId: &currentTrace, InputWorkItems: &workItems,
				InputWorkTypeIds: &inputTypes, PreviousChainingTraceIds: &previousTraces, Runner: runner,
				ScriptRequest: requestScript, StartedAt: &startedAt, TraceIds: &traceIDs,
			}, Response: response, TransitionId: "transition-1", WorkstationName: &workstationName,
		},
		"dispatch-sparse": {
			Request: recordings.WorkstationFactoryWorldWorkstationRequestRequestView{
				ConsumedTokens: &emptyTokens, InputWorkItems: &emptyItems, InputWorkTypeIds: &emptyStrings,
				PreviousChainingTraceIds: &emptyStrings, TraceIds: &emptyStrings,
			}, Response: &recordings.WorkstationFactoryWorldWorkstationRequestResponseView{
				OutputMutations: &emptyMutations, OutputWorkItems: &emptyItems,
			},
		},
	}
	generated := Generated(recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{WorkstationRequestsByDispatchId: &projection})
	assertGeneratedWorkstationProjection(t, generated, workstationName, runnerID)
}

func assertGeneratedWorkstationProjection(
	t *testing.T,
	generated factoryapi.FactoryWorldWorkstationRequestProjectionSlice,
	workstationName string,
	runnerID recordings.WorkstationRunnerID,
) {
	t.Helper()
	if generated.WorkstationRequestsByDispatchId == nil || len(*generated.WorkstationRequestsByDispatchId) != 2 {
		t.Fatalf("generated projection = %#v, want two dispatch views", generated)
	}
	got := (*generated.WorkstationRequestsByDispatchId)["dispatch-rich"]
	assertGeneratedWorkstationIdentity(t, got, workstationName, runnerID)
	assertGeneratedWorkstationInputs(t, got)
	assertGeneratedWorkstationResponse(t, got)
	assertGeneratedWorkstationSparseAndEmpty(t, generated)
}

func assertGeneratedWorkstationIdentity(
	t *testing.T,
	got factoryapi.FactoryWorldWorkstationRequestView,
	workstationName string,
	runnerID recordings.WorkstationRunnerID,
) {
	t.Helper()
	if got.DispatchId != "dispatch-rich" || got.Counts.RespondedCount != 3 || got.WorkstationName == nil || *got.WorkstationName != workstationName {
		t.Fatalf("generated request view = %#v, want mapped identity/count/name", got)
	}
	if got.Request.Runner == nil || got.Request.Runner.RunnerId == nil || *got.Request.Runner.RunnerId != factoryapi.RunnerID(runnerID) {
		t.Fatalf("generated runner = %#v, want runner identity", got.Request.Runner)
	}
}

func assertGeneratedWorkstationInputs(t *testing.T, got factoryapi.FactoryWorldWorkstationRequestView) {
	t.Helper()
	if got.Request.ConsumedTokens == nil || len(*got.Request.ConsumedTokens) != 1 || (*got.Request.ConsumedTokens)[0].Tags == nil {
		t.Fatalf("generated consumed tokens = %#v, want tagged token", got.Request.ConsumedTokens)
	}
	if got.Request.InputWorkItems == nil || len(*got.Request.InputWorkItems) != 1 || (*got.Request.InputWorkItems)[0].Content == nil {
		t.Fatalf("generated input work items = %#v, want content", got.Request.InputWorkItems)
	}
}

func assertGeneratedWorkstationResponse(t *testing.T, got factoryapi.FactoryWorldWorkstationRequestView) {
	t.Helper()
	if got.Response == nil || got.Response.FailureDetail == nil || got.Response.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout || got.Response.ScriptResponse == nil {
		t.Fatalf("generated response = %#v, want failure and script response", got.Response)
	}
}

func assertGeneratedWorkstationSparseAndEmpty(
	t *testing.T,
	generated factoryapi.FactoryWorldWorkstationRequestProjectionSlice,
) {
	t.Helper()
	if sparse := (*generated.WorkstationRequestsByDispatchId)["dispatch-sparse"]; sparse.Response == nil || sparse.Response.OutputMutations != nil || sparse.Request.InputWorkItems != nil {
		t.Fatalf("sparse generated projection = %#v, want empty slices omitted", sparse)
	}
	if got := Generated(recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}); got.WorkstationRequestsByDispatchId != nil {
		t.Fatalf("nil projection = %#v, want empty", got)
	}
	emptyProjection := map[string]recordings.WorkstationFactoryWorldWorkstationRequestView{}
	if got := Generated(recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{WorkstationRequestsByDispatchId: &emptyProjection}); got.WorkstationRequestsByDispatchId != nil {
		t.Fatalf("empty projection = %#v, want empty", got)
	}
}
