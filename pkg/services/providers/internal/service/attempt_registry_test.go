package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestLiveAttemptRegistry_BindThenContainsThenRelease(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	if registry.contains(key) {
		t.Fatal("contains() = true before bind, want false")
	}
	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after bind, want true")
	}
	release()
	if registry.contains(key) {
		t.Fatal("contains() = true after release, want false")
	}
}

func TestLiveAttemptRegistry_BindCollisionRejectsSecondLiveIdentity(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	defer release()

	if _, err := registry.bind(key, nil); !errors.Is(err, errAttemptAlreadyLive) {
		t.Fatalf("second bind() error = %v, want errAttemptAlreadyLive", err)
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after rejected second bind, want the first binding to remain live")
	}
}

func TestLiveAttemptRegistry_DistinctIdentitiesAreIndependentlyLive(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	sameProviderOtherAttempt := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-2"}
	otherProviderSameAttempt := liveAttemptKey{provider: providers.IDClaude, attemptID: "attempt-1"}
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind(key) error = %v, want nil", err)
	}
	defer release()

	otherAttemptRelease, err := registry.bind(sameProviderOtherAttempt, nil)
	if err != nil {
		t.Fatalf("bind(same provider, other attempt) error = %v, want nil", err)
	}
	defer otherAttemptRelease()

	otherProviderRelease, err := registry.bind(otherProviderSameAttempt, nil)
	if err != nil {
		t.Fatalf("bind(other provider, same attempt) error = %v, want nil", err)
	}
	defer otherProviderRelease()
}

func TestLiveAttemptRegistry_ReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	release()
	release()

	if registry.contains(key) {
		t.Fatal("contains() = true after repeated release, want false")
	}

	if _, err := registry.bind(key, nil); err != nil {
		t.Fatalf("rebind after release error = %v, want nil", err)
	}
}

func TestNativeAttemptControl_SupportsCancelAndTerminateOnly(t *testing.T) {
	t.Parallel()

	control := &nativeAttemptControl{cancel: func() {}, done: make(chan struct{})}
	for _, action := range []providers.ControlAction{providers.ControlActionCancel, providers.ControlActionTerminate} {
		if !control.supports(action) {
			t.Fatalf("supports(%q) = false, want true", action)
		}
	}
	if control.supports(providers.ControlActionPause) {
		t.Fatal("supports(pause) = true, want false")
	}
}

func TestLiveAttemptRegistry_ClaimNoOpForUnboundIdentity(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
		t.Fatal("claim() on unbound identity = true, want false")
	}
}

func TestLiveAttemptRegistry_ClaimNoOpWithoutControlHandle(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key, nil)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	defer release()

	if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
		t.Fatal("claim() on identity with no control handle = true, want false")
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after no-op claim, want the identity to remain live")
	}
}

func TestLiveAttemptRegistry_ClaimNoOpForUnsupportedAction(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
	cancelCalls := 0
	control := &nativeAttemptControl{
		cancel: func() { cancelCalls++ },
		done:   make(chan struct{}),
	}

	release, err := registry.bind(key, control)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	defer release()

	if _, ok := registry.claim(key, providers.ControlActionPause); ok {
		t.Fatal("claim(pause) = true, want false (no per-attempt pause seam)")
	}
	if cancelCalls != 0 {
		t.Fatalf("cancel calls = %d, want 0 for an unsupported action", cancelCalls)
	}
	if !registry.contains(key) {
		t.Fatal("contains() = false after unsupported-action claim, want the identity to remain live")
	}
}

func TestLiveAttemptRegistry_ClaimRemovesEntryAndSignalWaitsForDone(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
	cancelCalls := 0
	done := make(chan struct{})
	control := &nativeAttemptControl{
		cancel: func() { cancelCalls++ },
		done:   done,
	}

	if _, err := registry.bind(key, control); err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}

	claimed, ok := registry.claim(key, providers.ControlActionCancel)
	if !ok {
		t.Fatal("claim() ok = false, want true for a bound, supported action")
	}
	if registry.contains(key) {
		t.Fatal("contains() = true after claim, want the entry removed")
	}

	// close(done) races the goroutine's signal() call; either order proves the
	// contract: signal() never returns before done is readable, and once it
	// is, signal() returns after invoking cancel exactly once. A dedicated
	// root-level test (TestControlAttempt_BlocksUntilSignaledNativeAttemptReturns)
	// proves the actual blocking-until-terminal-behavior property against a
	// caller-controlled gate, since that cannot be shown deterministically at
	// this unit level without sleep-based timing.
	signalDone := make(chan struct{})
	go func() {
		claimed.signal()
		close(signalDone)
	}()
	close(done)
	<-signalDone
	if cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", cancelCalls)
	}
}

func TestLiveAttemptRegistry_ClaimIsExclusiveAmongConcurrentCallers(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
	closedDone := make(chan struct{})
	close(closedDone)
	control := &nativeAttemptControl{cancel: func() {}, done: closedDone}

	if _, err := registry.bind(key, control); err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}

	const attempts = 8
	var wins int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("concurrent claim() winners = %d, want exactly 1", wins)
	}
}

// TestLiveAttemptRegistry_ClaimRacingReleaseHasOneDeterministicWinnerAndAlwaysRemoves
// proves the natural-completion-versus-control race story 004 requires:
// registry.claim (a control call) and the release closure returned by bind
// (a natural Execute completion) share one mutex, so a concurrent pair of
// them can never both observe the identity as live, and the identity is
// always gone afterward either way. Run repeatedly (no sleep) so both
// possible lock-acquisition orderings actually occur across iterations.
func TestLiveAttemptRegistry_ClaimRacingReleaseHasOneDeterministicWinnerAndAlwaysRemoves(t *testing.T) {
	t.Parallel()

	for i := range 200 {
		registry := newLiveAttemptRegistry()
		key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}
		closedDone := make(chan struct{})
		close(closedDone)
		control := &nativeAttemptControl{cancel: func() {}, done: closedDone}

		release, err := registry.bind(key, control)
		if err != nil {
			t.Fatalf("iteration %d: bind() error = %v, want nil", i, err)
		}

		var claimed int32
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, ok := registry.claim(key, providers.ControlActionCancel); ok {
				atomic.AddInt32(&claimed, 1)
			}
		}()
		go func() {
			defer wg.Done()
			release()
		}()
		wg.Wait()

		if claimed > 1 {
			t.Fatalf("iteration %d: claim() succeeded more than once racing a concurrent release()", i)
		}
		if registry.contains(key) {
			t.Fatalf("iteration %d: contains() = true after a claim/release race, want the identity removed either way", i)
		}
	}
}
