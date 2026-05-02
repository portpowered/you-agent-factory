package runtime_api_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func scaffoldFactory(t *testing.T, cfg map[string]any) string {
	t.Helper()
	dir := t.TempDir()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	if workstations, ok := cfg["workstations"].([]map[string]any); ok {
		for _, ws := range workstations {
			name, _ := ws["name"].(string)
			if name == "" {
				continue
			}
			wsDir := filepath.Join(dir, "workstations", name)
			if err := os.MkdirAll(wsDir, 0o755); err != nil {
				t.Fatalf("create workstation dir %s: %v", name, err)
			}
			agentsMD := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
			if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
				t.Fatalf("write workstation AGENTS.md for %s: %v", name, err)
			}
		}
	}

	return dir
}

func twoStagePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "stage1", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}},
		"workstations": []map[string]any{
			{
				"name":      "worker-a",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "stage1"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "worker-b",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "stage1"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}

func skipSlowFunctionalSmokeInShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skip(reason)
	}
}

type blockingExecutor struct {
	releaseCh <-chan struct{}
	mu        *sync.Mutex
	calls     *int
}

func (e *blockingExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	*e.calls++
	e.mu.Unlock()

	<-e.releaseCh

	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}
