package process_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIWorkerFailureExitCode proves a terminal worker failure crosses the
// public CLI process boundary as a typed failure without using the MockWorkers
// feature outside its workers/mock functional cell.
func TestCLIWorkerFailureExitCode(t *testing.T) {
	factoryDir, factoryPath := scaffoldCLIExitCodeFactory(t)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("provider process failed with private detail"),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", factoryPath,
		"--provider", "codex", "--no-record", "--quiet",
		"worker-failure-exit",
	})
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("worker failure Process.Execute error = nil; want typed process failure")
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want one worker dispatch", runner.CallCount())
	}
	if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
		t.Fatalf("worker failure stdout = %q, want no false success output", stdout)
	}
	var diagnostic struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	stderr := strings.TrimSpace(inputs.Stderr())
	if err := json.Unmarshal([]byte(stderr), &diagnostic); err != nil {
		t.Fatalf("decode worker failure diagnostic: %v; stderr=%q", err, stderr)
	}
	if diagnostic.Code != "INVOCATION_RUNTIME_FAILURE" || diagnostic.Message == "" {
		t.Fatalf("worker failure diagnostic = %#v, want coded runtime failure", diagnostic)
	}
	if strings.Count(stderr, diagnostic.Code) != 1 || strings.Contains(stderr, "private detail") {
		t.Fatalf("worker failure stderr = %q, want one sanitized coded diagnostic", stderr)
	}
}

// TestCLISuccessExitCode proves a successful one-shot worker run reaches the
// public CLI process boundary through an injected provider command runner.
func TestCLISuccessExitCode(t *testing.T) {
	factoryDir, factoryPath := scaffoldCLIExitCodeFactory(t)
	const wantPrimaryResult = "worker success exit COMPLETE"
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(wantPrimaryResult),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", factoryPath,
		"--provider", "codex", "--no-record", "--quiet",
		"worker-success-exit",
	})
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if err != nil {
		t.Fatalf("successful worker Process.Execute failed: %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want one worker dispatch", runner.CallCount())
	}
	if got := strings.TrimSpace(inputs.Stdout()); got != wantPrimaryResult {
		t.Fatalf("worker success stdout = %q, want primary result %q", got, wantPrimaryResult)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("worker success stderr = %q, want empty diagnostics", inputs.Stderr())
	}
}

func scaffoldCLIExitCodeFactory(t *testing.T) (string, string) {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "cli-exit-code-factory",
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir, filepath.Join(dir, "factory.json")
}
