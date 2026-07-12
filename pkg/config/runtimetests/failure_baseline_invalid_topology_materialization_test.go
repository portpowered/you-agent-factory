package runtimetests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// Hermetic failure-baseline fixtures for invalid @you/goal topology on built-in
// named-factory materialization and upgrade resolution paths.

func TestFailureBaseline_InvalidTopology_MaterializedGoalUpgradePathReportsFindingsAndRecovery(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(materialize goal): %v", err)
	}
	corruptGoalFactoryPlanOutputStateForTest(t, resolution.FactoryDir, "missing-plan-state")

	_, err = ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err == nil {
		t.Fatal("expected corrupted materialized goal to fail upgrade resolution")
	}
	wrapped := MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(err, projectRoot, globalRoot, "@you/goal")
	assertInvalidTopologyMaterializationOperatorDiagnostics(t, wrapped, resolution.FactoryDir)
}

func TestFailureBaseline_InvalidTopology_PreExistingInvalidMaterializedTargetReportsFindingsAndRecovery(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	factoryDir := filepath.Join(globalRoot, "@you%2Fgoal")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", factoryDir, err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		[]byte(failureBaselineInvalidGoalTopologyJSON),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	_, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err == nil {
		t.Fatal("expected invalid pre-existing materialized target to fail")
	}
	wrapped := MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(err, projectRoot, globalRoot, "@you/goal")
	assertInvalidTopologyMaterializationOperatorDiagnostics(t, wrapped, factoryDir)
}

func TestFailureBaseline_InvalidTopology_MaterializeNamedFactoryFailureRetainsStructuredFindings(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, "@you%2Fgoal")

	_, err := PersistNamedFactory(rootDir, "@you/goal", []byte(failureBaselineInvalidGoalTopologyJSON))
	if err == nil {
		t.Fatal("expected invalid goal materialization to fail")
	}
	wrapped := MaybeFormatBlockingFactoryLoadOperatorError(err, factoryPath)
	assertInvalidTopologyMaterializationOperatorDiagnostics(t, wrapped, factoryPath)
}

const failureBaselineInvalidGoalTopologyJSON = `{
  "name": "@you/goal",
  "workTypes": [{
    "name": "goal",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "plan", "type": "PROCESSING"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "goal-planner", "type": "AGENT_WORKER"}],
  "workstations": [{
    "name": "plan-goal",
    "type": "AGENT_RUN",
    "worker": "goal-planner",
    "inputs": [{"workType": "goal", "state": "init"}],
    "outputs": [{"workType": "goal", "state": "missing-plan-state"}],
    "onFailure": [{"workType": "goal", "state": "failed"}]
  }]
}`

func assertInvalidTopologyMaterializationOperatorDiagnostics(t *testing.T, err error, wantFactoryDir string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected invalid topology materialization or upgrade failure")
	}
	got := err.Error()
	if !strings.Contains(got, "invalid graph references") {
		t.Fatalf("error = %q, want invalid graph references guidance", got)
	}
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
	if strings.Contains(got, "blocking validation targets") {
		t.Fatalf("error = %q, want findings instead of target count summary", got)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("error = %q, want exactly one recovery command", got)
	}
	wantFactoryDir = strings.TrimSpace(wantFactoryDir)
	if wantFactoryDir == "" {
		t.Fatal("wantFactoryDir is required")
	}
	recovery := FactoryConfigValidateRecoveryCommand(wantFactoryDir)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	if !strings.Contains(got, "@you%2Fgoal") {
		t.Fatalf("error = %q, want resolved @you%%2Fgoal factory path", got)
	}
}

func corruptGoalFactoryPlanOutputStateForTest(t *testing.T, factoryDir, stateName string) {
	t.Helper()

	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	workstations, ok := raw["workstations"].([]any)
	if !ok || len(workstations) == 0 {
		t.Fatal("factory.json workstations missing")
	}
	var workstation map[string]any
	for _, entry := range workstations {
		candidate, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if candidate["name"] == "plan-goal" {
			workstation = candidate
			break
		}
	}
	if workstation == nil {
		t.Fatal("factory.json plan-goal workstation not found")
	}
	outputs, ok := workstation["outputs"].([]any)
	if !ok || len(outputs) == 0 {
		t.Fatal("factory.json workstation outputs missing")
	}
	output, ok := outputs[0].(map[string]any)
	if !ok {
		t.Fatal("factory.json workstation output[0] is not an object")
	}
	output["state"] = stateName
	updated, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(factory.json): %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
}
