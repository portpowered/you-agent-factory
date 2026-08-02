package service_test

import (
	"context"
	"errors"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/internal/service"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type recordingProvidersBoundaryFake struct {
	providers.Service
	getProviderRequests []providers.GetProviderRequest
	providers           map[providers.ID]providers.Descriptor
	getProviderErrors   map[providers.ID]error
}

var _ providers.Service = (*recordingProvidersBoundaryFake)(nil)

func newRecordingProvidersBoundaryFake(
	entries ...providers.Descriptor,
) *recordingProvidersBoundaryFake {
	catalog := make(map[providers.ID]providers.Descriptor, len(entries))
	for _, entry := range entries {
		catalog[entry.ID] = entry.Clone()
	}
	return &recordingProvidersBoundaryFake{
		providers:         catalog,
		getProviderErrors: make(map[providers.ID]error),
	}
}

func (fake *recordingProvidersBoundaryFake) withGetProviderError(
	id providers.ID,
	err error,
) *recordingProvidersBoundaryFake {
	fake.getProviderErrors[id] = err
	return fake
}

func (fake *recordingProvidersBoundaryFake) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (fake *recordingProvidersBoundaryFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	fake.getProviderRequests = append(fake.getProviderRequests, request)
	if err, ok := fake.getProviderErrors[request.ID]; ok {
		return providers.GetProviderResult{}, err
	}
	descriptor, ok := fake.providers[request.ID]
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	if descriptor.Availability != providers.AvailabilitySelectable ||
		descriptor.Readiness != providers.ReadinessReady {
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	}
	return providers.GetProviderResult{Provider: descriptor.Clone()}, nil
}

func (fake *recordingProvidersBoundaryFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, errors.New("not implemented")
}

func newResolutionServiceWithProviders(
	t *testing.T,
	root providers.Service,
) resolution.Service {
	t.Helper()
	service, err := internalservice.New(root)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return service
}

// TestResolveEffective_IssuesGetProviderRequestForConcreteProvider seals the
// Settings→Providers consumer edge: effective resolution canonicalizes concrete
// provider identities by issuing a Providers root GetProvider request.
func TestResolveEffective_IssuesGetProviderRequestForConcreteProvider(t *testing.T) {
	t.Parallel()

	providersRoot := newRecordingProvidersBoundaryFake(providers.Descriptor{
		ID:           providers.IDCodex,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	})
	service := newResolutionServiceWithProviders(t, providersRoot)

	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "  codex  ",
			WorkerModel:         "gpt-5",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if len(providersRoot.getProviderRequests) != 1 {
		t.Fatalf("GetProvider calls = %d, want 1", len(providersRoot.getProviderRequests))
	}
	gotRequest := providersRoot.getProviderRequests[0]
	if gotRequest.ID != providers.ID("codex") {
		t.Fatalf("GetProviderRequest.ID = %q, want codex", gotRequest.ID)
	}
	if resolved.Selection.WorkerModelProvider != "CODEX" {
		t.Fatalf("resolved provider = %q, want CODEX", resolved.Selection.WorkerModelProvider)
	}
}

// TestResolveEffective_ProviderCatalogFailuresMapToSettingsResolutionKinds
// proves unknown, invalid, and unavailable Providers catalog outcomes still map
// to the existing Settings-observable resolution failure kinds.
func TestResolveEffective_ProviderCatalogFailuresMapToSettingsResolutionKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		root     *recordingProvidersBoundaryFake
		sentinel error
		kind     operatorsettings.ResolutionFailureKind
	}{
		{
			name:     "unknown provider",
			provider: "missing-provider",
			root:     newRecordingProvidersBoundaryFake(),
			sentinel: operatorsettings.ErrResolutionUnsupportedOverride,
			kind:     operatorsettings.ResolutionFailureKindUnsupportedOverride,
		},
		{
			name:     "invalid provider id",
			provider: "invalid-provider",
			root: newRecordingProvidersBoundaryFake().withGetProviderError(
				providers.ID("invalid-provider"),
				providers.ErrInvalidID,
			),
			sentinel: operatorsettings.ErrResolutionUnsupportedOverride,
			kind:     operatorsettings.ResolutionFailureKindUnsupportedOverride,
		},
		{
			name:     "unavailable provider",
			provider: "codex",
			root: newRecordingProvidersBoundaryFake(providers.Descriptor{
				ID:           providers.IDCodex,
				Availability: providers.AvailabilitySelectable,
				Readiness:    providers.ReadinessUnavailable,
			}),
			sentinel: operatorsettings.ErrResolutionConflict,
			kind:     operatorsettings.ResolutionFailureKindConflict,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := newResolutionServiceWithProviders(t, test.root)
			_, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
				DocumentBaseline: operatorsettings.DocumentDefaults{
					WorkerModelProvider: test.provider,
					WorkerModel:         "gpt-5",
				},
				ConfigPath: "/tmp/config.json",
			})
			assertResolutionFailure(
				t,
				err,
				test.sentinel,
				test.kind,
				"workerModelProvider",
			)
			if len(test.root.getProviderRequests) != 1 {
				t.Fatalf("GetProvider calls = %d, want 1", len(test.root.getProviderRequests))
			}
			if test.root.getProviderRequests[0].ID != providers.ID(test.provider) {
				t.Fatalf(
					"GetProviderRequest.ID = %q, want %q",
					test.root.getProviderRequests[0].ID,
					test.provider,
				)
			}
			if errors.Is(err, providers.ErrUnknownProvider) ||
				errors.Is(err, providers.ErrInvalidID) ||
				errors.Is(err, providers.ErrProviderUnavailable) {
				t.Fatalf("providers catalog error leaked across boundary: %v", err)
			}
		})
	}
}
