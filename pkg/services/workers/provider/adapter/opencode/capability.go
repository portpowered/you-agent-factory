// Package opencode owns OpenCode-native subprocess negotiation and decoding.
package opencode

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

const (
	defaultCacheEntries     = 64
	defaultDiscoveryTimeout = 5 * time.Second
)

var ErrCapabilityCacheFull = errors.New("opencode capability cache is full")

var ErrCapabilityDecisionStale = errors.New("opencode capability decision is not current")

// Mode is the selected OpenCode output protocol.
type Mode string

const (
	ModeStructured Mode = "structured"
	ModeFinalOnly  Mode = "final_only"
)

// Installation identifies the currently resolved executable contents. The
// fingerprint changes when the executable is replaced, even at the same path.
type Installation struct {
	Executable  string
	Fingerprint string
}

func (i Installation) cacheKey() string {
	return i.Executable + "\x00" + i.Fingerprint
}

// Decision is the reusable result of negotiating one installed OpenCode CLI.
type Decision struct {
	Installation Installation
	Version      string
	Mode         Mode
}

// Capabilities reports only fidelity supported by the negotiated mode.
func (d Decision) Capabilities() adapter.Capabilities {
	if d.Mode == ModeStructured {
		return adapter.Capabilities{
			NativeStreaming:  true,
			MessageSnapshots: true,
			StableItemIDs:    true,
		}
	}
	return adapter.Capabilities{MessageSnapshots: true, FinalOnly: true}
}

// Identifier resolves the executable before any capability subprocess runs.
type Identifier interface {
	Identify(context.Context, string) (Installation, error)
}

// Discoverer performs the bounded, prompt-free version and capability probe.
type Discoverer interface {
	Discover(context.Context, Installation) (Decision, error)
}

type cacheEntry struct {
	ready    chan struct{}
	decision Decision
	err      error
}

// Resolver shares successful negotiation results across invocations. Failed
// discoveries are removed, and capacity is strictly bounded. Caller
// cancellation stops only that caller's wait; discovery has its own timeout so
// one canceled caller cannot poison concurrent callers.
type Resolver struct {
	identifier Identifier
	discoverer Discoverer
	timeout    time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[string]*cacheEntry
	order   []string
}

// NewResolver constructs an isolated capability cache.
func NewResolver(
	identifier Identifier,
	discoverer Discoverer,
	maxEntries int,
	discoveryTimeout time.Duration,
) (*Resolver, error) {
	if identifier == nil {
		return nil, errors.New("opencode capability resolver requires an identifier")
	}
	if discoverer == nil {
		return nil, errors.New("opencode capability resolver requires a discoverer")
	}
	if maxEntries == 0 {
		maxEntries = defaultCacheEntries
	}
	if maxEntries < 1 {
		return nil, errors.New("opencode capability cache size must be positive")
	}
	timeout := discoveryTimeout
	if timeout == 0 {
		timeout = defaultDiscoveryTimeout
	}
	if timeout < 0 {
		return nil, errors.New("opencode capability discovery timeout must be positive")
	}
	return &Resolver{
		identifier: identifier, discoverer: discoverer,
		timeout: timeout, maxEntries: maxEntries, entries: make(map[string]*cacheEntry),
	}, nil
}

// Resolve returns the cached decision for the current executable identity or
// shares one in-flight discovery among concurrent callers.
func (r *Resolver) Resolve(ctx context.Context, executable string) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	installation, err := r.identifier.Identify(ctx, executable)
	if err != nil {
		return Decision{}, fmt.Errorf("identify opencode executable: %w", err)
	}
	key := installation.cacheKey()

	r.mu.Lock()
	if existing := r.entries[key]; existing != nil {
		r.touchLocked(key)
		r.mu.Unlock()
		return awaitDecision(ctx, existing)
	}
	if !r.makeRoomLocked() {
		r.mu.Unlock()
		return Decision{}, ErrCapabilityCacheFull
	}
	entry := &cacheEntry{ready: make(chan struct{})}
	r.entries[key] = entry
	r.order = append(r.order, key)
	r.mu.Unlock()

	go r.discover(key, installation, entry)
	return awaitDecision(ctx, entry)
}

// Downgrade replaces one current structured decision with final-only fidelity.
// The replacement entry is immutable so callers already awaiting the previous
// decision cannot race with the cache update.
func (r *Resolver) Downgrade(decision Decision) (Decision, error) {
	if r == nil || decision.Mode != ModeStructured {
		return Decision{}, ErrCapabilityDecisionStale
	}
	key := decision.Installation.cacheKey()
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[key]
	if entry == nil {
		return Decision{}, ErrCapabilityDecisionStale
	}
	select {
	case <-entry.ready:
	default:
		return Decision{}, ErrCapabilityDecisionStale
	}
	if entry.err != nil {
		return Decision{}, ErrCapabilityDecisionStale
	}
	if entry.decision.Installation == decision.Installation && entry.decision.Version == decision.Version && entry.decision.Mode == ModeFinalOnly {
		return entry.decision, nil
	}
	if entry.decision != decision {
		return Decision{}, ErrCapabilityDecisionStale
	}
	downgraded := decision
	downgraded.Mode = ModeFinalOnly
	replacement := &cacheEntry{ready: make(chan struct{}), decision: downgraded}
	close(replacement.ready)
	r.entries[key] = replacement
	r.touchLocked(key)
	return downgraded, nil
}

func (r *Resolver) discover(key string, installation Installation, entry *cacheEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	decision, err := r.discoverer.Discover(ctx, installation)
	if err == nil {
		decision.Installation = installation
		if decision.Mode != ModeStructured && decision.Mode != ModeFinalOnly {
			err = fmt.Errorf("opencode discovery returned invalid mode %q", decision.Mode)
		}
	}

	r.mu.Lock()
	entry.decision, entry.err = decision, err
	if err != nil {
		delete(r.entries, key)
		r.removeOrderLocked(key)
	}
	close(entry.ready)
	r.mu.Unlock()
}

func awaitDecision(ctx context.Context, entry *cacheEntry) (Decision, error) {
	select {
	case <-ctx.Done():
		return Decision{}, ctx.Err()
	case <-entry.ready:
		return entry.decision, entry.err
	}
}

func (r *Resolver) makeRoomLocked() bool {
	if len(r.entries) < r.maxEntries {
		return true
	}
	for _, key := range r.order {
		entry := r.entries[key]
		select {
		case <-entry.ready:
			delete(r.entries, key)
			r.removeOrderLocked(key)
			return true
		default:
		}
	}
	return false
}

func (r *Resolver) touchLocked(key string) {
	r.removeOrderLocked(key)
	r.order = append(r.order, key)
}

func (r *Resolver) removeOrderLocked(key string) {
	for index, candidate := range r.order {
		if candidate == key {
			r.order = append(r.order[:index], r.order[index+1:]...)
			return
		}
	}
}
