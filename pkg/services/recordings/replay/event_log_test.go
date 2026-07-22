package replay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerdiagnosticsmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerdiagnostics"
)

func testReplayArtifact(t *testing.T, events ...factoryapi.FactoryEvent) *interfaces.ReplayArtifact {
	t.Helper()

	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	generatedFactory := testGeneratedFactory()
	runStarted, err := runStartedEventFromSnapshot(recordedAt, mustFactorySnapshot(t, generatedFactory), &interfaces.ReplayWallClockMetadata{StartedAt: recordedAt}, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("build run started event: %v", err)
	}

	allEvents := []interfaces.FactoryEvent{runStarted}
	for _, event := range events {
		converted, err := interfaces.NewFactoryEvent(event)
		if err != nil {
			t.Fatalf("convert replay event %q: %v", event.Id, err)
		}
		allEvents = append(allEvents, converted)
	}
	assignEventSequences(allEvents)
	return &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		RecordedAt:    recordedAt,
		Events:        allEvents,
		Factory:       mustFactorySnapshot(t, generatedFactory),
		WallClock:     &interfaces.ReplayWallClockMetadata{StartedAt: recordedAt},
	}
}

func mustFactorySnapshot(t *testing.T, generated factoryapi.Factory) *interfaces.FactorySnapshot {
	t.Helper()
	snapshot, err := interfaces.NewFactorySnapshot(generated)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	return snapshot
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
			Outputs: &[]factoryapi.WorkstationIO{},
		}},
	}
}

func replayWorkRequestEvent(t *testing.T, requestID string, tick int, source string, works []factoryapi.Work, relations []factoryapi.Relation) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.WorkRequestEventPayload{
		Type:      factoryapi.WorkRequestType(work.WorkRequestTypeFactoryRequestBatch),
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

func replayDispatchCreatedEvent(t *testing.T, dispatch work.WorkDispatch, tick int) factoryapi.FactoryEvent {
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

func replayDispatchCompletedEvent(t *testing.T, completionID string, result workerexecution.WorkResult, tick int) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.DispatchResponseEventPayload{
		CompletionId:    stringPtrIfNotEmpty(completionID),
		TransitionId:    result.TransitionID,
		Outcome:         factoryapi.WorkOutcome(result.Outcome),
		Output:          stringPtrIfNotEmpty(result.Output),
		OutputWork:      generatedReplayOutputWorkPtr(result.RecordedOutputWork),
		Error:           stringPtrIfNotEmpty(result.Error),
		Feedback:        stringPtrIfNotEmpty(result.Feedback),
		ProviderFailure: workerdiagnosticsmapping.GeneratedWorkFailureMetadata(result.FailureMetadata),
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

func generatedReplayOutputWorkPtr(items []work.FactoryWorkItem) *[]factoryapi.Work {
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
			State:                    generatedWorkStatePtr(item.State),
			CurrentChainingTraceId:   stringPtrIfNotEmpty(currentChainingTraceID),
			PreviousChainingTraceIds: slicePtr(item.PreviousChainingTraceIDs),
			TraceId:                  stringPtrIfNotEmpty(item.TraceID),
			Tags:                     generatedStringMapPtr(item.Tags),
		})
	}
	return &out
}

func generatedWorkStatePtr(name string) *factoryapi.WorkState {
	if name == "" {
		return nil
	}
	return &factoryapi.WorkState{Name: name, Type: factoryapi.WorkStateTypePROCESSING}
}

func TestReduceReplayEvents_ThinDispatchRequestUsesContextIdentityAndFactoryTopology(t *testing.T) {
	artifact, dispatchEvent := thinDispatchReplayArtifact(t)
	assertThinReplayDispatchEventPayload(t, dispatchEvent)

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduceReplayEvents: %v", err)
	}
	assertThinReplayReduction(t, reduced)
}

func TestReduceReplayEvents_CompletionsPreserveRecordedOutputWork(t *testing.T) {
	artifact := testReplayArtifact(
		t,
		replayDispatchCompletedEvent(t, "completion-1", workerexecution.WorkResult{
			DispatchID:   "dispatch-1",
			TransitionID: "setup-workspace",
			Outcome:      workerexecution.OutcomeAccepted,
			RecordedOutputWork: []work.FactoryWorkItem{
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

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
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

func TestReduceReplayEvents_CompletionsRehydrateSafeDiagnosticsThroughInterfaces(t *testing.T) {
	artifact := safeDiagnosticReductionArtifact(t)

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduceReplayEvents: %v", err)
	}
	if len(reduced.Completions) != 1 {
		t.Fatalf("reduced completions = %d, want 1", len(reduced.Completions))
	}

	assertReducedCompletionSafeDiagnostics(t, reduced.Completions[0])
}

func TestReduceReplayEvents_MapsLegacyProviderFailureOnlyWireToFailureMetadata(t *testing.T) {
	family := factoryapi.WorkFailureFamily(workerexecution.WorkFailureFamilyRetryable)
	failureType := factoryapi.WorkFailureType(workerexecution.WorkFailureTypeTimeout)
	payload := factoryapi.DispatchResponseEventPayload{
		CompletionId: stringPtrIfNotEmpty("completion-legacy"),
		TransitionId: "process",
		Outcome:      factoryapi.WorkOutcomeFailed,
		Error:        stringPtrIfNotEmpty("provider timed out"),
		ProviderFailure: &factoryapi.ProviderFailureMetadata{
			Family: &family,
			Type:   &failureType,
		},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchResponseEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch completed payload: %v", err)
	}

	artifact := testReplayArtifact(t, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-completed/dispatch-legacy",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchResponse,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, 3, 0, time.UTC),
			Tick:       3,
			DispatchId: stringPtrIfNotEmpty("dispatch-legacy"),
		},
		Payload: union,
	})

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduceReplayEvents: %v", err)
	}
	if len(reduced.Completions) != 1 {
		t.Fatalf("reduced completions = %d, want 1", len(reduced.Completions))
	}
	completion := reduced.Completions[0].result
	if completion.FailureMetadata == nil {
		t.Fatal("failure metadata = nil, want retryable/timeout from wire provider_failure")
	}
	if completion.FailureMetadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("failure family = %q, want retryable", completion.FailureMetadata.Family)
	}
	if completion.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("failure type = %q, want timeout", completion.FailureMetadata.Type)
	}
}

func TestReduceReplayEvents_CompletionsOmitDiagnosticsWhenReplayArtifactOmitsThem(t *testing.T) {
	artifact := testReplayArtifact(
		t,
		replayInferenceResponseEvent(
			t,
			work.WorkDispatch{
				DispatchID: "dispatch-no-diagnostics",
				Execution: work.ExecutionMetadata{
					RequestID: "request-no-diagnostics",
					TraceID:   "trace-no-diagnostics",
					WorkIDs:   []string{"work-no-diagnostics"},
				},
			},
			"dispatch-no-diagnostics/inference-request/1",
			1,
			3,
			"recorded provider output",
			&workerexecution.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "response_id",
				ID:       "resp-no-diagnostics",
			},
			nil,
			"",
		),
		replayDispatchCompletedEvent(t, "completion-no-diagnostics", workerexecution.WorkResult{
			DispatchID:   "dispatch-no-diagnostics",
			TransitionID: "transition-no-diagnostics",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "recorded provider output",
		}, 4),
	)

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduceReplayEvents: %v", err)
	}
	if len(reduced.Completions) != 1 {
		t.Fatalf("reduced completions = %d, want 1", len(reduced.Completions))
	}

	completion := reduced.Completions[0]
	if completion.result.ProviderSession == nil || completion.result.ProviderSession.ID != "resp-no-diagnostics" {
		t.Fatalf("provider session = %#v, want resp-no-diagnostics", completion.result.ProviderSession)
	}
	if completion.result.Diagnostics != nil {
		t.Fatalf("completion diagnostics = %#v, want nil", completion.result.Diagnostics)
	}
}

func thinDispatchReplayArtifact(t *testing.T) (*interfaces.ReplayArtifact, factoryapi.FactoryEvent) {
	t.Helper()

	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "process",
		InputTokens: workers.InputTokens(
			workers.Token{
				ID: "token-work-1",
				Color: workers.Color{
					WorkID:     "work-1",
					WorkTypeID: "task",
					DataType:   workers.DataTypeWork,
					TraceID:    "trace-1",
					Name:       "story-1",
				},
			},
			workers.Token{
				ID:      "resource/executor-slot",
				PlaceID: "executor-slot:available",
				Color: workers.Color{
					WorkTypeID: "executor-slot",
					DataType:   workers.DataTypeResource,
					Name:       "executor-slot",
				},
			},
		),
		Execution: work.ExecutionMetadata{
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

func safeDiagnosticReductionArtifact(t *testing.T) *interfaces.ReplayArtifact {
	t.Helper()

	return testReplayArtifact(
		t,
		replayInferenceResponseEvent(
			t,
			work.WorkDispatch{
				DispatchID: "dispatch-safe",
				Execution: work.ExecutionMetadata{
					RequestID: "request-safe",
					TraceID:   "trace-safe",
					WorkIDs:   []string{"work-safe"},
				},
			},
			"dispatch-safe/inference-request/1",
			1,
			3,
			"recorded provider output",
			&workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: "response_id", ID: "resp-safe-123"},
			safeDiagnosticReductionFixture(),
			"",
		),
		replayDispatchCompletedEvent(t, "completion-safe", workerexecution.WorkResult{
			DispatchID:   "dispatch-safe",
			TransitionID: "transition-safe",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "recorded provider output",
			FailureMetadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyRetryable,
				Type:   workerexecution.WorkFailureTypeThrottled,
			},
		}, 4),
	)
}

func safeDiagnosticReductionFixture() *workerexecution.WorkDiagnostics {
	return &workerexecution.WorkDiagnostics{
		RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash-123",
			UserMessageHash:  "user-hash-456",
			Variables: map[string]string{
				"prompt_source": "factory-renderer", "work_type_name": "story", "system_prompt": "unsafe raw prompt",
			},
		},
		Provider: &workerexecution.ProviderDiagnostic{
			Provider: "codex",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"worker_type": "builder", "system_prompt_body": "unsafe request body",
			},
			ResponseMetadata: map[string]string{
				"provider_session_id": "resp-safe-123", "retry_count": "1", "env_secret": "unsafe response secret",
			},
		},
		Command: &workerexecution.CommandDiagnostic{Command: "echo", Stdin: "unsafe stdin"},
		Panic:   &workerexecution.PanicDiagnostic{Message: "unsafe panic", Stack: "unsafe stack"},
		Metadata: map[string]string{
			"debug": "unsafe metadata",
		},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper keeps the replay completion diagnostics contract together across all supported branches.
func assertReducedCompletionSafeDiagnostics(t *testing.T, completion replayCompletion) {
	t.Helper()

	if completion.result.Output != "recorded provider output" {
		t.Fatalf("completion output = %q, want recorded provider output", completion.result.Output)
	}
	if completion.result.ProviderSession == nil || completion.result.ProviderSession.ID != "resp-safe-123" {
		t.Fatalf("provider session = %#v, want resp-safe-123", completion.result.ProviderSession)
	}
	if completion.result.FailureMetadata == nil || completion.result.FailureMetadata.Type != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("failure metadata = %#v, want throttled", completion.result.FailureMetadata)
	}
	if completion.result.Diagnostics == nil || completion.result.Diagnostics.Provider == nil || completion.result.Diagnostics.RenderedPrompt == nil {
		t.Fatalf("completion diagnostics = %#v, want safe provider and rendered prompt diagnostics", completion.result.Diagnostics)
	}
	if got := completion.result.Diagnostics.Provider.ResponseMetadata["provider_session_id"]; got != "resp-safe-123" {
		t.Fatalf("response metadata provider_session_id = %q, want resp-safe-123", got)
	}
	if got := completion.result.Diagnostics.Provider.RequestMetadata["worker_type"]; got != "builder" {
		t.Fatalf("request metadata worker_type = %q, want builder", got)
	}
	if got := completion.result.Diagnostics.RenderedPrompt.Variables["work_type_name"]; got != "story" {
		t.Fatalf("rendered prompt work_type_name = %q, want story", got)
	}
	if _, ok := completion.result.Diagnostics.RenderedPrompt.Variables["system_prompt"]; ok {
		t.Fatalf("rendered prompt variables leaked unsafe raw prompt: %#v", completion.result.Diagnostics.RenderedPrompt.Variables)
	}
	if completion.result.Diagnostics.Command != nil {
		t.Fatalf("command diagnostics = %#v, want nil", completion.result.Diagnostics.Command)
	}
	if completion.result.Diagnostics.Panic != nil {
		t.Fatalf("panic diagnostics = %#v, want nil", completion.result.Diagnostics.Panic)
	}
	if completion.result.Diagnostics.Metadata != nil {
		t.Fatalf("metadata diagnostics = %#v, want nil", completion.result.Diagnostics.Metadata)
	}
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
	recorded work.WorkDispatch,
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

func assertThinReplayDispatchTokens(t *testing.T, recorded work.WorkDispatch) {
	t.Helper()

	inputTokens := workers.WorkDispatchInputTokens(recorded)
	if len(inputTokens) != 2 {
		t.Fatalf("dispatch input tokens = %#v, want work and resource tokens", inputTokens)
	}
	var sawWork bool
	var sawResource bool
	for _, token := range inputTokens {
		switch token.Color.DataType {
		case workers.DataTypeWork:
			if token.Color.WorkID != "work-1" || token.Color.TraceID != "trace-1" {
				t.Fatalf("work token = %#v, want canonical work identity", token)
			}
			sawWork = true
		case workers.DataTypeResource:
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

func assertReplayDispatchOwnedContract(t *testing.T, recorded work.WorkDispatch) {
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

func replayInferenceRequestEvent(t *testing.T, request workerexecution.ProviderInferenceRequest, inferenceRequestID string, attempt int, tick int) factoryapi.FactoryEvent {
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
	dispatch work.WorkDispatch,
	inferenceRequestID string,
	attempt int,
	tick int,
	response string,
	providerSession *workerexecution.ProviderSessionMetadata,
	diagnostics *workerexecution.WorkDiagnostics,
	errorClass string,
) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.InferenceResponseEventPayload{
		InferenceRequestId: inferenceRequestID,
		Attempt:            attempt,
		DurationMillis:     125,
		ProviderSession:    workerdiagnosticsmapping.GeneratedProviderSessionMetadata(providerSession),
		Diagnostics:        workerdiagnosticsmapping.GeneratedSafeWorkDiagnostics(workers.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)),
	}
	if errorClass != "" {
		payload.Outcome = factoryapi.InferenceOutcomeFailed
		payload.FailureDetail = &factoryapi.FailureDetail{Reason: factoryapi.WorkFailureTypeUnknown, Message: errorClass}
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

func replayWorkStateChangeEvent(
	t *testing.T,
	workID string,
	fromState string,
	toState string,
	fromPlaceID string,
	toPlaceID string,
	source factoryapi.WorkStateChangeSource,
	tick int,
) factoryapi.FactoryEvent {
	t.Helper()

	payload := factoryapi.WorkStateChangeEventPayload{
		WorkId:       workID,
		WorkTypeName: "task",
		FromState:    fromState,
		ToState:      toState,
		FromPlaceId:  fromPlaceID,
		ToPlaceId:    toPlaceID,
		Source:       source,
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromWorkStateChangeEventPayload(payload); err != nil {
		t.Fatalf("encode work state change payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            fmt.Sprintf("factory-event/work-state-change/%s/%d", workID, tick),
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeWorkStateChange,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:      tick,
			WorkIds:   slicePtr([]string{workID}),
		},
		Payload: union,
	}
}

func TestReduceReplayEvents_OperatorWorkStateChanges(t *testing.T) {
	artifact := testReplayArtifact(
		t,
		replayWorkStateChangeEvent(t, "work-recover", "failed", "init", "task:failed", "task:init", factoryapi.WorkStateChangeSourceAPI, 4),
		replayWorkStateChangeEvent(t, "work-cascade", "failed", "init", "task:failed", "task:init", factoryapi.WorkStateChangeSourceCascadingFailure, 5),
	)

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduceReplayEvents: %v", err)
	}
	if len(reduced.WorkStateChanges) != 1 {
		t.Fatalf("work state changes = %d, want 1 operator move", len(reduced.WorkStateChanges))
	}
	change := reduced.WorkStateChanges[0]
	if change.change.WorkID != "work-recover" || change.observedTick != 4 {
		t.Fatalf("work state change = %#v, want work-recover at tick 4", change)
	}
	if change.change.FromPlaceID != "task:failed" || change.change.ToPlaceID != "task:init" {
		t.Fatalf("places = %q -> %q, want task:failed -> task:init", change.change.FromPlaceID, change.change.ToPlaceID)
	}
	if change.change.Source != work.WorkStateChangeSourceAPI {
		t.Fatalf("source = %q, want api", change.change.Source)
	}
}

func TestRecorder_PersistsWorkStateChangeEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator-move.replay.json")
	artifact := testReplayArtifact(t)
	recorder, err := NewRecorder(testReplayStorage(), path, artifact, 0)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	moveEvent := replayWorkStateChangeEvent(
		t,
		"work-recover",
		"failed",
		"init",
		"task:failed",
		"task:init",
		factoryapi.WorkStateChangeSourceCLI,
		3,
	)
	domainEvent, err := interfaces.NewFactoryEvent(moveEvent)
	if err != nil {
		t.Fatalf("NewFactoryEvent: %v", err)
	}
	recorder.RecordEvent(domainEvent)
	if err := recorder.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	loaded := loadReplayArtifactForTest(t, path)
	if replayEventCount(loaded, factoryapi.FactoryEventTypeWorkStateChange) != 1 {
		t.Fatalf("WORK_STATE_CHANGE events = %d, want 1", replayEventCount(loaded, factoryapi.FactoryEventTypeWorkStateChange))
	}
	event := loaded.Events[len(loaded.Events)-1]
	generated := mustGeneratedReplayEvent(t, event)
	payload, err := generated.Payload.AsWorkStateChangeEventPayload()
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.WorkId != "work-recover" || payload.Source != factoryapi.WorkStateChangeSourceCLI {
		t.Fatalf("payload = %#v, want work-recover cli source", payload)
	}
}

func TestRecorder_RecordEventDetachesCanonicalPayload(t *testing.T) {
	artifact := testReplayArtifact(t)
	recorder, err := NewRecorder(testReplayStorage(), filepath.Join(t.TempDir(), "detached.replay.json"), artifact, 0)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	event := interfaces.FactoryEvent{
		Id:      "event-detached",
		Type:    interfaces.FactoryEventTypeWorkStateChange,
		Payload: []byte(`{"workId":"work-original"}`),
		Context: interfaces.FactoryEventContext{EventTime: time.Now().UTC()},
	}
	recorder.RecordEvent(event)
	copy(event.Payload, []byte(`{"workId":"work-mutated!"}`))

	if got := string(artifact.Events[len(artifact.Events)-1].Payload); got != `{"workId":"work-original"}` {
		t.Fatalf("recorded payload = %s, want detached original", got)
	}
}

func TestRecorder_RecordErrorRetainsProducerBoundaryFailure(t *testing.T) {
	recorder, err := NewRecorder(testReplayStorage(), filepath.Join(t.TempDir(), "producer-error.replay.json"), testReplayArtifact(t), 0)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	want := errors.New("convert generated event")
	recorder.RecordError(want)
	if !errors.Is(recorder.Err(), want) {
		t.Fatalf("Err() = %v, want %v", recorder.Err(), want)
	}
}

func TestRecorder_FlushFailureIsActionableAndRetained(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write occupied parent: %v", err)
	}
	recorder, err := NewRecorder(testReplayStorage(), filepath.Join(parent, "run.replay.json"), testReplayArtifact(t), 0)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	err = recorder.Flush()
	if err == nil {
		t.Fatal("Flush error = nil, want destination creation failure")
	}
	if retained := recorder.Err(); retained == nil || retained.Error() != err.Error() {
		t.Fatalf("retained error = %v, want %v", retained, err)
	}
}

func TestRecorder_StopJoinsPeriodicFlushLoop(t *testing.T) {
	recorder, err := NewRecorder(
		testReplayStorage(),
		filepath.Join(t.TempDir(), "run.replay.json"),
		testReplayArtifact(t),
		time.Millisecond)

	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	recorder.Start(context.Background())

	stopped := make(chan struct{})
	go func() {
		recorder.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not join the periodic flush loop")
	}

	recorder.Stop()
}

func loadReplayArtifactForTest(t *testing.T, path string) *interfaces.ReplayArtifact {
	t.Helper()
	loaded, err := Load(testReplayStorage(), path, testFactorySnapshotDecoder)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return loaded
}

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if string(event.Type) == string(eventType) {
			count++
		}
	}
	return count
}
