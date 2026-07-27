package service

import catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"

// Option configures catalog construction.
type Option func(*service)

// WithProbeQuery injects request-time readiness probing. Construction remains
// inert; probe side effects occur only during ListProviders and GetProvider.
func WithProbeQuery(query catalog.ProbeQuery) Option {
	return func(s *service) {
		s.probe = query
	}
}
