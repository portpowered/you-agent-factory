package config_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/builtingoal"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func TestFactoryConfigValidateRecoveryCommand_UsesSingleValidatePath(t *testing.T) {
	factoryPath := filepath.Join("global-root", "@you%2Fgoal")
	got := factoryconfig.FactoryConfigValidateRecoveryCommand(factoryPath)
	if !strings.HasSuffix(got, factoryPath) && !strings.Contains(got, "@you%2Fgoal") {
		t.Fatalf("recovery command = %q, want path %q", got, factoryPath)
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
	factoryPath := filepath.Join(rootDir, "@you%2Fgoal")

	_, err := factoryconfig.PersistNamedFactory(rootDir, "@you/goal", []byte(factoryvalidation.CrossPathInvalidFactoryJSON))
	if err == nil {
		t.Fatal("expected invalid named factory materialization to fail")
	}

	wrapped := factoryconfig.WrapBlockingFactoryLoadOperatorError(factoryPath, err)
	got := wrapped.Error()
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
	if strings.Contains(got, "blocking validation targets") {
		t.Fatalf("error = %q, want findings instead of target count summary", got)
	}
	recovery := factoryconfig.FactoryConfigValidateRecoveryCommand(factoryPath)
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
	mutateGoalFactoryWorkstationOutputStateForTest(t, factoryDir, "missing-plan-state")

	_, loadErr := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if loadErr == nil {
		t.Fatal("expected invalid on-disk goal factory to fail load")
	}

	wrapped := factoryconfig.MaybeFormatBlockingFactoryLoadOperatorError(loadErr, factoryDir)
	got := wrapped.Error()
	recovery := factoryconfig.FactoryConfigValidateRecoveryCommand(factoryDir)
	if !strings.Contains(got, recovery) {
		t.Fatalf("error = %q, want recovery command %q", got, recovery)
	}
	if !strings.Contains(got, "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", got)
	}
}

func mutateGoalFactoryWorkstationOutputStateForTest(t *testing.T, factoryDir, stateName string) {
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
