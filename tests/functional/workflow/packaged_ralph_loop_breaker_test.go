package workflow

import (
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/ralph"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestPackagedRalph_ConvergesOrRoutesExhaustedExecutorToFailed(t *testing.T) {
	cases := []struct {
		name            string
		plannerResult   interfaces.WorkResult
		executorResults []interfaces.WorkResult
		wantPlace       string
		wantCalls       int
		wantLoopBreaker bool
	}{
		{
			name:          "converges",
			plannerResult: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "planned work"},
			executorResults: []interfaces.WorkResult{
				{Outcome: interfaces.OutcomeContinue, Output: "continue the plan"},
				{Outcome: interfaces.OutcomeAccepted, Output: "completed the plan"},
			},
			wantPlace: "ralph:complete",
			wantCalls: 2,
		},
		{
			name:            "exhausts_continuations",
			plannerResult:   interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "planned work"},
			executorResults: repeatedRalphContinueResults(8),
			wantPlace:       "ralph:failed",
			wantCalls:       8,
			wantLoopBreaker: true,
		},
		{
			name:          "planner_failure",
			plannerResult: interfaces.WorkResult{Outcome: interfaces.OutcomeFailed, Error: "planner failed"},
			wantPlace:     "ralph:failed",
			wantCalls:     0,
		},
		{
			name:          "executor_failure",
			plannerResult: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "planned work"},
			executorResults: []interfaces.WorkResult{
				{Outcome: interfaces.OutcomeFailed, Error: "executor failed"},
			},
			wantPlace: "ralph:failed",
			wantCalls: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), ralph.PackagedFactoryName, ralph.BuiltInFactoryJSON)
			if err != nil {
				t.Fatalf("PersistNamedFactory: %v", err)
			}
			testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
				WorkTypeID: ralph.PackagedWorkTypeName,
				WorkID:     "ralph-loop-work",
				Payload:    []byte("finish the requested work"),
			})

			harness := testutil.NewServiceTestHarness(t, dir)
			harness.MockWorker("ralph-planner", tc.plannerResult)
			executor := harness.MockWorker("ralph-executor", tc.executorResults...)
			harness.RunUntilComplete(t, 15*time.Second)

			if got := executor.CallCount(); got != tc.wantCalls {
				t.Fatalf("executor calls = %d, want %d", got, tc.wantCalls)
			}
			harness.Assert().PlaceTokenCount(tc.wantPlace, 1)
			snapshot, err := harness.GetEngineStateSnapshot()
			if err != nil {
				t.Fatalf("GetEngineStateSnapshot: %v", err)
			}
			if tc.name == "converges" {
				assertRalphDispatchPrefix(t, snapshot.DispatchHistory, []string{
					ralph.PackagedPlanWorkstationName,
					ralph.PackagedExecuteWorkstationName,
					ralph.PackagedExecuteWorkstationName,
				})
			}
			if tc.wantLoopBreaker {
				assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, ralph.PackagedLoopBreakerName, "ralph:failed")
			}
		})
	}
}

func assertRalphDispatchPrefix(t *testing.T, history []interfaces.CompletedDispatch, want []string) {
	t.Helper()
	if len(history) < len(want) {
		t.Fatalf("dispatch history = %#v, want at least %d dispatches", history, len(want))
	}
	for index, workstationName := range want {
		if got := history[index].WorkstationName; got != workstationName {
			t.Fatalf("dispatch %d workstation = %q, want %q", index, got, workstationName)
		}
	}
}

func repeatedRalphContinueResults(count int) []interfaces.WorkResult {
	results := make([]interfaces.WorkResult, count)
	for index := range results {
		results[index] = interfaces.WorkResult{Outcome: interfaces.OutcomeContinue, Output: "continue the plan"}
	}
	return results
}
