package mock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
