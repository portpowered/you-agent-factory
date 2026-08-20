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

// NewDurableJavaScriptCheckpointStore constructs the filesystem-backed
// JavaScript checkpoint store beneath an explicitly supplied durable-state
// root. The root is shared safely with the opaque checkpoint store by the
// parent-private checkpoint-recovery wire.
func NewDurableJavaScriptCheckpointStore(
	durableRoot string,
) (factoryruntime.JavaScriptCheckpointStore, error) {
	return checkpointrecoverywire.NewDurableJavaScriptCheckpointStore(durableRoot)
}

// NewJavaScriptCheckpointSummaries constructs the default JavaScript checkpoint
// summary projector from the parent-private checkpoint recovery layout.
func NewJavaScriptCheckpointSummaries() factoryruntime.JavaScriptCheckpointSummaries {
	return checkpointrecoverywire.NewJavaScriptCheckpointSummaries()
}
