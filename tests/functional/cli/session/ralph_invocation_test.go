package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/ralph"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/root"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPackagedRalphInvocationThroughLiveCLIServiceSession(t *testing.T) {
	if testing.Short() {
		t.Skip("live CLI packaged Ralph invocation coverage")
	}
	cleanPackagedRalphCLIRuntimeState(t)

	factoryPath := materializePackagedRalphForCLISession(t)
	mockWorkersPath := writePackagedRalphMockWorkers(t)

	t.Run("default workflow completes after planner then executor", func(t *testing.T) {
		response, stderr, exitCode := runPackagedRalphCLI(t, factoryPath, mockWorkersPath, nil, "finish the default request")
		assertPackagedRalphCLICompletion(t, response, stderr, exitCode)
	})

	t.Run("configured parameters are accepted", func(t *testing.T) {
		response, stderr, exitCode := runPackagedRalphCLI(t, factoryPath, mockWorkersPath,
			[]string{"--planning-detail", "brief", "--execution-style", "direct"},
			"finish the configured request")
		assertPackagedRalphCLICompletion(t, response, stderr, exitCode)
	})

	t.Run("invalid parameter rejects before worker dispatch", func(t *testing.T) {
		err := executePackagedRalphCLI(t, factoryPath, mockWorkersPath,
			[]string{"--planning-detail", "verbose"}, "reject this request")
		if err == nil {
			t.Fatal("invalid planning detail error = nil, want failure")
		}
		if !strings.Contains(err.Error(), "planningDetail") || !strings.Contains(err.Error(), "declared choices") {
			t.Fatalf("invalid planning detail error = %v, want actionable planningDetail choice diagnostic", err)
		}
	})

	t.Run("model and provider flags preserve invocation", func(t *testing.T) {
		response, stderr, exitCode := runPackagedRalphCLI(t, factoryPath, mockWorkersPath,
			[]string{
				"--default-worker-model-provider", "CODEX",
				"--default-worker-model", "gpt-5-codex",
				"--planning-detail", "brief",
			},
			"finish the flagged request")
		assertPackagedRalphCLICompletion(t, response, stderr, exitCode)
	})
}

func cleanPackagedRalphCLIRuntimeState(t *testing.T) {
	t.Helper()
	const runtimeStateDir = ".you-agent-factory"
	if _, err := os.Stat(runtimeStateDir); err == nil {
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect CLI runtime state directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(runtimeStateDir); err != nil {
			t.Errorf("remove test-owned CLI runtime state: %v", err)
		}
	})
}

func materializePackagedRalphForCLISession(t *testing.T) string {
	t.Helper()
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), ralph.PackagedFactoryName, ralph.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/ralph): %v", err)
	}
	return filepath.Join(dir, interfaces.FactoryConfigFile)
}

func writePackagedRalphMockWorkers(t *testing.T) string {
	t.Helper()
	config := factoryconfig.MockWorkersConfig{MockWorkers: []factoryconfig.MockWorkerConfig{
		{
			WorkerName:      "ralph-planner",
			WorkstationName: ralph.PackagedPlanWorkstationName,
			RunType:         factoryconfig.MockWorkerRunTypeAccept,
		},
		{
			WorkerName:      "ralph-executor",
			WorkstationName: ralph.PackagedExecuteWorkstationName,
			RunType:         factoryconfig.MockWorkerRunTypeAccept,
		},
	}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal Ralph mock workers: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ralph-mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write Ralph mock workers: %v", err)
	}
	return path
}

func runPackagedRalphCLI(t *testing.T, factoryPath, mockWorkersPath string, options []string, request string) (factoryapi.InvocationResponse, string, int) {
	t.Helper()
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	args := packagedRalphCLIArgs(factoryPath, mockWorkersPath, options, request)
	input := BasicCliInputWithArgs(t, args)
	setPackagedRalphCLIHome(t, &input)
	input.Stdout = &stdout
	input.Stderr = &stderr

	exitCode := root.Run(input, root.Dependencies{})
	var response factoryapi.InvocationResponse
	if stdout.Len() > 0 {
		if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response); err != nil {
			t.Fatalf("decode @you/ralph CLI response: %v\nstdout:\n%s", err, stdout.String())
		}
	}
	return response, stderr.String(), exitCode
}

func executePackagedRalphCLI(t *testing.T, factoryPath, mockWorkersPath string, options []string, request string) error {
	t.Helper()
	input := BasicCliInputWithArgs(t, packagedRalphCLIArgs(factoryPath, mockWorkersPath, options, request))
	setPackagedRalphCLIHome(t, &input)
	return root.ExecuteWithDependencies(input, root.Dependencies{})
}

func setPackagedRalphCLIHome(t *testing.T, input *root.Input) {
	t.Helper()
	homeDir := t.TempDir()
	input.Env = append(input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func packagedRalphCLIArgs(factoryPath, mockWorkersPath string, options []string, request string) []string {
	args := []string{"you", "--json", "run", "--factory", factoryPath, "--with-mock-workers", mockWorkersPath, "--no-record", "--quiet"}
	args = append(args, options...)
	return append(args, request)
}

func assertPackagedRalphCLICompletion(t *testing.T, response factoryapi.InvocationResponse, stderr string, exitCode int) {
	t.Helper()
	if exitCode != 0 {
		t.Fatalf("@you/ralph CLI exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("@you/ralph status = %q, want COMPLETED; stderr: %s", response.Status, stderr)
	}
	if response.PrimaryResult == nil {
		t.Fatalf("@you/ralph primary result = nil, want the executor result")
	}
}
