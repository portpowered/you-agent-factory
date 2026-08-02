package service_test

import (
	"context"
	"errors"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/internal/service"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type countingProvidersRoot struct {
	providers.Service
	getProviderCalls int
}

var _ providers.Service = (*countingProvidersRoot)(nil)

func (fake *countingProvidersRoot) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (fake *countingProvidersRoot) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	fake.getProviderCalls++
	return providers.GetProviderResult{
		Provider: providers.Descriptor{
			ID:           request.ID,
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
	}, nil
}

func (fake *countingProvidersRoot) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, errors.New("not implemented")
}

// TestResolutionOwnerConstructionDoesNotQueryProviders seals the ownership
// fence: holding the private resolution owner performs no Providers catalog
// work until a concrete provider identity is resolved.
func TestResolutionOwnerConstructionDoesNotQueryProviders(t *testing.T) {
	t.Parallel()

	providersRoot := &countingProvidersRoot{}
	service, err := internalservice.New(providersRoot)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	if providersRoot.getProviderCalls != 0 {
		t.Fatalf("providers GetProvider calls during construction = %d, want 0", providersRoot.getProviderCalls)
	}
	if service == nil {
		t.Fatal("constructed resolution service is nil")
	}
}

// TestResolutionOwnerDoesNotMutateDocumentBaseline seals document ownership
// outside Resolution: resolve reads detached inputs without rewriting them.
func TestResolutionOwnerDoesNotMutateDocumentBaseline(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	clonedBaseline := baseline

	_, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if baseline != clonedBaseline {
		t.Fatalf("document baseline mutated: got %#v, want %#v", baseline, clonedBaseline)
	}
}
