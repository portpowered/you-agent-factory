package blockingloadtests

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func TestBlockingFactoryLoadError_PreservesCanonicalTargetsOnCanonicalJSONLoad(t *testing.T) {
	_, err := factorydefinitioncomposition.LoadCanonicalJSON([]byte(factoryfixtures.CrossPathInvalidFactoryJSON), nil)
	assertBlockingFactoryLoadError(t, err, "canonical JSON load")
}

func TestBlockingFactoryLoadError_PreservesTargetsThroughNamedFactoryMaterialization(t *testing.T) {
	rootDir := t.TempDir()

	_, err := factorydefinitioncomposition.PersistNamedFactory(
		rootDir,
		"@you/goal",
		[]byte(factoryfixtures.CrossPathInvalidFactoryJSON),
		factoryvalidation.New(nil),
	)
	assertBlockingFactoryLoadError(t, err, "named Factory materialization")
}

func TestBlockingFactoryLoadError_DistinguishesIOErrorsFromValidationFailures(t *testing.T) {
	missingDir := t.TempDir()
	_ = os.Remove(missingDir)

	_, err := factorydefinitioncomposition.LoadDirectory(missingDir, nil)
	if err == nil {
		t.Fatal("expected missing factory directory to fail")
	}
	if errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, did not want ErrInvalidNamedFactory", err)
	}
	if _, ok := factorydefinitions.AsBlockingFactoryLoadError(err); ok {
		t.Fatalf("error = %v, did not want BlockingFactoryLoadError", err)
	}
}

func TestMaybeFormatBlockingFactoryLoadOperatorError_IncludesRecoveryForOnDiskFactory(t *testing.T) {
	projectRoot := t.TempDir()
	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(
		projectRoot,
		"@you/goal",
		[]byte(validGoalTopologyForBlockingLoadJSON),
		factoryvalidation.New(nil),
	)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	mutateGoalFactoryWorkstationOutputStateForTest(t, factoryDir, "missing-output-state")

	_, loadErr := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	assertBlockingFactoryLoadError(t, loadErr, "invalid on-disk Factory load")
}

func TestFailureBaseline_InvalidTopology_MaterializeNamedFactoryFailureRetainsStructuredFindings(t *testing.T) {
	_, err := factorydefinitioncomposition.PersistNamedFactory(
		t.TempDir(),
		"@you/goal",
		[]byte(failureBaselineInvalidGoalTopologyJSON),
		factoryvalidation.New(nil),
	)
	assertBlockingFactoryLoadError(t, err, "invalid topology materialization")
}

func assertBlockingFactoryLoadError(t *testing.T, err error, operation string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s to fail", operation)
	}
	if !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, did not want os.ErrNotExist", err)
	}
	loadErr, ok := factorydefinitions.AsBlockingFactoryLoadError(err)
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
}

func mutateGoalFactoryWorkstationOutputStateForTest(t *testing.T, factoryDir, stateName string) {
	t.Helper()
	factoryPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
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
		candidate, candidateOK := entry.(map[string]any)
		if candidateOK && candidate["name"] == "execute-goal" {
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

const failureBaselineInvalidGoalTopologyJSON = `{
  "name": "@you/goal",
  "workTypes": [{
    "name": "goal",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "plan", "type": "PROCESSING"},
      {"name": "complete", "type":"TERMINAL"},
      {"name": "failed", "type":"FAILED"}
    ]
  }],
  "workers": [{"name":"goal-planner","type":"AGENT_WORKER"}],
  "workstations": [{
    "name":"plan-goal",
    "type":"AGENT_RUN",
    "worker":"goal-planner",
    "inputs":[{"workType":"goal","state":"init"}],
    "outputs":[{"workType":"goal","state":"missing-plan-state"}],
    "onFailure":[{"workType":"goal","state":"failed"}]
  }]
}`

const validGoalTopologyForBlockingLoadJSON = `{
  "name":"@you/goal",
  "workTypes":[{
    "name":"goal",
    "handlingBehavior":["DEFAULT"],
    "states":[
      {"name":"init","type":"INITIAL"},
      {"name":"complete","type":"TERMINAL"},
      {"name":"failed","type":"FAILED"}
    ]
  }],
  "workers":[{"name":"goal-runner","type":"AGENT_WORKER"}],
  "workstations":[{
    "name":"execute-goal",
    "type":"AGENT_RUN",
    "worker":"goal-runner",
    "inputs":[{"workType":"goal","state":"init"}],
    "outputs":[{"workType":"goal","state":"complete"}],
    "onFailure":[{"workType":"goal","state":"failed"}]
  }]
}`
