package service_test

import (
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
)

func TestRecoveryRestorePersistsCompatibleEnvelope(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	payload := []byte(`{"factoryState":"PAUSED"}`)
	restored, err := recovery.Restore(checkpointrecovery.RestoreRequest{
		Envelope: checkpointrecovery.Envelope{
			CheckpointID:  "checkpoint-1",
			SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
			StrategyKind:  checkpointrecovery.RuntimeOpaqueCheckpointStrategyKind,
			Payload:       payload,
		},
	})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.Facts.FactoryState != "PAUSED" {
		t.Fatalf("Restore() facts = %#v, want PAUSED factory state", restored.Facts)
	}

	loaded, err := recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "checkpoint-1"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Envelope.CheckpointID != restored.Envelope.CheckpointID ||
		loaded.Envelope.SchemaVersion != restored.Envelope.SchemaVersion ||
		string(loaded.Envelope.Payload) != string(restored.Envelope.Payload) {
		t.Fatalf("Load() after restore envelope = %#v, want %#v", loaded.Envelope, restored.Envelope)
	}
}

func TestRecoveryRestoreRejectsMissingCheckpointIdentity(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Restore(checkpointrecovery.RestoreRequest{})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("Restore() error = %v, want ErrCheckpointNotFound", err)
	}
}

func TestRecoveryRestoreRejectsCorruptEnvelope(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Restore(checkpointrecovery.RestoreRequest{
		Envelope: checkpointrecovery.Envelope{
			CheckpointID:  "checkpoint-1",
			SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
		},
	})
	if !errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
		t.Fatalf("Restore() error = %v, want ErrCorruptCheckpoint", err)
	}
}

func TestRecoveryRestoreRejectsIncompatibleSchema(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Restore(checkpointrecovery.RestoreRequest{
		Envelope: checkpointrecovery.Envelope{
			CheckpointID:  "checkpoint-1",
			SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion + 1,
			Payload:       []byte(`{"factoryState":"PAUSED"}`),
		},
	})
	if !errors.Is(err, factoryruntime.ErrIncompatibleCheckpoint) {
		t.Fatalf("Restore() error = %v, want ErrIncompatibleCheckpoint", err)
	}
}

func TestRecoveryRestoreRejectsIncompatibleStrategyKind(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Restore(checkpointrecovery.RestoreRequest{
		Envelope: checkpointrecovery.Envelope{
			CheckpointID:  "checkpoint-1",
			SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
			StrategyKind:  "petri",
			Payload:       []byte(`{"factoryState":"PAUSED"}`),
		},
	})
	if !errors.Is(err, factoryruntime.ErrIncompatibleCheckpoint) {
		t.Fatalf("Restore() error = %v, want ErrIncompatibleCheckpoint", err)
	}
}

func TestRecoveryRestoreDoesNotPersistCorruptPayload(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	_, err := recovery.Restore(checkpointrecovery.RestoreRequest{
		Envelope: checkpointrecovery.Envelope{
			CheckpointID:  "checkpoint-1",
			SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
			Payload:       []byte(`{}`),
		},
	})
	if !errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
		t.Fatalf("Restore() error = %v, want ErrCorruptCheckpoint", err)
	}
	_, err = recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "checkpoint-1"})
	if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
		t.Fatalf("Load() after failed restore error = %v, want ErrCheckpointNotFound", err)
	}
}
