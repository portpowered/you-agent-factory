package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestRootSelectionMethodsPreserveAliasPrecedenceAndPrerequisites(t *testing.T) {
	t.Parallel()

	root := mustSelectionRoot(t,
		providers.Descriptor{
			ID:           providers.IDCodex,
			Aliases:      []string{"openai-codex"},
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
		providers.Descriptor{
			ID:           providers.IDClaude,
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		},
		providers.Descriptor{
			ID:           providers.IDCursor,
			Availability: providers.AvailabilitySupportedButUnavailable,
			Readiness:    providers.ReadinessUnavailable,
			Prerequisites: []providers.Prerequisite{{
				Kind:   providers.PrerequisiteDependency,
				Name:   "cursor-agent",
				Status: providers.PrerequisiteMissing,
			}},
		},
	)

	identity, err := root.ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{
		Identity: " OPENAI-CODEX ",
	})
	if err != nil || identity.ID != providers.IDCodex {
		t.Fatalf("ResolveIdentity() = (%#v, %v), want codex", identity, err)
	}

	selection, err := root.ResolveSelection(context.Background(), providers.ResolveSelectionRequest{
		Factory:       " claude ",
		ModelProvider: "${MODEL_PROVIDER}",
	})
	if err != nil || selection.Provider != providers.IDClaude || selection.Source != providers.SelectionSourceFactory {
		t.Fatalf("ResolveSelection() = (%#v, %v), want claude/factory", selection, err)
	}

	selection, err = root.ResolveSelection(context.Background(), providers.ResolveSelectionRequest{})
	if err != nil || selection.Provider != providers.IDCodex || selection.Source != providers.SelectionSourceDefault {
		t.Fatalf("ResolveSelection(default) = (%#v, %v), want codex/default", selection, err)
	}

	if err := root.ValidatePrerequisites(context.Background(), providers.ValidatePrerequisitesRequest{ID: providers.IDCodex}); err != nil {
		t.Fatalf("ValidatePrerequisites(codex) = %v", err)
	}
	if err := root.ValidatePrerequisites(context.Background(), providers.ValidatePrerequisitesRequest{ID: providers.IDCursor}); !errors.Is(err, providers.ErrProviderUnavailable) {
		t.Fatalf("ValidatePrerequisites(cursor) = %v, want ErrProviderUnavailable", err)
	}
}

func TestRootSelectionMethodsPreserveValidationContextAndDefaultErrors(t *testing.T) {
	t.Parallel()

	root := mustSelectionRoot(t, providers.Descriptor{
		ID:           providers.IDClaude,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := root.ResolveIdentity(canceled, providers.ResolveIdentityRequest{Identity: "claude"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveIdentity(canceled) = %v, want context.Canceled", err)
	}
	if _, err := root.ResolveSelection(canceled, providers.ResolveSelectionRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveSelection(canceled) = %v, want context.Canceled", err)
	}
	if err := root.ValidatePrerequisites(canceled, providers.ValidatePrerequisitesRequest{ID: providers.IDClaude}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidatePrerequisites(canceled) = %v, want context.Canceled", err)
	}

	if _, err := root.ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{}); !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("ResolveIdentity(empty) = %v, want ErrInvalidID", err)
	}
	if _, err := root.ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{Identity: "unknown"}); !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("ResolveIdentity(unknown) = %v, want ErrUnknownProvider", err)
	}
	if _, err := root.ResolveSelection(context.Background(), providers.ResolveSelectionRequest{}); !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("ResolveSelection(default without codex) = %v, want ErrUnknownProvider", err)
	} else if !containsErrorText(err, "resolve default provider") {
		t.Fatalf("ResolveSelection(default without codex) = %v, want default context", err)
	}
	if err := root.ValidatePrerequisites(context.Background(), providers.ValidatePrerequisitesRequest{ID: providers.ID("unknown")}); !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("ValidatePrerequisites(unknown) = %v, want ErrUnknownProvider", err)
	}
}

func containsErrorText(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}

func mustSelectionRoot(t *testing.T, descriptors ...providers.Descriptor) providers.Service {
	t.Helper()

	catalog := &selectionCatalogStub{descriptors: descriptors}
	root, err := New(catalog, selectionExecutionStub{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return root
}

type selectionCatalogStub struct {
	descriptors []providers.Descriptor
}

func (stub *selectionCatalogStub) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	result := make([]providers.Descriptor, len(stub.descriptors))
	for index, descriptor := range stub.descriptors {
		result[index] = descriptor.Clone()
	}
	return providers.ListProvidersResult{Providers: result}, nil
}

func (stub *selectionCatalogStub) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	for _, descriptor := range stub.descriptors {
		if descriptor.ID != request.ID {
			continue
		}
		if descriptor.Availability != providers.AvailabilitySelectable ||
			descriptor.Readiness != providers.ReadinessReady ||
			missingSelectionPrerequisite(descriptor.Prerequisites) {
			return providers.GetProviderResult{}, providers.ErrProviderUnavailable
		}
		return providers.GetProviderResult{Provider: descriptor.Clone()}, nil
	}
	return providers.GetProviderResult{}, providers.ErrUnknownProvider
}

func (stub *selectionCatalogStub) ResolveProviderID(id providers.ID) (providers.ID, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	for _, descriptor := range stub.descriptors {
		if descriptor.ID == id {
			return id, nil
		}
	}
	return "", providers.ErrUnknownProvider
}

func (stub *selectionCatalogStub) RegistrationProvider(id providers.ID) (providers.Descriptor, error) {
	for _, descriptor := range stub.descriptors {
		if descriptor.ID == id {
			return descriptor.Clone(), nil
		}
	}
	return providers.Descriptor{}, providers.ErrUnknownProvider
}

func missingSelectionPrerequisite(prerequisites []providers.Prerequisite) bool {
	for _, prerequisite := range prerequisites {
		if prerequisite.Status == providers.PrerequisiteMissing {
			return true
		}
	}
	return false
}

type selectionExecutionStub struct{}

func (selectionExecutionStub) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}
