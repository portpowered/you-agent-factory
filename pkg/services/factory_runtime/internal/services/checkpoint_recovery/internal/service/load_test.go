package service_test

import (
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	checkpointrecoveryservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/service"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
)

func TestRecoveryLoadReturnsStoredEnvelope(t *testing.T) {
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

	loaded, err := recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "checkpoint-1"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Envelope.CheckpointID != captured.Envelope.CheckpointID ||
		loaded.Envelope.SchemaVersion != captured.Envelope.SchemaVersion ||
		loaded.Envelope.StrategyKind != captured.Envelope.StrategyKind ||
		string(loaded.Envelope.Payload) != string(captured.Envelope.Payload) {
		t.Fatalf("Load() envelope = %#v, want %#v", loaded.Envelope, captured.Envelope)
	}
	if loaded.Compatible {
		t.Fatal("Load() Compatible = true, want false when expected schema is zero")
	}
}

func TestRecoveryLoadReportsSchemaCompatibility(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		CheckpointID: "checkpoint-1",
		Payload:      []byte(`{"factoryState":"PAUSED"}`),
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	compatible, err := recovery.Load(checkpointrecovery.LoadRequest{
		CheckpointID:          "checkpoint-1",
		ExpectedSchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
	})
	if err != nil {
		t.Fatalf("Load(compatible) error = %v", err)
	}
	if !compatible.Compatible {
		t.Fatal("Load(compatible) Compatible = false, want true")
	}

	incompatible, err := recovery.Load(checkpointrecovery.LoadRequest{
		CheckpointID:          "checkpoint-1",
		ExpectedSchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion + 1,
	})
	if err != nil {
		t.Fatalf("Load(incompatible) error = %v", err)
	}
	if incompatible.Compatible {
		t.Fatal("Load(incompatible) Compatible = true, want false")
	}
}

func TestRecoveryLoadRejectsMissingCheckpointIdentity(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Load(checkpointrecovery.LoadRequest{})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("Load() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRecoveryLoadRejectsMissingCheckpoint(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "missing"})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("Load() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRecoveryLoadRejectsCorruptStoredEnvelope(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoveryservice.New(corruptEnvelopeStore{})
	_, err := recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "checkpoint-1"})
	if !errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
		t.Fatalf("Load() error = %v, want ErrCorruptCheckpoint", err)
	}
}

type corruptEnvelopeStore struct{}

func (corruptEnvelopeStore) Put(checkpointrecovery.Envelope) error {
	return nil
}

func (corruptEnvelopeStore) Get(string) (checkpointrecovery.Envelope, error) {
	return checkpointrecovery.Envelope{}, factoryruntime.ErrCorruptCheckpoint
}
