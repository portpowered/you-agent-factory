package service_test

import (
	"errors"
	"testing"

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
	if !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
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
	if !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
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
	if !errors.Is(err, checkpointrecovery.ErrIncompatibleCheckpoint) {
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
	if !errors.Is(err, checkpointrecovery.ErrIncompatibleCheckpoint) {
		t.Fatalf("Restore() error = %v, want ErrIncompatibleCheckpoint", err)
	}
}

func TestRecoveryRestoreFailuresDoNotMutatePreviouslyValidStoredEnvelope(t *testing.T) {
	t.Parallel()

	recovery := checkpointrecoverywire.New()
	validPayload := []byte(`{"factoryState":"PAUSED"}`)
	restored, err := recovery.Restore(checkpointrecovery.RestoreRequest{
		Envelope: checkpointrecovery.Envelope{
			CheckpointID:  "checkpoint-1",
			SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
			StrategyKind:  checkpointrecovery.RuntimeOpaqueCheckpointStrategyKind,
			Payload:       validPayload,
		},
	})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	failingRequests := []struct {
		name    string
		request checkpointrecovery.RestoreRequest
		wantErr error
	}{
		{
			name: "corrupt payload",
			request: checkpointrecovery.RestoreRequest{
				Envelope: checkpointrecovery.Envelope{
					CheckpointID:  "checkpoint-1",
					SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
					StrategyKind:  checkpointrecovery.RuntimeOpaqueCheckpointStrategyKind,
					Payload:       []byte(`{}`),
				},
			},
			wantErr: checkpointrecovery.ErrCorruptCheckpoint,
		},
		{
			name: "incompatible schema",
			request: checkpointrecovery.RestoreRequest{
				Envelope: checkpointrecovery.Envelope{
					CheckpointID:  "checkpoint-1",
					SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion + 1,
					StrategyKind:  checkpointrecovery.RuntimeOpaqueCheckpointStrategyKind,
					Payload:       validPayload,
				},
			},
			wantErr: checkpointrecovery.ErrIncompatibleCheckpoint,
		},
		{
			name: "incompatible strategy kind",
			request: checkpointrecovery.RestoreRequest{
				Envelope: checkpointrecovery.Envelope{
					CheckpointID:  "checkpoint-1",
					SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
					StrategyKind:  "petri",
					Payload:       validPayload,
				},
			},
			wantErr: checkpointrecovery.ErrIncompatibleCheckpoint,
		},
		{
			name:    "missing checkpoint identity",
			request: checkpointrecovery.RestoreRequest{},
			wantErr: checkpointrecovery.ErrCheckpointNotFound,
		},
	}

	for _, tc := range failingRequests {
		if _, err := recovery.Restore(tc.request); !errors.Is(err, tc.wantErr) {
			t.Fatalf("Restore(%s) error = %v, want %v", tc.name, err, tc.wantErr)
		}

		loaded, err := recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "checkpoint-1"})
		if err != nil {
			t.Fatalf("Load() after failed Restore(%s) error = %v", tc.name, err)
		}
		if loaded.Envelope.SchemaVersion != restored.Envelope.SchemaVersion ||
			loaded.Envelope.StrategyKind != restored.Envelope.StrategyKind ||
			string(loaded.Envelope.Payload) != string(restored.Envelope.Payload) {
			t.Fatalf(
				"Load() after failed Restore(%s) = %#v, want previously valid envelope %#v unmutated",
				tc.name, loaded.Envelope, restored.Envelope,
			)
		}
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
	if !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
		t.Fatalf("Restore() error = %v, want ErrCorruptCheckpoint", err)
	}
	_, err = recovery.Load(checkpointrecovery.LoadRequest{CheckpointID: "checkpoint-1"})
	if !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
		t.Fatalf("Load() after failed restore error = %v, want ErrCheckpointNotFound", err)
	}
}
