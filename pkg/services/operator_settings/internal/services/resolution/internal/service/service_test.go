package service_test

import (
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/internal/service"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

func newResolutionService(t *testing.T) resolution.Service {
	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return service
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

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	if service == nil {
		t.Fatal("constructed resolution service is nil")
	}
}
