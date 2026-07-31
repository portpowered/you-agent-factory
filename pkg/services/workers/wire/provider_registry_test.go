package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewProviderRegistryProjectsProvidersAuthority(t *testing.T) {
	t.Parallel()

	registry, err := NewProviderRegistry(context.Background(), &registryProvidersFake{})
	if err != nil {
		t.Fatalf("NewProviderRegistry() = %v", err)
	}

	canonical, err := registry.CanonicalIdentity("openai")
	if err != nil {
		t.Fatalf("CanonicalIdentity(openai) = %v", err)
	}
	if canonical != string(providers.IDCodex) {
		t.Fatalf("CanonicalIdentity(openai) = %q, want codex", canonical)
	}

	selection, err := registry.ResolveRunnerSelection("cursor-cli", "", "")
	if err != nil {
		t.Fatalf("ResolveRunnerSelection() = %v", err)
	}
	if selection.RunnerID != workers.RunnerIDCursorCLI {
		t.Fatalf("ResolveRunnerSelection() = %#v, want cursor-cli", selection)
	}
	if selection.Source != workers.RunnerSelectionSourceWorkstation {
		t.Fatalf("selection source = %q", selection.Source)
	}

	if err := registry.ValidateRunnerPrerequisites(nil, "codex"); err != nil {
		t.Fatalf("ValidateRunnerPrerequisites(codex) = %v", err)
	}
	err = registry.ValidateRunnerPrerequisites(nil, "cursor")
	if !errors.Is(err, providers.ErrProviderUnavailable) {
		t.Fatalf("ValidateRunnerPrerequisites(cursor) = %v, want ErrProviderUnavailable", err)
	}
}

type registryProvidersFake struct{}

func (*registryProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}

func (*registryProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{
		Providers: []providers.Descriptor{
			{
				ID:           providers.IDCodex,
				Aliases:      []string{"openai"},
				DisplayName:  "Codex",
				Availability: providers.AvailabilitySelectable,
				Readiness:    providers.ReadinessReady,
				Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
			},
			{
				ID:           providers.IDCursor,
				DisplayName:  "Cursor",
				Availability: providers.AvailabilitySupportedButUnavailable,
				Readiness:    providers.ReadinessUnavailable,
				Prerequisites: []providers.Prerequisite{{
					Kind:   providers.PrerequisiteDependency,
					Name:   "cursor-agent",
					Status: providers.PrerequisiteMissing,
				}},
			},
		},
	}, nil
}

func (*registryProvidersFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	switch request.ID {
	case providers.IDCodex:
		return providers.GetProviderResult{Provider: providers.Descriptor{
			ID:           providers.IDCodex,
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		}}, nil
	case providers.IDCursor:
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	default:
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
}
