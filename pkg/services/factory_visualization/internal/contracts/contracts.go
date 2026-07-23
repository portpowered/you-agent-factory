// Package contracts defines the narrow collaborator roles consumed by Factory
// Visualization without widening the service root interface inventory.
package contracts

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// RuntimeReader is the live-runtime observation role consumed by Factory
// Visualization.
type RuntimeReader interface {
	WithRuntimeRead(func(*factorysessions.LiveRuntime) error) error
}
