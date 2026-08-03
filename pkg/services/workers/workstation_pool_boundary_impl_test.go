package workers

import (
	"context"
	"errors"
	"testing"

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

	panicErr, ok := AsWorkerExecutorPanicError(err)
	if !ok || panicErr == nil {
		t.Fatalf("AsWorkerExecutorPanicError(err) = (_, false), want true; err = %v", err)
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
