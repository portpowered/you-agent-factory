package replay

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func testReplayArtifact(t *testing.T, events ...factoryapi.FactoryEvent) *interfaces.ReplayArtifact {
	t.Helper()

	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	generatedFactory := testGeneratedFactory()
	runStarted, err := runStartedEventFromFactory(recordedAt, generatedFactory, &interfaces.ReplayWallClockMetadata{StartedAt: recordedAt}, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("build run started event: %v", err)
	}

	allEvents := append([]factoryapi.FactoryEvent{runStarted}, events...)
	assignEventSequences(allEvents)
	return &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		RecordedAt:    recordedAt,
		Events:        allEvents,
		Factory:       generatedFactory,
		WallClock:     &interfaces.ReplayWallClockMetadata{StartedAt: recordedAt},
	}
}

func testGeneratedFactory() factoryapi.Factory {
	return factoryapi.Factory{
		Name:      "test-replay-factory",
		WorkTypes: &[]factoryapi.WorkType{{Name: "task"}},
		Workers:   &[]factoryapi.Worker{{Name: "worker-a"}},
		Workstations: &[]factoryapi.Workstation{{
			Name:    "process",
			Worker:  "worker-a",
			Inputs:  []factoryapi.WorkstationIO{},
			Outputs: []factoryapi.WorkstationIO{},
		}},
	}
}

func replayWorkRequestEvent(t *testing.T, requestID string, tick int, source string, works []factoryapi.Work, relations []factoryapi.Relation) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.WorkRequestEventPayload{
		Type:      factoryapi.WorkRequestType(interfaces.WorkRequestTypeFactoryRequestBatch),
		Works:     slicePtr(works),
		Relations: slicePtr(relations),
		Source:    stringPtrIfNotEmpty(source),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromWorkRequestEventPayload(payload); err != nil {
		t.Fatalf("encode work request payload: %v", err)
	}

	var traceIDs []string
	var workIDs []string
	for _, work := range works {
		traceIDs = append(traceIDs, stringValue(work.TraceId))
		workIDs = append(workIDs, stringValue(work.WorkId))
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/work-request/" + requestID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeWorkRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:      tick,
			RequestId: stringPtrIfNotEmpty(requestID),
			Source:    stringPtrIfNotEmpty(source),
			TraceIds:  slicePtr(uniqueNonEmpty(traceIDs)),
			WorkIds:   slicePtr(uniqueNonEmpty(workIDs)),
		},
		Payload: union,
	}
}

func replayDispatchCreatedEvent(t *testing.T, dispatch interfaces.WorkDispatch, tick int) factoryapi.FactoryEvent {
	t.Helper()

	metadata := map[string]string{}
	if dispatch.Execution.ReplayKey != "" {
		metadata[replayMetadataReplayKey] = dispatch.Execution.ReplayKey
	}
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: dispatch.TransitionID,
		Inputs:       generatedDispatchConsumedWorkRefsFromReplayDispatch(dispatch),
		Resources:    generatedResourcesFromReplayDispatch(dispatch),
		Metadata:     generatedDispatchRequestMetadata(metadata),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch created payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            fmt.Sprintf("factory-event/dispatch-created/%s", dispatch.DispatchID),
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: stringPtrIfNotEmpty(dispatch.DispatchID),
			RequestId:  stringPtrIfNotEmpty(dispatch.Execution.RequestID),
			TraceIds:   slicePtr(uniqueNonEmpty([]string{dispatch.Execution.TraceID})),
			WorkIds:    slicePtr(uniqueNonEmpty(dispatch.Execution.WorkIDs)),
		},
		Payload: union,
	}
}

func replayDispatchCompletedEvent(t *testing.T, completionID string, result interfaces.WorkResult, tick int) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.DispatchResponseEventPayload{
		CompletionId:    stringPtrIfNotEmpty(completionID),
		TransitionId:    result.TransitionID,
		Outcome:         factoryapi.WorkOutcome(result.Outcome),
		Output:          stringPtrIfNotEmpty(result.Output),
		OutputWork:      generatedReplayOutputWorkPtr(result.RecordedOutputWork),
		Error:           stringPtrIfNotEmpty(result.Error),
		Feedback:        stringPtrIfNotEmpty(result.Feedback),
		ProviderFailure: interfaces.GeneratedProviderFailureMetadata(result.ProviderFailure),
		Metrics:         generatedWorkMetrics(result.Metrics),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchResponseEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch completed payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            fmt.Sprintf("factory-event/dispatch-completed/%s", result.DispatchID),
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchResponse,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: stringPtrIfNotEmpty(result.DispatchID),
		},
		Payload: union,
	}
}

func generatedReplayOutputWorkPtr(items []interfaces.FactoryWorkItem) *[]factoryapi.Work {
	if len(items) == 0 {
		return nil
	}
	out := make([]factoryapi.Work, 0, len(items))
	for _, item := range items {
		currentChainingTraceID := item.CurrentChainingTraceID
		if currentChainingTraceID == "" {
			currentChainingTraceID = item.TraceID
		}
		out = append(out, factoryapi.Work{
			Name:                     item.DisplayName,
			WorkId:                   stringPtrIfNotEmpty(item.ID),
			WorkTypeName:             stringPtrIfNotEmpty(item.WorkTypeID),
			State:                    stringPtrIfNotEmpty(item.State),
			CurrentChainingTraceId:   stringPtrIfNotEmpty(currentChainingTraceID),
			PreviousChainingTraceIds: slicePtr(item.PreviousChainingTraceIDs),
			TraceId:                  stringPtrIfNotEmpty(item.TraceID),
			Tags:                     generatedStringMapPtr(item.Tags),
		})
	}
	return &out
}

func TestReduceReplayEvents_ThinDispatchRequestUsesContextIdentityAndFactoryTopology(t *testing.T) {
	artifact, dispatchEvent := thinDispatchReplayArtifact(t)
	assertThinReplayDispatchEventPayload(t, dispatchEvent)

	reduced, err := reduceReplayEvents(artifact)
	if err != nil {
		t.Fatalf("reduceReplayEvents: %v", err)
	}
	assertThinReplayReduction(t, reduced)
}

func TestReduceReplayEvents_CompletionsPreserveRecordedOutputWork(t *testing.T) {
	artifact := testReplayArtifact(
		t,
		replayDispatchCompletedEvent(t, "completion-1", interfaces.WorkResult{
			DispatchID:   "dispatch-1",
			TransitionID: "setup-workspace",
			Outcome:      interfaces.OutcomeAccepted,
			RecordedOutputWork: []interfaces.FactoryWorkItem{
				{
					ID:                     "work-plan-38",
					WorkTypeID:             "plan",
					DisplayName:            "story-1",
					CurrentChainingTraceID: "trace-1",
					TraceID:                "trace-1",
					Tags:                   map[string]string{"kind": "plan"},
				},
				{
					ID:                     "work-task-39",
					WorkTypeID:             "task",
					DisplayName:            "story-1",
					CurrentChainingTraceID: "trace-1",
					TraceID:                "trace-1",
				},
			},
		}, 3),
	)

	reduced, err := reduceReplayEvents(artifact)
	if err != nil {
		t.Fatalf("reduceReplayEvents: %v", err)
	}
	if len(reduced.Completions) != 1 {
		t.Fatalf("reduced completions = %d, want 1", len(reduced.Completions))
	}
	got := reduced.Completions[0].result.RecordedOutputWork
	if len(got) != 2 {
		t.Fatalf("recorded output work = %#v, want 2 items", got)
	}
	if got[0].ID != "work-plan-38" || got[0].WorkTypeID != "plan" {
		t.Fatalf("recorded output work[0] = %#v, want work-plan-38/plan", got[0])
	}
	if got[1].ID != "work-task-39" || got[1].WorkTypeID != "task" {
		t.Fatalf("recorded output work[1] = %#v, want work-task-39/task", got[1])
	}
}

func thinDispatchReplayArtifact(t *testing.T) (*interfaces.ReplayArtifact, factoryapi.FactoryEvent) {
	t.Helper()

	dispatch := interfaces.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "process",
		InputTokens: workers.InputTokens(
			interfaces.Token{
				ID: "token-work-1",
				Color: interfaces.TokenColor{
					WorkID:     "work-1",
					WorkTypeID: "task",
					DataType:   interfaces.DataTypeWork,
					TraceID:    "trace-1",
					Name:       "story-1",
				},
			},
			interfaces.Token{
				ID:      "resource/executor-slot",
				PlaceID: "executor-slot:available",
				Color: interfaces.TokenColor{
					WorkTypeID: "executor-slot",
					DataType:   interfaces.DataTypeResource,
					Name:       "executor-slot",
				},
			},
		),
		Execution: interfaces.ExecutionMetadata{
			RequestID: "request-1",
			ReplayKey: "process/trace-1/work-1",
			TraceID:   "trace-1",
			WorkIDs:   []string{"work-1"},
		},
	}
	workRequest := replayWorkRequestEvent(t, "request-1", 1, "api", []factoryapi.Work{{
		Name:         "story-1",
		WorkId:       stringPtrIfNotEmpty("work-1"),
		RequestId:    stringPtrIfNotEmpty("request-1"),
		WorkTypeName: stringPtrIfNotEmpty("task"),
		TraceId:      stringPtrIfNotEmpty("trace-1"),
	}}, nil)
	dispatchEvent := replayDispatchCreatedEvent(t, dispatch, 2)
	return testReplayArtifact(t, workRequest, dispatchEvent), dispatchEvent
}

func assertThinReplayDispatchEventPayload(t *testing.T, dispatchEvent factoryapi.FactoryEvent) {
	t.Helper()

	dispatchJSON, err := json.Marshal(dispatchEvent)
	if err != nil {
		t.Fatalf("Marshal dispatch event: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(dispatchJSON, &raw); err != nil {
		t.Fatalf("Unmarshal dispatch event: %v", err)
	}
	payloadMap, ok := raw["payload"].(map[string]any)
	if !ok {
		t.Fatalf("dispatch payload = %#v, want object", raw["payload"])
	}
	if _, ok := payloadMap["dispatchId"]; ok {
		t.Fatalf("dispatch payload unexpectedly carried dispatchId: %#v", payloadMap)
	}
	if _, ok := payloadMap["worker"]; ok {
		t.Fatalf("dispatch payload unexpectedly carried worker: %#v", payloadMap)
	}
	if _, ok := payloadMap["workstation"]; ok {
		t.Fatalf("dispatch payload unexpectedly carried workstation: %#v", payloadMap)
	}
	if metadata, ok := payloadMap["metadata"].(map[string]any); ok {
		if _, ok := metadata["requestId"]; ok {
			t.Fatalf("dispatch payload metadata unexpectedly carried requestId: %#v", metadata)
		}
	}
}

func assertThinReplayReduction(t *testing.T, reduced *replayEventLog) {
	t.Helper()

	if len(reduced.Submissions) != 1 {
		t.Fatalf("reduced submissions = %d, want 1", len(reduced.Submissions))
	}
	if len(reduced.Dispatches) != 1 {
		t.Fatalf("reduced dispatches = %d, want 1", len(reduced.Dispatches))
	}
	submission := reduced.Submissions[0]
	recorded := reduced.Dispatches[0].dispatch
	assertThinReplayDispatchIdentity(t, submission, recorded)
	assertThinReplayDispatchTokens(t, recorded)
	assertReplayDispatchOwnedContract(t, recorded)
}

func assertThinReplayDispatchIdentity(
	t *testing.T,
	submission replaySubmission,
	recorded interfaces.WorkDispatch,
) {
	t.Helper()

	if recorded.WorkstationName != "process" {
		t.Fatalf("dispatch workstation = %q, want process", recorded.WorkstationName)
	}
	if recorded.WorkerType != "worker-a" {
		t.Fatalf("dispatch worker = %q, want worker-a", recorded.WorkerType)
	}
	if recorded.Execution.RequestID != "request-1" {
		t.Fatalf("dispatch request ID = %q, want request-1", recorded.Execution.RequestID)
	}
	if recorded.Execution.TraceID != "trace-1" {
		t.Fatalf("dispatch trace ID = %q, want trace-1", recorded.Execution.TraceID)
	}
	if len(recorded.Execution.WorkIDs) != 1 || recorded.Execution.WorkIDs[0] != "work-1" {
		t.Fatalf("dispatch work IDs = %#v, want [work-1]", recorded.Execution.WorkIDs)
	}
	if recorded.Execution.ReplayKey != "process/trace-1/work-1" {
		t.Fatalf("dispatch replay key = %q, want process/trace-1/work-1", recorded.Execution.ReplayKey)
	}
	if submission.request.RequestID != recorded.Execution.RequestID {
		t.Fatalf("submission request ID = %q, want %q", submission.request.RequestID, recorded.Execution.RequestID)
	}
	if len(submission.request.Works) != 1 || submission.request.Works[0].WorkID != recorded.Execution.WorkIDs[0] {
		t.Fatalf("submission works = %#v, want joined work-1", submission.request.Works)
	}
	if len(submission.request.Works) != 1 || submission.request.Works[0].TraceID != recorded.Execution.TraceID {
		t.Fatalf("submission trace IDs = %#v, want joined trace-1", submission.request.Works)
	}
}

func assertThinReplayDispatchTokens(t *testing.T, recorded interfaces.WorkDispatch) {
	t.Helper()

	inputTokens := workers.WorkDispatchInputTokens(recorded)
	if len(inputTokens) != 2 {
		t.Fatalf("dispatch input tokens = %#v, want work and resource tokens", inputTokens)
	}
	var sawWork bool
	var sawResource bool
	for _, token := range inputTokens {
		switch token.Color.DataType {
		case interfaces.DataTypeWork:
			if token.Color.WorkID != "work-1" || token.Color.TraceID != "trace-1" {
				t.Fatalf("work token = %#v, want canonical work identity", token)
			}
			sawWork = true
		case interfaces.DataTypeResource:
			if token.Color.Name != "executor-slot" || token.PlaceID != "executor-slot:available" {
				t.Fatalf("resource token = %#v, want executor-slot usage", token)
			}
			sawResource = true
		}
	}
	if !sawWork || !sawResource {
		t.Fatalf("dispatch input tokens missing work/resource split: %#v", inputTokens)
	}
}

func assertReplayDispatchOwnedContract(t *testing.T, recorded interfaces.WorkDispatch) {
	t.Helper()

	payload, err := json.Marshal(recorded)
	if err != nil {
		t.Fatalf("Marshal replay dispatch: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("Unmarshal replay dispatch: %v", err)
	}

	for _, key := range []string{
		"dispatch_id",
		"transition_id",
		"worker_type",
		"workstation_name",
		"execution",
		"input_tokens",
	} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("replay dispatch missing %q: %s", key, string(payload))
		}
	}

	for _, key := range []string{
		"system_prompt",
		"user_message",
		"output_schema",
		"env_vars",
		"worktree",
		"working_directory",
		"model",
		"model_provider",
		"session_id",
	} {
		if _, ok := raw[key]; ok {
			t.Fatalf("replay dispatch unexpectedly carried worker-owned field %q: %s", key, string(payload))
		}
	}
}

func replayInferenceRequestEvent(t *testing.T, request interfaces.ProviderInferenceRequest, inferenceRequestID string, attempt int, tick int) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.InferenceRequestEventPayload{
		InferenceRequestId: inferenceRequestID,
		Attempt:            attempt,
		WorkingDirectory:   request.WorkingDirectory,
		Worktree:           request.Worktree,
		Prompt:             request.UserMessage,
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromInferenceRequestEventPayload(payload); err != nil {
		t.Fatalf("encode inference request payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            fmt.Sprintf("factory-event/inference-request/%s", inferenceRequestID),
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInferenceRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: stringPtrIfNotEmpty(request.Dispatch.DispatchID),
			RequestId:  stringPtrIfNotEmpty(request.Dispatch.Execution.RequestID),
			TraceIds:   slicePtr(uniqueNonEmpty([]string{request.Dispatch.Execution.TraceID})),
			WorkIds:    slicePtr(uniqueNonEmpty(request.Dispatch.Execution.WorkIDs)),
		},
		Payload: union,
	}
}

func replayInferenceResponseEvent(
	t *testing.T,
	dispatch interfaces.WorkDispatch,
	inferenceRequestID string,
	attempt int,
	tick int,
	response string,
	providerSession *interfaces.ProviderSessionMetadata,
	diagnostics *interfaces.WorkDiagnostics,
	errorClass string,
) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.InferenceResponseEventPayload{
		InferenceRequestId: inferenceRequestID,
		Attempt:            attempt,
		DurationMillis:     125,
		ProviderSession:    interfaces.GeneratedProviderSessionMetadata(providerSession),
		Diagnostics:        interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(diagnostics),
	}
	if errorClass != "" {
		payload.Outcome = factoryapi.InferenceOutcomeFailed
		payload.ErrorClass = stringPtrIfNotEmpty(errorClass)
	} else {
		payload.Outcome = factoryapi.InferenceOutcomeSucceeded
		payload.Response = stringPtrIfNotEmpty(response)
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromInferenceResponseEventPayload(payload); err != nil {
		t.Fatalf("encode inference response payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            fmt.Sprintf("factory-event/inference-response/%s", inferenceRequestID),
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInferenceResponse,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: stringPtrIfNotEmpty(dispatch.DispatchID),
			RequestId:  stringPtrIfNotEmpty(dispatch.Execution.RequestID),
			TraceIds:   slicePtr(uniqueNonEmpty([]string{dispatch.Execution.TraceID})),
			WorkIds:    slicePtr(uniqueNonEmpty(dispatch.Execution.WorkIDs)),
		},
		Payload: union,
	}
}
