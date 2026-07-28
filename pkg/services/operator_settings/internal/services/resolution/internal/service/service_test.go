package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/internal/service"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func newResolutionService(t *testing.T) resolution.Service {
	t.Helper()
	service, err := internalservice.New(mustProvidersRoot(t))
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return service
}

func mustProvidersRoot(t *testing.T) providers.Service {
	t.Helper()
	return testproviders.StandardCatalog()
}

type resolutionProvidersFake struct {
	providers map[providers.ID]providers.Descriptor
}

var _ providers.Service = (*resolutionProvidersFake)(nil)

func newResolutionProvidersFake(entries ...providers.Descriptor) *resolutionProvidersFake {
	catalog := make(map[providers.ID]providers.Descriptor, len(entries))
	for _, entry := range entries {
		catalog[entry.ID] = entry.Clone()
	}
	return &resolutionProvidersFake{providers: catalog}
}

func (fake *resolutionProvidersFake) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	results := make([]providers.Descriptor, 0, len(fake.providers))
	for _, descriptor := range fake.providers {
		results = append(results, descriptor.Clone())
	}
	return providers.ListProvidersResult{Providers: results}, nil
}

func (fake *resolutionProvidersFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	descriptor, ok := fake.lookup(request.ID)
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	if descriptor.Availability != providers.AvailabilitySelectable ||
		descriptor.Readiness != providers.ReadinessReady {
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	}
	return providers.GetProviderResult{Provider: descriptor.Clone()}, nil
}

func (fake *resolutionProvidersFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, errors.New("not implemented")
}

func (fake *resolutionProvidersFake) lookup(id providers.ID) (providers.Descriptor, bool) {
	if descriptor, ok := fake.providers[id]; ok {
		return descriptor, true
	}
	normalized := strings.ToLower(strings.TrimSpace(id.String()))
	for _, descriptor := range fake.providers {
		if strings.ToLower(descriptor.ID.String()) == normalized {
			return descriptor, true
		}
		for _, alias := range descriptor.Aliases {
			if alias == normalized {
				return descriptor, true
			}
		}
	}
	return providers.Descriptor{}, false
}

func TestResolveEffective_AppliesFlagPrecedence(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	configPath := "/home/operator/.you-agent-factory/config.json"
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}

	selection := resolved.Selection
	if selection.WorkerModelProvider != "GEMINI" {
		t.Fatalf("provider = %q, want GEMINI", selection.WorkerModelProvider)
	}
	if selection.WorkerModel != "flag-model" {
		t.Fatalf("model = %q, want flag-model", selection.WorkerModel)
	}
	if selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("provider source = %q, want flag", selection.WorkerModelProviderSource)
	}
	if selection.WorkerModelSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("model source = %q, want flag", selection.WorkerModelSource)
	}
	if selection.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", selection.ConfigPath, configPath)
	}
}

func TestResolveEffective_EnvOverridesFileWhenFlagsUnset(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}

	selection := resolved.Selection
	if selection.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", selection.WorkerModelProvider)
	}
	if selection.WorkerModel != "env-model" {
		t.Fatalf("model = %q, want env-model", selection.WorkerModel)
	}
	if selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceEnv {
		t.Fatalf("provider source = %q, want env", selection.WorkerModelProviderSource)
	}
	if selection.WorkerModelSource != operatorsettings.EffectiveLayerSourceEnv {
		t.Fatalf("model source = %q, want env", selection.WorkerModelSource)
	}
}

func TestResolveEffective_PrecedenceIsIndependentPerField(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}

	selection := resolved.Selection
	if selection.WorkerModelProvider != "GEMINI" {
		t.Fatalf("provider = %q, want GEMINI", selection.WorkerModelProvider)
	}
	if selection.WorkerModel != "env-model" {
		t.Fatalf("model = %q, want env-model", selection.WorkerModel)
	}
	if selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("provider source = %q, want flag", selection.WorkerModelProviderSource)
	}
	if selection.WorkerModelSource != operatorsettings.EffectiveLayerSourceEnv {
		t.Fatalf("model source = %q, want env", selection.WorkerModelSource)
	}
}

func TestResolveEffective_IncludesBackendScopeFromDocumentBaseline(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	scopeID := "local-11111111-1111-4111-8111-111111111111"
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		BackendScopeID: scopeID,
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "file-model",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.BackendScopeID != scopeID {
		t.Fatalf("backend scope = %q, want %q", resolved.Selection.BackendScopeID, scopeID)
	}
}

func TestResolveEffective_PresetInfluencesInvocationLayerWhenUnset(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	presets := []operatorsettings.DocumentWorkerPreset{{
		ID:            "careful-review",
		ModelProvider: "codex",
		Model:         "preset-model",
		ReasoningEffort: "high",
	}}
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		WorkerPresets: presets,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerPresetID: "careful-review",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}

	selection := resolved.Selection
	if selection.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", selection.WorkerModelProvider)
	}
	if selection.WorkerModel != "preset-model" {
		t.Fatalf("model = %q, want preset-model", selection.WorkerModel)
	}
	if selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("provider source = %q, want flag", selection.WorkerModelProviderSource)
	}
	if selection.WorkerModelSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("model source = %q, want flag", selection.WorkerModelSource)
	}
	if !reflect.DeepEqual(selection.WorkerPresets, presets) {
		t.Fatalf("worker presets = %#v, want %#v", selection.WorkerPresets, presets)
	}
}

func TestResolveEffective_ExplicitInvocationOverridesWinOverPreset(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		WorkerPresets: []operatorsettings.DocumentWorkerPreset{{
			ID:            "careful-review",
			ModelProvider: "codex",
			Model:         "preset-model",
		}},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerPresetID:      "careful-review",
			WorkerModelProvider: "gemini",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}

	selection := resolved.Selection
	if selection.WorkerModelProvider != "GEMINI" {
		t.Fatalf("provider = %q, want GEMINI", selection.WorkerModelProvider)
	}
	if selection.WorkerModel != "preset-model" {
		t.Fatalf("model = %q, want preset-model from preset", selection.WorkerModel)
	}
}

func TestResolveEffective_EquivalentInputsProduceIdenticalSelections(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	request := operatorsettings.ResolveEffectiveRequest{
		BackendScopeID: "local-22222222-2222-4222-8222-222222222222",
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "codex",
		},
		WorkerPresets: []operatorsettings.DocumentWorkerPreset{{
			ID:            "research",
			ModelProvider: "CODEX",
			Model:         "preset-model",
		}},
		ConfigPath: "/tmp/config.json",
	}

	first, err := service.ResolveEffective(request)
	if err != nil {
		t.Fatalf("first ResolveEffective() = %v", err)
	}
	second, err := service.ResolveEffective(request)
	if err != nil {
		t.Fatalf("second ResolveEffective() = %v", err)
	}
	if !reflect.DeepEqual(first.Selection, second.Selection) {
		t.Fatalf("selections differ: first = %#v, second = %#v", first.Selection, second.Selection)
	}
}

func TestResolveEffective_ConstructionIsInert(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New(mustProvidersRoot(t))
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	if service == nil {
		t.Fatal("constructed resolution service is nil")
	}
}

func TestResolveEffective_CanonicalizesProviderAliasThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "cursor",
			WorkerModel:         "file-model",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "CURSOR" {
		t.Fatalf("provider = %q, want CURSOR", resolved.Selection.WorkerModelProvider)
	}
}

func TestResolveEffective_UnavailableProviderDoesNotMutateDocumentBaseline(t *testing.T) {
	t.Parallel()

	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "file-model",
	}
	request := operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "codex",
		},
		ConfigPath: "/tmp/config.json",
	}

	service, err := internalservice.New(newResolutionProvidersFake(providers.Descriptor{
		ID:           providers.IDCodex,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessUnavailable,
	}))
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	_, err = service.ResolveEffective(request)
	if !errors.Is(err, operatorsettings.ErrResolutionConflict) {
		t.Fatalf("ResolveEffective() error = %v, want ErrResolutionConflict", err)
	}
	if request.DocumentBaseline != baseline {
		t.Fatalf("document baseline mutated: got %#v, want %#v", request.DocumentBaseline, baseline)
	}
}

func TestResolveEffective_UnknownProviderSurfacesUnsupportedOverride(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	_, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "not-a-real-provider",
			WorkerModel:         "file-model",
		},
		ConfigPath: "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("ResolveEffective() error = %v, want ErrResolutionUnsupportedOverride", err)
	}
}

func TestResolveEffective_SymbolicDefaultResolvesThroughLowerPrecedenceProvider(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "file-model",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "DEFAULT",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", resolved.Selection.WorkerModelProvider)
	}
	if resolved.Selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("provider source = %q, want flag", resolved.Selection.WorkerModelProviderSource)
	}
}

func TestResolveEffective_UnresolvedSymbolicDefaultReturnsInvalidInput(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)
	_, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "DEFAULT",
		},
		ConfigPath: "/tmp/config.json",
	})
	assertResolutionFailure(
		t,
		err,
		operatorsettings.ErrResolutionInvalidInput,
		operatorsettings.ResolutionFailureKindInvalidInput,
		"workerModelProvider",
	)
	if errors.Is(err, operatorsettings.ErrDocumentMalformed) ||
		errors.Is(err, operatorsettings.ErrDocumentConflict) {
		t.Fatalf("unresolved DEFAULT leaked document failure: %v", err)
	}
}

func TestResolveEffective_TypedResolutionFailures(t *testing.T) {
	t.Parallel()

	service := newResolutionService(t)

	_, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: "/tmp/config.json",
	})
	assertResolutionFailure(
		t,
		err,
		operatorsettings.ErrResolutionUnsupportedOverride,
		operatorsettings.ResolutionFailureKindUnsupportedOverride,
		"workerModelProvider",
	)
	if errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("unsupported override leaked providers error: %v", err)
	}

	_, err = service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		WorkerPresets: []operatorsettings.DocumentWorkerPreset{{
			ID:            "research",
			ModelProvider: "codex",
		}},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerPresetID: "missing-preset",
		},
		ConfigPath: "/tmp/config.json",
	})
	assertResolutionFailure(
		t,
		err,
		operatorsettings.ErrResolutionUnsupportedOverride,
		operatorsettings.ResolutionFailureKindUnsupportedOverride,
		"workerPresetID",
	)

	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	staleBaseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "claude",
		WorkerModel:         "gpt-5",
	}
	_, err = service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: staleBaseline,
		ExpectedDocumentBaseline: &operatorsettings.DocumentDefaults{
			WorkerModelProvider: baseline.WorkerModelProvider,
			WorkerModel:         baseline.WorkerModel,
		},
		ConfigPath: "/tmp/config.json",
	})
	assertResolutionFailure(
		t,
		err,
		operatorsettings.ErrResolutionConflict,
		operatorsettings.ResolutionFailureKindConflict,
		"documentBaseline",
	)
}

func TestResolveEffective_FailurePathsDoNotMutateDocumentBaseline(t *testing.T) {
	t.Parallel()

	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "file-model",
	}
	request := operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "not-a-real-provider",
		},
		ConfigPath: "/tmp/config.json",
	}

	service := newResolutionService(t)
	_, err := service.ResolveEffective(request)
	if err == nil {
		t.Fatal("ResolveEffective() succeeded, want unsupported provider failure")
	}
	if request.DocumentBaseline != baseline {
		t.Fatalf("document baseline mutated after unsupported provider: got %#v, want %#v", request.DocumentBaseline, baseline)
	}
}

func TestResolveEffective_UnexpectedProviderErrorDoesNotLeakProvidersType(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New(&unexpectedProviderErrorFake{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	_, err = service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "file-model",
		},
		ConfigPath: "/tmp/config.json",
	})
	assertResolutionFailure(
		t,
		err,
		operatorsettings.ErrResolutionConflict,
		operatorsettings.ResolutionFailureKindConflict,
		"workerModelProvider",
	)
	if errors.Is(err, providers.ErrUnknownProvider) ||
		errors.Is(err, providers.ErrProviderUnavailable) {
		t.Fatalf("unexpected provider error leaked providers sentinel: %v", err)
	}
}

type unexpectedProviderErrorFake struct{}

var _ providers.Service = (*unexpectedProviderErrorFake)(nil)

func (fake *unexpectedProviderErrorFake) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, errors.New("unexpected list failure")
}

func (fake *unexpectedProviderErrorFake) GetProvider(
	_ context.Context,
	_ providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, errors.New("unexpected catalog failure")
}

func (fake *unexpectedProviderErrorFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, errors.New("not implemented")
}

func assertResolutionFailure(
	t *testing.T,
	err error,
	sentinel error,
	kind operatorsettings.ResolutionFailureKind,
	field string,
) {
	t.Helper()
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
	var failure operatorsettings.ResolutionFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T(%v), want operatorsettings.ResolutionFailure", err, err)
	}
	if failure.Kind != kind {
		t.Fatalf("failure kind = %q, want %q", failure.Kind, kind)
	}
	if failure.Field != field {
		t.Fatalf("failure field = %q, want %q", failure.Field, field)
	}
}
