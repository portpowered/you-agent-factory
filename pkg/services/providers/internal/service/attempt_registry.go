package service

import (
	"errors"
	"fmt"
	"sync"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// errAttemptAlreadyLive reports that another execution already holds the
// exact canonical provider identity plus attempt ID this attempt requested.
var errAttemptAlreadyLive = errors.New("provider attempt id is already live for this provider")

// liveAttemptKey identifies one live execution by canonical provider identity
// plus the exact caller-supplied attempt ID. A control request must match
// both dimensions to reach this attempt and no other.
type liveAttemptKey struct {
	provider  providers.ID
	attemptID string
}

// liveAttemptRegistry correlates in-flight Execute calls with their canonical
// provider identity plus exact attempt ID, so a later control request can
// reach only the exact live attempt it names. It holds no provider-specific
// cancellation behavior; it only tracks which identities are currently live.
type liveAttemptRegistry struct {
	mu   sync.Mutex
	live map[liveAttemptKey]struct{}
}

func newLiveAttemptRegistry() *liveAttemptRegistry {
	return &liveAttemptRegistry{live: make(map[liveAttemptKey]struct{})}
}

// bind registers key as live and returns a release function that removes it.
// bind fails with errAttemptAlreadyLive when key is already live, so a second
// execution cannot replace or steal an existing live identity. release is
// idempotent and safe to call from any terminal path, including a panic
// unwind, exactly once per successful bind.
func (registry *liveAttemptRegistry) bind(key liveAttemptKey) (func(), error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.live[key]; exists {
		return nil, fmt.Errorf("%w: provider %q attempt %q", errAttemptAlreadyLive, key.provider, key.attemptID)
	}
	registry.live[key] = struct{}{}

	var once sync.Once
	release := func() {
		once.Do(func() {
			registry.mu.Lock()
			defer registry.mu.Unlock()
			delete(registry.live, key)
		})
	}
	return release, nil
}

// contains reports whether key is currently live. Exposed for direct
// same-package tests; peers observe liveness only indirectly through bind.
func (registry *liveAttemptRegistry) contains(key liveAttemptKey) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	_, exists := registry.live[key]
	return exists
}
