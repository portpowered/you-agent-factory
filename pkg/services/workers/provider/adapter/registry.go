package adapter

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

var (
	// ErrUnknownProvider identifies a lookup for an unregistered adapter.
	ErrUnknownProvider = errors.New("unknown provider adapter")
	// ErrDuplicateProvider identifies duplicate normalized registry identities.
	ErrDuplicateProvider = errors.New("duplicate provider adapter")
	// ErrInvalidProvider identifies an adapter with no usable identity.
	ErrInvalidProvider = errors.New("invalid provider adapter")
)

// Registry is an immutable deterministic provider adapter lookup table.
type Registry struct {
	byIdentity map[Identity]Adapter
	identities []Identity
}

// NewRegistry validates and indexes adapters by normalized identity. Duplicate
// registration is rejected instead of depending on registration order.
func NewRegistry(adapters ...Adapter) (*Registry, error) {
	registry := &Registry{byIdentity: make(map[Identity]Adapter, len(adapters))}
	for _, candidate := range adapters {
		if isNilAdapter(candidate) {
			return nil, fmt.Errorf("%w: adapter is nil", ErrInvalidProvider)
		}
		identity := NormalizeIdentity(candidate.Identity())
		if identity == "" {
			return nil, fmt.Errorf("%w: identity is empty", ErrInvalidProvider)
		}
		if _, exists := registry.byIdentity[identity]; exists {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateProvider, identity)
		}
		registry.byIdentity[identity] = candidate
		registry.identities = append(registry.identities, identity)
	}
	sort.Slice(registry.identities, func(i, j int) bool {
		return registry.identities[i] < registry.identities[j]
	})
	return registry, nil
}

func isNilAdapter(candidate Adapter) bool {
	if candidate == nil {
		return true
	}
	value := reflect.ValueOf(candidate)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// NormalizeIdentity applies the registry's stable lookup normalization.
func NormalizeIdentity(identity Identity) Identity {
	return Identity(strings.ToLower(strings.TrimSpace(string(identity))))
}

// Lookup returns exactly the registered adapter for identity or an explicit
// unknown-provider error. No default provider is selected.
func (r *Registry) Lookup(identity Identity) (Adapter, error) {
	normalized := NormalizeIdentity(identity)
	if r == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, normalized)
	}
	registered, ok := r.byIdentity[normalized]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, normalized)
	}
	return registered, nil
}

// Identities returns registered identities in lexical order as a detached
// snapshot, independent of registration or map iteration order.
func (r *Registry) Identities() []Identity {
	if r == nil {
		return nil
	}
	return append([]Identity(nil), r.identities...)
}
