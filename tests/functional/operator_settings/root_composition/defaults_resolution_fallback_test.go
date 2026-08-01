package root_composition_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestResolveFromHomeFallbackPreservesAcceptedSemantics proves defaults
// resolution through the customer process. The file, environment, and runner
// effects are supplied through the process input and edges.Edges; no Settings
// or Providers composition root is constructed by the functional test.
func TestResolveFromHomeFallbackPreservesAcceptedSemantics(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir operator config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{
  "defaults": {
    "workerModelProvider": "claude",
    "workerModel": "file-model"
  }
}`), 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}

	dir := support.ScaffoldFactory(t, operatorConfigActivationFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"defaults fallback"}`))
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
stopToken: COMPLETE
---
Use the resolved operator defaults.
`)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("defaults fallback complete"),
	})
	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      server.Start,
		ProviderCommandRunner: runner,
	})

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = append(
		os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		operatorsettings.EnvDefaultWorkerModelProvider+"="+string(modelprovider.ProviderCodex),
		operatorsettings.EnvDefaultWorkerModel+"=env-model",
	)
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	support.WaitForTerminalStatus(t, baseURL, 15*time.Second)
	daemon.Stop(t)

	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("command = %q, want %q from environment fallback", call.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", "env-model"})
}
