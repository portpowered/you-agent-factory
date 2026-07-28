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

// ProbeFacts are live readiness and prerequisite facts for one projected catalog
// provider. Descriptions must stay bounded and must not include raw environment
// values, filesystem paths, or native probe output.
type ProbeFacts struct {
	Readiness     providers.Readiness
	Prerequisites []providers.Prerequisite
}

// ProbeQuery reports current readiness facts for one projected catalog provider.
// Inputs and outputs are detached Providers-owned values; implementations must
// honor context cancellation and must not expose Workers types through the
// catalog boundary.
type ProbeQuery func(context.Context, providers.Descriptor) (ProbeFacts, error)
