package checkpoint_recovery_test

import (
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

func TestEncodeRuntimeOpaquePayloadProducesNonEmptyBytes(t *testing.T) {
	t.Parallel()

	payload, err := checkpointrecovery.EncodeRuntimeOpaquePayload(checkpointrecovery.ExecutionCaptureFacts{
		FactoryState: "PAUSED",
	})
	if err != nil {
		t.Fatalf("EncodeRuntimeOpaquePayload() error = %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("EncodeRuntimeOpaquePayload() payload is empty, want opaque bytes")
	}
}

func TestEncodeRuntimeOpaquePayloadRejectsMissingFactoryState(t *testing.T) {
	t.Parallel()

	_, err := checkpointrecovery.EncodeRuntimeOpaquePayload(checkpointrecovery.ExecutionCaptureFacts{})
	if !errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
		t.Fatalf("EncodeRuntimeOpaquePayload() error = %v, want ErrCorruptCheckpoint", err)
	}
}
