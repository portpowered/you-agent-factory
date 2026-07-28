// Package http owns HTTP adaptation for Factory Runtime operations.
//
// The top-level HTTP transport registers generated routes and composes this
// adapter when PSS fan-in arrives. Request decoding, generated contract mapping,
// Runtime root invocation, error mapping, and cancel/timeout policy for
// Runtime-owned HTTP operations remain here with the owning service.
package http

import (
	"errors"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// RuntimeRoot is the accepted Factory Runtime root contract used by the HTTP
// adapter. Adapter-owned operations invoke this surface rather than Runtime
// internal packages.
type RuntimeRoot = factoryruntime.Service

// Adapter maps Factory Runtime HTTP operations through the accepted root
// contract without importing Runtime internals or owning canonical state.
type Adapter struct {
	root RuntimeRoot
}

// NewAdapter constructs the Factory Runtime HTTP adapter bound to the accepted
// root Service seam. Nil roots fail closed by returning nil.
func NewAdapter(root RuntimeRoot) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
}

// Root returns the injected Factory Runtime root consumed by adapter-owned
// operations.
func (a *Adapter) Root() RuntimeRoot {
	if a == nil {
		return nil
	}
	return a.root
}

func (a *Adapter) runtimeRoot() (RuntimeRoot, error) {
	if a == nil || a.root == nil {
		return nil, errors.New("factory runtime service is required")
	}
	return a.root, nil
}
