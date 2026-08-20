// Package wire constructs the parent-private Factory Runtime checkpoint recovery
// capability.
package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptsummary"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/processlocal"
	checkpointrecoveryservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/service"
)

// NewProcessLocalCheckpointStore constructs the default process-local adapter
// for versioned opaque checkpoint envelopes.
func NewProcessLocalCheckpointStore() checkpointrecovery.CheckpointStore {
	return processlocal.New()
}

// NewDurableCheckpointStore returns a constructor for the filesystem-backed
// opaque checkpoint adapter. Callers supply the exact filesystem effect and
// the project-local durable-state root; the opaque and JavaScript stores use
// separate child directories so the same ID cannot make formats collide.
var NewDurableCheckpointStore = processlocal.NewDurableCheckpointStore

// New constructs the default checkpoint recovery capability backed by the
// process-local CheckpointStore adapter.
func New() checkpointrecovery.Service {
	return checkpointrecoveryservice.New(NewProcessLocalCheckpointStore())
}

// NewJavaScriptCheckpointStore constructs the default JavaScript checkpoint
// store used by Sessions durable execution wiring.
func NewJavaScriptCheckpointStore() factoryruntime.JavaScriptCheckpointStore {
	return javascriptstore.New()
}

// NewDurableJavaScriptCheckpointStore returns a constructor for the
// filesystem-backed JavaScript checkpoint adapter. Callers supply the exact
// filesystem effect and the project-local durable-state root.
var NewDurableJavaScriptCheckpointStore = processlocal.NewDurableJavaScriptCheckpointStore

// NewJavaScriptCheckpointSummaries constructs the default JavaScript checkpoint
// summary projector used by Sessions durable execution wiring.
func NewJavaScriptCheckpointSummaries() factoryruntime.JavaScriptCheckpointSummaries {
	return javascriptsummary.New()
}
