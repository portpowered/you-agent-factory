// Package checkpoint_recovery defines the parent-private Factory Runtime
// checkpoint recovery capability. Peers interact through Runtime root contracts;
// CheckpointStore types remain inside this package.
package checkpoint_recovery

import (
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ErrCheckpointNotFound indicates a checkpoint identity is not present in the
// Runtime-owned checkpoint store.
var ErrCheckpointNotFound = factoryruntime.ErrCheckpointNotFound

// ErrCorruptCheckpoint indicates the checkpoint envelope failed integrity or
// shape checks without exposing strategy codec internals.
var ErrCorruptCheckpoint = factoryruntime.ErrCorruptCheckpoint

// ErrIncompatibleCheckpoint indicates the checkpoint schema or opaque payload
// is incompatible with the Runtime restore surface.
var ErrIncompatibleCheckpoint = factoryruntime.ErrIncompatibleCheckpoint

// Envelope is a versioned opaque checkpoint envelope persisted by checkpoint
// identity inside checkpoint_recovery. Payload bytes are opaque strategy data.
type Envelope struct {
	CheckpointID  string
	SchemaVersion int
	StrategyKind  string
	Payload       []byte
}

// CheckpointStore persists and retrieves versioned opaque checkpoint envelopes
// by checkpoint identity. It is the private persistence seam for recovery;
// peers must not import this type.
type CheckpointStore interface {
	Put(Envelope) error
	Get(checkpointID string) (Envelope, error)
}

// ValidateEnvelope reports whether an envelope is structurally loadable.
func ValidateEnvelope(envelope Envelope) error {
	if strings.TrimSpace(envelope.CheckpointID) == "" {
		return ErrCorruptCheckpoint
	}
	if envelope.SchemaVersion <= 0 {
		return ErrCorruptCheckpoint
	}
	if len(envelope.Payload) == 0 {
		return ErrCorruptCheckpoint
	}
	return nil
}
