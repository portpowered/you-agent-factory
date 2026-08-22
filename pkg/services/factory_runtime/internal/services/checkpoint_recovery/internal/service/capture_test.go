package service_test

import (
	"errors"
	"testing"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	checkpointrecoveryservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/service"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
)

func TestRecoveryCaptureStoresOpaqueEnvelope(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	payload := []byte(`{"factoryState":"PAUSED"}`)

	captured, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		CheckpointID: "checkpoint-1",
		Payload:      payload,
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if captured.Envelope.CheckpointID != "checkpoint-1" ||
		captured.Envelope.SchemaVersion != checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion ||
		captured.Envelope.StrategyKind != checkpointrecovery.RuntimeOpaqueCheckpointStrategyKind ||
		string(captured.Envelope.Payload) != string(payload) {
		t.Fatalf("Capture() envelope = %#v, want stored opaque envelope", captured.Envelope)
	}
}

func TestRecoveryCaptureRejectsMissingCheckpointIdentity(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		Payload: []byte(`{"factoryState":"RUNNING"}`),
	})
	if !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
		t.Fatalf("Capture() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRecoveryCaptureRejectsCorruptPayload(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoveryservice.New(checkpointrecoverywire.NewProcessLocalCheckpointStore())
	_, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		CheckpointID: "checkpoint-1",
	})
	if !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
		t.Fatalf("Capture() error = %v, want ErrCorruptCheckpoint", err)
	}
}

func TestRecoveryCaptureFailureDoesNotMutatePreviouslyValidStoredEnvelope(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	validPayload := []byte(`{"factoryState":"PAUSED"}`)
	captured, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		CheckpointID: "checkpoint-1",
		Payload:      validPayload,
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	_, err = recovery.Capture(checkpointrecovery.CaptureRequest{
		CheckpointID: "checkpoint-1",
	})
	if !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
		t.Fatalf("Capture(corrupt) error = %v, want ErrCorruptCheckpoint", err)
	}

	loaded, err := recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "checkpoint-1"})
	if err != nil {
		t.Fatalf("Load() after failed Capture error = %v", err)
	}
	if loaded.Envelope.SchemaVersion != captured.Envelope.SchemaVersion ||
		loaded.Envelope.StrategyKind != captured.Envelope.StrategyKind ||
		string(loaded.Envelope.Payload) != string(captured.Envelope.Payload) {
		t.Fatalf("Load() after failed Capture = %#v, want previously valid envelope %#v unmutated", loaded.Envelope, captured.Envelope)
	}
}
