// Package wire constructs the parent-private Factory Runtime checkpoint recovery
// capability.
package wire

import (
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/processlocal"
)

// NewProcessLocalCheckpointStore constructs the default process-local adapter
// for versioned opaque checkpoint envelopes.
func NewProcessLocalCheckpointStore() checkpointrecovery.CheckpointStore {
	return processlocal.New()
}
