package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const successStdoutPrimaryResult = "mock worker accepted"

var compiledCLIBinary struct {
	once     sync.Once
	tempDir  string
	path     string
	err      error
	buildLog []byte
}

var configuredGoalOperatorConfigSeed struct {
	once sync.Once
	body []byte
	err  error
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	if compiledCLIBinary.tempDir != "" {
		if err := removeCompiledCLIBinaryDirectory(compiledCLIBinary.tempDir); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "remove compiled CLI binary directory %s: %v\n", compiledCLIBinary.tempDir, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

const (
	compiledCLICleanupTimeout = 2 * time.Second
	compiledCLICleanupRetry   = 10 * time.Millisecond
)

func removeCompiledCLIBinaryDirectory(dir string) error {
	var lastErr error
	deadline := time.Now().Add(compiledCLICleanupTimeout)
	for {
		lastErr = os.RemoveAll(dir)
		if lastErr == nil {
			if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
				return nil
			}
			lastErr = fmt.Errorf("compiled CLI binary directory still exists")
		}
		if runtime.GOOS != "windows" || time.Now().After(deadline) {
			return lastErr
		}
		// Windows can release the executable handle shortly after the child
		// process has exited. A bounded retry keeps CLEAN-001 deterministic
		// without masking a persistent cleanup failure.
		time.Sleep(compiledCLICleanupRetry)
	}
}

func buildYouBinary(t testing.TB, ctx context.Context, repoRoot string) string {
	t.Helper()
	compiledCLIBinary.once.Do(func() {
		compiledCLIBinary.tempDir, compiledCLIBinary.err = os.MkdirTemp("", "you-cli-mock-worker-package-")
		if compiledCLIBinary.err != nil {
			return
		}
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		compiledCLIBinary.path = filepath.Join(compiledCLIBinary.tempDir, binaryName)
		command := exec.CommandContext(ctx, "go", "build", "-o", compiledCLIBinary.path, "./cmd/factory")
		command.Dir = repoRoot
		compiledCLIBinary.buildLog, compiledCLIBinary.err = command.CombinedOutput()
	})
	if compiledCLIBinary.err != nil {
		t.Fatalf("build you CLI: %v\n%s", compiledCLIBinary.err, compiledCLIBinary.buildLog)
	}
	return compiledCLIBinary.path
}

func runBuiltYouBinary(
	ctx context.Context,
	binaryPath string,
	session *builtcliacceptance.Session,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	var stdout, stderr strings.Builder
	command := exec.CommandContext(ctx, binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return builtcliacceptance.RunResult{Stdout: stdout.String(), Stderr: stderr.String()}, err
		}
		exitCode = exitErr.ExitCode()
	}
	return builtcliacceptance.RunResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, err
}

func newConfiguredGoalSession(
	t testing.TB,
	ctx context.Context,
	harness *builtcliacceptance.Harness,
	scenario string,
) *builtcliacceptance.Session {
	t.Helper()
	configBody := configuredGoalOperatorConfig(t, ctx, harness)
	session := harness.NewSession(t).WithNoExternalServer(t)
	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("%s: create isolated operator config directory: %v", scenario, err)
	}
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatalf("%s: write isolated operator config %q: %v", scenario, configPath, err)
	}
	return session
}

// configuredGoalOperatorConfig materializes the initializer-owned operator
// config once, validates that the real initializer created readable JSON, and
// then retains only an immutable value snapshot. Each target session receives
// a fresh restrictive-permission copy, so sessions never share HOME state.
func configuredGoalOperatorConfig(
	t testing.TB,
	ctx context.Context,
	harness *builtcliacceptance.Harness,
) []byte {
	t.Helper()
	configuredGoalOperatorConfigSeed.once.Do(func() {
		seedSession := harness.NewSession(t).WithNoExternalServer(t)
		configPath := filepath.Join(seedSession.HomeDir, ".you-agent-factory", "config.json")
		missingFactory := filepath.Join(seedSession.WorkDir, "missing-initialization-factory.json")
		result, err := seedSession.Run(ctx, "run", "--factory", missingFactory)
		if err == nil {
			configuredGoalOperatorConfigSeed.err = fmt.Errorf(
				"run missing Factory unexpectedly succeeded: stdout=%q stderr=%q",
				result.Stdout,
				result.Stderr,
			)
			return
		}
		materialized, err := os.ReadFile(configPath)
		if err != nil {
			configuredGoalOperatorConfigSeed.err = fmt.Errorf(
				"read initializer-owned config %q: %w",
				configPath,
				err,
			)
			return
		}
		var materializedConfig map[string]json.RawMessage
		if err := json.Unmarshal(materialized, &materializedConfig); err != nil {
			configuredGoalOperatorConfigSeed.err = fmt.Errorf(
				"decode initializer-owned config %q: %w",
				configPath,
				err,
			)
			return
		}
		if materializedConfig == nil {
			configuredGoalOperatorConfigSeed.err = fmt.Errorf(
				"initializer-owned config %q is not a JSON object",
				configPath,
			)
			return
		}

		configured := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
		if err := validateConfiguredGoalOperatorConfig(configured); err != nil {
			configuredGoalOperatorConfigSeed.err = err
			return
		}
		if err := os.WriteFile(configPath, configured, 0o600); err != nil {
			configuredGoalOperatorConfigSeed.err = fmt.Errorf(
				"write configured operator config %q: %w",
				configPath,
				err,
			)
			return
		}
		configuredGoalOperatorConfigSeed.body = append([]byte(nil), configured...)
	})
	if configuredGoalOperatorConfigSeed.err != nil {
		t.Fatalf("materialize configured operator config: %v", configuredGoalOperatorConfigSeed.err)
	}
	return append([]byte(nil), configuredGoalOperatorConfigSeed.body...)
}

func validateConfiguredGoalOperatorConfig(body []byte) error {
	var config struct {
		Defaults struct {
			WorkerModelProvider string `json:"workerModelProvider"`
			WorkerModel         string `json:"workerModel"`
		} `json:"defaults"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return fmt.Errorf("validate configured operator config: %w", err)
	}
	if config.Defaults.WorkerModelProvider != "codex" || config.Defaults.WorkerModel != "gpt-5-codex" {
		return fmt.Errorf("validate configured operator config: worker defaults changed")
	}
	return nil
}

func writeAcceptingGoalMockWorkers(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{WorkerName: "goal-planner", WorkstationName: "plan-goal", RunType: workers.MockWorkerRunTypeAccept},
			{WorkerName: "goal-executor", WorkstationName: "execute-goal", RunType: workers.MockWorkerRunTypeAccept},
			{WorkerName: "goal-checker", WorkstationName: "check-goal", RunType: workers.MockWorkerRunTypeAccept},
			{WorkerName: "goal-reviewer", WorkstationName: "review-goal", RunType: workers.MockWorkerRunTypeAccept},
		},
	})
	if err != nil {
		t.Fatalf("marshal accepting mock workers: %v", err)
	}
	path := filepath.Join(t.TempDir(), "accepting-mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write accepting mock workers: %v", err)
	}
	return path
}

func writeRejectingGoalMockWorkers(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
		{WorkerName: "goal-planner", WorkstationName: "plan-goal", RunType: workers.MockWorkerRunTypeReject},
		{WorkerName: "goal-executor", WorkstationName: "execute-goal", RunType: workers.MockWorkerRunTypeReject},
		{WorkerName: "goal-checker", WorkstationName: "check-goal", RunType: workers.MockWorkerRunTypeReject},
		{WorkerName: "goal-reviewer", WorkstationName: "review-goal", RunType: workers.MockWorkerRunTypeReject},
	}})
	if err != nil {
		t.Fatalf("marshal rejecting mock workers: %v", err)
	}
	path := filepath.Join(t.TempDir(), "rejecting-mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write rejecting mock workers: %v", err)
	}
	return path
}

const (
	stdinRunWorkTypeName    = "prompt-task"
	stdinRunWorkstationName = "process-prompt"
	stdinRunWorkerName      = "mock-worker"
)

func writeStdinRunFactory(t testing.TB, workDir string) string {
	t.Helper()
	factoryDir := filepath.Join(workDir, "stdin-run-factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create stdin run factory directory: %v", err)
	}
	factoryJSON := fmt.Sprintf(`{
  "name": "stdin-run-process",
  "workTypes": [{
    "name": %q,
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": %q}],
  "workstations": [{
    "name": %q,
    "worker": %q,
    "inputs": [{"workType": %q, "state": "init"}],
    "outputs": [{"workType": %q, "state": "complete"}],
    "onFailure": [{"workType": %q, "state": "failed"}]
  }]
}`,
		stdinRunWorkTypeName,
		stdinRunWorkerName,
		stdinRunWorkstationName,
		stdinRunWorkerName,
		stdinRunWorkTypeName,
		stdinRunWorkTypeName,
		stdinRunWorkTypeName,
	)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(factoryJSON), 0o600); err != nil {
		t.Fatalf("write stdin run factory.json: %v", err)
	}
	workstationPath := filepath.Join(factoryDir, "workstations", stdinRunWorkstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workstationPath), 0o755); err != nil {
		t.Fatalf("create stdin run workstation directory: %v", err)
	}
	workstationConfig := "---\ntype: MODEL_WORKSTATION\n---\nProcess {{ (index .Inputs 0).Payload }}.\n"
	if err := os.WriteFile(workstationPath, []byte(workstationConfig), 0o644); err != nil {
		t.Fatalf("write stdin run workstation config: %v", err)
	}
	return factoryPath
}
