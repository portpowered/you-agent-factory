package cursors

import (
	"context"
	"errors"
	"sync"
)

// Tracker coordinates one consumer's durable and in-memory acknowledged
// position. It advances memory only after persistence succeeds.
type Tracker struct {
	store    Store
	identity StorageIdentity

	mu      sync.RWMutex
	current Checkpoint
	found   bool
}

// NewTracker validates the explicit store and storage identity.
func NewTracker(store Store, identity StorageIdentity) (*Tracker, error) {
	if store == nil {
		return nil, errors.New("factory session reconnect cursor store is required")
	}
	identity = NormalizeStorageIdentity(identity)
	if err := ValidateStorageIdentity(identity); err != nil {
		return nil, err
	}
	return &Tracker{store: store, identity: identity}, nil
}

// Restore loads the durable position after process or session reconstruction.
func (t *Tracker) Restore(ctx context.Context) (Checkpoint, bool, error) {
	if err := contextError(ctx); err != nil {
		return Checkpoint{}, false, err
	}
	checkpoint, found, err := t.store.Load(ctx, t.identity)
	if err != nil {
		return Checkpoint{}, false, err
	}
	if found {
		checkpoint = NormalizeCheckpoint(checkpoint)
	}
	t.mu.Lock()
	t.current = checkpoint
	t.found = found
	t.mu.Unlock()
	return NormalizeCheckpoint(checkpoint), found, nil
}

// Advance durably records an acknowledged position before exposing it as the
// current in-memory checkpoint.
func (t *Tracker) Advance(ctx context.Context, checkpoint Checkpoint) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	checkpoint = NormalizeCheckpoint(checkpoint)
	if err := ValidateCheckpoint(checkpoint); err != nil {
		return err
	}
	if err := t.store.Save(ctx, t.identity, checkpoint); err != nil {
		return err
	}
	t.mu.Lock()
	t.current = checkpoint
	t.found = true
	t.mu.Unlock()
	return nil
}

// Current returns a detached snapshot of the in-memory acknowledged position.
func (t *Tracker) Current() (Checkpoint, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return NormalizeCheckpoint(t.current), t.found
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("factory session reconnect cursor context is required")
	}
	return ctx.Err()
}
