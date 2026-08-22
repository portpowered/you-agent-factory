package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
)

// NewJavaScriptCheckpointStore constructs the default JavaScript checkpoint
// store from the parent-private checkpoint recovery layout.
func NewJavaScriptCheckpointStore() factoryruntime.JavaScriptCheckpointStore {
	return checkpointrecoverywire.NewJavaScriptCheckpointStore()
}

// NewDurableJavaScriptCheckpointStore returns a constructor for the
// filesystem-backed JavaScript checkpoint store. Callers supply the exact
// filesystem effect and an explicitly supplied durable-state root; the root is
// shared safely with the opaque checkpoint store by the parent-private
// checkpoint-recovery wire.
var NewDurableJavaScriptCheckpointStore = checkpointrecoverywire.NewDurableJavaScriptCheckpointStore

// NewJavaScriptCheckpointSummaries constructs the default JavaScript checkpoint
// summary projector from the parent-private checkpoint recovery layout.
func NewJavaScriptCheckpointSummaries() factoryruntime.JavaScriptCheckpointSummaries {
	return checkpointrecoverywire.NewJavaScriptCheckpointSummaries()
}
