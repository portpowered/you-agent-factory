// Package runners defines the Workers-private registry for common Runner
// implementations. Peer services consume only the Workers root service.
package runners

import "github.com/portpowered/infinite-you/pkg/services/workers"

// Registration explicitly associates one canonical identity and metadata
// snapshot with its common Workers Runner implementation.
type Registration struct {
	Identity string
	Metadata workers.RunnerMetadata
	Runner   workers.Runner
}

// Binding is one resolved registry entry. Metadata collections are detached
// from registry state on every resolution.
type Binding struct {
	Identity string
	Metadata workers.RunnerMetadata
	Runner   workers.Runner
}

// Service resolves immutable runner registrations without executing or
// probing their implementations.
type Service interface {
	Resolve(identity string) (Binding, bool)
}
