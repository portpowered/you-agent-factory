package runtimetests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestLoadRuntimeConfig_AllowsMissingPortableLayout(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, runtimeFactoryWithoutLayout())
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
		t.Fatalf("expected missing layout to remain absent, got %#v", loaded.FactoryConfig().Layout)
	}
	if _, ok := loaded.Worker("executor"); !ok {
		t.Fatal("expected worker definition to load without layout metadata")
	}
}

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

func TestLoadRuntimeConfig_PreservesStalePortableLayoutWithoutBlockingRuntime(t *testing.T) {
	factoryDir := t.TempDir()

	cfg := runtimeFactoryWithoutLayout()
	cfg["layout"] = portableLayoutFixture("workstation:stale-node", "output:workstation:stale-node->work-type:story")
	writeRuntimeFactoryJSON(t, factoryDir, cfg)
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
	assertLoadedPortableLayoutConfig(t, loaded.FactoryConfig(), "workstation:stale-node")
	if _, ok := loaded.Worker("executor"); !ok {
		t.Fatal("expected worker definition to load despite stale layout references")
	}
}

func TestReplaceFactoryLayoutAtDir_PreservesPortableLayoutThroughBackendLoadSave(t *testing.T) {
	targetDir := t.TempDir()
	initial := namedFactoryPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	updated := namedFactoryPayloadWithPortableLayout(t, "alpha")
	if _, err := factorydefinitioncomposition.ReplaceFactoryLayout(
		targetDir,
		updated,
		ownerFactoryDefinitionValidator(),
	); err != nil {
		t.Fatalf("ReplaceFactoryLayout: %v", err)
	}

	loaded, err := LoadRuntimeConfig(targetDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	assertLoadedPortableLayoutConfig(t, loaded.FactoryConfig(), "workstation:execute-alpha")

	factoryJSON, err := os.ReadFile(filepath.Join(targetDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(factoryJSON, &persisted); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	assertPortableLayoutJSONPayload(t, persisted["layout"], "workstation:execute-alpha")
}
