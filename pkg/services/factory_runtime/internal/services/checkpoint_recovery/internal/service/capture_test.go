package service_test

import (
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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

	root := checkpointrecovery.RootCheckpointFromEnvelope(captured.Envelope)
	if root.CheckpointID != "checkpoint-1" ||
		root.SchemaVersion != checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion ||
		root.StrategyKind != checkpointrecovery.RuntimeOpaqueCheckpointStrategyKind ||
		string(root.Payload) != string(payload) {
		t.Fatalf("RootCheckpointFromEnvelope() = %#v, want root checkpoint mapping", root)
	}
}

func TestRecoveryCaptureRejectsMissingCheckpointIdentity(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		Payload: []byte(`{"factoryState":"RUNNING"}`),
	})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("Capture() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRecoveryCaptureRejectsCorruptPayload(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoveryservice.New(checkpointrecoverywire.NewProcessLocalCheckpointStore())
	_, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		CheckpointID: "checkpoint-1",
	})
	if !errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
		t.Fatalf("Capture() error = %v, want ErrCorruptCheckpoint", err)
	}
}
