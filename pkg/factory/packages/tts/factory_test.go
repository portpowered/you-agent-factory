package tts

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestBuiltInFactoryJSON_LoadsRunnablePackagedTTSFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	if cfg.Name != "@you/tts" {
		t.Fatalf("factory name = %q, want @you/tts", cfg.Name)
	}
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].HandlingBehavior[0] != interfaces.WorkTypeHandlingBehaviorDefault {
		t.Fatalf("workTypes = %#v, want one DEFAULT handling work type", cfg.WorkTypes)
	}
	if len(cfg.Workstations) != 1 || cfg.Workstations[0].Name != PackagedInvokeWorkstationName {
		t.Fatalf("workstations = %#v, want packaged invoke workstation", cfg.Workstations)
	}
}
