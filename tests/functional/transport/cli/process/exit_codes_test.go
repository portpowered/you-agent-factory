package process_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIWorkerFailureExitCode proves a terminal worker failure crosses the
// reusable customer Process.Execute boundary through the injected command
// runner edge.
func TestCLIWorkerFailureExitCode(t *testing.T) {
	factoryDir, factoryPath := scaffoldCLIExitCodeFactory(t)
	inputs := newCLIExitCodeInputs(t, factoryDir, factoryPath, workerFailurePrompt)
	fixture := sharedWorkerOutcomeProcess(t)
	fixture.bind(t, workerFailurePrompt, factoryDir)
	err := fixture.execute(inputs.Input)
	if err == nil {
		t.Fatal("worker failure Process.Execute error = nil; want typed process failure")
	}
	if calls := fixture.router.callCount(workerFailurePrompt); calls != 1 {
		t.Fatalf("worker failure provider command calls = %d, want one worker dispatch", calls)
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
// reusable customer Process.Execute boundary through an injected provider
// command runner.
func TestCLISuccessExitCode(t *testing.T) {
	factoryDir, factoryPath := scaffoldCLIExitCodeFactory(t)
	inputs := newCLIExitCodeInputs(t, factoryDir, factoryPath, workerSuccessPrompt)
	fixture := sharedWorkerOutcomeProcess(t)
	fixture.bind(t, workerSuccessPrompt, factoryDir)
	err := fixture.execute(inputs.Input)
	if err != nil {
		t.Fatalf("successful worker Process.Execute failed: %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if calls := fixture.router.callCount(workerSuccessPrompt); calls != 1 {
		t.Fatalf("worker success provider command calls = %d, want one worker dispatch", calls)
	}
	if got := strings.TrimSpace(inputs.Stdout()); got != workerSuccessPrimaryResult {
		t.Fatalf("worker success stdout = %q, want primary result %q", got, workerSuccessPrimaryResult)
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
