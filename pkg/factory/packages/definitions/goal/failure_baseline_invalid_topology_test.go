package builtingoal_test

import (
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	builtingoal "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/goal"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

// Hermetic S02 failure-baseline fixtures for invalid @you/goal factory topology
// on built-in goal config surfaces.

func TestFailureBaseline_InvalidTopology_BuiltInGoalWorkstationRejectsDanglingOutputRoute(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	mutateBuiltInGoalPlanWorkstationOutputState(t, cfg, "missing-plan-state")

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
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

func containsCanonicalFindingCode(findings []factoryconfig.Finding, code string) bool {
	for _, finding := range findings {
		if finding.Rule == code {
			return true
		}
	}
	return false
}

func containsCanonicalFindingPath(findings []factoryconfig.Finding, path string) bool {
	for _, finding := range findings {
		if strings.Contains(finding.Path, path) {
			return true
		}
	}
	return false
}
