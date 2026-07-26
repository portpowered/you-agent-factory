package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
)

type recordingRuntimeAssembly struct {
	request workers.RuntimeBuildRequest
	result  workers.RuntimeBuildResult
	err     error
}

var _ runtimeassembly.Service = (*recordingRuntimeAssembly)(nil)

type inertConstructionSpy struct {
	currentRuntimeCalls int
	commandCalls        int
}

type rootDispatchExecutor struct {
	request workers.WorkstationExecutionRequest
	result  workers.WorkResult
}

type rootBlockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (executor *rootBlockingExecutor) Execute(
	ctx context.Context,
	_ workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	select {
	case executor.started <- struct{}{}:
	default:
	}
	select {
	case <-executor.release:
		return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
	case <-ctx.Done():
		return workers.WorkResult{}, ctx.Err()
	}
}

func (executor *rootDispatchExecutor) Execute(
	_ context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	executor.request = request
	return executor.result, nil
}

func (spy *inertConstructionSpy) CurrentRuntime() *factorysessions.LiveRuntime {
	spy.currentRuntimeCalls++
	return nil
}

func (spy *inertConstructionSpy) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	spy.commandCalls++
	return workers.CommandResult{}, errors.New("unexpected runner execution")
}

func TestServiceDelegatesWorkstationLifecycleThroughWorkersRoot(t *testing.T) {
	t.Parallel()

	var root workers.Service = &Service{workstations: workstationswire.NewService()}
	request := workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{
		{RoleName: "review", RoleKind: workers.RuntimeBuildRoleKindWorkstation},
		{RoleName: "implement", RoleKind: workers.RuntimeBuildRoleKindWorkstation},
	}}

	started, err := root.StartWorkstationPool(context.Background(), request)
	if err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}
	if started.Outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("StartWorkstationPool() outcome = %q, want STARTED", started.Outcome)
	}
	route, err := root.WorkstationRoute(
		context.Background(),
		workers.WorkstationRouteRequest{WorkstationName: "review"},
	)
	if err != nil {
		t.Fatalf("WorkstationRoute() error = %v", err)
	}
	if !route.Available || route.WorkstationName != "review" {
		t.Fatalf("WorkstationRoute() = %#v", route)
	}

	stopped, err := root.StopWorkstationPool(context.Background())
	if err != nil {
		t.Fatalf("StopWorkstationPool() error = %v", err)
	}
	if stopped.Outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("StopWorkstationPool() outcome = %q, want STOPPED", stopped.Outcome)
	}
	if _, err := root.WorkstationRoute(
		context.Background(),
		workers.WorkstationRouteRequest{WorkstationName: "review"},
	); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("WorkstationRoute() after stop error = %v, want ErrWorkstationPoolStopped", err)
	}
}

func TestServiceWorkstationLifecyclePreservesTypedFailures(t *testing.T) {
	t.Parallel()

	var unavailable workers.Service = (*Service)(nil)
	if _, err := unavailable.StartWorkstationPool(
		context.Background(),
		workers.WorkstationPoolStartRequest{},
	); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil StartWorkstationPool() error = %v, want ErrWorkstationPoolUnavailable", err)
	}

	root := &Service{workstations: workstationswire.NewService()}
	if _, err := root.StartWorkstationPool(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: "worker",
			RoleKind: workers.RuntimeBuildRoleKindWorker,
		}}},
	); !errors.Is(err, workers.ErrInvalidWorkstationPoolStart) {
		t.Fatalf("worker binding start error = %v, want ErrInvalidWorkstationPoolStart", err)
	}
}

func TestServiceDelegatesWorkstationDispatchThroughWorkersRoot(t *testing.T) {
	t.Parallel()

	executor := &rootDispatchExecutor{
		result: workers.WorkResult{Outcome: workers.OutcomeAccepted, Output: "routed"},
	}
	var root workers.Service = &Service{workstations: workstationswire.NewService()}
	if _, err := root.StartWorkstationPool(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: "review",
			RoleKind: workers.RuntimeBuildRoleKindWorkstation,
			RunnerSelection: workers.ResolvedRunnerSelection{
				RunnerID: workers.RunnerIDCodex,
				Source:   workers.RunnerSelectionSourceWorkstation,
			},
			Executor: executor,
		}}},
	); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}

	result, err := root.DispatchWorkstation(
		context.Background(),
		workers.WorkstationDispatchRequest{
			WorkstationName: "review",
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID:      "dispatch-1",
					TransitionID:    "transition-1",
					WorkstationName: "review",
					Execution: work.ExecutionMetadata{
						RequestID: "request-1",
						TraceID:   "trace-1",
						WorkIDs:   []string{"work-1"},
					},
					InputTokens: []any{"input"},
				},
				WorkerType:       "reviewer",
				FactorySessionID: "session-1",
			},
		},
	)
	if err != nil {
		t.Fatalf("DispatchWorkstation() error = %v", err)
	}
	if result.DispatchID != "dispatch-1" ||
		result.WorkstationName != "review" ||
		result.Result.DispatchID != "dispatch-1" ||
		result.Result.TransitionID != "transition-1" ||
		result.Result.Output != "routed" {
		t.Fatalf("DispatchWorkstation() = %#v", result)
	}
	if executor.request.RunnerID != workers.RunnerIDCodex ||
		executor.request.RunnerSelectionSource != workers.RunnerSelectionSourceWorkstation ||
		executor.request.WorkerType != "reviewer" ||
		executor.request.FactorySessionID != "session-1" ||
		executor.request.Dispatch.Execution.RequestID != "request-1" ||
		executor.request.Dispatch.Execution.TraceID != "trace-1" {
		t.Fatalf("executor request = %#v", executor.request)
	}
}

func TestServicePreservesCanonicalWorkstationSaturationThroughRoot(t *testing.T) {
	t.Parallel()

	executor := &rootBlockingExecutor{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	var root workers.Service = &Service{workstations: workstationswire.NewService()}
	if _, err := root.StartWorkstationPool(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{{
			RoleName:      "review",
			RoleKind:      workers.RuntimeBuildRoleKindWorkstation,
			Executor:      executor,
			Capacity:      1,
			QueueCapacity: 1,
		}}},
	); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}

	results := make(chan error, 33)
	dispatch := func(id string) {
		go func() {
			_, err := root.DispatchWorkstation(
				context.Background(),
				rootDispatchRequest(id, "review"),
			)
			results <- err
		}()
	}
	dispatch("running")
	<-executor.started
	for index := range 32 {
		dispatch(fmt.Sprintf("waiting-%d", index))
	}

	saturated := 0
	for saturated < 31 {
		select {
		case err := <-results:
			if !errors.Is(err, workers.ErrWorkstationSaturated) {
				t.Fatalf("concurrent DispatchWorkstation() error = %v", err)
			}
			saturated++
		case <-time.After(time.Second):
			t.Fatalf("saturation results = %d, want 31", saturated)
		}
	}
	close(executor.release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("accepted DispatchWorkstation() error = %v", err)
		}
	}
}

func TestServiceDelegatesExplicitWorkstationCancellationThroughRoot(t *testing.T) {
	t.Parallel()

	executor := &rootBlockingExecutor{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	var root workers.Service = &Service{workstations: workstationswire.NewService()}
	if _, err := root.StartWorkstationPool(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{{
			RoleName:      "review",
			RoleKind:      workers.RuntimeBuildRoleKindWorkstation,
			Executor:      executor,
			Capacity:      1,
			QueueCapacity: 1,
		}}},
	); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}

	completed := make(chan struct {
		result workers.WorkstationDispatchResult
		err    error
	}, 1)
	go func() {
		result, err := root.DispatchWorkstation(
			context.Background(),
			rootDispatchRequest("dispatch-cancel", "review"),
		)
		completed <- struct {
			result workers.WorkstationDispatchResult
			err    error
		}{result: result, err: err}
	}()
	<-executor.started

	cancelled, err := root.CancelWorkstationDispatch(
		context.Background(),
		workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-cancel"},
	)
	if err != nil || cancelled.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("CancelWorkstationDispatch() = %#v, %v", cancelled, err)
	}
	dispatch := <-completed
	if !errors.Is(dispatch.err, workers.ErrWorkstationDispatchCanceled) ||
		dispatch.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled ||
		dispatch.result.DispatchID != "dispatch-cancel" {
		t.Fatalf("DispatchWorkstation() = %#v, %v", dispatch.result, dispatch.err)
	}
}

func rootDispatchRequest(dispatchID string, workstationName string) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: workstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{
				DispatchID:      dispatchID,
				TransitionID:    "transition-" + dispatchID,
				WorkstationName: workstationName,
			},
		},
	}
}

func (assembly *recordingRuntimeAssembly) Build(
	_ context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	assembly.request = request
	return assembly.result, assembly.err
}

func TestBuildRuntimeDelegatesThroughWorkersRoot(t *testing.T) {
	t.Parallel()

	want := workers.RuntimeBuildResult{
		RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDCodex,
			Source:   workers.RunnerSelectionSourceFactory,
		},
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: "writer",
			RoleKind: workers.RuntimeBuildRoleKindWorker,
			RunnerSelection: workers.ResolvedRunnerSelection{
				RunnerID: workers.RunnerIDCodex,
				Source:   workers.RunnerSelectionSourceFactory,
			},
		}},
	}
	assembly := &recordingRuntimeAssembly{result: want}
	var root workers.Service = &Service{runtimeAssembly: assembly}
	request := workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "writer",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	}

	got, err := root.BuildRuntime(t.Context(), request)
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if assembly.request.RunnerID != request.RunnerID ||
		len(assembly.request.Roles) != 1 ||
		assembly.request.Roles[0] != request.Roles[0] {
		t.Fatalf("delegated request = %#v, want %#v", assembly.request, request)
	}
	if got.RunnerSelection != want.RunnerSelection ||
		len(got.Bindings) != 1 ||
		got.Bindings[0] != want.Bindings[0] {
		t.Fatalf("BuildRuntime() = %#v, want %#v", got, want)
	}
}

func TestBuildRuntimePreservesAssemblyErrorIdentity(t *testing.T) {
	t.Parallel()

	for _, wantErr := range []error{
		workers.ErrInvalidRuntimeBuildRequest,
		workers.ErrMissingRunnerSelection,
		workers.ErrUnknownRunnerSelection,
		workers.ErrRuntimeAssemblyRejected,
		workers.ErrIncompleteRuntimeAssembly,
	} {
		wantErr := wantErr
		t.Run(wantErr.Error(), func(t *testing.T) {
			t.Parallel()

			assembly := &recordingRuntimeAssembly{err: wantErr}
			var root workers.Service = &Service{runtimeAssembly: assembly}

			result, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{})
			if !errors.Is(err, wantErr) {
				t.Fatalf("BuildRuntime() error = %v, want errors.Is(_, %v)", err, wantErr)
			}
			if result.RunnerSelection != (workers.ResolvedRunnerSelection{}) ||
				len(result.Bindings) != 0 {
				t.Fatalf("BuildRuntime() result = %#v, want no usable bindings", result)
			}
		})
	}
}

func TestBuildRuntimeRequiresPrivateAssemblyCapability(t *testing.T) {
	t.Parallel()

	for _, root := range []workers.Service{
		(*Service)(nil),
		&Service{},
	} {
		result, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{})
		if !errors.Is(err, workers.ErrIncompleteRuntimeAssembly) {
			t.Fatalf(
				"BuildRuntime() error = %v, want errors.Is(_, ErrIncompleteRuntimeAssembly)",
				err,
			)
		}
		if result.RunnerSelection != (workers.ResolvedRunnerSelection{}) ||
			len(result.Bindings) != 0 {
			t.Fatalf("BuildRuntime() result = %#v, want no usable bindings", result)
		}
	}
}

func TestRuntimeAssemblyConstructionAndBuildAreInert(t *testing.T) {
	t.Parallel()

	spy := &inertConstructionSpy{}
	assembly, err := newRuntimeAssembly(nil)
	if err != nil {
		t.Fatalf("newRuntimeAssembly() error = %v", err)
	}
	var root workers.Service = &Service{
		sessions:              spy,
		providerCommandRunner: spy,
		scriptCommandRunner:   spy,
		runtimeAssembly:       assembly,
	}
	if spy.currentRuntimeCalls != 0 || spy.commandCalls != 0 {
		t.Fatalf(
			"construction side effects = current runtime %d, commands %d; want zero",
			spy.currentRuntimeCalls,
			spy.commandCalls,
		)
	}

	result, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Roles: []workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
			{Name: "review", Kind: workers.RuntimeBuildRoleKindWorkstation},
		},
	})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if len(result.Bindings) != 2 {
		t.Fatalf("BuildRuntime() bindings = %#v, want two", result.Bindings)
	}
	if spy.currentRuntimeCalls != 0 || spy.commandCalls != 0 {
		t.Fatalf(
			"assembly side effects = current runtime %d, commands %d; want zero",
			spy.currentRuntimeCalls,
			spy.commandCalls,
		)
	}
}
