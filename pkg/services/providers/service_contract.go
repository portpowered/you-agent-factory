package providers

import "context"

// Service is the singular cross-service Providers root authority. Peer packages
// depend on this one named interface for Providers-owned catalog enumeration,
// availability/capability facts, and one normalized execution attempt rather
// than Workers provider registry/conductor types or concrete adapter packages.
// Execute publishes additively on this same interface in later CTR-PROV slices.
type Service interface {
	// ListProviders returns detached catalog descriptors for every known
	// provider, including availability and capability facts. Unavailable or
	// prerequisite-blocked providers remain listed with their catalog facts.
	ListProviders(context.Context, ListProvidersRequest) (ListProvidersResult, error)
	// GetProvider returns one detached catalog descriptor for a Providers-owned
	// provider identity. Invalid identity fails with ErrInvalidID, unknown
	// identity fails with ErrUnknownProvider, and blocked availability or
	// prerequisite facts fail with ErrProviderUnavailable.
	GetProvider(context.Context, GetProviderRequest) (GetProviderResult, error)
}
