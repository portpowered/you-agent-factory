package wire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
)

// TestNewWorkstationPool_RunsAWorkerThroughItsOwnBinding proves the standalone
// pool is a complete execution route.
//
// It exists so a session whose Factory has no authored workstations can still
// run its Workers through the one real route. A JavaScript Factory is exactly
// that case: its children are Workers, but no Petri runtime is composed for it
// and therefore no pool exists. Without this the session reaches Worker
// Sessions through nothing at all.
func TestNewWorkstationPool_RunsAWorkerThroughItsOwnBinding(t *testing.T) {
	executor := &recordingWorkstationExecutor{
		result: workers.WorkResult{Outcome: workers.OutcomeAccepted, Output: "done"},
	}
	pool := workerswire.NewWorkstationPool(logging.NoopLogger{})
	ctx := context.Background()

	if _, err := pool.StartWorkstationPool(ctx, workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: workers.ProviderInvocationRoute,
			RoleKind: workers.RuntimeBuildRoleKindWorkstation,
			Executor: executor,
		}},
	}); err != nil {
		t.Fatalf("StartWorkstationPool: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.StopWorkstationPool(context.Background()) })

	result, err := pool.DispatchWorkstation(ctx, providerInvocationDispatch("dispatch-1"))
	if err != nil {
		t.Fatalf("DispatchWorkstation: %v", err)
	}
	if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("terminal outcome = %q, want COMPLETED", result.TerminalOutcome)
	}
	if result.Result.Output != "done" {
		t.Fatalf("output = %q, want the executor's own", result.Result.Output)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

// TestNewWorkstationPool_IsInertUntilStarted keeps composition honest: a
// constructed pool admits nothing until its binding snapshot is committed, so
// a half-built session cannot dispatch into an empty route table.
func TestNewWorkstationPool_IsInertUntilStarted(t *testing.T) {
	pool := workerswire.NewWorkstationPool(logging.NoopLogger{})

	_, err := pool.DispatchWorkstation(context.Background(), providerInvocationDispatch("dispatch-1"))
	if !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("DispatchWorkstation before start = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := pool.CancelWorkstationDispatch(context.Background(), workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-1"}); err == nil {
		t.Fatal("CancelWorkstationDispatch before start error = nil, want unavailable pool error")
	}
}

// TestNewWorkstationPool_StoppedPoolRefusesFurtherWork pins the lifecycle a
// session's shutdown depends on.
func TestNewWorkstationPool_StoppedPoolRefusesFurtherWork(t *testing.T) {
	pool := workerswire.NewWorkstationPool(logging.NoopLogger{})
	ctx := context.Background()
	if _, err := pool.StartWorkstationPool(ctx, workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: workers.ProviderInvocationRoute,
			RoleKind: workers.RuntimeBuildRoleKindWorkstation,
			Executor: &recordingWorkstationExecutor{},
		}},
	}); err != nil {
		t.Fatalf("StartWorkstationPool: %v", err)
	}
	if _, err := pool.StopWorkstationPool(ctx); err != nil {
		t.Fatalf("StopWorkstationPool: %v", err)
	}
	if _, err := pool.DispatchWorkstation(ctx, providerInvocationDispatch("dispatch-1")); !errors.Is(
		err, workers.ErrWorkstationPoolStopped,
	) {
		t.Fatalf("DispatchWorkstation after stop = %v, want ErrWorkstationPoolStopped", err)
	}
}

// TestNewProviderInvocationExecutor_AbsentInvocationYieldsNoExecutor lets
// composition treat "this process cannot reach a provider" as an absent route
// rather than one that fails at dispatch time.
func TestNewProviderInvocationExecutor_AbsentInvocationYieldsNoExecutor(t *testing.T) {
	if executor := workerswire.NewProviderInvocationExecutor(nil); executor != nil {
		t.Fatalf("NewProviderInvocationExecutor(nil) = %#v, want nil", executor)
	}
}

func providerInvocationDispatch(dispatchID string) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: workers.ProviderInvocationRoute,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{
				DispatchID:      dispatchID,
				WorkstationName: workers.ProviderInvocationRoute,
			},
		},
	}
}

type recordingWorkstationExecutor struct {
	calls  int
	result workers.WorkResult
}

func (e *recordingWorkstationExecutor) Execute(
	context.Context,
	workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	e.calls++
	return e.result, nil
}
