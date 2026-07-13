package blockingload_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/blockingload"
	"github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/goal"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBlockingFactoryLoadError_PreservesCanonicalTargetsOnCanonicalJSONLoad(t *testing.T) {
	_, err := load.LoadFromCanonicalJSON([]byte(factoryvalidation.CrossPathInvalidFactoryJSON), load.LoadOptions{})
	if err == nil {
		t.Fatal("expected cross-path invalid factory to fail load")
	}
	if !load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, did not want os.ErrNotExist", err)
	}

	loadErr, ok := load.AsBlockingFactoryLoadError(err)
	if !ok {
		t.Fatalf("error = %v, want BlockingFactoryLoadError", err)
	}
	if len(loadErr.Targets) == 0 {
		t.Fatal("expected non-empty blocking validation targets")
	}
	for _, target := range loadErr.Targets {
		if strings.TrimSpace(target.Message) == "" {
			t.Fatalf("target = %#v, want non-empty message", target)
		}
		if strings.TrimSpace(target.Code) == "" {
			t.Fatalf("target = %#v, want non-empty code", target)
		}
	}

	findings := load.BlockingFactoryLoadFindings(err)
	if len(findings) != len(loadErr.Targets) {
		t.Fatalf("findings = %d, targets = %d, want equivalent coverage", len(findings), len(loadErr.Targets))
	}
}

func TestBlockingFactoryLoadError_PreservesTargetsThroughNamedFactoryMaterialization(t *testing.T) {
	rootDir := t.TempDir()

	_, err := factoryconfig.PersistNamedFactory(rootDir, "@you/goal", []byte(factoryvalidation.CrossPathInvalidFactoryJSON))
	if err == nil {
		t.Fatal("expected invalid named factory materialization to fail")
	}
	if !errors.Is(err, factoryconfig.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, did not want os.ErrNotExist", err)
	}

	loadErr, ok := factoryconfig.AsBlockingFactoryLoadError(err)
	if !ok {
		t.Fatalf("error = %v, want BlockingFactoryLoadError through materialization", err)
	}
	if len(loadErr.Targets) == 0 {
		t.Fatal("expected non-empty blocking validation targets")
	}
}

func TestBlockingFactoryLoadError_DistinguishesIOErrorsFromValidationFailures(t *testing.T) {
	missingDir := t.TempDir()
	_ = os.Remove(missingDir)

	_, err := load.LoadFromFactoryDir(missingDir, nil)
	if err == nil {
		t.Fatal("expected missing factory directory to fail")
	}
	if load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, did not want ErrInvalidNamedFactory", err)
	}
	if _, ok := load.AsBlockingFactoryLoadError(err); ok {
		t.Fatalf("error = %v, did not want BlockingFactoryLoadError", err)
	}
}

func TestFactoryConfigValidateRecoveryCommand_UsesSingleValidatePath(t *testing.T) {
	factoryPath, err := namedfactorypath.MapDir("global-root", "@you/goal")
	if err != nil {
		t.Fatalf("MapDir: %v", err)
	}
	got := blockingload.FactoryConfigValidateRecoveryCommand(factoryPath)
	normalized := strings.ReplaceAll(got, "\\", "/")
	if !strings.Contains(normalized, "@you/goal") {
		t.Fatalf("recovery command = %q, want @you/goal path", got)
	}
	if !strings.HasPrefix(got, "you factory config validate ") {
		t.Fatalf("recovery command = %q, want validate prefix", got)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("recovery command = %q, want exactly one validate command", got)
	}
}

func TestWrapBlockingFactoryLoadOperatorError_IncludesFindingsAndRecoveryCommand(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath, err := namedfactorypath.MapDir(rootDir, "@you/goal")
	if err != nil {
		t.Fatalf("MapDir: %v", err)
	}

	_, err = factoryconfig.PersistNamedFactory(rootDir, "@you/goal", []byte(factoryvalidation.CrossPathInvalidFactoryJSON))
	if err == nil {
		t.Fatal("expected invalid named factory materialization to fail")
	}

	wrapped := blockingload.WrapBlockingFactoryLoadOperatorError(factoryPath, err)
	got := wrapped.Error()
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
	if strings.Contains(got, "blocking validation targets") {
		t.Fatalf("error = %q, want findings instead of target count summary", got)
	}
	recovery := blockingload.FactoryConfigValidateRecoveryCommand(factoryPath)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	if strings.Count(got, "you factory config validate") != 1 {
		t.Fatalf("error = %q, want exactly one recovery command", got)
	}
	if !errors.Is(wrapped, factoryconfig.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", wrapped)
	}
}

func TestMaybeFormatBlockingFactoryLoadOperatorError_IncludesRecoveryForOnDiskFactory(t *testing.T) {
	projectRoot := t.TempDir()
	factoryDir, err := factoryconfig.PersistNamedFactory(projectRoot, "@you/goal", builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	mutateGoalFactoryWorkstationOutputStateForTest(t, factoryDir, "missing-output-state")

	_, loadErr := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if loadErr == nil {
		t.Fatal("expected invalid on-disk goal factory to fail load")
	}

	wrapped := blockingload.MaybeFormatBlockingFactoryLoadOperatorError(loadErr, factoryDir)
	got := wrapped.Error()
	recovery := blockingload.FactoryConfigValidateRecoveryCommand(factoryDir)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
}

func TestFailureBaseline_InvalidTopology_MaterializedGoalUpgradePathReportsFindingsAndRecovery(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(materialize goal): %v", err)
	}
	corruptGoalFactoryExecuteOutputStateForTest(t, resolution.FactoryDir, "missing-output-state")

	_, err = factoryconfig.ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err == nil {
		t.Fatal("expected corrupted materialized goal to fail upgrade resolution")
	}
	wrapped := blockingload.MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(err, projectRoot, globalRoot, "@you/goal")
	assertInvalidTopologyMaterializationOperatorDiagnostics(t, wrapped, resolution.FactoryDir)
}

func TestFailureBaseline_InvalidTopology_PreExistingInvalidMaterializedTargetReportsFindingsAndRecovery(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	factoryDir, err := namedfactorypath.MapDir(globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("MapDir: %v", err)
	}
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

	_, err = factoryconfig.ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err == nil {
		t.Fatal("expected invalid pre-existing materialized target to fail")
	}
	wrapped := blockingload.MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(err, projectRoot, globalRoot, "@you/goal")
	assertInvalidTopologyMaterializationOperatorDiagnostics(t, wrapped, factoryDir)
}

func TestFailureBaseline_InvalidTopology_MaterializeNamedFactoryFailureRetainsStructuredFindings(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath, err := namedfactorypath.MapDir(rootDir, "@you/goal")
	if err != nil {
		t.Fatalf("MapDir: %v", err)
	}

	_, err = factoryconfig.PersistNamedFactory(rootDir, "@you/goal", []byte(failureBaselineInvalidGoalTopologyJSON))
	if err == nil {
		t.Fatal("expected invalid goal materialization to fail")
	}
	wrapped := blockingload.MaybeFormatBlockingFactoryLoadOperatorError(err, factoryPath)
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
	recovery := blockingload.FactoryConfigValidateRecoveryCommand(wantFactoryDir)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	normalized := strings.ReplaceAll(got, "\\", "/")
	if !strings.Contains(normalized, "@you/goal") {
		t.Fatalf("error = %q, want resolved @you/goal factory path", got)
	}
}

func corruptGoalFactoryExecuteOutputStateForTest(t *testing.T, factoryDir, stateName string) {
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
		if candidate["name"] == goal.PackagedExecuteWorkstationName {
			workstation = candidate
			break
		}
	}
	if workstation == nil {
		t.Fatal("factory.json execute-goal workstation not found")
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

func mutateGoalFactoryWorkstationOutputStateForTest(t *testing.T, factoryDir, stateName string) {
	t.Helper()
	corruptGoalFactoryExecuteOutputStateForTest(t, factoryDir, stateName)
}
