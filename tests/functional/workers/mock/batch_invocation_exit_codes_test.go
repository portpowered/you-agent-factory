package mock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type batchProcessReport struct {
	Status   string                `json:"status"`
	Failures []batchProcessFailure `json:"failures"`
}

type batchProcessFailure struct {
	WorkID    string `json:"workId,omitempty"`
	WorkName  string `json:"workName"`
	WorkState string `json:"workState"`
	Reason    string `json:"reason"`
}

type batchWorkSpec struct {
	Name       string
	WorkTypeID string
}

func decodeBatchProcessReport(t testing.TB, stdout string) batchProcessReport {
	t.Helper()
	var report batchProcessReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("batch JSON stdout is not parseable: %v\nstdout:\n%s", err, stdout)
	}
	return report
}

func writeBatchCurrentFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	sourcePath := writeStdinRunFactory(t, workingDirectory)
	sourceDir := filepath.Dir(sourcePath)
	destinationDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(filepath.Join(destinationDir, "workstations", stdinRunWorkstationName), 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	factoryJSON, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read batch Current Factory fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destinationDir, "factory.json"), factoryJSON, 0o600); err != nil {
		t.Fatalf("write batch Current Factory fixture: %v", err)
	}
	workstationConfig, err := os.ReadFile(filepath.Join(sourceDir, "workstations", stdinRunWorkstationName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read batch workstation fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destinationDir, "workstations", stdinRunWorkstationName, "AGENTS.md"), workstationConfig, 0o644); err != nil {
		t.Fatalf("write batch workstation fixture: %v", err)
	}
	workerDir := filepath.Join(destinationDir, "workers", stdinRunWorkerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create batch worker directory: %v", err)
	}
	workerConfig := "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - batch-exit-fixture\n---\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(workerConfig), 0o644); err != nil {
		t.Fatalf("write batch worker fixture: %v", err)
	}
}

func writeBatchWorksWithTypes(t testing.TB, specs ...batchWorkSpec) string {
	t.Helper()
	works := make([]work.Work, 0, len(specs))
	for _, spec := range specs {
		workID := strings.ToLower(strings.ReplaceAll(spec.Name, " ", "-"))
		works = append(works, work.Work{
			Name: spec.Name, WorkID: workID, WorkTypeID: spec.WorkTypeID,
			TraceID: workID + "-trace", Payload: "batch customer behavior",
		})
	}
	path := filepath.Join(t.TempDir(), "batch-work.json")
	request := work.WorkRequest{
		Type:  work.WorkRequestTypeFactoryRequestBatch,
		Works: works,
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal batch Work: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write batch Work: %v", err)
	}
	return path
}

func writeBatchMockWorkers(t testing.TB, runType workers.MockWorkerRunType) string {
	t.Helper()
	return writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: stdinRunWorkerName, WorkstationName: stdinRunWorkstationName, RunType: runType,
	}}})
}

func writeBatchMockWorkersConfig(t testing.TB, config workers.MockWorkersConfig) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch-mock-workers.json")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal batch mock workers: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write batch mock workers: %v", err)
	}
	return path
}

func writeBatchMixedMockWorkers(t testing.TB) string {
	t.Helper()
	return writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
		{WorkerName: "successful-worker", WorkstationName: "successful-work", RunType: workers.MockWorkerRunTypeAccept},
		{WorkerName: "failed-worker", WorkstationName: "failed-work", RunType: workers.MockWorkerRunTypeReject},
	}})
}

func writeBatchMixedFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	writeBatchFactoryFiles(t, workingDirectory, map[string]any{
		"name": "batch-mixed-outcomes",
		"workTypes": []map[string]any{
			batchWorkTypeConfig("successful-task"),
			batchWorkTypeConfig("failed-task"),
		},
		"workers": []map[string]string{
			{"name": "successful-worker"}, {"name": "failed-worker"},
		},
		"workstations": []map[string]any{
			batchWorkstationConfig("successful-work", "successful-worker", "successful-task", "complete", "failed"),
			batchWorkstationConfig("failed-work", "failed-worker", "failed-task", "complete", "failed"),
		},
	}, map[string]string{
		"successful-worker": batchModelWorkerConfig(),
		"failed-worker":     batchModelWorkerConfig(),
	}, map[string]string{
		"successful-work": batchModelWorkstationConfig(),
		"failed-work":     batchModelWorkstationConfig(),
	})
}

func writeBatchRetryFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	writeBatchFactoryFiles(t, workingDirectory, map[string]any{
		"name":      "batch-circuit-breaker",
		"workTypes": []map[string]any{batchWorkTypeConfig("retry-task")},
		"workers":   []map[string]string{{"name": "retry-worker"}},
		"workstations": []map[string]any{batchWorkstationConfigWithLimits(
			"retry-work", "retry-worker", "retry-task", "complete", "init", map[string]any{"maxRetries": 1},
		)},
	}, map[string]string{
		"retry-worker": batchScriptWorkerConfig(),
	}, map[string]string{
		"retry-work": batchModelWorkstationConfig(),
	})
}

func writeBatchScriptFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	writeBatchFactoryFiles(t, workingDirectory, map[string]any{
		"name":      "batch-script-failure",
		"workTypes": []map[string]any{batchWorkTypeConfig("script-task")},
		"workers":   []map[string]string{{"name": "script-worker"}},
		"workstations": []map[string]any{
			batchWorkstationConfig("script-work", "script-worker", "script-task", "complete", "failed"),
		},
	}, map[string]string{
		"script-worker": batchScriptWorkerConfig(),
	}, map[string]string{
		"script-work": batchModelWorkstationConfig(),
	})
}

func writeBatchFactoryFiles(
	t testing.TB,
	workingDirectory string,
	factoryConfig map[string]any,
	workerConfigs map[string]string,
	workstationConfigs map[string]string,
) {
	t.Helper()
	factoryDirectory := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create batch Factory directory: %v", err)
	}
	factoryJSON, err := json.MarshalIndent(factoryConfig, "", "  ")
	if err != nil {
		t.Fatalf("marshal batch Factory fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), factoryJSON, 0o600); err != nil {
		t.Fatalf("write batch Factory fixture: %v", err)
	}
	for workerName, config := range workerConfigs {
		path := filepath.Join(factoryDirectory, "workers", workerName, "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create batch worker directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
			t.Fatalf("write batch worker %q: %v", workerName, err)
		}
	}
	for workstationName, config := range workstationConfigs {
		path := filepath.Join(factoryDirectory, "workstations", workstationName, "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create batch workstation directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
			t.Fatalf("write batch workstation %q: %v", workstationName, err)
		}
	}
}

func batchWorkTypeConfig(name string) map[string]any {
	return map[string]any{
		"name": name,
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}
}

func batchWorkstationConfig(name, worker, workType, output, failure string) map[string]any {
	return batchWorkstationConfigWithLimits(name, worker, workType, output, failure, nil)
}

func batchWorkstationConfigWithLimits(
	name, worker, workType, output, failure string,
	limits map[string]any,
) map[string]any {
	config := map[string]any{
		"name": name, "worker": worker,
		"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
		"outputs":   []map[string]string{{"workType": workType, "state": output}},
		"onFailure": []map[string]string{{"workType": workType, "state": failure}},
	}
	if limits != nil {
		config["limits"] = limits
	}
	return config
}

func batchModelWorkerConfig() string {
	return "---\ntype: MODEL_WORKSTATION\n---\nProcess the batch Work.\n"
}

func batchScriptWorkerConfig() string {
	return "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - batch-script\n---\nRun the batch script.\n"
}

func batchModelWorkstationConfig() string {
	return "---\ntype: MODEL_WORKSTATION\n---\nProcess the batch Work.\n"
}
