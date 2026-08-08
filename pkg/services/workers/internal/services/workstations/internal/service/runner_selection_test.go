package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

// TestPoolDispatchKeepsRequestRunnerWhenRouteResolvesNone proves a route that
// pinned no runner leaves the dispatch's own selection alone.
//
// A route assembled from an authored workstation resolves a runner, and that
// choice is authoritative -- the case above pins that. A provider-invocation
// route resolves none, because the Worker it serves has no workstation
// definition and its caller named the runner per dispatch: one JavaScript
// workflow child may run on codex and the next on claude. Overwriting from an
// empty route selection blanked the only selection that existed, and the
// provider rejected every such Worker as a bad request.
func TestPoolDispatchKeepsRequestRunnerWhenRouteResolvesNone(t *testing.T) {
	t.Parallel()

	executor := &recordingExecutor{result: workers.WorkResult{Outcome: workers.OutcomeAccepted, Output: "done"}}
	pool := New()
	if _, err := pool.start(context.Background(), []workstations.Route{{
		WorkstationName: workers.ProviderInvocationRoute,
		Executor:        executor,
	}}); err != nil {
		t.Fatalf("start() error = %v", err)
	}

	request := dispatchRequest("dispatch-child", "", workers.ProviderInvocationRoute)
	request.Execution.RunnerID = workers.RunnerIDClaude
	request.Execution.RunnerSelectionSource = workers.RunnerSelectionSourceFactory
	if _, err := pool.Dispatch(context.Background(), request); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if len(executor.requests) != 1 {
		t.Fatalf("executor calls = %d, want 1", len(executor.requests))
	}
	if got := executor.requests[0].RunnerID; got != workers.RunnerIDClaude {
		t.Fatalf("RunnerID = %q, want %q preserved from the dispatch", got, workers.RunnerIDClaude)
	}
	if got := executor.requests[0].RunnerSelectionSource; got != workers.RunnerSelectionSourceFactory {
		t.Fatalf("RunnerSelectionSource = %q, want %q preserved", got, workers.RunnerSelectionSourceFactory)
	}
}
