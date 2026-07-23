// Package contracts defines the narrow collaborator roles consumed by Factory
// Visualization without widening the service root interface inventory.
package contracts

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// ResponseEventCursor is the retained response-event role consumed by response
// presentation.
type ResponseEventCursor interface {
	Next(context.Context) ([]factorysessions.FactoryResponseEvent, error)
	Drain() ([]factorysessions.FactoryResponseEvent, error)
	Detach()
}

// RuntimeReader is the live-runtime observation role consumed by Factory
// Visualization.
type RuntimeReader interface {
	WithRuntimeRead(func(*factorysessions.LiveRuntime) error) error
}
