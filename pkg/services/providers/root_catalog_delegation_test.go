package providers_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestRootCatalogDelegation_FulfillsPublishedListAndGet(t *testing.T) {
	t.Parallel()

	var root providers.Service
	root, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) < 2 {
		t.Fatalf("ListProviders() = %#v, want multiple known providers", list.Providers)
	}

	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("GetProvider(codex).Provider.ID = %q", got.Provider.ID)
	}

	assertGetErrorIs(t, root, providers.GetProviderRequest{}, providers.ErrInvalidID)

	agy, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDAntigravity})
	if err != nil {
		t.Fatalf("GetProvider(agy) = %v", err)
	}
	if agy.Provider.Availability != providers.AvailabilitySelectable {
		t.Fatalf("agy availability = %q, want selectable", agy.Provider.Availability)
	}
}

func TestRootCatalogDelegation_RegistersCodexAdapter(t *testing.T) {
	t.Parallel()

	service, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil")
	}

	_, err = service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "root-delegation-attempt",
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency ||
		!strings.Contains(failure.Message, "Codex") {
		t.Fatalf("Execute() error = %#v, want Codex adapter dependency failure", err)
	}
}

func TestProvidersRootWireBoundaryPublishesExternalRegistrationThroughService(t *testing.T) {
	t.Parallel()

	integration := providerswire.ProgressingExternalIntegration("sealed-external", "sealed output")
	root, err := providerswire.NewService(providerswire.WithRegistrations(providerswire.Registration{
		Manifest: providerswire.Manifest{
			ID:                           "sealed-external",
			DisplayName:                  providerswire.LocalizedValue{Value: "Sealed External"},
			ImplementationAvailability:   providerswire.ImplementationExternallySupplied,
			TechnicalSupportLevel:        providerswire.SupportProduction,
			MaximumExecutionCapabilities: providerswire.ExecutionCapabilities{PromptSubmission: true},
		},
		Integration: integration,
	}))
	if err != nil {
		t.Fatalf("providers/wire.NewService() error = %v", err)
	}
	var service providers.Service = root

	listed, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if descriptor, ok := findProviderDescriptor(listed.Providers, providers.ID("sealed-external")); !ok {
		t.Fatalf("ListProviders() = %#v, want sealed-external", listed.Providers)
	} else if descriptor.DisplayName != "Sealed External" || descriptor.Availability != providers.AvailabilitySelectable {
		t.Fatalf("sealed-external descriptor = %#v, want selectable named descriptor", descriptor)
	}
	if stats := integration.Stats(); stats != (providerswire.ProgressingIntegrationStats{}) {
		t.Fatalf("construction/catalog calls = %#v, want inert external integration", stats)
	}

	got, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.ID("sealed-external")})
	if err != nil {
		t.Fatalf("GetProvider(sealed-external) error = %v", err)
	}
	if got.Provider.ID != providers.ID("sealed-external") {
		t.Fatalf("GetProvider(sealed-external).Provider.ID = %q", got.Provider.ID)
	}

	result, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider: providers.ID("sealed-external"), AttemptID: "sealed-attempt", UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Execute(sealed-external) error = %v", err)
	}
	if result.Content != "sealed output" {
		t.Fatalf("Execute(sealed-external).Content = %q, want sealed output", result.Content)
	}
	if result.Diagnostics == nil || len(result.Diagnostics.Progress) != 1 {
		t.Fatalf("Execute(sealed-external).Diagnostics = %#v, want one progress diagnostic", result.Diagnostics)
	}
	if stats := integration.Stats(); stats.InvokeCalls != 1 || stats.ProgressWrites != 1 || stats.TerminalCloses != 1 {
		t.Fatalf("external integration stats = %#v, want one invoke/progress/terminal close", stats)
	}
}

func TestProvidersRootWireBoundaryPreservesTypedFailuresAndRegistrationValidation(t *testing.T) {
	t.Parallel()

	root, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providers/wire.NewService() error = %v", err)
	}

	if _, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: "not-registered"}); !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("GetProvider(not-registered) error = %v, want ErrUnknownProvider", err)
	}
	_, err = root.Execute(context.Background(), providers.ExecuteRequest{Provider: providers.IDCodex})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("Execute(invalid request) error = %#v, want invalid-request ExecuteFailure", err)
	}

	for _, test := range []struct {
		name         string
		registration providerswire.Registration
		want         string
	}{
		{
			name:         "missing integration",
			registration: providerswire.Registration{Manifest: providerswire.Manifest{ID: "missing-integration"}},
			want:         "integration is required",
		},
		{
			name: "identity mismatch",
			registration: providerswire.Registration{
				Manifest:    providerswire.Manifest{ID: "manifest-id"},
				Integration: providerswire.ProgressingExternalIntegration("integration-id", "ignored"),
			},
			want: "does not match manifest",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, constructErr := providerswire.NewService(providerswire.WithRegistrations(test.registration))
			if constructErr == nil || service != nil || !strings.Contains(constructErr.Error(), test.want) {
				t.Fatalf("NewService(%s) = (%#v, %v), want nil service and error containing %q", test.name, service, constructErr, test.want)
			}
		})
	}
}

func findProviderDescriptor(descriptors []providers.Descriptor, id providers.ID) (providers.Descriptor, bool) {
	for _, descriptor := range descriptors {
		if descriptor.ID == id {
			return descriptor, true
		}
	}
	return providers.Descriptor{}, false
}
