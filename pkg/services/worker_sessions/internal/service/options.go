package service

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// Option configures the optional read-side projections of the Worker
// Sessions registry. The existing boundary and Events appender remain the
// required lifecycle dependencies.
type Option func(*registry)

// WithClock injects the clock used for observation start/end timestamps and
// active duration. A nil clock is replaced with the real wall clock.
func WithClock(source platformclock.Source) Option {
	return func(registry *registry) {
		if source != nil {
			registry.clock = source
		}
	}
}

// WithProviderSessions injects the Provider Sessions root used to enrich an
// observation with normalized transcript, token, and parse facts.
func WithProviderSessions(service providersessions.Service) Option {
	return func(registry *registry) {
		registry.providerSessions = service
	}
}
