package runtime

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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

func testMaterializationService() work.Service {
	return workwire.NewRuntimeService(nil, nil, nil, nil, nil)
}
