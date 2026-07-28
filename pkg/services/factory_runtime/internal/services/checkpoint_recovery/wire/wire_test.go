package wire_test

import (
	"testing"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
)

func TestNewProcessLocalCheckpointStoreConstructsWorkingAdapter(t *testing.T) {
	t.Parallel()

	var store checkpointrecovery.CheckpointStore = checkpointrecoverywire.NewProcessLocalCheckpointStore()
	if store == nil {
		t.Fatal("NewProcessLocalCheckpointStore() = nil, want process-local adapter")
	}
}
