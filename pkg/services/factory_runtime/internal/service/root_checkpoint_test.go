package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jonboulle/clockwork"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type captureDelegate struct {
	called bool
}

func (d *captureDelegate) ControlPause(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
}
func (d *captureDelegate) ControlResume(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	return factoryruntime.ResumeResult{}, factoryruntime.ErrNotRunning
}
func (d *captureDelegate) ControlTerminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{}, factoryruntime.ErrNotRunning
}
func (*captureDelegate) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}
func (d *captureDelegate) ControlMoveWork(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, factoryruntime.ErrNotRunning
}
func (d *captureDelegate) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, factoryruntime.ErrNotRunning
}
func (d *captureDelegate) PlanDispatch(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{}, factoryruntime.ErrNotRunning
}
func (d *captureDelegate) AcceptDispatchResult(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{}, factoryruntime.ErrNotRunning
}
func (d *captureDelegate) CaptureCheckpoint(_ context.Context, req factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	d.called = true
	return factoryruntime.CaptureCheckpointResult{
		Outcome: factoryruntime.CheckpointOutcomeCaptured,
		Checkpoint: factoryruntime.Checkpoint{
			CheckpointID: req.CheckpointID, SchemaVersion: 1, Payload: []byte(`{"opaque":true}`),
		},
	}, nil
}
func (d *captureDelegate) LoadCheckpoint(context.Context, factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
}
func (d *captureDelegate) RestoreCheckpoint(context.Context, factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
}

func TestRootCaptureCheckpointDelegatesToActiveRuntime(t *testing.T) {
	t.Parallel()

	instanceHost, err := instancehostwire.New(instancehost.Dependencies{Clock: clockwork.NewFakeClock()})
	if err != nil {
		t.Fatalf("instance host wire: %v", err)
	}
	delegate := &captureDelegate{}
	root := &Root{
		orchestration: orchestrationwire.New(func() string { return "id" }, nil, nil),
		instanceHost:  instanceHost,
		dispatchPlan:  dispatchplanningwire.New(nil, nil),
		active:        delegate,
	}

	result, err := root.CaptureCheckpoint(context.Background(), factoryruntime.CaptureCheckpointRequest{
		CheckpointID: "checkpoint-1",
	})
	if err != nil {
		t.Fatalf("CaptureCheckpoint() error = %v", err)
	}
	if !delegate.called {
		t.Fatal("CaptureCheckpoint() did not delegate to active runtime")
	}
	if result.Outcome != factoryruntime.CheckpointOutcomeCaptured ||
		result.Checkpoint.CheckpointID != "checkpoint-1" ||
		result.Checkpoint.SchemaVersion != 1 ||
		len(result.Checkpoint.Payload) == 0 {
		t.Fatalf("CaptureCheckpoint() = %#v, want delegated CAPTURED checkpoint", result)
	}
}

func TestRootCaptureCheckpointRejectsMissingIdentity(t *testing.T) {
	t.Parallel()

	root, err := NewRoot(
		func() string { return "id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	_, err = root.CaptureCheckpoint(context.Background(), factoryruntime.CaptureCheckpointRequest{})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("CaptureCheckpoint() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRootCaptureCheckpointWithoutActiveRuntimeReportsUnavailable(t *testing.T) {
	t.Parallel()

	root, err := NewRoot(
		func() string { return "id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	_, err = root.CaptureCheckpoint(context.Background(), factoryruntime.CaptureCheckpointRequest{
		CheckpointID: "checkpoint-1",
	})
	if !errors.Is(err, factoryruntime.ErrCapabilityUnavailable) {
		t.Fatalf("CaptureCheckpoint() error = %v, want ErrCapabilityUnavailable", err)
	}
}
