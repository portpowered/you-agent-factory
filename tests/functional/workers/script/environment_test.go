package script_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	declaredScriptEnvName  = "FACTORY_SCRIPT_ENV"
	declaredScriptEnvValue = "declared-value"
	undeclaredHostEnvName  = "SCRIPT_ENV_LEAK_PROBE"
	undeclaredHostEnvValue = "must-not-reach-script-command"
)

// TestScriptWorkerReceivesDeclaredEnvironmentOnly proves a root-built script
// worker passes only Factory-declared environment values to the external command
// edge and does not leak planted undeclared host environment material.
func TestScriptWorkerReceivesDeclaredEnvironmentOnly(t *testing.T) {
	t.Setenv(undeclaredHostEnvName, undeclaredHostEnvValue)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	updateScriptWorkstationEnv(t, dir, map[string]string{
		declaredScriptEnvName: declaredScriptEnvValue,
	})
	testutil.WriteSeedFile(t, dir, "task", []byte("environment-boundary-input"))

	runner := support.NewRecordingCommandRunner("script-output-ok")
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1 successful script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want exactly one external command effect", runner.CallCount())
	}

	env := runner.LastRequest().Env
	if !envContains(env, declaredScriptEnvName+"="+declaredScriptEnvValue) {
		t.Fatalf("captured command env missing declared %s=%q in %v", declaredScriptEnvName, declaredScriptEnvValue, env)
	}
	if envContainsKey(env, undeclaredHostEnvName) {
		t.Fatalf("captured command env leaked undeclared host value %s in %v", undeclaredHostEnvName, env)
	}
}

func updateScriptWorkstationEnv(t *testing.T, dir string, env map[string]string) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}
	workstations, ok := cfg["workstations"].([]any)
	if !ok || len(workstations) == 0 {
		t.Fatal("factory.json missing workstations")
	}
	workstation, ok := workstations[0].(map[string]any)
	if !ok {
		t.Fatal("factory.json workstation entry has unexpected shape")
	}
	workstation["env"] = env
	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func envContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func envContainsKey(env []string, name string) bool {
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
