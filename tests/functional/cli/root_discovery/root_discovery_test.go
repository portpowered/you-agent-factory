package root_discovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestBareRootPrintsConciseHelpWithoutProductEffects(t *testing.T) {
	var effects atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		FactoryDefinitionLoadingFileSystem:  countingFactoryFileSystem{calls: &effects},
		FactoryDefinitionScaffoldFileSystem: countingFactoryFileSystem{calls: &effects},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			effects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			effects.Add(1)
			return nil
		},
		OperatorSettingsFileSystem: failingOperatorSettingsFileSystem{calls: &effects},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			effects.Add(1)
		},
		ProviderOverride: countingProvider{calls: &effects},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if effects.Load() != 0 {
		t.Fatalf("BuildProcess() external effect calls = %d, want 0", effects.Load())
	}

	redirected := executeBareRoot(t, process, false)
	terminal := executeBareRoot(t, process, true)
	if redirected != terminal {
		t.Fatalf("terminal and redirected help differ:\nterminal:\n%s\nredirected:\n%s", terminal, redirected)
	}
	if effects.Load() != 0 {
		t.Fatalf("bare root external effect calls = %d, want 0", effects.Load())
	}
}

func TestCurrentFactoryFailsBeforeProductActivation(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, string)
		wantCode factoryapi.ErrorResponseCode
	}{
		{
			name: "missing exact factory json ignores current pointer",
			prepare: func(t *testing.T, factoryDir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(factoryDir, "alternate"), 0o755); err != nil {
					t.Fatalf("create alternate Factory: %v", err)
				}
				if err := os.WriteFile(filepath.Join(factoryDir, ".current-factory"), []byte("alternate\n"), 0o600); err != nil {
					t.Fatalf("write current pointer: %v", err)
				}
				if err := os.WriteFile(filepath.Join(factoryDir, "alternate", "factory.json"), []byte(`{}`), 0o600); err != nil {
					t.Fatalf("write alternate Factory: %v", err)
				}
			},
			wantCode: "CURRENT_FACTORY_NOT_FOUND",
		},
		{
			name: "invalid exact factory json",
			prepare: func(t *testing.T, factoryDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(`{"name":`), 0o600); err != nil {
					t.Fatalf("write invalid Current Factory: %v", err)
				}
			},
			wantCode: "CURRENT_FACTORY_INVALID",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runCurrentFactoryFailureCase(t, test.prepare, test.wantCode)
		})
	}
}

func runCurrentFactoryFailureCase(
	t *testing.T,
	prepare func(*testing.T, string),
	wantCode factoryapi.ErrorResponseCode,
) {
	t.Helper()
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	prepare(t, factoryDir)

	var effects atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			effects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			effects.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			effects.Add(1)
		},
		FactorySessionIDGenerator: func() string {
			effects.Add(1)
			return "unexpected-session"
		},
		ProviderOverride: countingProvider{calls: &effects},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeCurrentFactory(t, process, workingDirectory)
	if executeErr == nil {
		t.Fatalf("Process.Execute(Current Factory) succeeded; stdout=%q stderr=%q", stdout, stderr)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stderr)), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", err, stderr)
	}
	if response.Code != wantCode {
		t.Fatalf("ErrorResponse = %#v, want code %s", response, wantCode)
	}
	if stdout != "" {
		t.Fatalf("Current Factory failure stdout = %q, want empty", stdout)
	}
	if effects.Load() != 0 {
		t.Fatalf("Current Factory failure product effects = %d, want 0", effects.Load())
	}
}

func TestCurrentFactoryRunsToIdleWithoutStartingServer(t *testing.T) {
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, "factory.json"),
		[]byte(idleCurrentFactoryJSON),
		0o600,
	); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, ".current-factory"), []byte("alternate\n"), 0o600); err != nil {
		t.Fatalf("write ignored current pointer: %v", err)
	}

	var effects atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			effects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			effects.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			effects.Add(1)
		},
		ProviderOverride: countingProvider{calls: &effects},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeCurrentFactory(t, process, workingDirectory)
	if executeErr != nil {
		t.Fatalf("Process.Execute(Current Factory) error = %v; stdout=%q stderr=%q", executeErr, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("Current Factory stderr = %q, want empty", stderr)
	}
	for _, expected := range []string{
		"Factory initiated: " + factoryDir,
		"Dashboard server disabled",
		"Factory:",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("Current Factory stdout omitted %q:\n%s", expected, stdout)
		}
	}
	if effects.Load() != 0 {
		t.Fatalf("idle Current Factory external effects = %d, want no listener, browser, or provider call", effects.Load())
	}
}

func executeCurrentFactory(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	workingDirectory string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	home := t.TempDir()
	err := process.Execute(root.Input{
		Args:             []string{"you", "run", "--no-record"},
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	return stdout.String(), stderr.String(), err
}

const idleCurrentFactoryJSON = `{
  "name": "current",
  "workTypes": [
    {
      "name": "task",
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [{"name": "processor"}],
  "workstations": [
    {
      "name": "process",
      "inputs": [{"workType": "task", "state": "init"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "worker": "processor"
    }
  ]
}`

func executeBareRoot(t *testing.T, process interface{ Execute(root.Input) error }, stdoutIsTTY bool) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdinIsTTY := false
	home := t.TempDir()
	err := process.Execute(root.Input{
		Args:             []string{"you"},
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: t.TempDir(),
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil {
		t.Fatalf("Process.Execute(bare root) error = %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Process.Execute(bare root) stderr = %q, want empty", stderr.String())
	}
	for _, expected := range []string{
		"Run and manage CPN-based workflow factories",
		"Available Commands:",
		"run",
		"server",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("Process.Execute(bare root) stdout omitted %q:\n%s", expected, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "How to use:") {
		t.Fatalf("Process.Execute(bare root) emitted long-form help:\n%s", stdout.String())
	}
	return stdout.String()
}

type countingFactoryFileSystem struct {
	calls *atomic.Int32
}

func (fileSystem countingFactoryFileSystem) Stat(string) (fs.FileInfo, error) {
	fileSystem.calls.Add(1)
	return nil, fs.ErrNotExist
}

func (fileSystem countingFactoryFileSystem) ReadFile(string) ([]byte, error) {
	fileSystem.calls.Add(1)
	return nil, fs.ErrNotExist
}

func (fileSystem countingFactoryFileSystem) MkdirAll(string, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return nil
}

func (fileSystem countingFactoryFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return nil
}

type countingProvider struct {
	calls *atomic.Int32
}

var _ providercontract.Provider = countingProvider{}

func (provider countingProvider) Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	provider.calls.Add(1)
	return workers.InferenceResponse{}, nil
}

type failingOperatorSettingsFileSystem struct {
	calls *atomic.Int32
}

func (fileSystem failingOperatorSettingsFileSystem) ReadFile(string) ([]byte, error) {
	fileSystem.calls.Add(1)
	return nil, fs.ErrPermission
}

func (fileSystem failingOperatorSettingsFileSystem) MkdirAll(string, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return fs.ErrPermission
}

func (fileSystem failingOperatorSettingsFileSystem) Remove(string) error {
	fileSystem.calls.Add(1)
	return fs.ErrPermission
}

func (fileSystem failingOperatorSettingsFileSystem) Chmod(string, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return fs.ErrPermission
}

func (fileSystem failingOperatorSettingsFileSystem) Rename(string, string) error {
	fileSystem.calls.Add(1)
	return fs.ErrPermission
}
