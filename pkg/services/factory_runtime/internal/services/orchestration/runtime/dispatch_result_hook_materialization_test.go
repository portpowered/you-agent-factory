package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
