package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimehttpcheckpoint "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/checkpoint"
	runtimehttpcontrol "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/control"
	runtimehttpdispatch "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/dispatch"
	runtimehttpcommon "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	runtimehttpobservation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/observation"
)

func TestAdapter_BindsRuntimeRootViaFakeRootSeam(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &runtimeRootFake{
		observe: func(_ context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
			invoked = true
			return factoryruntime.ObserveResult{}, factoryruntime.ErrNotRunning
		},
	}

	adapter := NewAdapter(fake)
	if adapter.Root() != fake {
		t.Fatal("adapter must expose the injected Runtime root")
	}

	root, err := adapter.runtimeRoot()
	if err != nil {
		t.Fatalf("runtimeRoot() error = %v", err)
	}
	if root != fake {
		t.Fatal("runtimeRoot() must return the injected Runtime root")
	}

	_, err = root.Observe(context.Background(), factoryruntime.ObserveRequest{})
	if !invoked {
		t.Fatal("adapter-owned operation did not reach the injected Runtime root")
	}
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe error = %v, want ErrNotRunning", err)
	}
}

func TestNewAdapter_RejectsNilRoot(t *testing.T) {
	t.Parallel()

	if NewAdapter(nil) != nil {
		t.Fatal("NewAdapter(nil) must return nil")
	}
}

func TestNewAdapter_RejectsTypedNilRoot(t *testing.T) {
	t.Parallel()

	var typedNil *typedNilRuntimeRoot
	if NewAdapter(typedNil) != nil {
		t.Fatal("NewAdapter(typed nil root) must return nil")
	}

	adapter := &Adapter{root: typedNil}
	if adapter.Root() != nil {
		t.Fatal("Root() must return nil for a typed nil root")
	}
}

func TestOperationHandlers_RejectTypedNilRoot(t *testing.T) {
	t.Parallel()

	var typedNil *typedNilRuntimeRoot
	if root, err := runtimehttpcommon.RequireRuntimeRoot(typedNil); root != nil || err == nil {
		t.Fatalf("RequireRuntimeRoot(typed nil root) = (%v, %v), want (nil, error)", root, err)
	}
	for _, test := range []struct {
		name  string
		isNil func() bool
	}{
		{name: "observation", isNil: func() bool { return runtimehttpobservation.NewHandler(typedNil) == nil }},
		{name: "control", isNil: func() bool { return runtimehttpcontrol.NewHandler(typedNil) == nil }},
		{name: "dispatch", isNil: func() bool { return runtimehttpdispatch.NewHandler(typedNil) == nil }},
		{name: "checkpoint", isNil: func() bool { return runtimehttpcheckpoint.NewHandler(typedNil) == nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !test.isNil() {
				t.Fatal("NewHandler(typed nil root) must return nil")
			}
		})
	}
}

func TestOperationHandlers_TypedNilRootFailClosed(t *testing.T) {
	t.Parallel()

	var typedNil *runtimeRootFake
	handler := runtimehttpcontrol.NewHandler(typedNil)
	recorder := httptest.NewRecorder()
	handler.ControlPause(recorder, httptest.NewRequest(http.MethodPost, "/control/pause", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("ControlPause typed nil root status = %d, want 500", recorder.Code)
	}
}

type typedNilRuntimeRoot struct {
	factoryruntime.Service
}

type runtimeRootFake struct {
	observe              func(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error)
	pause                func(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error)
	resume               func(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error)
	terminate            func(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error)
	moveWork             func(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error)
	planDispatch         func(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error)
	acceptDispatchResult func(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error)
	captureCheckpoint    func(context.Context, factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error)
	loadCheckpoint       func(context.Context, factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error)
	restoreCheckpoint    func(context.Context, factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error)
}

var _ factoryruntime.Service = (*runtimeRootFake)(nil)

func (fake *runtimeRootFake) ControlPause(ctx context.Context, req factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	if fake.pause != nil {
		return fake.pause(ctx, req)
	}
	return factoryruntime.PauseResult{}, nil
}
func (fake *runtimeRootFake) ControlResume(ctx context.Context, req factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	if fake.resume != nil {
		return fake.resume(ctx, req)
	}
	return factoryruntime.ResumeResult{}, nil
}
func (fake *runtimeRootFake) ControlTerminate(ctx context.Context, req factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	if fake.terminate != nil {
		return fake.terminate(ctx, req)
	}
	return factoryruntime.TerminateResult{}, nil
}
func (*runtimeRootFake) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}
func (fake *runtimeRootFake) ControlMoveWork(ctx context.Context, req factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	if fake.moveWork != nil {
		return fake.moveWork(ctx, req)
	}
	return factoryruntime.MoveWorkResult{}, nil
}
func (fake *runtimeRootFake) Observe(ctx context.Context, req factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	if fake.observe != nil {
		return fake.observe(ctx, req)
	}
	return factoryruntime.ObserveResult{}, nil
}
func (fake *runtimeRootFake) PlanDispatch(ctx context.Context, req factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	if fake.planDispatch != nil {
		return fake.planDispatch(ctx, req)
	}
	return factoryruntime.PlanDispatchResult{}, nil
}
func (fake *runtimeRootFake) AcceptDispatchResult(ctx context.Context, req factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	if fake.acceptDispatchResult != nil {
		return fake.acceptDispatchResult(ctx, req)
	}
	return factoryruntime.AcceptDispatchResultResult{}, nil
}
func (fake *runtimeRootFake) CaptureCheckpoint(ctx context.Context, req factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	if fake.captureCheckpoint != nil {
		return fake.captureCheckpoint(ctx, req)
	}
	return factoryruntime.CaptureCheckpointResult{}, nil
}
func (fake *runtimeRootFake) LoadCheckpoint(ctx context.Context, req factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	if fake.loadCheckpoint != nil {
		return fake.loadCheckpoint(ctx, req)
	}
	return factoryruntime.LoadCheckpointResult{}, nil
}
func (fake *runtimeRootFake) RestoreCheckpoint(ctx context.Context, req factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	if fake.restoreCheckpoint != nil {
		return fake.restoreCheckpoint(ctx, req)
	}
	return factoryruntime.RestoreCheckpointResult{}, nil
}
