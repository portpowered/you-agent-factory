package service

import (
	"context"
	"errors"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
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

type configureACPStub struct {
	internalDisabledACP
	configure func(context.Context, []providers.ACPIntegration) error
}

func (stub configureACPStub) Configure(ctx context.Context, integrations []providers.ACPIntegration) error {
	return stub.configure(ctx, integrations)
}

func TestRootConfigurationUsesCompletedACPAndKeepsPeerIsolated(t *testing.T) {
	t.Parallel()
	failure := errors.New("configuration rejected")
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
			owned := configureACPStub{configure: func(received context.Context, integrations []providers.ACPIntegration) error {
				calls++
				assertOwnedACPConfiguration(t, ctx, received, integrations)
				integrations[0].Command = "changed by collaborator"
				return outcome.err
			}}
			peer := configureACPStub{configure: func(context.Context, []providers.ACPIntegration) error {
				peerCalls++
				return nil
			}}
			first, err := NewWithACP(internalCatalogStub{}, internalExecutionStub{}, owned, nil, logging.NoopLogger{}, owned)
			if err != nil {
				t.Fatal(err)
			}
			second, err := NewWithACP(internalCatalogStub{}, internalExecutionStub{}, peer, nil, logging.NoopLogger{}, peer)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 || peerCalls != 0 {
				t.Fatal("construction configured an ACP collaborator")
			}
			configured := []providers.ACPIntegration{{Name: "owned", Command: "owned-agent acp"}}
			if err := first.ConfigureACPIntegrations(ctx, configured); err != outcome.err {
				t.Fatalf("ConfigureACPIntegrations() = %v, want exact error %v", err, outcome.err)
			}
			if calls != 1 || peerCalls != 0 || configured[0].Command != "owned-agent acp" {
				t.Fatalf("configuration leaked: owned/peer calls %d/%d, input %#v", calls, peerCalls, configured)
			}
			if err := second.ConfigureACPIntegrations(t.Context(), nil); err != nil || peerCalls != 1 || calls != 1 {
				t.Fatalf("peer ConfigureACPIntegrations() = %v, owned/peer calls %d/%d", err, calls, peerCalls)
			}
		})
	}
}

func assertOwnedACPConfiguration(t *testing.T, expected, received context.Context, integrations []providers.ACPIntegration) {
	t.Helper()
	if received != expected {
		t.Fatal("Configure replaced the supplied context")
	}
	if len(integrations) != 1 || integrations[0].Command != "owned-agent acp" {
		t.Fatalf("Configure received integrations = %#v", integrations)
	}
}

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

func (internalExecutionStub) Continue(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "provider continuation adapter is unavailable"}
}

func (internalDisabledACP) Continue(context.Context, providers.ID, providers.ExecuteRequest, providers.SessionRef) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "ACP provider continuation is unavailable"}
}

func TestRootDispatchContinuationUsesCompletedCapabilities(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"native", "acp"} {
		for _, outcome := range []struct {
			name string
			err  error
		}{
			{"success", nil},
			{"dependency", providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "selected continuation unavailable"}},
			{"canceled", context.Canceled},
		} {
			t.Run(route+"/"+outcome.name, func(t *testing.T) {
				t.Parallel()
				calls, ordinaryCalls := 0, 0
				reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "exact-session"}
				request := providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "continued-attempt", UserMessage: "next turn"}
				observe := func(got providers.ExecuteRequest, ref providers.SessionRef) (providers.ExecuteResult, error) {
					calls++
					if got.Provider != request.Provider || got.AttemptID != request.AttemptID || got.UserMessage != request.UserMessage || ref != reference {
						t.Fatalf("continuation input = (%#v, %#v), want (%#v, %#v)", got, ref, request, reference)
					}
					return providers.ExecuteResult{Content: "selected continuation"}, outcome.err
				}
				native := completedNativeContinuation{ordinaryCalls: &ordinaryCalls, run: func(request providers.ExecuteRequest, reference providers.SessionRef) (providers.ExecuteResult, error) {
					if route != "native" {
						t.Fatal("ACP continuation reached the native peer")
					}
					return observe(request, reference)
				}}
				peer := completedACPContinuation{selected: route == "acp", ordinaryCalls: &ordinaryCalls, run: func(request providers.ExecuteRequest, reference providers.SessionRef) (providers.ExecuteResult, error) {
					if route != "acp" {
						t.Fatal("native continuation reached the ACP peer")
					}
					return observe(request, reference)
				}}
				root, err := NewWithACP(internalCatalogStub{}, native, peer, nil, logging.NoopLogger{}, internalDisabledACP{})
				if err != nil {
					t.Fatal(err)
				}
				if calls != 0 || ordinaryCalls != 0 {
					t.Fatal("construction invoked an execution capability")
				}
				// Reusing the exact attempt after each return proves dispatch released its
				// live binding, including when the supplied continuation failed.
				for i := 0; i < 2; i++ {
					result, err := root.dispatchContinuation(t.Context(), request, reference)
					if result.Content != "selected continuation" || err != outcome.err {
						t.Fatalf("dispatchContinuation() = (%#v, %v), want selected result and exact error %v", result, err, outcome.err)
					}
				}
				if calls != 2 || ordinaryCalls != 0 {
					t.Fatalf("continuation/ordinary calls = %d/%d, want 2/0", calls, ordinaryCalls)
				}
			})
		}
	}
}

type completedNativeContinuation struct {
	internalExecutionStub
	ordinaryCalls *int
	run           func(providers.ExecuteRequest, providers.SessionRef) (providers.ExecuteResult, error)
}

func (fixture completedNativeContinuation) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	*fixture.ordinaryCalls++
	return providers.ExecuteResult{}, errors.New("ordinary execution must not be selected")
}

func (fixture completedNativeContinuation) Continue(_ context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
	return fixture.run(request.ExecuteRequest, *request.ResumeSession)
}

type completedACPContinuation struct {
	internalDisabledACP
	selected      bool
	ordinaryCalls *int
	run           func(providers.ExecuteRequest, providers.SessionRef) (providers.ExecuteResult, error)
}

func (fixture completedACPContinuation) Resolve(id providers.ID) (providers.ID, bool) {
	return id, fixture.selected
}

func (fixture completedACPContinuation) Execute(context.Context, providers.ID, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	*fixture.ordinaryCalls++
	return providers.ExecuteResult{}, errors.New("ordinary execution must not be selected")
}

func (fixture completedACPContinuation) Continue(_ context.Context, id providers.ID, request providers.ExecuteRequest, reference providers.SessionRef) (providers.ExecuteResult, error) {
	if id != reference.Provider {
		return providers.ExecuteResult{}, errors.New("ACP identity differs from exact reference")
	}
	return fixture.run(request, reference)
}
