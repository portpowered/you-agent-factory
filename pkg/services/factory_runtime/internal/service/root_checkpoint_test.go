package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jonboulle/clockwork"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type checkpointDelegate struct {
	captureCalled bool
	loadCalled    bool
	restoreCalled bool
}

func (d *checkpointDelegate) ControlPause(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
}
func (d *checkpointDelegate) ControlResume(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	return factoryruntime.ResumeResult{}, factoryruntime.ErrNotRunning
}
func (d *checkpointDelegate) ControlTerminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{}, factoryruntime.ErrNotRunning
}
func (*checkpointDelegate) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}
func (d *checkpointDelegate) ControlMoveWork(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, factoryruntime.ErrNotRunning
}
func (d *checkpointDelegate) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, factoryruntime.ErrNotRunning
}
func (d *checkpointDelegate) PlanDispatch(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{}, factoryruntime.ErrNotRunning
}
func (d *checkpointDelegate) AcceptDispatchResult(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{}, factoryruntime.ErrNotRunning
}
func (d *checkpointDelegate) CaptureCheckpoint(_ context.Context, req factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	d.captureCalled = true
	return factoryruntime.CaptureCheckpointResult{
		Outcome: factoryruntime.CheckpointOutcomeCaptured,
		Checkpoint: factoryruntime.Checkpoint{
			CheckpointID: req.CheckpointID, SchemaVersion: 1, Payload: []byte(`{"opaque":true}`),
		},
	}, nil
}
func (d *checkpointDelegate) LoadCheckpoint(_ context.Context, req factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	d.loadCalled = true
	return factoryruntime.LoadCheckpointResult{
		Outcome: factoryruntime.CheckpointOutcomeLoaded,
		Checkpoint: factoryruntime.Checkpoint{
			CheckpointID: req.CheckpointID, SchemaVersion: 1, Payload: []byte(`{"opaque":true}`),
		},
		Compatible: req.ExpectedSchemaVersion == 0 || req.ExpectedSchemaVersion == 1,
	}, nil
}
func (d *checkpointDelegate) RestoreCheckpoint(_ context.Context, req factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	d.restoreCalled = true
	return factoryruntime.RestoreCheckpointResult{
		Outcome:      factoryruntime.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func TestRootCaptureCheckpointDelegatesToActiveRuntime(t *testing.T) {
	t.Parallel()

	delegate := &checkpointDelegate{}
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
	root.BindActiveService(delegate)

	result, err := root.CaptureCheckpoint(context.Background(), factoryruntime.CaptureCheckpointRequest{
		CheckpointID: "checkpoint-1",
	})
	if err != nil {
		t.Fatalf("CaptureCheckpoint() error = %v", err)
	}
	if !delegate.captureCalled {
		t.Fatal("CaptureCheckpoint() did not delegate to active runtime")
	}
	if result.Outcome != factoryruntime.CheckpointOutcomeCaptured ||
		result.Checkpoint.CheckpointID != "checkpoint-1" ||
		result.Checkpoint.SchemaVersion != 1 ||
		len(result.Checkpoint.Payload) == 0 {
		t.Fatalf("CaptureCheckpoint() = %#v, want delegated CAPTURED checkpoint", result)
	}
}

func TestRootLoadCheckpointDelegatesToActiveRuntime(t *testing.T) {
	t.Parallel()

	delegate := &checkpointDelegate{}
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
	root.BindActiveService(delegate)

	result, err := root.LoadCheckpoint(context.Background(), factoryruntime.LoadCheckpointRequest{
		CheckpointID:          "checkpoint-1",
		ExpectedSchemaVersion: 1,
	})
	if err != nil {
		t.Fatalf("LoadCheckpoint() error = %v", err)
	}
	if !delegate.loadCalled {
		t.Fatal("LoadCheckpoint() did not delegate to active runtime")
	}
	if result.Outcome != factoryruntime.CheckpointOutcomeLoaded ||
		result.Checkpoint.CheckpointID != "checkpoint-1" ||
		!result.Compatible {
		t.Fatalf("LoadCheckpoint() = %#v, want delegated LOADED compatible checkpoint", result)
	}
}

func TestRootLoadCheckpointRejectsMissingIdentity(t *testing.T) {
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
	_, err = root.LoadCheckpoint(context.Background(), factoryruntime.LoadCheckpointRequest{})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("LoadCheckpoint() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRootLoadCheckpointWithoutActiveRuntimeReportsUnavailable(t *testing.T) {
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
	_, err = root.LoadCheckpoint(context.Background(), factoryruntime.LoadCheckpointRequest{
		CheckpointID: "checkpoint-1",
	})
	if !errors.Is(err, factoryruntime.ErrCapabilityUnavailable) {
		t.Fatalf("LoadCheckpoint() error = %v, want ErrCapabilityUnavailable", err)
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

func TestRootRestoreCheckpointDelegatesToActiveRuntime(t *testing.T) {
	t.Parallel()

	delegate := &checkpointDelegate{}
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
	root.BindActiveService(delegate)

	result, err := root.RestoreCheckpoint(context.Background(), factoryruntime.RestoreCheckpointRequest{
		Checkpoint: factoryruntime.Checkpoint{
			CheckpointID: "checkpoint-1", SchemaVersion: 1, Payload: []byte(`{"opaque":true}`),
		},
	})
	if err != nil {
		t.Fatalf("RestoreCheckpoint() error = %v", err)
	}
	if !delegate.restoreCalled {
		t.Fatal("RestoreCheckpoint() did not delegate to active runtime")
	}
	if result.Outcome != factoryruntime.CheckpointOutcomeRestored ||
		result.CheckpointID != "checkpoint-1" {
		t.Fatalf("RestoreCheckpoint() = %#v, want delegated RESTORED checkpoint", result)
	}
}

func TestRootRestoreCheckpointRejectsMissingIdentity(t *testing.T) {
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
	_, err = root.RestoreCheckpoint(context.Background(), factoryruntime.RestoreCheckpointRequest{})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("RestoreCheckpoint() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRootRestoreCheckpointWithoutActiveRuntimeReportsUnavailable(t *testing.T) {
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
	_, err = root.RestoreCheckpoint(context.Background(), factoryruntime.RestoreCheckpointRequest{
		Checkpoint: factoryruntime.Checkpoint{CheckpointID: "checkpoint-1", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	if !errors.Is(err, factoryruntime.ErrCapabilityUnavailable) {
		t.Fatalf("RestoreCheckpoint() error = %v, want ErrCapabilityUnavailable", err)
	}
}
