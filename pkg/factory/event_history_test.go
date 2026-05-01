package factory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"
	"github.com/portpowered/agent-factory/pkg/workers"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/petri"
)

func TestFactoryEventHistory_RecordInitialStructure_UsesRuntimeConfigProjection(t *testing.T) {
	runtimeConfig := eventHistoryRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"builder": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "codex-cli",
				ModelProvider:    "openai",
				Model:            "gpt-5.4",
			},
		},
	}
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig,
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 {
		t.Fatalf("Workers = %#v, want one runtime worker", payload.Factory.Workers)
	}
	worker := (*payload.Factory.Workers)[0]
	if worker.Name != "builder" || stringValueForEventHistoryTest(worker.ExecutorProvider) != "SCRIPT_WRAP" ||
		stringValueForEventHistoryTest(worker.ModelProvider) != "CODEX" ||
		stringValueForEventHistoryTest(worker.Type) != string(factoryapi.WorkerTypeModelWorker) ||
		stringValueForEventHistoryTest(worker.Model) != "gpt-5.4" {
		t.Fatalf("worker metadata = %#v, want runtime-config provider/model metadata", worker)
	}
}

func TestFactoryEventHistory_RecordInitialStructure_EmitsCanonicalPublicWorkstationKinds(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryRuntimeConfig{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"build": {Name: "Build", Kind: interfaces.WorkstationKindRepeater},
			},
		},
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one generated workstation", payload.Factory.Workstations)
	}
	workstation := (*payload.Factory.Workstations)[0]
	if workstation.Kind == nil || *workstation.Kind != factoryapi.WorkstationKindRepeater {
		t.Fatalf("workstation kind = %#v, want REPEATER", workstation.Kind)
	}

	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal initial structure event: %v", err)
	}
	if !strings.Contains(string(data), `"kind":"REPEATER"`) {
		t.Fatalf("initial structure event JSON = %s, want canonical uppercase workstation kind", data)
	}
}

func TestFactoryEventHistory_RecordWorkstationRequest_UsesContextForRequestIdentity(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkstationRequest(4, interfaces.FactoryDispatchRecord{
		DispatchID:  "dispatch-1",
		CreatedTick: 4,
		Dispatch: interfaces.WorkDispatch{
			DispatchID:      "dispatch-1",
			TransitionID:    "build",
			WorkerType:      "builder",
			WorkstationName: "Build",
			Execution: interfaces.ExecutionMetadata{
				RequestID: "request-1",
				ReplayKey: "replay-1",
			},
		},
	}, eventTime)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].Type != factoryapi.FactoryEventTypeDispatchRequest {
		t.Fatalf("event type = %s, want %s", events[0].Type, factoryapi.FactoryEventTypeDispatchRequest)
	}

	payload, err := events[0].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch request payload: %v", err)
	}
	if stringValueForEventHistoryTest(events[0].Context.RequestId) != "request-1" {
		t.Fatalf("context requestId = %q, want request-1", stringValueForEventHistoryTest(events[0].Context.RequestId))
	}
	if payload.Metadata == nil {
		t.Fatal("metadata = nil, want replay metadata object")
	}
	if stringValueForEventHistoryTest(payload.Metadata.ReplayKey) != "replay-1" {
		t.Fatalf("metadata replayKey = %q, want replay-1", stringValueForEventHistoryTest(payload.Metadata.ReplayKey))
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_FailedResultIncludesFailureDetails(t *testing.T) {
	eventTime := time.Date(2026, 4, 17, 9, 30, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	result := interfaces.WorkResult{
		DispatchID:   "dispatch-failed",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
		Output:       "partial output",
		Error:        "provider error: throttled: selected model is at capacity",
		Feedback:     "retry later",
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyThrottle,
			Type:   interfaces.ProviderErrorTypeThrottled,
		},
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      "dispatch-failed",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeFailed,
		Reason:          result.Error,
		EndTime:         eventTime,
		Duration:        2 * time.Second,
	}

	history.RecordWorkstationResponse(9, result, completed)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].Type != factoryapi.FactoryEventTypeDispatchResponse {
		t.Fatalf("event type = %s, want %s", events[0].Type, factoryapi.FactoryEventTypeDispatchResponse)
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if stringValueForEventHistoryTest(payload.FailureReason) != "throttled" {
		t.Fatalf("failure reason = %q, want throttled", stringValueForEventHistoryTest(payload.FailureReason))
	}
	if stringValueForEventHistoryTest(payload.FailureMessage) != result.Error {
		t.Fatalf("failure message = %q, want %q", stringValueForEventHistoryTest(payload.FailureMessage), result.Error)
	}

	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	payloadObject := assertJSONObject(t, decoded, "payload")
	assertJSONField(t, payloadObject, "outcome", "FAILED")
	assertJSONField(t, payloadObject, "output", "partial output")
	assertJSONField(t, payloadObject, "error", result.Error)
	assertJSONField(t, payloadObject, "feedback", "retry later")
	assertJSONField(t, payloadObject, "failureReason", "throttled")
	assertJSONField(t, payloadObject, "failureMessage", result.Error)
	providerFailure := assertJSONObject(t, payloadObject, "providerFailure")
	assertJSONField(t, providerFailure, "family", "throttle")
	assertJSONField(t, providerFailure, "type", "throttled")
}

func TestFactoryEventHistory_RecordWorkstationResponse_CodexWindowsExitCode4294967295UsesRetryableProviderFailureMetadata(t *testing.T) {
	eventTime := time.Date(2026, 4, 21, 1, 15, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	errorText := "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)"
	result := interfaces.WorkResult{
		DispatchID:   "dispatch-codex-windows-4294967295",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
		Error:        errorText,
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeInternalServerError,
		},
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      result.DispatchID,
		TransitionID:    result.TransitionID,
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeFailed,
		Reason:          errorText,
		EndTime:         eventTime,
		Duration:        3 * time.Second,
	}

	history.RecordWorkstationResponse(12, result, completed)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if stringValueForEventHistoryTest(payload.FailureReason) != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("failure reason = %q, want %q", stringValueForEventHistoryTest(payload.FailureReason), interfaces.ProviderErrorTypeInternalServerError)
	}
	if stringValueForEventHistoryTest(payload.FailureMessage) != errorText {
		t.Fatalf("failure message = %q, want %q", stringValueForEventHistoryTest(payload.FailureMessage), errorText)
	}
	if payload.ProviderFailure == nil {
		t.Fatal("expected provider failure metadata on dispatch completed payload")
	}
	if stringValueForEventHistoryTest(payload.ProviderFailure.Family) != string(interfaces.ProviderErrorFamilyRetryable) {
		t.Fatalf("provider failure family = %q, want %q", stringValueForEventHistoryTest(payload.ProviderFailure.Family), interfaces.ProviderErrorFamilyRetryable)
	}
	if stringValueForEventHistoryTest(payload.ProviderFailure.Type) != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("provider failure type = %q, want %q", stringValueForEventHistoryTest(payload.ProviderFailure.Type), interfaces.ProviderErrorTypeInternalServerError)
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_OmitsRetiredProviderAttemptFields(t *testing.T) {
	eventTime := time.Date(2026, 4, 18, 10, 15, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkstationResponse(12, safeDiagnosticsWorkResult(), safeDiagnosticsCompletedDispatch(eventTime))

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertThinDispatchResponseSerializedEvent(t, events[0])
}

func TestFactoryEventHistory_RecordScriptEvent_AppendsScriptBoundaryEvents(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 14, 5, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	scriptRequestID := "dispatch-script/script-request/1"

	recordScriptBoundaryEvents(history, eventTime, scriptRequestID)

	events := history.Events()
	assertRecordedScriptBoundaryEvents(t, events)
	assertRecordedScriptRequestPayload(t, events[0], scriptRequestID)
	assertRecordedScriptResponsePayload(t, events[1], scriptRequestID)
}

func TestFactoryEventHistory_RecordScriptEvent_IgnoresNonScriptEvents(t *testing.T) {
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordScriptEvent(factoryEvent(
		factoryapi.FactoryEventTypeInferenceRequest,
		"factory-event/inference-request/dispatch-script/1",
		factoryapi.FactoryEventContext{
			Tick:       1,
			EventTime:  time.Unix(0, 0).UTC(),
			DispatchId: stringPtr("dispatch-script"),
		},
		factoryapi.InferenceRequestEventPayload{
			InferenceRequestId: "dispatch-script/inference-request/1",
			Attempt:            1,
			WorkingDirectory:   "/tmp/ignored",
			Worktree:           "/tmp/ignored/worktree",
			Prompt:             "ignored",
		},
	))

	if events := history.Events(); len(events) != 0 {
		t.Fatalf("event count = %d, want 0 when script recorder receives non-script event", len(events))
	}
}

func recordScriptBoundaryEvents(history *FactoryEventHistory, eventTime time.Time, scriptRequestID string) {
	context := factoryapi.FactoryEventContext{
		Tick:       14,
		EventTime:  eventTime,
		DispatchId: stringPtr("dispatch-script"),
		RequestId:  stringPtr("request-script"),
		TraceIds:   stringSlicePtr([]string{"trace-script"}),
		WorkIds:    stringSlicePtr([]string{"work-script-1", "work-script-2"}),
	}

	history.RecordScriptEvent(factoryEvent(
		factoryapi.FactoryEventTypeScriptRequest,
		"factory-event/script-request/dispatch-script/1",
		context,
		factoryapi.ScriptRequestEventPayload{
			ScriptRequestId: scriptRequestID,
			DispatchId:      "dispatch-script",
			TransitionId:    "build",
			Attempt:         1,
			Command:         "python",
			Args:            []string{"main.py", "--mode", "review"},
		},
	))
	history.RecordScriptEvent(factoryEvent(
		factoryapi.FactoryEventTypeScriptResponse,
		"factory-event/script-response/dispatch-script/1",
		context,
		factoryapi.ScriptResponseEventPayload{
			ScriptRequestId: scriptRequestID,
			DispatchId:      "dispatch-script",
			TransitionId:    "build",
			Attempt:         1,
			Outcome:         factoryapi.ScriptExecutionOutcomeSucceeded,
			Stdout:          "ok",
			Stderr:          "",
			DurationMillis:  1250,
		},
	))
}

func assertRecordedScriptBoundaryEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].Type != factoryapi.FactoryEventTypeScriptRequest {
		t.Fatalf("first event type = %s, want %s", events[0].Type, factoryapi.FactoryEventTypeScriptRequest)
	}
	if events[1].Type != factoryapi.FactoryEventTypeScriptResponse {
		t.Fatalf("second event type = %s, want %s", events[1].Type, factoryapi.FactoryEventTypeScriptResponse)
	}
	if events[0].Id != "factory-event/script-request/dispatch-script/1" {
		t.Fatalf("script request event id = %q, want stable request id", events[0].Id)
	}
	if events[1].Id != "factory-event/script-response/dispatch-script/1" {
		t.Fatalf("script response event id = %q, want stable response id", events[1].Id)
	}
	if events[0].Context.Sequence != 0 || events[1].Context.Sequence != 1 {
		t.Fatalf("event sequences = %d/%d, want 0/1", events[0].Context.Sequence, events[1].Context.Sequence)
	}
	if stringValueForEventHistoryTest(events[0].Context.DispatchId) != "dispatch-script" ||
		stringValueForEventHistoryTest(events[0].Context.RequestId) != "request-script" {
		t.Fatalf("script request context = %#v, want canonical dispatch/request correlation", events[0].Context)
	}
	if got := stringSliceValueForEventHistoryTest(events[0].Context.TraceIds); len(got) != 1 || got[0] != "trace-script" {
		t.Fatalf("trace IDs = %#v, want canonical trace correlation", got)
	}
	if got := stringSliceValueForEventHistoryTest(events[0].Context.WorkIds); len(got) != 2 || got[0] != "work-script-1" || got[1] != "work-script-2" {
		t.Fatalf("work IDs = %#v, want canonical work correlation", got)
	}
}

func assertRecordedScriptRequestPayload(t *testing.T, event factoryapi.FactoryEvent, scriptRequestID string) {
	t.Helper()

	requestPayload, err := event.Payload.AsScriptRequestEventPayload()
	if err != nil {
		t.Fatalf("script request payload: %v", err)
	}
	if requestPayload.ScriptRequestId != scriptRequestID ||
		requestPayload.DispatchId != "dispatch-script" ||
		requestPayload.TransitionId != "build" ||
		requestPayload.Attempt != 1 ||
		requestPayload.Command != "python" ||
		strings.Join(requestPayload.Args, ",") != "main.py,--mode,review" {
		t.Fatalf("script request payload = %#v, want canonical request fields", requestPayload)
	}
}

func assertRecordedScriptResponsePayload(t *testing.T, event factoryapi.FactoryEvent, scriptRequestID string) {
	t.Helper()

	responsePayload, err := event.Payload.AsScriptResponseEventPayload()
	if err != nil {
		t.Fatalf("script response payload: %v", err)
	}
	if responsePayload.ScriptRequestId != scriptRequestID ||
		responsePayload.DispatchId != "dispatch-script" ||
		responsePayload.TransitionId != "build" ||
		responsePayload.Attempt != 1 ||
		responsePayload.Outcome != factoryapi.ScriptExecutionOutcomeSucceeded ||
		responsePayload.Stdout != "ok" ||
		responsePayload.Stderr != "" ||
		responsePayload.DurationMillis != 1250 {
		t.Fatalf("script response payload = %#v, want canonical response fields", responsePayload)
	}
}

func TestFactoryEventHistory_RecordWorkRequest_PreservesGeneratedWorkChainingTraceLineage(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 0, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkRequest(7, interfaces.WorkRequestRecord{
		RequestID: "request-generated-lineage",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-generated-current",
		WorkItems: []interfaces.FactoryWorkItem{{
			ID:                       "work-generated-lineage",
			WorkTypeID:               "task",
			DisplayName:              "generated-lineage",
			CurrentChainingTraceID:   "trace-generated-current",
			PreviousChainingTraceIDs: []string{"trace-a", "trace-z"},
			TraceID:                  "trace-generated-current",
		}},
	}, eventTime)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if payload.Works == nil || len(*payload.Works) != 1 {
		t.Fatalf("payload works = %#v, want one generated work item", payload.Works)
	}
	work := (*payload.Works)[0]
	if stringValueForEventHistoryTest(work.CurrentChainingTraceId) != "trace-generated-current" {
		t.Fatalf("work current chaining trace ID = %q, want trace-generated-current", stringValueForEventHistoryTest(work.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(work.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("work previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
}

func TestFactoryEventHistory_RecordWorkstationEvents_PreserveChainingTraceLineage(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 5, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	consumed := chainingTraceLineageConsumedTokens()
	history.RecordWorkstationRequest(8, chainingTraceLineageDispatchRecord(consumed), eventTime)
	history.RecordWorkstationResponse(9, chainingTraceLineageResult(), chainingTraceLineageCompletion(eventTime, consumed))

	events := history.Events()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}

	assertEventHistoryRequestLineage(t, events[0])
	assertEventHistoryResponseLineage(t, events[1])
}

func chainingTraceLineageConsumedTokens() []interfaces.Token {
	return []interfaces.Token{
		{
			ID:      "tok-z",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				DataType:                 interfaces.DataTypeWork,
				WorkID:                   "work-z",
				WorkTypeID:               "task",
				Name:                     "source-z",
				RequestID:                "request-z",
				CurrentChainingTraceID:   "trace-z",
				PreviousChainingTraceIDs: []string{"trace-origin-z"},
				TraceID:                  "trace-z",
			},
		},
		{
			ID:      "tok-a",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				DataType:                 interfaces.DataTypeWork,
				WorkID:                   "work-a",
				WorkTypeID:               "task",
				Name:                     "source-a",
				RequestID:                "request-a",
				CurrentChainingTraceID:   "trace-a",
				PreviousChainingTraceIDs: []string{"trace-origin-a-1", "trace-origin-a-2"},
				TraceID:                  "trace-a",
			},
		},
	}
}

func chainingTraceLineageDispatchRecord(consumed []interfaces.Token) interfaces.FactoryDispatchRecord {
	return interfaces.FactoryDispatchRecord{
		DispatchID:  "dispatch-lineage",
		CreatedTick: 8,
		Dispatch: interfaces.WorkDispatch{
			DispatchID:               "dispatch-lineage",
			TransitionID:             "build",
			WorkstationName:          "Build",
			CurrentChainingTraceID:   "trace-z",
			PreviousChainingTraceIDs: []string{"trace-a", "trace-z"},
			InputTokens:              workers.InputTokens(consumed...),
			Execution: interfaces.ExecutionMetadata{
				RequestID: "request-z",
				TraceID:   "trace-z",
				WorkIDs:   []string{"work-z", "work-a"},
			},
		},
	}
}

func chainingTraceLineageResult() interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   "dispatch-lineage",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "merged output",
	}
}

func chainingTraceLineageCompletion(eventTime time.Time, consumed []interfaces.Token) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-lineage",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeAccepted,
		EndTime:         eventTime,
		Duration:        1500 * time.Millisecond,
		ConsumedTokens:  consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type: interfaces.MutationCreate,
			Token: &interfaces.Token{
				ID:      "tok-output",
				PlaceID: "task:done",
				Color: interfaces.TokenColor{
					DataType:   interfaces.DataTypeWork,
					WorkID:     "work-output",
					WorkTypeID: "task",
					Name:       "merged-output",
					TraceID:    "trace-output",
				},
			},
		}},
	}
}

func assertEventHistoryRequestLineage(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	requestPayload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch request payload: %v", err)
	}
	if stringValueForEventHistoryTest(requestPayload.CurrentChainingTraceId) != "trace-z" {
		t.Fatalf("dispatch request current chaining trace ID = %q, want trace-z", stringValueForEventHistoryTest(requestPayload.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(requestPayload.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch request previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if len(requestPayload.Inputs) != 2 {
		t.Fatalf("dispatch request inputs = %#v, want two consumed work refs", requestPayload.Inputs)
	}
	if requestPayload.Inputs[0].WorkId != "work-z" {
		t.Fatalf("first dispatch request input work ID = %q, want work-z", requestPayload.Inputs[0].WorkId)
	}
	if requestPayload.Inputs[1].WorkId != "work-a" {
		t.Fatalf("second dispatch request input work ID = %q, want work-a", requestPayload.Inputs[1].WorkId)
	}
}

func assertEventHistoryResponseLineage(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	responsePayload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if stringValueForEventHistoryTest(responsePayload.CurrentChainingTraceId) != "trace-z" {
		t.Fatalf("dispatch response current chaining trace ID = %q, want trace-z", stringValueForEventHistoryTest(responsePayload.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(responsePayload.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch response previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if responsePayload.OutputWork == nil || len(*responsePayload.OutputWork) != 1 {
		t.Fatalf("output work = %#v, want one generated output work item", responsePayload.OutputWork)
	}
	outputWork := (*responsePayload.OutputWork)[0]
	if stringValueForEventHistoryTest(outputWork.CurrentChainingTraceId) != "trace-output" {
		t.Fatalf("output work current chaining trace ID = %q, want trace-output", stringValueForEventHistoryTest(outputWork.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(outputWork.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("output work previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
}

func safeDiagnosticsWorkResult() interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   "dispatch-diagnostics",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "completed",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "response_id",
			ID:       "resp-safe-123",
		},
		Diagnostics: &interfaces.WorkDiagnostics{
			RenderedPrompt: &interfaces.RenderedPromptDiagnostic{
				SystemPromptHash: "system-hash-123",
				UserMessageHash:  "user-hash-456",
				Variables: map[string]string{
					"prompt_source":  "factory-renderer",
					"work_type_name": "story",
					"system_prompt":  "raw rendered system prompt must stay private",
					"user_message":   "raw rendered user message must stay private",
					"stdin":          "raw rendered stdin must stay private",
					"env":            "raw rendered environment must stay private",
				},
			},
			Provider: &interfaces.ProviderDiagnostic{
				Provider: "codex",
				Model:    "gpt-5.4",
				RequestMetadata: map[string]string{
					"prompt_source":       "provider-renderer",
					"worker_type":         "builder",
					"system_prompt":       "raw system prompt must stay private",
					"raw_system_prompt":   "raw variant system prompt must stay private",
					"system_prompt_body":  "raw prompt body must stay private",
					"user_message_text":   "raw user message text must stay private",
					"stdin_payload":       "raw stdin payload must stay private",
					"env_secret":          "raw env secret must stay private",
					"unreviewed_metadata": "unreviewed provider metadata must stay private",
				},
				ResponseMetadata: map[string]string{
					"retry_count":         "1",
					"provider_session_id": "resp-safe-123",
					"system_prompt_body":  "raw response prompt body must stay private",
					"user_message_text":   "raw response user message text must stay private",
					"stdin_payload":       "raw response stdin payload must stay private",
					"env_secret":          "raw response env secret must stay private",
				},
			},
			Command: &interfaces.CommandDiagnostic{
				Stdin: "raw command stdin must stay private",
				Env: map[string]string{
					"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
				},
			},
			Panic: &interfaces.PanicDiagnostic{Stack: "panic stack should not be dashboard-facing"},
		},
	}
}

func safeDiagnosticsCompletedDispatch(eventTime time.Time) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-diagnostics",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeAccepted,
		EndTime:         eventTime,
		Duration:        3 * time.Second,
	}
}

func assertThinDispatchResponseSerializedEvent(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	body := string(data)
	for _, unsafe := range unsafeDiagnosticEventValues() {
		if strings.Contains(body, unsafe) {
			t.Fatalf("event JSON leaked unsafe diagnostic value %q: %s", unsafe, body)
		}
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	payload := assertJSONObject(t, decoded, "payload")
	for _, retiredField := range []string{"inputs", "providerSession", "diagnostics"} {
		if _, ok := payload[retiredField]; ok {
			t.Fatalf("dispatch response payload must not serialize retired %q: %#v", retiredField, payload)
		}
	}
}

func unsafeDiagnosticEventValues() []string {
	return []string{
		"raw system prompt must stay private",
		"raw variant system prompt must stay private",
		"raw prompt body must stay private",
		"raw user message text must stay private",
		"raw stdin payload must stay private",
		"raw env secret must stay private",
		"unreviewed provider metadata must stay private",
		"raw response prompt body must stay private",
		"raw response user message text must stay private",
		"raw response stdin payload must stay private",
		"raw response env secret must stay private",
		"raw rendered system prompt must stay private",
		"raw rendered user message must stay private",
		"raw rendered stdin must stay private",
		"raw rendered environment must stay private",
		"raw command stdin must stay private",
		"raw environment value must stay private",
		"AGENT_FACTORY_AUTH_TOKEN",
		"panic stack should not be dashboard-facing",
	}
}

func TestFailureDetailsForResult_NonFailedResultsOmitFailureDetails(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:   "dispatch-rejected",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeRejected,
		Feedback:     "needs revision",
	})

	if reason != "" || message != "" {
		t.Fatalf("failure details = %q/%q, want empty for rejected result", reason, message)
	}
}

func TestFailureDetailsForResult_FailedWorkerErrorUsesStableFailureDetails(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:   "dispatch-worker-error",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
		Error:        "script exited with code 1",
	})

	if reason != failureReasonWorkerError {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonWorkerError)
	}
	if message != "script exited with code 1" {
		t.Fatalf("failure message = %q, want script error", message)
	}
}

func TestFailureDetailsForResult_FailedWithoutDetailsUsesUnavailableMessage(t *testing.T) {
	reason, message := failureDetailsForResult(interfaces.WorkResult{
		DispatchID:   "dispatch-unknown",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeFailed,
	})

	if reason != failureReasonUnknown {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonUnknown)
	}
	if message != failureMessageUnavailable {
		t.Fatalf("failure message = %q, want unavailable message", message)
	}
}

type eventHistoryRuntimeConfig = runtimefixtures.RuntimeDefinitionLookupFixture

func eventHistoryProjectionNet() *state.Net {
	return &state.Net{
		ID: "event-history-projection-net",
		Places: map[string]*petri.Place{
			"story:init":   {ID: "story:init", TypeID: "story", State: "init"},
			"story:review": {ID: "story:review", TypeID: "story", State: "review"},
			"story:done":   {ID: "story:done", TypeID: "story", State: "done"},
			"story:failed": {ID: "story:failed", TypeID: "story", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{
			"build": {
				ID:         "build",
				Name:       "Build",
				WorkerType: "builder",
				InputArcs:  []petri.Arc{{Name: "work", PlaceID: "story:init"}},
				OutputArcs: []petri.Arc{{PlaceID: "story:review"}},
				FailureArcs: []petri.Arc{
					{PlaceID: "story:failed"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"story": {
				ID:   "story",
				Name: "Story",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "done", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	}
}

func stringValueForEventHistoryTest[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSliceValueForEventHistoryTest(value *[]string) []string {
	if value == nil {
		return nil
	}
	out := make([]string, len(*value))
	copy(out, *value)
	return out
}
