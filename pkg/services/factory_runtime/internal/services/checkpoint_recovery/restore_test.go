package checkpoint_recovery_test

import (
	"errors"
	"testing"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

func TestRestoreRuntimeOpaquePayloadDecodesFactoryState(t *testing.T) {
	t.Parallel()

	payload, err := checkpointrecovery.EncodeRuntimeOpaquePayload(checkpointrecovery.ExecutionCaptureFacts{
		FactoryState: "PAUSED",
	})
	if err != nil {
		t.Fatalf("EncodeRuntimeOpaquePayload() error = %v", err)
	}

	facts, err := checkpointrecovery.RestoreRuntimeOpaquePayload(payload)
	if err != nil {
		t.Fatalf("RestoreRuntimeOpaquePayload() error = %v", err)
	}
	if facts.FactoryState != "PAUSED" {
		t.Fatalf("RestoreRuntimeOpaquePayload() facts = %#v, want PAUSED factory state", facts)
	}
}

func TestRestoreRuntimeOpaquePayloadRejectsCorruptPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "empty payload", payload: nil},
		{name: "invalid json", payload: []byte("{")},
		{name: "missing factory state", payload: []byte(`{}`)},
		{name: "blank factory state", payload: []byte(`{"factoryState":"   "}`)},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := checkpointrecovery.RestoreRuntimeOpaquePayload(tc.payload)
			if !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
				t.Fatalf("RestoreRuntimeOpaquePayload() error = %v, want ErrCorruptCheckpoint", err)
			}
		})
	}
}
