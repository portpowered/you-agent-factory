// Package service implements the parent-private checkpoint recovery capability.
package service

import (
	"strings"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

// Recovery persists versioned opaque checkpoint envelopes through the private
// CheckpointStore port.
type Recovery struct {
	store checkpointrecovery.CheckpointStore
}

var _ checkpointrecovery.Service = (*Recovery)(nil)

// New constructs checkpoint recovery over the supplied store.
func New(store checkpointrecovery.CheckpointStore) *Recovery {
	return &Recovery{store: store}
}

// Capture stores one versioned opaque checkpoint envelope by checkpoint identity.
func (r *Recovery) Capture(req checkpointrecovery.CaptureRequest) (checkpointrecovery.CaptureResult, error) {
	checkpointID := strings.TrimSpace(req.CheckpointID)
	if checkpointID == "" {
		return checkpointrecovery.CaptureResult{}, checkpointrecovery.ErrCheckpointNotFound
	}
	strategyKind := strings.TrimSpace(req.StrategyKind)
	if strategyKind == "" {
		strategyKind = checkpointrecovery.RuntimeOpaqueCheckpointStrategyKind
	}
	envelope := checkpointrecovery.Envelope{
		CheckpointID:  checkpointID,
		SchemaVersion: checkpointrecovery.RuntimeOpaqueCheckpointSchemaVersion,
		StrategyKind:  strategyKind,
		Payload:       append([]byte(nil), req.Payload...),
	}
	if err := r.store.Put(envelope); err != nil {
		return checkpointrecovery.CaptureResult{}, err
	}
	stored, err := r.store.Get(checkpointID)
	if err != nil {
		return checkpointrecovery.CaptureResult{}, err
	}
	return checkpointrecovery.CaptureResult{Envelope: stored}, nil
}
