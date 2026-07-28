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

// NewJavaScriptCheckpointSummaries constructs the default JavaScript checkpoint
// summary projector from the parent-private checkpoint recovery layout.
func NewJavaScriptCheckpointSummaries() factoryruntime.JavaScriptCheckpointSummaries {
	return checkpointrecoverywire.NewJavaScriptCheckpointSummaries()
}
