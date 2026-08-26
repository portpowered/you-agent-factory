package process_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type operatorConfigOutcome struct {
	ConfigPath string
}

func initializeOperatorConfig(
	t testing.TB,
	sessionDir string,
	scenario string,
	configBody []byte,
) operatorConfigOutcome {
	t.Helper()

	configDir := filepath.Join(sessionDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("%s: create config directory: %v", scenario, err)
	}
	configPath := filepath.Join(configDir, "config.json")
	if configBody != nil {
		if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
			t.Fatalf("%s: write config: %v", scenario, err)
		}
	}
	return operatorConfigOutcome{ConfigPath: configPath}
}

const defaultGoalTestConfig = `{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`

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
