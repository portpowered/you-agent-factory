package http

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestAdapter_ListProvidersInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}
	fake := &rootFake{
		listProviders: func(
			_ context.Context,
			request providers.ListProvidersRequest,
		) (providers.ListProvidersResult, error) {
			invoked = true
			if request != (providers.ListProvidersRequest{}) {
				t.Fatalf("ListProvidersRequest = %#v, want empty request", request)
			}
			return providers.ListProvidersResult{Providers: []providers.Descriptor{codex}}, nil
		},
	}
	adapter := NewAdapter(fake)

	response, err := adapter.ListProviders(context.Background())
	if !invoked {
		t.Fatal("ListProviders did not invoke the injected Providers root")
	}
	if err != nil {
		t.Fatalf("ListProviders error = %v", err)
	}
	if len(response.Providers) != 1 ||
		response.Providers[0].ID != "codex" ||
		response.Providers[0].DisplayName != "Codex" ||
		response.Providers[0].Capabilities[0] != string(providers.CapabilityPromptSubmission) {
		t.Fatalf("response = %#v, want encoded codex descriptor", response)
	}
}

func TestAdapter_GetProviderInvokesFakeRootWithDecodedID(t *testing.T) {
	t.Parallel()

	var invokedID providers.ID
	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}
	fake := &rootFake{
		getProvider: func(
			_ context.Context,
			request providers.GetProviderRequest,
		) (providers.GetProviderResult, error) {
			invokedID = request.ID
			return providers.GetProviderResult{Provider: codex}, nil
		},
	}
	adapter := NewAdapter(fake)

	response, err := adapter.GetProvider(context.Background(), GetProviderInput{ProviderID: "codex"})
	if invokedID != providers.IDCodex {
		t.Fatalf("GetProvider invoked root with id = %q, want codex", invokedID)
	}
	if err != nil {
		t.Fatalf("GetProvider error = %v", err)
	}
	if response.Provider.ID != "codex" || response.Provider.DisplayName != "Codex" {
		t.Fatalf("response = %#v, want encoded codex descriptor", response)
	}
}

func TestAdapter_GetProviderRejectsInvalidIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			t.Fatal("fake root must not be invoked for invalid provider id")
			return providers.GetProviderResult{}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.GetProvider(context.Background(), GetProviderInput{ProviderID: "   "})
	if err == nil || !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("GetProvider error = %v, want ErrInvalidID", err)
	}
}

func TestAdapter_GetProviderMapsUnknownProviderFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{}, providers.ErrUnknownProvider
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.GetProvider(context.Background(), GetProviderInput{ProviderID: "missing"})
	if err == nil || !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("GetProvider error = %v, want ErrUnknownProvider", err)
	}
}

func TestAdapter_GetProviderMapsUnavailableProviderFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{}, providers.ErrProviderUnavailable
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.GetProvider(context.Background(), GetProviderInput{ProviderID: "cursor"})
	if err == nil || !errors.Is(err, providers.ErrProviderUnavailable) {
		t.Fatalf("GetProvider error = %v, want ErrProviderUnavailable", err)
	}
}
