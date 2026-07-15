// Package cursors owns Factory Session reconnect-cursor identity, persistence
// contracts, and recovery classification. Storage implementations live at the
// platform boundary and receive these values through explicit construction.
package cursors

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrStoreClosed reports use of a reconnect-cursor store after its lifecycle
// has ended.
var ErrStoreClosed = errors.New("factory session reconnect cursor store is closed")

// StorageIdentity identifies one independently persisted stream consumer.
// ConsumerID lets multiple clients resume the same Factory Session without
// overwriting each other's acknowledged position.
type StorageIdentity struct {
	BackendScopeID      string `json:"backendScopeId"`
	LogicalSessionKeyID string `json:"logicalSessionKeyId,omitempty"`
	FactorySessionID    string `json:"factorySessionId"`
	StreamGenerationID  string `json:"streamGenerationId"`
	ConsumerID          string `json:"consumerId"`
}

// NormalizeStorageIdentity removes boundary whitespace without choosing
// session-remapping or stale-cursor policy.
func NormalizeStorageIdentity(identity StorageIdentity) StorageIdentity {
	return StorageIdentity{
		BackendScopeID:      strings.TrimSpace(identity.BackendScopeID),
		LogicalSessionKeyID: strings.TrimSpace(identity.LogicalSessionKeyID),
		FactorySessionID:    strings.TrimSpace(identity.FactorySessionID),
		StreamGenerationID:  strings.TrimSpace(identity.StreamGenerationID),
		ConsumerID:          strings.TrimSpace(identity.ConsumerID),
	}
}

// ValidateStorageIdentity rejects ambiguous persistence identities.
func ValidateStorageIdentity(identity StorageIdentity) error {
	identity = NormalizeStorageIdentity(identity)
	for name, value := range map[string]string{
		"backend scope id":     identity.BackendScopeID,
		"factory session id":   identity.FactorySessionID,
		"stream generation id": identity.StreamGenerationID,
		"consumer id":          identity.ConsumerID,
	} {
		if value == "" {
			return fmt.Errorf("factory session reconnect cursor %s is required", name)
		}
	}
	return nil
}

// Checkpoint is the last Factory Event acknowledged by one stream consumer.
// When both fields are present, AfterEventID takes precedence during replay.
type Checkpoint struct {
	AfterEventID  string `json:"afterEventId,omitempty"`
	AfterSequence *int   `json:"afterSequence,omitempty"`
}

// NormalizeCheckpoint returns a detached, boundary-normalized checkpoint.
func NormalizeCheckpoint(checkpoint Checkpoint) Checkpoint {
	checkpoint.AfterEventID = strings.TrimSpace(checkpoint.AfterEventID)
	if checkpoint.AfterSequence != nil {
		sequence := *checkpoint.AfterSequence
		checkpoint.AfterSequence = &sequence
	}
	return checkpoint
}

// ValidateCheckpoint rejects an empty or negative acknowledged position.
func ValidateCheckpoint(checkpoint Checkpoint) error {
	checkpoint = NormalizeCheckpoint(checkpoint)
	if checkpoint.AfterEventID == "" && checkpoint.AfterSequence == nil {
		return errors.New("factory session reconnect cursor position is required")
	}
	if checkpoint.AfterSequence != nil && *checkpoint.AfterSequence < 0 {
		return fmt.Errorf("factory session reconnect cursor sequence must not be negative: %d", *checkpoint.AfterSequence)
	}
	return nil
}

// Store persists reconnect positions. A missing identity is an empty start and
// returns found=false with no error. Save must make the complete checkpoint
// durable before returning nil.
type Store interface {
	Load(ctx context.Context, identity StorageIdentity) (checkpoint Checkpoint, found bool, err error)
	Save(ctx context.Context, identity StorageIdentity, checkpoint Checkpoint) error
	Close() error
}
