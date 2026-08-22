package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// NewProviderFromService returns the Providers root selected by composition.
// Provider execution is no longer adapted into a Workers-owned client port.
func NewProviderFromService(service providers.Service) (providers.Service, error) {
	if service == nil {
		return nil, fmt.Errorf("construct Providers service: service is required")
	}
	return service, nil
}
