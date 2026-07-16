package cursors

import (
	"context"
	"errors"
	"testing"
)

type controlledStore struct {
	checkpoint Checkpoint
	found      bool
	saveErr    error
}

func (s *controlledStore) Load(context.Context, StorageIdentity) (Checkpoint, bool, error) {
	return s.checkpoint, s.found, nil
}

func (s *controlledStore) Save(_ context.Context, _ StorageIdentity, checkpoint Checkpoint) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.checkpoint = checkpoint
	s.found = true
	return nil
}

func (*controlledStore) Close() error { return nil }

func TestTrackerAdvanceDoesNotMoveMemoryWhenPersistenceFails(t *testing.T) {
	store := &controlledStore{}
	tracker, err := NewTracker(store, testIdentity())
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	firstSequence := 4
	if err := tracker.Advance(context.Background(), Checkpoint{AfterEventID: "event-4", AfterSequence: &firstSequence}); err != nil {
		t.Fatalf("Advance(first): %v", err)
	}

	store.saveErr = errors.New("disk unavailable")
	secondSequence := 5
	if err := tracker.Advance(context.Background(), Checkpoint{AfterEventID: "event-5", AfterSequence: &secondSequence}); err == nil {
		t.Fatal("Advance(second) = nil, want persistence error")
	}
	current, found := tracker.Current()
	if !found || current.AfterEventID != "event-4" || current.AfterSequence == nil || *current.AfterSequence != firstSequence {
		t.Fatalf("Current = %#v, %v; want unchanged event-4 checkpoint", current, found)
	}
}

func TestTrackerRestoreMissingIsExplicitEmptyStart(t *testing.T) {
	tracker, err := NewTracker(&controlledStore{}, testIdentity())
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	checkpoint, found, err := tracker.Restore(context.Background())
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if found || checkpoint != (Checkpoint{}) {
		t.Fatalf("Restore = %#v, %v; want empty start", checkpoint, found)
	}
}

func TestTrackerRejectsInvalidConstructionAndCanceledOperations(t *testing.T) {
	if _, err := NewTracker(nil, testIdentity()); err == nil {
		t.Fatal("NewTracker(nil) = nil, want store error")
	}
	invalidIdentity := testIdentity()
	invalidIdentity.ConsumerID = ""
	if _, err := NewTracker(&controlledStore{}, invalidIdentity); err == nil {
		t.Fatal("NewTracker(invalid identity) = nil, want identity error")
	}
	tracker, err := NewTracker(&controlledStore{}, testIdentity())
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := tracker.Restore(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Restore(canceled) error = %v", err)
	}
	if err := tracker.Advance(canceled, Checkpoint{AfterEventID: "event-1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Advance(canceled) error = %v", err)
	}
	if err := tracker.Advance(context.Background(), Checkpoint{}); err == nil {
		t.Fatal("Advance(empty checkpoint) = nil, want validation error")
	}
}

func testIdentity() StorageIdentity {
	return StorageIdentity{
		BackendScopeID:     "backend-a",
		FactorySessionID:   "session-a",
		StreamGenerationID: "generation-a",
		ConsumerID:         "dashboard-a",
	}
}
