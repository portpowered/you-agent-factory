package service

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

// Option configures catalog construction.
type Option func(*service)

// WithDescriptors contributes Providers-owned descriptors in addition to the
// packaged native catalog.
func WithDescriptors(descriptors ...providers.Descriptor) Option {
	return func(s *service) { s.extraDescriptors = append(s.extraDescriptors, descriptors...) }
}

// WithProbeQuery injects request-time readiness probing. Construction remains
// inert; probe side effects occur only during ListProviders and GetProvider.
func WithProbeQuery(query catalog.ProbeQuery) Option {
	return func(s *service) {
		s.probe = query
	}
}
