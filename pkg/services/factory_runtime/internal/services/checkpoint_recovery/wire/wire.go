// Package wire constructs the parent-private Factory Runtime checkpoint recovery
// capability.
package wire

import (
	"errors"
	"path/filepath"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptfilesystem"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptsummary"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/processlocal"
	checkpointrecoveryservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/service"
)

const (
	durableOpaqueCheckpointDirectory     = "opaque-checkpoints"
	durableJavaScriptCheckpointDirectory = "javascript-checkpoints"
)

// NewProcessLocalCheckpointStore constructs the default process-local adapter
// for versioned opaque checkpoint envelopes.
func NewProcessLocalCheckpointStore() checkpointrecovery.CheckpointStore {
	return processlocal.New()
}

// NewDurableCheckpointStore constructs the filesystem-backed opaque checkpoint
// adapter beneath one explicitly supplied project-local durable-state root.
// The opaque and JavaScript stores use separate child directories so the same
// checkpoint ID cannot make the two persisted formats collide.
func NewDurableCheckpointStore(durableRoot string) (checkpointrecovery.CheckpointStore, error) {
	dir, err := durableCheckpointDirectory(durableRoot, durableOpaqueCheckpointDirectory)
	if err != nil {
		return nil, err
	}
	return filesystem.New(dir)
}

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

// NewDurableJavaScriptCheckpointStore constructs the filesystem-backed
// JavaScript checkpoint adapter beneath one explicitly supplied project-local
// durable-state root.
func NewDurableJavaScriptCheckpointStore(
	durableRoot string,
) (factoryruntime.JavaScriptCheckpointStore, error) {
	dir, err := durableCheckpointDirectory(durableRoot, durableJavaScriptCheckpointDirectory)
	if err != nil {
		return nil, err
	}
	return javascriptfilesystem.New(dir)
}

// NewJavaScriptCheckpointSummaries constructs the default JavaScript checkpoint
// summary projector used by Sessions durable execution wiring.
func NewJavaScriptCheckpointSummaries() factoryruntime.JavaScriptCheckpointSummaries {
	return javascriptsummary.New()
}

func durableCheckpointDirectory(root, name string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "", errors.New("durable checkpoint root is required")
	}
	return filepath.Join(filepath.Clean(trimmed), name), nil
}
