package builtingoal_test

import (
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	builtingoal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/goal"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

// Hermetic S02 failure-baseline fixtures for invalid @you/goal factory topology
// on built-in goal config surfaces.

func TestFailureBaseline_InvalidTopology_BuiltInGoalWorkstationRejectsDanglingOutputRoute(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	mutateBuiltInGoalPlanWorkstationOutputState(t, cfg, "missing-plan-state")

	findings := factoryvalidation.New(nil).
		ValidateTopology(t.Context(), cfg, nil).
		Findings
	if !containsCanonicalFindingCode(findings, factoryvalidation.CodeDanglingPlaceReference) {
		t.Fatalf("findings = %#v, want dangling place reference", findings)
	}
	if !containsCanonicalFindingPath(findings, "factory.workstations[0].outputs[0]") {
		t.Fatalf("findings = %#v, want execute-goal output path", findings)
	}
}

func mutateBuiltInGoalPlanWorkstationOutputState(t *testing.T, cfg *interfaces.FactoryConfig, stateName string) {
	t.Helper()
	for workstationIndex := range cfg.Workstations {
		workstation := &cfg.Workstations[workstationIndex]
		if workstation.Name != "execute-goal" {
			continue
		}
		if len(workstation.Outputs) == 0 {
			t.Fatal("built-in goal execute-goal workstation outputs not found")
		}
		workstation.Outputs[0].StateName = stateName
		return
	}
	t.Fatal("built-in goal execute-goal workstation not found")
}

func containsCanonicalFindingCode(findings []interfaces.TopologyFinding, code string) bool {
	for _, finding := range findings {
		if finding.Rule == code {
			return true
		}
	}
	return false
}

func containsCanonicalFindingPath(findings []interfaces.TopologyFinding, path string) bool {
	for _, finding := range findings {
		if strings.Contains(finding.Path, path) {
			return true
		}
	}
	return false
}
