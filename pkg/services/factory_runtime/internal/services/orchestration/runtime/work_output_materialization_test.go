package runtime

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

type countingMaterializationWorkService struct {
	work.Service
	calls atomic.Int32
}

func (service *countingMaterializationWorkService) MaterializeWorkerOutput(
	ctx context.Context,
	request work.MaterializeWorkerOutputRequest,
) (work.MaterializeWorkerOutputResult, error) {
	service.calls.Add(1)
	return service.Service.MaterializeWorkerOutput(ctx, request)
}

func TestAcceptWorkersResultMaterializesDetachedProposalOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	planner := dispatchplanningwire.New(func(context.Context, workers.WorkstationDispatchRequest) error {
		return nil
	}, nil)
	workService := &countingMaterializationWorkService{Service: testMaterializationService()}
	hook := newCanonicalDispatchPlanningResultHook(
		planner,
		buildSimpleNet(),
		buffers.NewTypedBuffer[workers.WorkResult](4),
		nil,
		workService,
		func() string { return "proposal-id" },
		"session-1",
	)
	dispatch := work.WorkDispatch{
		DispatchID:      "dispatch-proposal",
		TransitionID:    "t-process",
		WorkerType:      "mock",
		WorkstationName: "workstation-a",
		Execution: work.ExecutionMetadata{
			RequestID: "request-proposal",
			TraceID:   "trace-proposal",
			ReplayKey: "replay-proposal",
			WorkIDs:   []string{"source-work"},
		},
		InputTokens: workers.InputTokens(workers.Token{
			Color: workers.Color{
				WorkID:     "source-work",
				WorkTypeID: "task",
				DataType:   workers.DataTypeWork,
				TraceID:    "trace-proposal",
			},
		}),
	}
	if err := hook.SubmitDispatch(ctx, dispatch); err != nil {
		t.Fatalf("SubmitDispatch() error = %v", err)
	}
	intent, ok := planner.Intent(dispatch.DispatchID)
	if !ok {
		t.Fatalf("planner intent for %q is missing", dispatch.DispatchID)
	}
	request := intent.Action.Request
	proposedOutput := &workers.ProposedOutput{
		Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "materialized primary",
		}},
		ProposedWork: []workers.ProposedWork{{
			WorkTypeID: "task",
			Name:       "follow-up",
			State:      "done",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "follow-up body",
			}},
		}},
	}
	result := workers.WorkstationDispatchResult{
		DispatchID:      dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workers.OutcomeAccepted,
			Output:       "legacy output",
		},
		ProposedOutput: proposedOutput,
	}

	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			hook.acceptWorkersResult(ctx, request, result, nil)
		}()
	}
	wait.Wait()
	assertCanonicalProposal(t, workService, hook)
}

func assertCanonicalProposal(
	t *testing.T,
	workService *countingMaterializationWorkService,
	hook *dispatchPlanningResultHook,
) {
	t.Helper()
	if calls := workService.calls.Load(); calls != 1 {
		t.Fatalf("MaterializeWorkerOutput() calls = %d, want exactly one", calls)
	}
	if got := hook.resultBuffer.Len(); got != 1 {
		t.Fatalf("buffered canonical results = %d, want one", got)
	}
	canonical, ok := hook.resultBuffer.Read()
	if !ok {
		t.Fatal("buffered canonical result is missing")
	}
	if canonical.Output != "materialized primary" {
		t.Fatalf("canonical output = %q, want Work-materialized primary", canonical.Output)
	}
	if len(canonical.RecordedOutputWork) != 1 {
		t.Fatalf("canonical materialized Work = %#v, want one item", canonical.RecordedOutputWork)
	}
	if got := canonical.RecordedOutputWork[0].ID; got != "work-proposal-id" {
		t.Fatalf("canonical Work ID = %q, want Work-owned identity", got)
	}
}

func TestCanceledAttemptDoesNotMaterializeCompletedOutput(t *testing.T) {
	ctx := context.Background()
	planner := dispatchplanningwire.New(func(context.Context, workers.WorkstationDispatchRequest) error {
		return nil
	}, nil)
	workService := &countingMaterializationWorkService{Service: testMaterializationService()}
	hook := newCanonicalDispatchPlanningResultHook(
		planner,
		buildSimpleNet(),
		buffers.NewTypedBuffer[workers.WorkResult](4),
		nil,
		workService,
		func() string { return "work-cancel-id" },
		"session-cancel",
	)
	dispatch := work.WorkDispatch{
		DispatchID: "dispatch-cancel-output", TransitionID: "t-process", WorkerType: "mock",
		WorkstationName: "workstation-a",
		Execution: work.ExecutionMetadata{
			RequestID: "request-cancel-output", TraceID: "trace-cancel-output", ReplayKey: "replay-cancel-output",
			WorkIDs: []string{"source-work"},
		},
		InputTokens: workers.InputTokens(workers.Token{Color: workers.Color{
			WorkID: "source-work", WorkTypeID: "task", DataType: workers.DataTypeWork,
			TraceID: "trace-cancel-output",
		}}),
	}
	if err := hook.SubmitDispatch(ctx, dispatch); err != nil {
		t.Fatalf("SubmitDispatch() error = %v", err)
	}
	intent, ok := planner.Intent(dispatch.DispatchID)
	if !ok {
		t.Fatalf("planner intent for %q is missing", dispatch.DispatchID)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	execute := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		close(started)
		<-release
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
			Output: workers.ProposedOutput{
				Primary:      []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "losing output"}},
				ProposedWork: []workers.ProposedWork{{WorkTypeID: "task", Name: "losing-follow-up"}},
			},
		}, nil
	})
	cfg := &runtimeConfig{
		executeService: execute,
		recordingID:    "runtime-cancel-output",
		eventHistory:   &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "generation-cancel-output"},
		clock:          testRuntimeClock{},
		newID:          func() string { return "attempt-cancel-output" },
		attempts:       newAttemptLifecycle(execute, func() string { return "attempt-cancel-output" }, 1),
		net:            buildSimpleNet(), workService: workService,
		workRequestIDs: func() string { return "work-cancel-id" },
	}
	callbackDone := make(chan struct{})
	if err := startThroughStatelessWorkers(ctx, cfg, intent.Action.Request, func(
		callbackCtx context.Context,
		request workers.WorkstationDispatchRequest,
		result workers.WorkstationDispatchResult,
		err error,
	) {
		hook.acceptWorkersResult(callbackCtx, request, result, err)
		close(callbackDone)
	}); err != nil {
		t.Fatalf("startThroughStatelessWorkers() error = %v", err)
	}
	<-started
	if _, err := cancelStatelessAttempt(ctx, cfg, workers.WorkstationDispatchCancelRequest{DispatchID: dispatch.DispatchID}); err != nil {
		t.Fatalf("cancelStatelessAttempt() error = %v", err)
	}
	close(release)
	<-callbackDone
	if calls := workService.calls.Load(); calls != 0 {
		t.Fatalf("MaterializeWorkerOutput() calls = %d, want zero for canceled output", calls)
	}
	canonical, ok := hook.resultBuffer.Read()
	if !ok {
		t.Fatal("canceled terminal result is missing")
	}
	if canonical.Output != "" || len(canonical.RecordedOutputWork) != 0 {
		t.Fatalf("canceled result carried downstream output: %#v", canonical)
	}
}

func TestMaterializeWorkerOutputForDispatchAssignsWorkOwnedIDs(t *testing.T) {
	t.Parallel()

	ids := 0
	result := materializeWorkerOutputForDispatch(
		context.Background(),
		testMaterializationService(),
		&state.Net{
			WorkTypes: map[string]*state.WorkType{
				"review": {
					ID: "review",
					States: []state.StateDefinition{{
						Value:    "init",
						Category: state.StateCategoryInitial,
					}},
				},
			},
		},
		func() string {
			ids++
			return "owned"
		},
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID: "dispatch-1",
					Execution: work.ExecutionMetadata{
						RequestID: "request-1",
						TraceID:   "trace-1",
						WorkIDs:   []string{"work-source"},
					},
					CurrentChainingTraceID: "chain-1",
				},
			},
		},
		workerexecution.WorkResult{
			DispatchID:   "dispatch-1",
			TransitionID: "t1",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "reviewer output",
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:          "agent-chosen-id",
				WorkTypeID:  "review",
				DisplayName: "review-1",
				State:       "init",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "body",
				}},
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if len(result.RecordedOutputWork) != 1 {
		t.Fatalf("RecordedOutputWork = %#v", result.RecordedOutputWork)
	}
	item := result.RecordedOutputWork[0]
	if item.ID == "agent-chosen-id" || !strings.HasPrefix(item.ID, "work-") {
		t.Fatalf("canonical ID = %q, want Work-owned work-* identity", item.ID)
	}
	if item.ParentID != "work-source" {
		t.Fatalf("ParentID = %q, want work-source", item.ParentID)
	}
	if item.DisplayName != "review-1" || item.WorkTypeID != "review" {
		t.Fatalf("item = %#v", item)
	}
}

func TestMaterializeWorkerOutputForDispatchRejectsUnknownTypeAsFailed(t *testing.T) {
	t.Parallel()

	result := materializeWorkerOutputForDispatch(
		context.Background(),
		testMaterializationService(),
		&state.Net{WorkTypes: map[string]*state.WorkType{"task": {ID: "task"}}},
		func() string { return "1" },
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID: "dispatch-1",
					Execution:  work.ExecutionMetadata{WorkIDs: []string{"work-1"}},
				},
			},
		},
		workerexecution.WorkResult{
			Outcome: workerexecution.OutcomeAccepted,
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:          "agent-id",
				WorkTypeID:  "missing",
				DisplayName: "bad",
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if len(result.RecordedOutputWork) != 0 {
		t.Fatalf("invalid proposals must not enter Runtime state: %#v", result.RecordedOutputWork)
	}
	if !strings.Contains(result.Error, "worker output materialization") {
		t.Fatalf("error = %q, want materialization detail", result.Error)
	}
}

func TestMaterializeWorkerOutputForDispatchRejectsInvalidDetachedProposal(t *testing.T) {
	t.Parallel()

	result := materializeWorkerOutputForDispatchWithProposal(
		context.Background(),
		testMaterializationService(),
		&state.Net{WorkTypes: map[string]*state.WorkType{"task": {ID: "task"}}},
		func() string { return "1" },
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID: "dispatch-detached-invalid",
					Execution:  work.ExecutionMetadata{WorkIDs: []string{"work-1"}},
				},
			},
		},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		&workerexecution.ProposedOutput{
			ProposedWork: []workerexecution.ProposedWork{{
				WorkTypeID: "missing",
				Name:       "invalid-follow-up",
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if len(result.RecordedOutputWork) != 0 {
		t.Fatalf("invalid detached proposal entered Runtime state: %#v", result.RecordedOutputWork)
	}
	if !strings.Contains(result.Error, "worker output materialization") {
		t.Fatalf("error = %q, want materialization detail", result.Error)
	}
}

func TestMaterializeWorkerOutputForDispatchNoopWithoutProposals(t *testing.T) {
	t.Parallel()

	original := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeContinue,
		Output:  "partial",
	}
	result := materializeWorkerOutputForDispatch(
		context.Background(),
		nil,
		nil,
		nil,
		workerexecution.WorkstationDispatchRequest{},
		original,
	)
	if result.Outcome != workerexecution.OutcomeContinue || result.Output != "partial" {
		t.Fatalf("result = %#v, want unchanged", result)
	}
}

func TestMaterializeWorkerOutputForDispatchFailsWithoutWorkService(t *testing.T) {
	t.Parallel()

	result := materializeWorkerOutputForDispatch(
		context.Background(),
		nil,
		nil,
		func() string { return "1" },
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
			},
		},
		workerexecution.WorkResult{
			Outcome: workerexecution.OutcomeAccepted,
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:          "agent-id",
				WorkTypeID:  "task",
				DisplayName: "proposal",
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if len(result.RecordedOutputWork) != 0 {
		t.Fatalf("RecordedOutputWork = %#v, want empty when Work service is unavailable", result.RecordedOutputWork)
	}
	if !strings.Contains(result.Error, "Work service is required") {
		t.Fatalf("error = %q, want Work-service-required detail", result.Error)
	}
}

func TestMaterializeWorkerOutputForDispatchPreservesExistingErrorOnFailure(t *testing.T) {
	t.Parallel()

	result := materializeWorkerOutputForDispatch(
		context.Background(),
		testMaterializationService(),
		&state.Net{WorkTypes: map[string]*state.WorkType{"task": {ID: "task"}}},
		func() string { return "1" },
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
			},
		},
		workerexecution.WorkResult{
			Outcome: workerexecution.OutcomeAccepted,
			Error:   "prior warning",
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:          "agent-id",
				WorkTypeID:  "missing",
				DisplayName: "bad",
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if !strings.Contains(result.Error, "prior warning") || !strings.Contains(result.Error, "worker output materialization") {
		t.Fatalf("error = %q, want both prior warning and materialization detail preserved", result.Error)
	}
}

func TestMaterializeWorkerOutputForDispatchAppliesFeedbackAndClassificationWithoutNet(t *testing.T) {
	t.Parallel()

	result := materializeWorkerOutputForDispatch(
		context.Background(),
		testMaterializationService(),
		nil,
		func() string { return "owned" },
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
			},
		},
		workerexecution.WorkResult{
			Outcome:                     workerexecution.OutcomeAccepted,
			Feedback:                    "needs another pass",
			SelectedClassificationLabel: "accepted",
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:          "agent-id",
				WorkTypeID:  "any-type",
				DisplayName: "proposal",
				State:       "any-state",
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Feedback != "needs another pass" {
		t.Fatalf("Feedback = %q, want propagated materialized feedback", result.Feedback)
	}
	if result.SelectedClassificationLabel != "accepted" {
		t.Fatalf("SelectedClassificationLabel = %q, want propagated materialized classification", result.SelectedClassificationLabel)
	}
	if len(result.RecordedOutputWork) != 1 {
		t.Fatalf("RecordedOutputWork = %#v, want a materialized item without a net's type/state restriction", result.RecordedOutputWork)
	}
}

func TestValidStatesByTypeFromNetHandlesNilNet(t *testing.T) {
	t.Parallel()

	if got := validStatesByTypeFromNet(nil); got != nil {
		t.Fatalf("validStatesByTypeFromNet(nil) = %#v, want nil", got)
	}
}

func TestValidWorkTypesFromNetHandlesNilAndEmptyNet(t *testing.T) {
	t.Parallel()

	if got := validWorkTypesFromNet(nil); got != nil {
		t.Fatalf("validWorkTypesFromNet(nil) = %#v, want nil", got)
	}
	if got := validWorkTypesFromNet(&state.Net{}); got != nil {
		t.Fatalf("validWorkTypesFromNet(empty) = %#v, want nil", got)
	}
}

func TestRecordDetachedAgentRunResponsePreservesSafeDiagnosticsAndTranscript(t *testing.T) {
	t.Parallel()

	ledger := &agentRunRecordingLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{},
	}
	cfg := &runtimeConfig{
		eventHistory: ledger,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"agent": {Name: "agent", Type: interfaces.WorkstationTypeAgent},
			},
			Workers: map[string]*interfaces.FactoryWorkerConfig{},
		},
	}
	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1"},
		Target: workers.ExecutionTarget{
			WorkstationName: "agent",
			Prompt: workers.PromptPolicy{
				SystemPrompt: "Reasoning effort: low",
				UserMessage:  "run the task",
			},
		},
	}
	result := workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "completed",
		}}},
		Diagnostics: &workers.SafeDiagnostics{Provider: &workers.SafeProviderDiagnostic{
			Provider: "codex",
		}},
		Metrics: workers.ExecutionMetrics{Duration: 1250 * time.Millisecond},
	}

	recordDetachedAgentRunResponse(cfg, request, result, nil)

	event := ledger.event
	if event.ID != "factory-event/agent-run-response/dispatch-1" ||
		event.DispatchID != "dispatch-1" ||
		event.Payload.AgentRunID != "dispatch-1/agent-run/1" ||
		event.Payload.Outcome != string(workers.ExecutionOutcomeAccepted) ||
		event.Payload.DurationMillis != 1250 {
		t.Fatalf("recorded event = %#v, want detached agent-run identity and duration", event)
	}
	diagnostics, err := workers.SafeWorkDiagnosticsFromEventPayload(event.Payload.Diagnostics)
	if err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if diagnostics.Provider == nil || diagnostics.Provider.Provider != "codex" {
		t.Fatalf("provider diagnostics = %#v, want codex", diagnostics.Provider)
	}
	if diagnostics.AgentRun == nil || len(diagnostics.AgentRun.Transcript) != 3 {
		t.Fatalf("agent-run diagnostics = %#v, want system/user/assistant transcript", diagnostics.AgentRun)
	}
	if diagnostics.AgentRun.Transcript[0].Summary != "Reasoning effort: low" ||
		diagnostics.AgentRun.Transcript[2].Summary != "completed" {
		t.Fatalf("transcript = %#v, want interpolated prompt and output", diagnostics.AgentRun.Transcript)
	}
}

type agentRunRecordingLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
	event workers.AgentRunResponseEvent
}

func (ledger *agentRunRecordingLedger) RecordAgentRunEvent(event workers.AgentRunResponseEvent) {
	ledger.event = event
}

func testMaterializationService() work.Service {
	return workwire.NewRuntimeService(nil, nil, nil, nil, nil)
}
