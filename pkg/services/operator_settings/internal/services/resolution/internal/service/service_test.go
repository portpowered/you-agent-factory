package service_test

import (
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/internal/service"
)

func TestResolveEffective_AppliesFlagPrecedence(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

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
