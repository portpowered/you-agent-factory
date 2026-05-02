package functional_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func scaffoldFactory(t *testing.T, cfg map[string]any) string {
	t.Helper()
	dir := t.TempDir()

	// Write factory.json.
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	// Create workstations/<name>/AGENTS.md for each workstation in the config.
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
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "processing", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-b"},
		},
		"workstations": []map[string]any{
			{
				"name":      "step-one",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "step-two",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}
