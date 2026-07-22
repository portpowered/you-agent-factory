package factoryrun

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestResolveFactoryRootFromConfigFile_ReturnsParentDirectory(t *testing.T) {
	called := false
	resolve := interfaces.FactoryConfigRootResolver(func(path string) (string, error) {
		called = true
		if path != "factory.json" {
			t.Fatalf("path = %q", path)
		}
		return "factory-root", nil
	})
	got, err := resolve("factory.json")
	if err != nil {
		t.Fatalf("ResolveFactoryRootFromConfigFile: %v", err)
	}
	if !called || got != "factory-root" {
		t.Fatalf("result = (%q, %t)", got, called)
	}
}

func TestResolveFactoryRootFromConfigFile_RequiresOperation(t *testing.T) {
	var resolve interfaces.FactoryConfigRootResolver
	if resolve != nil {
		t.Fatal("nil resolver role unexpectedly callable")
	}
}

func TestLoadFactoryConfigFromConfigFile_LoadsExpandedConfig(t *testing.T) {
	load := interfaces.FactoryConfigFileLoader(func(path string) (*interfaces.FactoryConfig, error) {
		if path != "factory.json" {
			t.Fatalf("path = %q", path)
		}
		return &interfaces.FactoryConfig{WorkTypes: []interfaces.WorkTypeConfig{{Name: "story"}}}, nil
	})
	cfg, err := load("factory.json")
	if err != nil {
		t.Fatalf("LoadFactoryConfigFromConfigFile: %v", err)
	}
	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].Name != "story" {
		t.Fatalf("work types = %#v, want one story work type", cfg.WorkTypes)
	}
}

func TestValidateFactoryForPromptRun_RequiresDefaultHandlingWorkType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "story", States: testStoryStates()}},
	}

	result, err := promptRunValidationFailureValidator{}.ValidateEffectiveDefinition(
		t.Context(),
		interfaces.EffectiveDefinitionValidationRequest{Config: cfg},
	)
	if err != nil {
		t.Fatalf("ValidateEffectiveDefinition: %v", err)
	}
	if len(result.Targets) != 1 || !strings.Contains(result.Targets[0].Message, "handlingBehavior DEFAULT") {
		t.Fatalf("result = %#v, want DEFAULT handling guidance", result)
	}
}

func testStoryStates() []interfaces.StateConfig {
	return []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
		{Name: "complete", Type: interfaces.StateTypeTerminal},
	}
}
