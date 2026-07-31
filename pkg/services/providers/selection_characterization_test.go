package providers_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestSelectionContract_ResolveIdentityAliasesAndCompatibility(t *testing.T) {
	t.Parallel()

	service := newCatalogPeerFake(
		providers.Descriptor{
			ID:           providers.IDCodex,
			Aliases:      []string{"openai-codex"},
			DisplayName:  "Codex",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
		providers.Descriptor{
			ID:           providers.IDClaude,
			DisplayName:  "Claude",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
		providers.Descriptor{
			ID:           providers.IDCursor,
			DisplayName:  "Cursor",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
	)

	tests := []struct {
		identity string
		want     providers.ID
	}{
		{identity: "codex", want: providers.IDCodex},
		{identity: "openai-codex", want: providers.IDCodex},
		{identity: "openai", want: providers.IDCodex},
		{identity: "anthropic", want: providers.IDClaude},
		{identity: "cursor-cli", want: providers.IDCursor},
		{identity: "agent", want: providers.IDCursor},
	}
	for _, test := range tests {
		test := test
		t.Run(test.identity, func(t *testing.T) {
			t.Parallel()
			got, err := providers.ResolveIdentity(
				context.Background(),
				service,
				providers.ResolveIdentityRequest{Identity: test.identity},
			)
			if err != nil {
				t.Fatalf("ResolveIdentity(%q) = %v", test.identity, err)
			}
			if got.ID != test.want {
				t.Fatalf("ResolveIdentity(%q) = %q, want %q", test.identity, got.ID, test.want)
			}
		})
	}

	_, err := providers.ResolveIdentity(
		context.Background(),
		service,
		providers.ResolveIdentityRequest{Identity: "unknown"},
	)
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("ResolveIdentity(unknown) = %v, want ErrUnknownProvider", err)
	}
}

func TestSelectionContract_ResolveSelectionPrecedenceAndDefault(t *testing.T) {
	t.Parallel()

	service := newCatalogPeerFake(
		providers.Descriptor{
			ID:           providers.IDCodex,
			DisplayName:  "Codex",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
		providers.Descriptor{
			ID:           providers.IDClaude,
			DisplayName:  "Claude",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
		providers.Descriptor{
			ID:           providers.IDCursor,
			DisplayName:  "Cursor",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
	)

	got, err := providers.ResolveSelection(context.Background(), service, providers.ResolveSelectionRequest{
		Workstation:   "cursor",
		Factory:       "claude",
		ModelProvider: "openai",
	})
	if err != nil {
		t.Fatalf("ResolveSelection() = %v", err)
	}
	if got.Provider != providers.IDCursor || got.Source != providers.SelectionSourceWorkstation {
		t.Fatalf("ResolveSelection() = %#v, want cursor/workstation", got)
	}

	got, err = providers.ResolveSelection(context.Background(), service, providers.ResolveSelectionRequest{
		Factory:       "claude",
		ModelProvider: "openai",
	})
	if err != nil {
		t.Fatalf("ResolveSelection(factory) = %v", err)
	}
	if got.Provider != providers.IDClaude || got.Source != providers.SelectionSourceFactory {
		t.Fatalf("ResolveSelection(factory) = %#v", got)
	}

	got, err = providers.ResolveSelection(context.Background(), service, providers.ResolveSelectionRequest{})
	if err != nil {
		t.Fatalf("ResolveSelection(default) = %v", err)
	}
	if got.Provider != providers.IDCodex || got.Source != providers.SelectionSourceDefault {
		t.Fatalf("ResolveSelection(default) = %#v", got)
	}
}

func TestSelectionContract_ValidatePrerequisitesUsesCatalogAuthority(t *testing.T) {
	t.Parallel()

	service := newCatalogPeerFake(
		providers.Descriptor{
			ID:           providers.IDCodex,
			DisplayName:  "Codex",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
		providers.Descriptor{
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
	)

	if err := providers.ValidatePrerequisites(
		context.Background(),
		service,
		providers.ValidatePrerequisitesRequest{ID: providers.IDCodex},
	); err != nil {
		t.Fatalf("ValidatePrerequisites(codex) = %v", err)
	}
	err := providers.ValidatePrerequisites(
		context.Background(),
		service,
		providers.ValidatePrerequisitesRequest{ID: providers.IDCursor},
	)
	if !errors.Is(err, providers.ErrProviderUnavailable) {
		t.Fatalf("ValidatePrerequisites(cursor) = %v, want ErrProviderUnavailable", err)
	}
}
