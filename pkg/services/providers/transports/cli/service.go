// Package cli defines the Providers service-owned CLI adapter.
package cli

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service exposes Providers CLI command operations to Cobra composition.
type Service interface {
	List(ListConfig) error
	isProvidersCLIService()
}

type service struct {
	root providers.Service
}

// New constructs the Providers CLI service from the accepted Providers root
// contract. Construction is inert: it does not call ListProviders,
// GetProvider, or Execute on the injected root.
func New(root providers.Service) Service {
	if root == nil {
		return nil
	}
	return &service{root: root}
}

func (s *service) isProvidersCLIService() {}
