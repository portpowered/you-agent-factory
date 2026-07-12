package subsystems_test

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
)

func TestBuiltInGoalFactoryJSON_ExecuteRepeaterConsumesInitAndExecuteInputs(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	execute, ok := findPackagedGoalWorkstation(cfg.Workstations, goal.PackagedExecuteWorkstationName)
	if !ok {
		t.Fatalf("missing workstation %q", goal.PackagedExecuteWorkstationName)
	}
	if execute.Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("execute workstation kind = %q, want %q", execute.Kind, interfaces.WorkstationKindRepeater)
	}

	wantInputStates := map[string]bool{"init": true, "execute": true}
	gotInputStates := make(map[string]bool, len(execute.Inputs))
	for _, input := range execute.Inputs {
		if input.WorkTypeName != goal.PackagedGoalWorkTypeName {
			t.Fatalf("input work type = %q, want %s", input.WorkTypeName, goal.PackagedGoalWorkTypeName)
		}
		gotInputStates[input.StateName] = true
	}
	for state := range wantInputStates {
		if !gotInputStates[state] {
			t.Fatalf("execute inputs = %#v, want loop entry states init and execute", execute.Inputs)
		}
	}
}

func findPackagedGoalWorkstation(workstations []interfaces.FactoryWorkstationConfig, name string) (interfaces.FactoryWorkstationConfig, bool) {
	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation, true
		}
	}
	return interfaces.FactoryWorkstationConfig{}, false
}
