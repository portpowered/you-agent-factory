// Package wire constructs the parent-private Providers catalog subservice.
package wire

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/internal/service"
)

// IdentityProbe is the completed no-I/O catalog projection.
func IdentityProbe(ctx context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
	return catalogservice.IdentityProbe(ctx, descriptor)
}

// NewProbeOperation completes the bounded readiness projection over a query.
func NewProbeOperation(query catalog.ProbeQuery) catalog.ProbeOperation {
	return catalogservice.NewProbeOperation(query)
}

// NewService constructs an inert catalog over the accepted standardized
// provider catalog publication.
func NewService(probe catalog.ProbeOperation, descriptors []providers.Descriptor, overrides []catalog.CapabilityOverride) (catalog.Service, error) {
	return catalogservice.New(probe, descriptors, overrides)
}
