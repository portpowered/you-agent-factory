// Package contracts defines the narrow collaborator roles consumed by Factory
// Visualization without widening the service root interface inventory.
package contracts

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

// RuntimeReader is the live-runtime observation role consumed by Factory
// Visualization.
type RuntimeReader interface {
	WithRuntimeRead(func(*factorysessions.LiveRuntime) error) error
}

// RuntimeSinkID is the opaque selection identity retained by composition
// owners. It is implementation-facing so the Visualization service root does
// not acquire another named interface declaration.
type RuntimeSinkID string

// RuntimeSinkOwner retains operation-scoped sinks without widening the
// customer-facing Visualization service root.
type RuntimeSinkOwner interface {
	RegisterRuntimeSink(liveviewprojection.Sink) (RuntimeSinkID, error)
	RuntimeSink(RuntimeSinkID) (liveviewprojection.Sink, bool)
	CloseRuntimeSink(RuntimeSinkID)
}
