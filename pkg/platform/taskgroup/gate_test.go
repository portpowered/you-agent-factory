package taskgroup

import (
	"testing"
	"time"
)

func TestGateEnterSucceedsBeforeClose(t *testing.T) {
	var g Gate
	release, ok := g.Enter()
	if !ok {
		t.Fatal("Enter() ok = false, want true before Close")
	}
	if release == nil {
		t.Fatal("Enter() release = nil, want a non-nil release func when ok is true")
	}
	release()
}

func TestGateEnterFailsAfterClose(t *testing.T) {
	var g Gate
	g.Close()

	release, ok := g.Enter()
	if ok {
		t.Fatal("Enter() ok = true after Close(), want false")
	}
	if release != nil {
		t.Fatal("Enter() release != nil after Close(), want nil")
	}
}

func TestGateCloseIsIdempotent(t *testing.T) {
	var g Gate
	g.Close()
	g.Close()

	if _, ok := g.Enter(); ok {
		t.Fatal("Enter() ok = true after two Close() calls, want false")
	}
}

// TestGateCloseBlocksUntilInFlightEnterReleases proves Close cannot return
// while an Enter call it should have waited for is still unreleased --
// deterministically, not by timing: Close calls the underlying RWMutex's
// Lock, which the Go runtime guarantees cannot return while this test's own
// unreleased Enter still holds a read lock, so observing the "closed"
// channel as not-yet-closed at that point is a logical certainty, not a
// scheduling race. This is the guarantee serveConnection depends on to keep
// a "session/prompt" registration or a "session/cancel" dispatch from ever
// starting after Serve has already decided to return.
func TestGateCloseBlocksUntilInFlightEnterReleases(t *testing.T) {
	var g Gate
	release, ok := g.Enter()
	if !ok {
		t.Fatal("Enter() ok = false, want true before Close")
	}

	closed := make(chan struct{})
	go func() {
		g.Close()
		close(closed)
	}()

	select {
	case <-closed:
		t.Fatal("Close() returned while an Enter call it should have waited for was still unreleased")
	default:
	}

	release()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() never returned after the in-flight Enter released")
	}

	if _, ok := g.Enter(); ok {
		t.Fatal("Enter() ok = true after Close() completed, want false")
	}
}
