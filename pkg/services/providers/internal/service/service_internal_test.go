package service

import (
	"context"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestServiceInternalDelegationCoverage(t *testing.T) {
	catalog := internalCatalogStub{}
	execution := internalExecutionStub{}
	root, err := New(catalog, execution)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := root.ListProviders(t.Context(), providers.ListProvidersRequest{}); err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if _, err := root.GetProvider(
		t.Context(),
		providers.GetProviderRequest{ID: providers.IDCodex},
	); err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if _, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt"},
	); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if root, err := New(nil, execution); err == nil || root != nil {
		t.Fatalf("New(nil, execution) = (%v, %v), want error", root, err)
	}
	if root, err := New(catalog, nil); err == nil || root != nil {
		t.Fatalf("New(catalog, nil) = (%v, %v), want error", root, err)
	}
}

type internalCatalogStub struct{}

func (internalCatalogStub) ResolveProviderID(id providers.ID) (providers.ID, error) {
	return id, nil
}

func (internalCatalogStub) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (internalCatalogStub) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{
		Provider: providers.Descriptor{ID: request.ID},
	}, nil
}

type internalExecutionStub struct{}

func (internalExecutionStub) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{Content: "ok"}, nil
}
