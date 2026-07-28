package http

import (
	"context"
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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

type runtimeRootFake struct {
	observe func(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error)
}

var _ factoryruntime.Service = (*runtimeRootFake)(nil)

func (fake *runtimeRootFake) ControlPause(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	return factoryruntime.PauseResult{}, nil
}
func (fake *runtimeRootFake) ControlResume(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	return factoryruntime.ResumeResult{}, nil
}
func (fake *runtimeRootFake) ControlTerminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{}, nil
}
func (*runtimeRootFake) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}
func (fake *runtimeRootFake) ControlMoveWork(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, nil
}
func (fake *runtimeRootFake) Observe(ctx context.Context, req factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	if fake.observe != nil {
		return fake.observe(ctx, req)
	}
	return factoryruntime.ObserveResult{}, nil
}
func (fake *runtimeRootFake) PlanDispatch(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{}, nil
}
func (fake *runtimeRootFake) AcceptDispatchResult(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{}, nil
}
func (fake *runtimeRootFake) CaptureCheckpoint(context.Context, factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	return factoryruntime.CaptureCheckpointResult{}, nil
}
func (fake *runtimeRootFake) LoadCheckpoint(context.Context, factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	return factoryruntime.LoadCheckpointResult{}, nil
}
func (fake *runtimeRootFake) RestoreCheckpoint(context.Context, factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{}, nil
}
