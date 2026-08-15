package workers

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type poolBoundaryTestExecutor struct{}

func (poolBoundaryTestExecutor) Execute(context.Context, work.WorkDispatch) (WorkResult, error) {
	return WorkResult{}, nil
}

type poolBoundaryPanicExecutor struct {
	panicValue any
}

func (e poolBoundaryPanicExecutor) Execute(context.Context, work.WorkDispatch) (WorkResult, error) {
	panic(e.panicValue)
}

type poolBoundaryErrorExecutor struct {
	err error
}

func (e poolBoundaryErrorExecutor) Execute(context.Context, work.WorkDispatch) (WorkResult, error) {
	return WorkResult{Outcome: OutcomeFailed, Error: e.err.Error()}, e.err
}

func poolBoundaryDispatchRequest(dispatchID, transitionID, workerType string) WorkstationExecutionRequest {
	return WorkstationExecutionRequest{
		WorkerType: workerType,
		Dispatch: work.WorkDispatch{
			DispatchID:   dispatchID,
			TransitionID: transitionID,
			WorkerType:   workerType,
		},
	}
}

func TestWorkerExecutorRequestAdapterExecuteRecoversErrorPanic(t *testing.T) {
	cause := errors.New("boom")
	adapter := workerExecutorRequestAdapter{
		executors: map[string]WorkerExecutor{"swe": poolBoundaryPanicExecutor{panicValue: cause}},
	}
	request := poolBoundaryDispatchRequest("dispatch-1", "transition-1", "swe")

	result, err := adapter.Execute(context.Background(), request)

	if err == nil {
		t.Fatalf("Execute() err = nil, want non-nil typed panic error")
	}
	var panicErr *WorkerExecutorPanicError
	if !errors.As(err, &panicErr) || panicErr == nil {
		t.Fatalf("errors.As(err, *WorkerExecutorPanicError) = false, want true; err = %v", err)
	}
	if panicErr.Cause != any(cause) {
		t.Fatalf("panicErr.Cause = %v, want %v", panicErr.Cause, cause)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true via Unwrap")
	}
	wantText := "executor panic: boom"
	if err.Error() != wantText {
		t.Fatalf("err.Error() = %q, want %q", err.Error(), wantText)
	}
	if result.Outcome != OutcomeFailed {
		t.Fatalf("result.Outcome = %q, want %q", result.Outcome, OutcomeFailed)
	}
	if result.Error != wantText {
		t.Fatalf("result.Error = %q, want %q", result.Error, wantText)
	}
	if result.DispatchID != "dispatch-1" || result.TransitionID != "transition-1" {
		t.Fatalf(
			"result identity = (%q, %q), want (%q, %q)",
			result.DispatchID, result.TransitionID, "dispatch-1", "transition-1",
		)
	}
}

func TestWorkerExecutorRequestAdapterExecuteRecoversNonErrorPanic(t *testing.T) {
	adapter := workerExecutorRequestAdapter{
		executors: map[string]WorkerExecutor{"swe": poolBoundaryPanicExecutor{panicValue: "catastrophic failure"}},
	}
	request := poolBoundaryDispatchRequest("dispatch-2", "transition-2", "swe")

	result, err := adapter.Execute(context.Background(), request)

	var panicErr *WorkerExecutorPanicError
	if !errors.As(err, &panicErr) || panicErr == nil {
		t.Fatalf("errors.As(err, *WorkerExecutorPanicError) = false, want true; err = %v", err)
	}
	if panicErr.Cause != any("catastrophic failure") {
		t.Fatalf("panicErr.Cause = %v, want %q", panicErr.Cause, "catastrophic failure")
	}
	wantText := "executor panic: catastrophic failure"
	if err.Error() != wantText {
		t.Fatalf("err.Error() = %q, want %q", err.Error(), wantText)
	}
	if result.Error != wantText || result.Outcome != OutcomeFailed {
		t.Fatalf("result = %#v, want Error=%q Outcome=%q", result, wantText, OutcomeFailed)
	}
}

func TestWorkerExecutorRequestAdapterExecuteSuccessUnaffected(t *testing.T) {
	want := WorkResult{Outcome: OutcomeAccepted}
	adapter := workerExecutorRequestAdapter{
		executors: map[string]WorkerExecutor{"swe": poolBoundaryErrorExecutorSuccess{result: want}},
	}
	request := poolBoundaryDispatchRequest("dispatch-3", "transition-3", "swe")

	result, err := adapter.Execute(context.Background(), request)

	if err != nil {
		t.Fatalf("Execute() err = %v, want nil", err)
	}
	if result.Outcome != want.Outcome {
		t.Fatalf("Execute() result = %#v, want %#v", result, want)
	}
}

type poolBoundaryResolvedExecutor struct {
	request WorkstationExecutionRequest
}

func (e *poolBoundaryResolvedExecutor) Execute(context.Context, work.WorkDispatch) (WorkResult, error) {
	return WorkResult{Outcome: OutcomeFailed, Error: "legacy request path used"}, nil
}

func (e *poolBoundaryResolvedExecutor) ExecuteResolved(_ context.Context, request WorkstationExecutionRequest) (WorkResult, error) {
	e.request = request
	return WorkResult{Outcome: OutcomeAccepted}, nil
}

func TestWorkerExecutorRequestAdapterPreservesResolvedContinuation(t *testing.T) {
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-1",
	}
	executor := &poolBoundaryResolvedExecutor{}
	adapter := workerExecutorRequestAdapter{executors: map[string]WorkerExecutor{"swe": executor}}
	request := poolBoundaryDispatchRequest("dispatch-resume", "transition-resume", "swe")
	continuation := reference.ContinuationRef()
	request.Continuation = &continuation

	result, err := adapter.Execute(context.Background(), request)

	if err != nil {
		t.Fatalf("Execute() err = %v, want nil", err)
	}
	if result.Outcome != OutcomeAccepted {
		t.Fatalf("Execute() result = %#v, want accepted resolved path", result)
	}
	if executor.request.Continuation == nil {
		t.Fatalf("resolved request Continuation = nil, want %#v", reference)
	}
	resolved, resolveErr := executor.request.Continuation.ToSessionRef()
	if resolveErr != nil || resolved != reference {
		t.Fatalf("resolved request Continuation = %#v (%v), want %#v", executor.request.Continuation, resolveErr, reference)
	}
}

func TestWorkerExecutorRequestAdapterExecuteOrdinaryErrorUnaffected(t *testing.T) {
	wantErr := errors.New("ordinary executor failure")
	adapter := workerExecutorRequestAdapter{
		executors: map[string]WorkerExecutor{"swe": poolBoundaryErrorExecutor{err: wantErr}},
	}
	request := poolBoundaryDispatchRequest("dispatch-4", "transition-4", "swe")

	result, err := adapter.Execute(context.Background(), request)

	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() err = %v, want %v", err, wantErr)
	}
	var panicErr *WorkerExecutorPanicError
	if errors.As(err, &panicErr) {
		t.Fatalf("errors.As(err, *WorkerExecutorPanicError) = true, want false for an ordinary error")
	}
	if result.Outcome != OutcomeFailed || result.Error != wantErr.Error() {
		t.Fatalf("result = %#v, want Outcome=%q Error=%q", result, OutcomeFailed, wantErr.Error())
	}
}

type poolBoundaryErrorExecutorSuccess struct {
	result WorkResult
}

func (e poolBoundaryErrorExecutorSuccess) Execute(context.Context, work.WorkDispatch) (WorkResult, error) {
	return e.result, nil
}

func TestWorkstationPoolBoundaryBindingsPreserveLegacyConcurrency(t *testing.T) {
	boundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Executors:  map[string]WorkerExecutor{"swe": poolBoundaryTestExecutor{}},
		RouteNames: []string{"swe"},
		Async:      true,
	})
	pool, ok := boundary.(*workstationPoolBoundary)
	if !ok {
		t.Fatalf("boundary type = %T, want *workstationPoolBoundary", boundary)
	}
	if len(pool.bindings) != 1 {
		t.Fatalf("bindings = %d, want 1", len(pool.bindings))
	}
	binding := pool.bindings[0]
	if binding.Capacity != DefaultRuntimePoolBindingCapacity ||
		binding.QueueCapacity != DefaultRuntimePoolBindingCapacity {
		t.Fatalf(
			"binding capacity = (%d, %d), want (%d, %d)",
			binding.Capacity,
			binding.QueueCapacity,
			DefaultRuntimePoolBindingCapacity,
			DefaultRuntimePoolBindingCapacity,
		)
	}
}

func TestWorkstationPoolBoundaryUsesOnePrivatePoolPerRuntime(t *testing.T) {
	shared := &poolBoundaryFakeService{}
	var created []*poolBoundaryFakeService
	newBoundary := func() WorkstationPoolBoundary {
		return NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
			Service: shared,
			ServiceFactory: func() WorkstationExecutionService {
				pool := &poolBoundaryFakeService{}
				created = append(created, pool)
				return pool
			},
			Executors:  map[string]WorkerExecutor{"swe": poolBoundaryTestExecutor{}},
			RouteNames: []string{"swe"},
		})
	}

	first := newBoundary()
	second := newBoundary()
	if err := first.Start(context.Background()); err != nil {
		t.Fatalf("first.Start() error = %v", err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatalf("second.Start() error = %v", err)
	}
	if len(created) != 2 || created[0] == created[1] {
		t.Fatalf("runtime pool instances = %#v, want two distinct instances", created)
	}
	if shared.routes != nil {
		t.Fatalf("shared process pool was mutated during runtime start: %#v", shared.routes)
	}
	if len(created[0].routes) != 1 || len(created[1].routes) != 1 {
		t.Fatalf("private route snapshots = (%d, %d), want one route each", len(created[0].routes), len(created[1].routes))
	}
}

type poolBoundaryRequestExecutor struct{}

func (poolBoundaryRequestExecutor) Execute(
	context.Context,
	WorkstationExecutionRequest,
) (WorkResult, error) {
	return WorkResult{Outcome: OutcomeAccepted}, nil
}

// TestWorkstationPoolBoundaryBindsProviderInvocationAsWorkstationRole proves the
// provider-invocation route joins the pool as an ordinary workstation-kind
// binding.
//
// The pool rejects a start request carrying any other role kind, so a
// worker-kind binding here made every route in the snapshot unusable, not just
// this one: the first dispatch of the session failed to start. The kind is
// about how the pool routes, and every pool route is a workstation route. What
// is absent from a provider-invocation Worker is an authored workstation
// definition, and that absence lives in the executor.
func TestWorkstationPoolBoundaryBindsProviderInvocationAsWorkstationRole(t *testing.T) {
	boundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Executors:          map[string]WorkerExecutor{"swe": poolBoundaryTestExecutor{}},
		RouteNames:         []string{"swe"},
		ProviderInvocation: poolBoundaryRequestExecutor{},
	})
	pool, ok := boundary.(*workstationPoolBoundary)
	if !ok {
		t.Fatalf("boundary type = %T, want *workstationPoolBoundary", boundary)
	}

	var found *AssembledRuntimeBinding
	for i := range pool.bindings {
		if pool.bindings[i].RoleName == ProviderInvocationRoute {
			found = &pool.bindings[i]
		}
	}
	if found == nil {
		t.Fatalf("bindings = %+v, want one named %q", pool.bindings, ProviderInvocationRoute)
	}
	if found.RoleKind != RuntimeBuildRoleKindWorkstation {
		t.Fatalf("RoleKind = %q, want %q", found.RoleKind, RuntimeBuildRoleKindWorkstation)
	}
	if found.Executor == nil {
		t.Fatal("Executor = nil, want the supplied provider-invocation executor")
	}
	if found.RunnerSelection.RunnerID != "" {
		t.Fatalf(
			"RunnerSelection.RunnerID = %q, want empty so each dispatch keeps its own runner",
			found.RunnerSelection.RunnerID,
		)
	}
}

// TestWorkstationPoolBoundaryOmitsProviderInvocationRouteWhenAbsent proves a
// session with no provider-invocation executor has no such route at all, rather
// than a route present and failing at dispatch time.
func TestWorkstationPoolBoundaryOmitsProviderInvocationRouteWhenAbsent(t *testing.T) {
	boundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Executors:  map[string]WorkerExecutor{"swe": poolBoundaryTestExecutor{}},
		RouteNames: []string{"swe"},
	})
	pool, ok := boundary.(*workstationPoolBoundary)
	if !ok {
		t.Fatalf("boundary type = %T, want *workstationPoolBoundary", boundary)
	}
	for _, binding := range pool.bindings {
		if binding.RoleName == ProviderInvocationRoute {
			t.Fatalf("bindings contain %q, want it omitted entirely", ProviderInvocationRoute)
		}
	}
}
