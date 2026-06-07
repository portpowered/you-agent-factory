package runtimetests

import (
	. "github.com/portpowered/infinite-you/pkg/config"
	"testing"
)

func TestLoadRuntimeConfig_IgnoresMalformedPortableLayoutMetadata(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"layout": map[string]any{
			"schemaVersion": "broken",
			"nodes": []map[string]any{{
				"id":       "workstation:execute-story",
				"position": "invalid",
			}},
		},
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{
			"name": "executor",
		}},
		"workstations": []map[string]any{{
			"id":      "execute-story",
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			"type":    "MODEL_WORKSTATION",
		}},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
---
You are the executor worker.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
---
Implement {{ .WorkID }}.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.FactoryConfig().Layout != nil {
		t.Fatalf("expected malformed layout metadata to be ignored at runtime load, got %#v", loaded.FactoryConfig().Layout)
	}
	if _, ok := loaded.Worker("executor"); !ok {
		t.Fatal("expected worker definition to load despite malformed layout metadata")
	}
	workstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected workstation definition to load despite malformed layout metadata")
	}
	if workstation.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("workstation prompt template = %q, want split AGENTS.md content", workstation.PromptTemplate)
	}
}
