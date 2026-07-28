package definitions

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	globalDefaultsProviderAlias = "codex"
	globalDefaultsModel         = "operator-default-model"
	factoryOverrideProvider     = modelprovider.ProviderClaude
	factoryOverrideModel        = "factory-authored-model"
)

// TestGlobalConfigSuppliesDefaultProviderAndModel proves workers that omit
// provider and model inherit operator global defaults at run time and dispatch
// through the resolved provider-process edge with the configured default model.
func TestGlobalConfigSuppliesDefaultProviderAndModel(t *testing.T) {
	dir := support.ScaffoldFactory(t, defaultsFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
stopToken: COMPLETE
---
Process the input task.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"global defaults supply provider and model"}`))

	homeDir := writeOperatorGlobalDefaultsConfig(t, globalDefaultsProviderAlias, globalDefaultsModel)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Done. COMPLETE"),
	})

	_, listed := runFactoryWithOperatorHome(
		t,
		dir,
		homeDir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("command = %q, want global default provider %q", call.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", globalDefaultsModel})
}

// TestExplicitFactoryConfigOverridesGlobalDefaults proves Factory-authored
// provider and model on a worker win over operator global defaults at run time
// and dispatch through the resolved provider-process edge with the authored
// model selection.
func TestExplicitFactoryConfigOverridesGlobalDefaults(t *testing.T) {
	dir := support.ScaffoldFactory(t, defaultsFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(factoryOverrideProvider, factoryOverrideModel),
	)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"explicit factory config overrides global defaults"}`))

	homeDir := writeOperatorGlobalDefaultsConfig(t, globalDefaultsProviderAlias, globalDefaultsModel)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.ClaudeSuccessStdout("Done. COMPLETE"),
	})

	_, listed := runFactoryWithOperatorHome(
		t,
		dir,
		homeDir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	call := runner.LastRequest()
	if call.Command != string(factoryOverrideProvider) {
		t.Fatalf(
			"command = %q, want factory-authored provider %q (not global default %q)",
			call.Command,
			factoryOverrideProvider,
			modelprovider.ProviderCodex,
		)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", factoryOverrideModel})
}

func defaultsFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeOperatorGlobalDefaultsConfig(t *testing.T, providerAlias, model string) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {
    "workerModelProvider": "` + providerAlias + `",
    "workerModel": "` + model + `"
  }
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	return homeDir
}

func runFactoryWithOperatorHome(
	t *testing.T,
	dir string,
	homeDir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse) {
	t.Helper()

	server := support.NewProcessAPIServer()
	overrides.APIServerStarter = server.Start
	process := support.BuildProcess(t, overrides)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	support.WaitForTerminalStatus(t, baseURL, timeout)

	session := support.GetDefaultSession(t, baseURL)
	work := support.ListDefaultSessionWork(t, baseURL)
	daemon.Stop(t)
	return session, work
}
