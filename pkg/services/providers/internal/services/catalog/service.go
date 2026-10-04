// Package catalog defines the parent-private Providers catalog service.
package catalog

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service serves detached, deterministically ordered provider descriptors
// projected from the accepted standardized provider catalog.
type Service interface {
	ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error)
	GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error)
	// ResolveProviderID returns the canonical identity for a catalog ID or
	// accepted alias without probing readiness or performing adapter I/O.
	ResolveProviderID(providers.ID) (providers.ID, error)
	// RegistrationProvider returns the detached static catalog facts used to
	// bind an execution registration. It never performs readiness probing.
	RegistrationProvider(providers.ID) (providers.Descriptor, error)
}

// CapabilityOverride replaces the static capability facts for one existing
// catalog provider identity during process construction. It is a construction
// seam for hosts that supply an authoritative route-specific capability view;
// it cannot add a provider that is absent from the catalog.
type CapabilityOverride struct {
	Provider     providers.ID
	Capabilities []providers.Capability
}

// ProbeOperation supplies the completed request-time descriptor projection.
// Composition selects identity or readiness probing before constructing Service.
type ProbeOperation func(context.Context, providers.Descriptor) (providers.Descriptor, error)
