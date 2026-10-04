package service

import (
	"context"
	"errors"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	_ "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestServiceInternalDelegationCoverage(t *testing.T) {
	catalog := internalCatalogStub{}
	execution := internalExecutionStub{}
	root, err := NewWithACP(catalog, execution, internalDisabledACP{}, nil, logging.NoopLogger{}, internalDisabledACP{})
	if err != nil {
		t.Fatalf("NewWithACP() error = %v", err)
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
	if root, err := NewWithACP(nil, execution, internalDisabledACP{}, nil, logging.NoopLogger{}, internalDisabledACP{}); err == nil || root != nil {
		t.Fatalf("New(nil, execution) = (%v, %v), want error", root, err)
	}
	if root, err := NewWithACP(catalog, nil, internalDisabledACP{}, nil, logging.NoopLogger{}, internalDisabledACP{}); err == nil || root != nil {
		t.Fatalf("New(catalog, nil) = (%v, %v), want error", root, err)
	}
}

func TestEffectiveACPIntegrationsPreservesUnchangedPackageRuntimeBinding(t *testing.T) {
	packaged := []providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor", Aliases: []string{"factory-cursor"},
		Transport: "stdio", Command: "cursor-agent acp", Arguments: []string{"acp"},
		RuntimePosture: "installed_executable", ImplementationProfile: "cursor-acp",
	}}
	configured := []providers.ACPIntegration{{
		ID: "saved-entry", Name: "cursor", Transport: "stdio", Command: "cursor-agent acp",
	}}

	got := effectiveACPIntegrations(packaged, configured)
	if len(got) != 1 || got[0].ImplementationProfile != "cursor-acp" || got[0].RuntimePosture != "installed_executable" || len(got[0].Arguments) != 1 || got[0].Arguments[0] != "acp" || len(got[0].Aliases) != 1 || got[0].Aliases[0] != "factory-cursor" {
		t.Fatalf("effectiveACPIntegrations() = %#v, want package runtime binding preserved", got)
	}
}

type internalCatalogStub struct{}

func (internalCatalogStub) ResolveProviderID(id providers.ID) (providers.ID, error) {
	return id, nil
}

func (internalCatalogStub) RegistrationProvider(
	id providers.ID,
) (providers.Descriptor, error) {
	return providers.Descriptor{
		ID:           id,
		Availability: providers.AvailabilitySelectable,
	}, nil
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

type internalDisabledACP struct{ acp.Service }

func (internalDisabledACP) Resolve(providers.ID) (providers.ID, bool) { return "", false }
func (internalDisabledACP) Integrations() []providers.ACPIntegration  { return nil }
func (internalDisabledACP) Close(context.Context) error               { return nil }

type closeOperation func(context.Context) error

func (operation closeOperation) Close(ctx context.Context) error { return operation(ctx) }

func TestRootCloseUsesSuppliedLifecycleAndKeepsPeerIsolated(t *testing.T) {
	t.Parallel()
	failure := errors.New("owned cleanup failed")
	for _, outcome := range []struct {
		name string
		err  error
	}{
		{"success", nil}, {"failure", failure}, {"canceled", context.Canceled},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if outcome.name == "canceled" {
				cancel()
			}
			calls, peerCalls := 0, 0
			owned := closeOperation(func(received context.Context) error {
				calls++
				if received != ctx {
					t.Fatal("Close replaced the supplied context")
				}
				return outcome.err
			})
			peer := closeOperation(func(context.Context) error { peerCalls++; return nil })
			first, err := NewWithACP(internalCatalogStub{}, internalExecutionStub{},
				internalDisabledACP{}, nil, logging.NoopLogger{}, owned)
			if err != nil {
				t.Fatal(err)
			}
			second, err := NewWithACP(internalCatalogStub{}, internalExecutionStub{},
				internalDisabledACP{}, nil, logging.NoopLogger{}, peer)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 || peerCalls != 0 {
				t.Fatal("construction closed a lifecycle")
			}
			if err := first.Close(ctx); err != outcome.err {
				t.Fatalf("Close() = %v, want exact supplied error %v", err, outcome.err)
			}
			if calls != 1 || peerCalls != 0 {
				t.Fatalf("owned/peer close calls = %d/%d, want 1/0", calls, peerCalls)
			}
			if err := second.Close(t.Context()); err != nil || peerCalls != 1 || calls != 1 {
				t.Fatalf("peer Close() = %v, owned/peer calls = %d/%d", err, calls, peerCalls)
			}
		})
	}
}
