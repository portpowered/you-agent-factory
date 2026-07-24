package factory_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// peerCheckpointService extends the singular root Service fake with checkpoint
// slice outcomes. It depends only on published root types plus approved peer
// contracts and never imports factory_runtime/internal.
type peerCheckpointService struct {
	peerRootService

	captureErr    error
	captureResult factoryruntime.CaptureCheckpointResult
	loadErr       error
	loadResult    factoryruntime.LoadCheckpointResult
	restoreErr    error
	restoreOut    factoryruntime.RestoreCheckpointResult
}

var _ factoryruntime.Service = (*peerCheckpointService)(nil)

func (s *peerCheckpointService) CaptureCheckpoint(
	_ context.Context,
	req factoryruntime.CaptureCheckpointRequest,
) (factoryruntime.CaptureCheckpointResult, error) {
	if s.captureErr != nil {
		return factoryruntime.CaptureCheckpointResult{}, s.captureErr
	}
	if s.captureResult.Checkpoint.CheckpointID != "" || s.captureResult.Outcome != "" {
		return s.captureResult, nil
	}
	id := req.CheckpointID
	if id == "" {
		id = "checkpoint-1"
	}
	return factoryruntime.CaptureCheckpointResult{
		Outcome: factoryruntime.CheckpointOutcomeCaptured,
		Checkpoint: factoryruntime.Checkpoint{
			CheckpointID:  id,
			SchemaVersion: 1,
			StrategyKind:  "runtime",
			Payload:       []byte(`{"opaque":true}`),
		},
	}, nil
}

func (s *peerCheckpointService) LoadCheckpoint(
	_ context.Context,
	req factoryruntime.LoadCheckpointRequest,
) (factoryruntime.LoadCheckpointResult, error) {
	if s.loadErr != nil {
		return factoryruntime.LoadCheckpointResult{}, s.loadErr
	}
	if s.loadResult.Checkpoint.CheckpointID != "" || s.loadResult.Outcome != "" {
		return s.loadResult, nil
	}
	checkpoint := factoryruntime.Checkpoint{
		CheckpointID:  req.CheckpointID,
		SchemaVersion: 1,
		StrategyKind:  "runtime",
		Payload:       []byte(`{"opaque":true}`),
	}
	compatible := true
	if req.ExpectedSchemaVersion != 0 {
		compatible = checkpoint.SchemaVersion == req.ExpectedSchemaVersion
	}
	return factoryruntime.LoadCheckpointResult{
		Outcome:    factoryruntime.CheckpointOutcomeLoaded,
		Checkpoint: checkpoint,
		Compatible: compatible,
	}, nil
}

func (s *peerCheckpointService) RestoreCheckpoint(
	_ context.Context,
	req factoryruntime.RestoreCheckpointRequest,
) (factoryruntime.RestoreCheckpointResult, error) {
	if s.restoreErr != nil {
		return factoryruntime.RestoreCheckpointResult{}, s.restoreErr
	}
	if s.restoreOut.Outcome != "" {
		return s.restoreOut, nil
	}
	return factoryruntime.RestoreCheckpointResult{
		Outcome:      factoryruntime.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func TestRootCheckpoint_FakePeerCaptureAndRestoreSuccessShapes(t *testing.T) {
	t.Parallel()

	var runtime factoryruntime.Service = &peerCheckpointService{}

	captured, err := factoryruntime.ApplyCaptureCheckpoint(context.Background(), runtime, factoryruntime.CaptureCheckpointRequest{
		CheckpointID: "checkpoint-1",
	})
	if err != nil {
		t.Fatalf("ApplyCaptureCheckpoint error = %v, want nil", err)
	}
	if captured.Outcome != factoryruntime.CheckpointOutcomeCaptured {
		t.Fatalf("CaptureCheckpointResult.Outcome = %q, want CAPTURED", captured.Outcome)
	}
	if captured.Checkpoint.CheckpointID != "checkpoint-1" || captured.Checkpoint.SchemaVersion != 1 {
		t.Fatalf("CaptureCheckpointResult.Checkpoint = %#v, want versioned root checkpoint", captured.Checkpoint)
	}
	if len(captured.Checkpoint.Payload) == 0 {
		t.Fatal("CaptureCheckpointResult.Checkpoint.Payload is empty, want opaque strategy bytes")
	}

	restored, err := factoryruntime.ApplyRestoreCheckpoint(context.Background(), runtime, factoryruntime.RestoreCheckpointRequest{
		Checkpoint: captured.Checkpoint,
	})
	if err != nil {
		t.Fatalf("ApplyRestoreCheckpoint error = %v, want nil", err)
	}
	if restored != (factoryruntime.RestoreCheckpointResult{
		Outcome:      factoryruntime.CheckpointOutcomeRestored,
		CheckpointID: "checkpoint-1",
	}) {
		t.Fatalf("RestoreCheckpointResult = %#v, want plain root success shape", restored)
	}
}

func TestRootCheckpoint_FakePeerLoadInspectCompatibility(t *testing.T) {
	t.Parallel()

	var runtime factoryruntime.Service = &peerCheckpointService{}

	loaded, err := factoryruntime.ApplyLoadCheckpoint(context.Background(), runtime, factoryruntime.LoadCheckpointRequest{
		CheckpointID:          "checkpoint-1",
		ExpectedSchemaVersion: 1,
	})
	if err != nil {
		t.Fatalf("ApplyLoadCheckpoint error = %v, want nil", err)
	}
	if loaded.Outcome != factoryruntime.CheckpointOutcomeLoaded || !loaded.Compatible {
		t.Fatalf("LoadCheckpointResult = %#v, want LOADED compatible inspect shape", loaded)
	}
	if !bytes.Equal(loaded.Checkpoint.Payload, []byte(`{"opaque":true}`)) {
		t.Fatalf("LoadCheckpointResult.Checkpoint.Payload = %q, want opaque strategy bytes", loaded.Checkpoint.Payload)
	}
}

func TestRootCheckpoint_FakePeerTypedFailures(t *testing.T) {
	t.Parallel()

	t.Run("missing checkpoint", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerCheckpointService{
			loadErr: factoryruntime.ErrCheckpointNotFound,
		}
		_, err := factoryruntime.ApplyLoadCheckpoint(context.Background(), runtime, factoryruntime.LoadCheckpointRequest{
			CheckpointID: "missing",
		})
		if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
			t.Fatalf("ApplyLoadCheckpoint error = %v, want ErrCheckpointNotFound", err)
		}
	})

	t.Run("corrupt checkpoint", func(t *testing.T) {
		t.Parallel()
		_, err := factoryruntime.ApplyRestoreCheckpoint(context.Background(), &peerCheckpointService{}, factoryruntime.RestoreCheckpointRequest{
			Checkpoint: factoryruntime.Checkpoint{
				CheckpointID:  "checkpoint-1",
				SchemaVersion: 1,
				Payload:       nil,
			},
		})
		if !errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
			t.Fatalf("ApplyRestoreCheckpoint error = %v, want ErrCorruptCheckpoint", err)
		}
	})

	t.Run("incompatible checkpoint", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerCheckpointService{
			restoreErr: factoryruntime.ErrIncompatibleCheckpoint,
		}
		_, err := factoryruntime.ApplyRestoreCheckpoint(context.Background(), runtime, factoryruntime.RestoreCheckpointRequest{
			Checkpoint: factoryruntime.Checkpoint{
				CheckpointID:  "checkpoint-1",
				SchemaVersion: 99,
				Payload:       []byte(`{"opaque":true}`),
			},
		})
		if !errors.Is(err, factoryruntime.ErrIncompatibleCheckpoint) {
			t.Fatalf("ApplyRestoreCheckpoint error = %v, want ErrIncompatibleCheckpoint", err)
		}
	})

	t.Run("nil runtime not found", func(t *testing.T) {
		t.Parallel()
		_, err := factoryruntime.ApplyCaptureCheckpoint(context.Background(), nil, factoryruntime.CaptureCheckpointRequest{
			CheckpointID: "checkpoint-1",
		})
		if !errors.Is(err, factoryruntime.ErrNotFound) {
			t.Fatalf("ApplyCaptureCheckpoint(nil) error = %v, want ErrNotFound", err)
		}
	})
}
