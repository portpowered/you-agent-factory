// Package http owns HTTP adaptation for Providers operations.
//
// The top-level HTTP transport registers generated routes and composes this
// adapter when PSS fan-in arrives. Request decoding, representation mapping,
// Providers root invocation, error mapping, and cancel/timeout policy for
// Providers-owned HTTP operations remain here with the owning service.
package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// Adapter maps Providers service values at the outward HTTP boundary.
type Adapter struct {
	root providers.Service
}

// NewAdapter constructs the Providers HTTP representation adapter. Construction
// is inert aside from retaining the injected root; it does not start provider
// processes, probes, or lifecycle loops.
func NewAdapter(root providers.Service) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
}

// Root returns the accepted Providers root consumed by adapter-owned operations.
func (a *Adapter) Root() providers.Service {
	if a == nil {
		return nil
	}
	return a.root
}

func (a *Adapter) invokeListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	if a == nil || a.root == nil {
		return providers.ListProvidersResult{}, errors.New("providers service is required")
	}
	return a.root.ListProviders(ctx, request)
}

func (a *Adapter) invokeGetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if a == nil || a.root == nil {
		return providers.GetProviderResult{}, errors.New("providers service is required")
	}
	return a.root.GetProvider(ctx, request)
}

func (a *Adapter) invokeExecute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if a == nil || a.root == nil {
		return providers.ExecuteResult{}, errors.New("providers service is required")
	}
	return a.root.Execute(ctx, request)
}
