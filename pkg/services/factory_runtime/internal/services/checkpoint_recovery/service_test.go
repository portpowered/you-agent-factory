package checkpoint_recovery_test

import (
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

func TestValidateEnvelopeRejectsStructurallyCorruptValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		envelope  checkpointrecovery.Envelope
		wantError error
	}{
		{
			name:      "empty checkpoint id",
			envelope:  checkpointrecovery.Envelope{SchemaVersion: 1, Payload: []byte("payload")},
			wantError: factoryruntime.ErrCorruptCheckpoint,
		},
		{
			name:      "non-positive schema version",
			envelope:  checkpointrecovery.Envelope{CheckpointID: "checkpoint-1", Payload: []byte("payload")},
			wantError: factoryruntime.ErrCorruptCheckpoint,
		},
		{
			name:      "empty payload",
			envelope:  checkpointrecovery.Envelope{CheckpointID: "checkpoint-1", SchemaVersion: 1},
			wantError: factoryruntime.ErrCorruptCheckpoint,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkpointrecovery.ValidateEnvelope(tc.envelope)
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("ValidateEnvelope() error = %v, want %v", err, tc.wantError)
			}
		})
	}
}
