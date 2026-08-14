package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
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
	commandCalls int
}

type rootDispatchExecutor struct {
	request workers.WorkstationExecutionRequest
	result  workers.WorkResult
	err     error
	panic   any
	calls   int
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
	executor.calls++
	executor.request = request
	if executor.panic != nil {
		panic(executor.panic)
	}
	return executor.result, executor.err
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

	var root workers.Service = &Service{Root: RootFrom(nil, workstationswire.NewService())}
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

	root := &Service{Root: RootFrom(nil, workstationswire.NewService())}
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
	var root workers.Service = &Service{Root: RootFrom(nil, workstationswire.NewService())}
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
	var root workers.Service = &Service{Root: RootFrom(nil, workstationswire.NewService())}
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
	var root workers.Service = &Service{Root: RootFrom(nil, workstationswire.NewService())}
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

func TestServicePreservesWorkstationExecutorFailureThroughRoot(t *testing.T) {
	t.Parallel()

	executeErr := errors.New("executor failed")
	root := startedRootService(t, "review", &rootDispatchExecutor{err: executeErr})
	result, err := root.DispatchWorkstation(
		context.Background(),
		rootDispatchRequest("dispatch-failed", "review"),
	)
	if !errors.Is(err, executeErr) ||
		result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		result.DispatchID != "dispatch-failed" ||
		result.WorkstationName != "review" {
		t.Fatalf("DispatchWorkstation() = %#v, %v", result, err)
	}
}

func TestServiceNormalizesWorkstationExecutorPanicThroughRoot(t *testing.T) {
	t.Parallel()

	root := startedRootService(t, "review", &rootDispatchExecutor{panic: "boom"})
	result, err := root.DispatchWorkstation(
		context.Background(),
		rootDispatchRequest("dispatch-panic", "review"),
	)
	if err == nil || !strings.Contains(err.Error(), "workstation executor panic: boom") ||
		result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		result.DispatchID != "dispatch-panic" ||
		result.Result.DispatchID != "dispatch-panic" ||
		result.Result.Outcome != workers.OutcomeFailed ||
		result.Result.Error != "workstation executor panic: boom" {
		t.Fatalf("DispatchWorkstation() = %#v, %v", result, err)
	}
}

func TestServiceRejectsUnknownWorkstationRouteThroughRoot(t *testing.T) {
	t.Parallel()

	executor := &rootDispatchExecutor{}
	root := startedRootService(t, "review", executor)
	result, err := root.DispatchWorkstation(
		context.Background(),
		rootDispatchRequest("dispatch-unknown", "implement"),
	)
	if !errors.Is(err, workers.ErrUnknownWorkstationRoute) ||
		!emptyWorkstationResult(result) ||
		executor.calls != 0 {
		t.Fatalf(
			"DispatchWorkstation() = %#v, %v; executor calls = %d",
			result,
			err,
			executor.calls,
		)
	}
}

func TestServiceRejectsWorkstationDispatchAfterStopThroughRoot(t *testing.T) {
	t.Parallel()

	executor := &rootDispatchExecutor{}
	root := startedRootService(t, "review", executor)
	if _, err := root.StopWorkstationPool(context.Background()); err != nil {
		t.Fatalf("StopWorkstationPool() error = %v", err)
	}
	result, err := root.DispatchWorkstation(
		context.Background(),
		rootDispatchRequest("dispatch-stopped", "review"),
	)
	if !errors.Is(err, workers.ErrWorkstationPoolStopped) ||
		!emptyWorkstationResult(result) ||
		executor.calls != 0 {
		t.Fatalf(
			"DispatchWorkstation() = %#v, %v; executor calls = %d",
			result,
			err,
			executor.calls,
		)
	}
}

func emptyWorkstationResult(result workers.WorkstationDispatchResult) bool {
	return result.DispatchID == "" &&
		result.WorkstationName == "" &&
		result.TerminalOutcome == "" &&
		result.Result.DispatchID == ""
}

func startedRootService(
	t *testing.T,
	workstationName string,
	executor workers.WorkstationRequestExecutor,
) workers.Service {
	t.Helper()
	var root workers.Service = &Service{Root: RootFrom(nil, workstationswire.NewService())}
	if _, err := root.StartWorkstationPool(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{{
			RoleName:      workstationName,
			RoleKind:      workers.RuntimeBuildRoleKindWorkstation,
			Executor:      executor,
			Capacity:      1,
			QueueCapacity: 1,
		}}},
	); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}
	return root
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
	var root workers.Service = &Service{Root: RootFrom(assembly, nil)}
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
		workers.ErrUnsupportedRunnerCapability,
		workers.ErrRuntimeAssemblyRejected,
		workers.ErrIncompleteRuntimeAssembly,
	} {
		wantErr := wantErr
		t.Run(wantErr.Error(), func(t *testing.T) {
			t.Parallel()

			assembly := &recordingRuntimeAssembly{err: wantErr}
			var root workers.Service = &Service{Root: RootFrom(assembly, nil)}

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

type runtimeRunnerSpy struct {
	calls int
}

func (runner *runtimeRunnerSpy) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	runner.calls++
	return workers.RunnerExecutionResult{}, errors.New("unexpected runner execution")
}

func TestRuntimeBuildUsesPrivateRegistryWithoutRunnerExecution(t *testing.T) {
	t.Parallel()

	runner := &runtimeRunnerSpy{}
	metadata, ok := workerrunner.BuiltInRunnerMetadata(workers.RunnerIDCodex)
	if !ok {
		t.Fatal("Codex metadata is unavailable")
	}
	assembly, err := newRuntimeAssemblyFromRegistrations([]runners.Registration{{
		Identity: workers.RunnerIDCodex,
		Metadata: metadata,
		Runner:   runner,
	}})
	if err != nil {
		t.Fatalf("newRuntimeAssemblyFromRegistrations() error = %v", err)
	}
	var root workers.Service = &Service{Root: RootFrom(assembly, nil)}
	valid := workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		RequiredRunnerCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorktree,
		},
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "writer",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	}

	result, err := root.BuildRuntime(t.Context(), valid)
	if err != nil {
		t.Fatalf("BuildRuntime(valid) error = %v", err)
	}
	if result.RunnerSelection.RunnerID != workers.RunnerIDCodex ||
		len(result.Bindings) != 1 ||
		result.Bindings[0].RunnerSelection != result.RunnerSelection {
		t.Fatalf("BuildRuntime(valid) = %#v, want registry-backed binding", result)
	}

	cases := []struct {
		name    string
		request workers.RuntimeBuildRequest
		want    error
	}{
		{
			name: "missing",
			request: workers.RuntimeBuildRequest{
				Roles: valid.Roles,
			},
			want: workers.ErrMissingRunnerSelection,
		},
		{
			name: "unknown",
			request: workers.RuntimeBuildRequest{
				RunnerID: "unknown",
				Roles:    valid.Roles,
			},
			want: workers.ErrUnknownRunnerSelection,
		},
		{
			name: "unsupported",
			request: workers.RuntimeBuildRequest{
				RunnerID: workers.RunnerIDCodex,
				RequiredRunnerCapabilities: []workers.RunnerOptionalCapability{
					workers.RunnerOptionalCapability("unsupported"),
				},
				Roles: valid.Roles,
			},
			want: workers.ErrUnsupportedRunnerCapability,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			failed, buildErr := root.BuildRuntime(t.Context(), test.request)
			if !errors.Is(buildErr, test.want) {
				t.Fatalf("BuildRuntime() error = %v, want %v", buildErr, test.want)
			}
			if len(failed.Bindings) != 0 {
				t.Fatalf("BuildRuntime() result = %#v, want unusable result", failed)
			}
		})
	}
	if runner.calls != 0 {
		t.Fatalf("runner execution calls = %d, want zero", runner.calls)
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
		providerCommandRunner: spy,
		scriptCommandRunner:   spy,
		Root:                  RootFrom(assembly, nil),
	}
	if spy.commandCalls != 0 {
		t.Fatalf(
			"construction side effects = commands %d; want zero",
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
	if spy.commandCalls != 0 {
		t.Fatalf(
			"assembly side effects = commands %d; want zero",
			spy.commandCalls,
		)
	}
}

func TestRootCompatibilityDelegatesAdmissionAndReplacement(t *testing.T) {
	t.Parallel()

	firstAssembly := &recordingRuntimeAssembly{}
	secondAssembly := &recordingRuntimeAssembly{
		result: workers.RuntimeBuildResult{RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDCodex,
		}},
	}
	replaced := RootFrom(firstAssembly, workstationswire.NewService())
	replaced = replaced.ReplaceRuntimeAssembly(secondAssembly)
	replaced = replaced.ReplaceWorkstations(workstationswire.NewService())

	if _, err := replaced.BuildRuntime(context.Background(), workers.RuntimeBuildRequest{}); err != nil {
		t.Fatalf("BuildRuntime() after runtime replacement error = %v", err)
	}
	if secondAssembly.request.RunnerID != "" {
		t.Fatalf("replacement runtime received unexpected runner ID %q", secondAssembly.request.RunnerID)
	}

	service := workers.Service(&replaced)
	if _, err := service.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: workers.RunnerIDCodex,
			RoleKind: workers.RuntimeBuildRoleKindWorkstation,
			Executor: &compatibilityWorkstationExecutor{},
		}},
	}); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}
	var admitted bool
	result, err := service.DispatchWorkstationWithAdmission(
		context.Background(),
		rootDispatchRequest("compatibility-dispatch", workers.RunnerIDCodex),
		func() { admitted = true },
	)
	if err != nil {
		t.Fatalf("DispatchWorkstationWithAdmission() error = %v", err)
	}
	if !admitted || result.DispatchID != "compatibility-dispatch" {
		t.Fatalf("admission/result = %t/%#v, want admitted dispatch", admitted, result)
	}

	if _, err := service.StopWorkstationPool(context.Background()); err != nil {
		t.Fatalf("StopWorkstationPool() error = %v", err)
	}
	if _, err := service.InvokeModel(context.Background(), "model", models.Request{}); err == nil {
		t.Fatal("Root.InvokeModel() error = nil, want unavailable runtime")
	}

	delegating := startedRootService(t, "service-admission", &compatibilityWorkstationExecutor{})
	var serviceAdmission bool
	if _, err := delegating.DispatchWorkstationWithAdmission(
		context.Background(),
		rootDispatchRequest("service-admission-dispatch", "service-admission"),
		func() { serviceAdmission = true },
	); err != nil || !serviceAdmission {
		t.Fatalf("Service.DispatchWorkstationWithAdmission() = %v, admitted=%t; want successful admission", err, serviceAdmission)
	}
	if err := (*Service)(nil).Close(context.Background()); err != nil {
		t.Fatalf("nil Service.Close() error = %v, want nil", err)
	}
}

func TestNilRootCapabilitiesRemainUnavailable(t *testing.T) {
	t.Parallel()

	var root *Root
	if _, err := root.Execute(context.Background(), workers.ExecuteRequest{}); !errors.Is(err, workers.ErrExecuteUnavailable) {
		t.Fatalf("nil Root.Execute() error = %v, want ErrExecuteUnavailable", err)
	}
	if _, err := root.BuildRuntime(context.Background(), workers.RuntimeBuildRequest{}); !errors.Is(err, workers.ErrIncompleteRuntimeAssembly) {
		t.Fatalf("nil Root.BuildRuntime() error = %v, want ErrIncompleteRuntimeAssembly", err)
	}
	if _, err := root.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.StartWorkstationPool() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.StopWorkstationPool(context.Background()); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.StopWorkstationPool() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.WorkstationRoute() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.DispatchWorkstation(context.Background(), workers.WorkstationDispatchRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.DispatchWorkstation() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.DispatchWorkstationWithAdmission(context.Background(), workers.WorkstationDispatchRequest{}, nil); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.DispatchWorkstationWithAdmission() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := root.CancelWorkstationDispatch(context.Background(), workers.WorkstationDispatchCancelRequest{}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("nil Root.CancelWorkstationDispatch() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
}

func TestOwnedProviderLifecyclesClosesInReverseAndRejectsLateServices(t *testing.T) {
	t.Parallel()

	var closed []string
	owned := &ownedProviderLifecycles{}
	if !owned.Add(testProvidersService{}) {
		t.Fatal("Add(non-lifecycle service) = false, want true")
	}
	if !owned.Add(&compatibilityProviderLifecycle{label: "first", closed: &closed, err: errors.New("first close")}) {
		t.Fatal("Add(first lifecycle) = false, want true")
	}
	if !owned.Add(&compatibilityProviderLifecycle{label: "second", closed: &closed}) {
		t.Fatal("Add(second lifecycle) = false, want true")
	}

	err := owned.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), "first close") {
		t.Fatalf("Close() error = %v, want joined lifecycle error", err)
	}
	if strings.Join(closed, ",") != "second,first" {
		t.Fatalf("closed lifecycle order = %v, want reverse registration order", closed)
	}
	if owned.Add(&compatibilityProviderLifecycle{label: "late", closed: &closed}) {
		t.Fatal("Add(after Close) = true, want false")
	}
	if err := owned.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}

	var nilOwned *ownedProviderLifecycles
	if nilOwned.Add(testProvidersService{}) {
		t.Fatal("nil owned lifecycle Add() = true, want false")
	}
	if err := nilOwned.Close(context.Background()); err != nil {
		t.Fatalf("nil owned lifecycle Close() error = %v, want nil", err)
	}
}

func TestRuntimeCompatibilityRejectsUnsupportedAndUsesInjectedExecutor(t *testing.T) {
	t.Parallel()

	if _, err := BuildRuntimeExecutors(nil, nil, nil, "", nil, nil, false, nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("BuildRuntimeExecutors(nil) error = nil, want unsupported runtime error")
	}
	if _, err := (runtimeAssemblyRunner{}).Execute(context.Background(), workers.RunnerExecutionRequest{}); !errors.Is(err, workers.ErrIncompleteRuntimeAssembly) {
		t.Fatalf("runtime assembly runner error = %v, want ErrIncompleteRuntimeAssembly", err)
	}

	service := &Service{}
	if service.CurrentModelRuntimeConfig() != nil {
		t.Fatal("CurrentModelRuntimeConfig() != nil, want no implicit runtime")
	}
	var called bool
	want := &compatibilityWorkstationExecutor{}
	service.modelInvocationExecutorOverride = func(
		interfaces.RuntimeConfigLookup,
		*interfaces.FactoryConfig,
		string,
	) (workers.WorkstationRequestExecutor, error) {
		called = true
		return want, nil
	}
	got, err := service.modelInvocationExecutor(nil, nil, "worker")
	if err != nil || got != want || !called {
		t.Fatalf("modelInvocationExecutor() = %#v, %v, called=%t; want injected executor", got, err, called)
	}
}

func TestRuntimeAssemblyRegistrationsUseProviderRegistryMetadata(t *testing.T) {
	t.Parallel()

	registrations, err := runtimeAssemblyRegistrations(compatibilityProviderRegistry{})
	if err != nil {
		t.Fatalf("runtimeAssemblyRegistrations() error = %v", err)
	}
	if len(registrations) != 1 || registrations[0].Identity != workers.RunnerIDCodex {
		t.Fatalf("runtime assembly registrations = %#v, want one Codex registration", registrations)
	}
}

func TestServiceCommandClockUsesInjectedClockAndSafeFallback(t *testing.T) {
	t.Parallel()

	fallback := serviceCommandClock(nil).Now()
	if fallback.IsZero() {
		t.Fatal("serviceCommandClock(nil) returned zero time")
	}
	want := time.Unix(42, 0)
	if got := serviceCommandClock(&Service{clock: func() time.Time { return want }}).Now(); !got.Equal(want) {
		t.Fatalf("serviceCommandClock(injected) = %v, want %v", got, want)
	}
}

type compatibilityProviderLifecycle struct {
	providers.Service
	label  string
	closed *[]string
	err    error
}

func (lifecycle *compatibilityProviderLifecycle) Close(context.Context) error {
	*lifecycle.closed = append(*lifecycle.closed, lifecycle.label)
	return lifecycle.err
}

type compatibilityWorkstationExecutor struct{}

func (compatibilityWorkstationExecutor) Execute(
	context.Context,
	workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
}

type compatibilityProviderRegistry struct{}

func (compatibilityProviderRegistry) UsesNativeRunner(string) bool { return true }

func (compatibilityProviderRegistry) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}

func (compatibilityProviderRegistry) RunnerIdentities() []string {
	return []string{workers.RunnerIDCodex}
}

func (compatibilityProviderRegistry) RunnerMetadata(string) (workers.RunnerMetadata, error) {
	metadata, _ := workerrunner.BuiltInRunnerMetadata(workers.RunnerIDCodex)
	return metadata, nil
}

func (compatibilityProviderRegistry) ValidateRunnerPrerequisites(platformprocess.ExecutableLocator, string) error {
	return nil
}

func (compatibilityProviderRegistry) ResolveRunnerSelection(string, string, string) (workers.ResolvedRunnerSelection, error) {
	return workers.ResolvedRunnerSelection{RunnerID: workers.RunnerIDCodex}, nil
}
