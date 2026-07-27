package service_test

import (
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
)

func TestRootDelegatesResolveEffectiveToPrivateOwner(t *testing.T) {
	t.Parallel()

	resolutionService, err := resolutionwire.NewService()
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}
	root, err := operatorservice.New(resolutionService)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	configPath := "/home/operator/.you-agent-factory/config.json"
	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" ||
		resolved.Selection.WorkerModel != "flag-model" ||
		resolved.Selection.ConfigPath != configPath {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}
}

func TestNew_RejectsNilResolution(t *testing.T) {
	t.Parallel()

	service, err := operatorservice.New(nil)
	if err == nil || service != nil {
		t.Fatalf("New(nil) = (%v, %v), want error", service, err)
	}
}
