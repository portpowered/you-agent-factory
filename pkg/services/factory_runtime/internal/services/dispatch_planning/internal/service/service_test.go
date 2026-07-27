package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPlanPreservesSchedulerOrderAndCanonicalWorkersFacts(t *testing.T) {
	t.Parallel()

	decisions := []dispatchplanning.RunnableDecision{
		runnableDecision("dispatch-review", "correlation-review", "review", "reviewer", "work-2"),
		runnableDecision("dispatch-implement", "correlation-implement", "implement", "implementer", "work-1"),
	}
	decisions[0].Execution.FactorySessionID = "session-1"
	decisions[0].Dispatch.Execution.RequestID = "request-1"
	decisions[0].Dispatch.Execution.TraceID = "trace-1"
	decisions[0].Dispatch.InputBindings = map[string][]string{"work": {"token-2"}}
	decisions[0].Execution.EnvVars = map[string]string{"MODE": "review"}
	wantExecutions := []workers.WorkstationExecutionRequest{
		expectedExecution(decisions[0]),
		expectedExecution(decisions[1]),
	}

	result, err := New().Plan(context.Background(), dispatchplanning.PlanRequest{Decisions: decisions})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(result.Actions) != 2 {
		t.Fatalf("Plan() actions = %d, want 2", len(result.Actions))
	}
	for index, want := range wantExecutions {
		action := result.Actions[index]
		if action.CorrelationID != decisions[index].CorrelationID {
			t.Fatalf("action %d correlation = %q, want %q", index, action.CorrelationID, decisions[index].CorrelationID)
		}
		if action.Request.WorkstationName != want.Dispatch.WorkstationName {
			t.Fatalf("action %d workstation = %q, want %q", index, action.Request.WorkstationName, want.Dispatch.WorkstationName)
		}
		if !reflect.DeepEqual(action.Request.Execution, want) {
			t.Fatalf("action %d execution = %#v, want %#v", index, action.Request.Execution, want)
		}
	}

	decisions[0].Dispatch.InputTokens[0] = "mutated"
	decisions[0].Execution.InputPayload[0] = "mutated"
	decisions[0].Dispatch.Execution.WorkIDs[0] = "mutated"
	decisions[0].Dispatch.InputBindings["work"][0] = "mutated"
	decisions[0].Execution.EnvVars["MODE"] = "mutated"
	if !reflect.DeepEqual(result.Actions[0].Request.Execution, wantExecutions[0]) {
		t.Fatal("planned action retained mutable decision payload aliases")
	}
}

func TestPlanRejectsWholeBatchBeforeReturningActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		decisions []dispatchplanning.RunnableDecision
	}{
		{
			name: "incomplete later decision",
			decisions: []dispatchplanning.RunnableDecision{
				runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
				runnableDecision("dispatch-2", "", "implement", "implementer", "work-2"),
			},
		},
		{
			name: "duplicate identity",
			decisions: []dispatchplanning.RunnableDecision{
				runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
				runnableDecision("dispatch-1", "correlation-2", "implement", "implementer", "work-2"),
			},
		},
		{
			name: "missing input payload",
			decisions: []dispatchplanning.RunnableDecision{
				runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
			},
		},
	}
	tests[2].decisions[0].Dispatch.InputTokens = nil

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := New().Plan(context.Background(), dispatchplanning.PlanRequest{Decisions: test.decisions})
			if !errors.Is(err, dispatchplanning.ErrInvalidRunnableDecision) {
				t.Fatalf("Plan() error = %v, want ErrInvalidRunnableDecision", err)
			}
			if len(result.Actions) != 0 {
				t.Fatalf("Plan() actions = %#v, want no partially visible actions", result.Actions)
			}
		})
	}
}

func TestPlanHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := New().Plan(ctx, dispatchplanning.PlanRequest{
		Decisions: []dispatchplanning.RunnableDecision{
			runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Plan() error = %v, want context.Canceled", err)
	}
	if len(result.Actions) != 0 {
		t.Fatalf("Plan() actions = %#v, want none", result.Actions)
	}
}

func runnableDecision(
	dispatchID string,
	correlationID string,
	workstationName string,
	workerType string,
	workID string,
) dispatchplanning.RunnableDecision {
	return dispatchplanning.RunnableDecision{
		CorrelationID: correlationID,
		Dispatch: work.WorkDispatch{
			DispatchID:      dispatchID,
			WorkerType:      workerType,
			WorkstationName: workstationName,
			Execution: work.ExecutionMetadata{
				WorkIDs:   []string{workID},
				ReplayKey: "replay/" + workID,
			},
			InputTokens: []any{"payload-" + workID},
		},
		Execution: dispatchplanning.ExecutionFacts{
			WorkerType:   workerType,
			InputPayload: []any{"payload-" + workID},
		},
	}
}

func expectedExecution(decision dispatchplanning.RunnableDecision) workers.WorkstationExecutionRequest {
	facts := decision.Execution
	return workers.CloneWorkstationExecutionRequest(workers.WorkstationExecutionRequest{
		Dispatch:                 decision.Dispatch,
		WorkerType:               facts.WorkerType,
		WorkstationType:          facts.WorkstationType,
		RunnerID:                 facts.RunnerID,
		RunnerSelectionSource:    facts.RunnerSelectionSource,
		ProjectID:                facts.ProjectID,
		FactorySessionID:         facts.FactorySessionID,
		InputTokens:              facts.InputPayload,
		ModelOperation:           facts.ModelOperation,
		ModelBindings:            facts.ModelBindings,
		Model:                    facts.Model,
		ModelProvider:            facts.ModelProvider,
		SystemPrompt:             facts.SystemPrompt,
		UserMessage:              facts.UserMessage,
		OutputSchema:             facts.OutputSchema,
		EnvVars:                  facts.EnvVars,
		ProcessEnvironment:       facts.ProcessEnvironment,
		Worktree:                 facts.Worktree,
		WorkingDirectory:         facts.WorkingDirectory,
		WorkingDirectoryAuthored: facts.WorkingDirectoryAuthored,
	})
}
