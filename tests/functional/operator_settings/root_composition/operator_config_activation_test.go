package root_composition_test

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	activationConfigProviderAlias = "codex"
	activationConfigModel         = "operator-configured-model"
	activationOverrideModel       = "flag-override-model"
	activationUpdatedProvider     = "claude"
	activationUpdatedModel        = "operator-updated-model"
)

// TestOperatorConfigLoadAndDefaultsResolutionActivateThroughRootBuildProcessAfterLifecycle
// proves operator-config document load and defaults resolution activate through
// public Operator Settings surfaces after runtime lifecycle on a process composed
// only via root.BuildProcess with edges.Edges effect replacement.
func TestOperatorConfigLoadAndDefaultsResolutionActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, activationConfigProviderAlias, activationConfigModel)
	recorder := newOperatorSettingsActivationRecorder()
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Done. COMPLETE"),
	})
	server := support.NewProcessAPIServer()
	edges := recorder.edges()
	edges.APIServerStarter = server.Start
	edges.ProviderCommandRunner = runner
	process := support.BuildProcess(t, edges)

	if got := recorder.fileSystemCalls(); got != 0 {
		t.Fatalf("operator-config filesystem effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.createTemporaryFileCalls(); got != 0 {
		t.Fatalf("operator-config CreateTemporaryFile calls = %d during BuildProcess, want 0", got)
	}

	dir := support.ScaffoldFactory(t, operatorConfigActivationFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
stopToken: COMPLETE
---
Process the input task.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"operator settings defaults activation"}`))

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
		"--model", activationOverrideModel,
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, baseURL)
	support.WaitForSessionWorkTerminalFromFactoryEvents(t, baseURL, "~default", 15*time.Second)
	daemon.Stop(t)
	terminalObservation.Wait(15 * time.Second)

	if got := recorder.readFileCalls(); got == 0 {
		t.Fatalf("operator-config ReadFile calls after runtime lifecycle = %d, want > 0 via edges", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("command = %q, want global default provider %q", call.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", activationOverrideModel})
}

// TestOperatorConfigDocumentUpdateActivatesThroughRootBuildProcessPublicCLISurface
// proves persisted provider-model updates activate through the public you init CLI
// surface on the same root.BuildProcess composition path with edges.Edges effect
// replacement after runtime lifecycle has started and stopped on that process.
func TestOperatorConfigDocumentUpdateActivatesThroughRootBuildProcessPublicCLISurface(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, activationConfigProviderAlias, activationConfigModel)
	recorder := newOperatorSettingsActivationRecorder()
	dir := support.ScaffoldSingleStepFactory(t, "operator-config-update")
	server := support.NewProcessAPIServer()
	edges := recorder.edges()
	edges.APIServerStarter = server.Start
	process := support.BuildProcess(t, edges)

	runInputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	runInputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	runInputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, runInputs.Input)
	baseURL := server.WaitForURL(t)
	support.WaitForStatus(t, baseURL, 5*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	daemon.Stop(t)

	beforeUpdate := recorder.fileSystemCalls()
	var stdout bytes.Buffer
	initErr := process.Execute(root.Input{
		Args: []string{
			"you", "init",
			"--provider", activationUpdatedProvider,
			"--model", activationUpdatedModel,
		},
		Env: append(
			os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           io.Discard,
		Context:          t.Context(),
		WorkingDirectory: dir,
	})
	if initErr != nil {
		t.Fatalf("Process.Execute(you init) error = %v", initErr)
	}

	if got := recorder.fileSystemCalls() - beforeUpdate; got == 0 {
		t.Fatalf("operator-config filesystem effect calls during init = %d, want > 0 via edges", got)
	}
	if got := recorder.createTemporaryFileCalls(); got == 0 {
		t.Fatalf("operator-config CreateTemporaryFile calls during init = %d, want > 0 via edges", got)
	}

	configPath := operatorsettings.DefaultConfigPath(homeDir)
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated operator config: %v", err)
	}
	updated := string(payload)
	for _, want := range []string{
		`"workerModelProvider": "claude"`,
		`"workerModel": "operator-updated-model"`,
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated operator config omitted %q:\n%s", want, updated)
		}
	}
	if got := stdout.String(); !strings.Contains(got, "Configured default provider claude and model operator-updated-model") {
		t.Fatalf("stdout = %q, want documented configure success", got)
	}
}

type operatorSettingsActivationRecorder struct {
	readFile        atomic.Int32
	mkdirAll        atomic.Int32
	remove          atomic.Int32
	chmod           atomic.Int32
	rename          atomic.Int32
	createTempCalls atomic.Int32
}

func newOperatorSettingsActivationRecorder() *operatorSettingsActivationRecorder {
	return &operatorSettingsActivationRecorder{}
}

func (recorder *operatorSettingsActivationRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		OperatorSettingsFileSystem:          &operatorSettingsActivationFileSystem{recorder: recorder},
		OperatorSettingsCreateTemporaryFile: recorder.createTemporaryFile,
	}
}

func (recorder *operatorSettingsActivationRecorder) fileSystemCalls() int32 {
	return recorder.readFile.Load() +
		recorder.mkdirAll.Load() +
		recorder.remove.Load() +
		recorder.chmod.Load() +
		recorder.rename.Load()
}

func (recorder *operatorSettingsActivationRecorder) readFileCalls() int32 {
	return recorder.readFile.Load()
}

func (recorder *operatorSettingsActivationRecorder) createTemporaryFileCalls() int32 {
	return recorder.createTempCalls.Load()
}

func (recorder *operatorSettingsActivationRecorder) createTemporaryFile(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	recorder.createTempCalls.Add(1)
	return os.CreateTemp(dir, pattern)
}

type operatorSettingsActivationFileSystem struct {
	recorder *operatorSettingsActivationRecorder
}

func (adapter *operatorSettingsActivationFileSystem) ReadFile(path string) ([]byte, error) {
	adapter.recorder.readFile.Add(1)
	return os.ReadFile(path)
}

func (adapter *operatorSettingsActivationFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	adapter.recorder.mkdirAll.Add(1)
	return os.MkdirAll(path, mode)
}

func (adapter *operatorSettingsActivationFileSystem) Remove(path string) error {
	adapter.recorder.remove.Add(1)
	return os.Remove(path)
}

func (adapter *operatorSettingsActivationFileSystem) Chmod(path string, mode fs.FileMode) error {
	adapter.recorder.chmod.Add(1)
	return os.Chmod(path, mode)
}

func (adapter *operatorSettingsActivationFileSystem) Rename(oldPath, newPath string) error {
	adapter.recorder.rename.Add(1)
	return os.Rename(oldPath, newPath)
}

func writeOperatorConfigForActivation(t *testing.T, providerAlias, model string) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir operator config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {
    "workerModelProvider": "` + providerAlias + `",
    "workerModel": "` + model + `"
  }
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	return homeDir
}

func operatorConfigActivationFactoryConfig() map[string]any {
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
