package goal

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuiltInFactoryJSON_LoadsRunnablePackagedGoalFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	if cfg.Name != "@you/goal" {
		t.Fatalf("factory name = %q, want @you/goal", cfg.Name)
	}
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].HandlingBehavior[0] != interfaces.WorkTypeHandlingBehaviorDefault {
		t.Fatalf("workTypes = %#v, want one DEFAULT handling work type", cfg.WorkTypes)
	}
	if len(cfg.Workstations) != 1 || cfg.Workstations[0].Name != PackagedInvokeWorkstationName {
		t.Fatalf("workstations = %#v, want packaged goal workstation", cfg.Workstations)
	}
}
