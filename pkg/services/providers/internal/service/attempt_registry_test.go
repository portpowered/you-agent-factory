package service

import (
	"errors"
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
	release, err := registry.bind(key)
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

	release, err := registry.bind(key)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	defer release()

	if _, err := registry.bind(key); !errors.Is(err, errAttemptAlreadyLive) {
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

	release, err := registry.bind(key)
	if err != nil {
		t.Fatalf("bind(key) error = %v, want nil", err)
	}
	defer release()

	otherAttemptRelease, err := registry.bind(sameProviderOtherAttempt)
	if err != nil {
		t.Fatalf("bind(same provider, other attempt) error = %v, want nil", err)
	}
	defer otherAttemptRelease()

	otherProviderRelease, err := registry.bind(otherProviderSameAttempt)
	if err != nil {
		t.Fatalf("bind(other provider, same attempt) error = %v, want nil", err)
	}
	defer otherProviderRelease()
}

func TestLiveAttemptRegistry_ReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	registry := newLiveAttemptRegistry()
	key := liveAttemptKey{provider: providers.IDCodex, attemptID: "attempt-1"}

	release, err := registry.bind(key)
	if err != nil {
		t.Fatalf("bind() error = %v, want nil", err)
	}
	release()
	release()

	if registry.contains(key) {
		t.Fatal("contains() = true after repeated release, want false")
	}

	if _, err := registry.bind(key); err != nil {
		t.Fatalf("rebind after release error = %v, want nil", err)
	}
}
