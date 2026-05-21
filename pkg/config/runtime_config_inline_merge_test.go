package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInlineRuntimeDefinitions_LoadsSplitDefinitionsIntoFactoryConfig(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"resources": []map[string]any{},
		"workers":   []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
---
Run tests.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
---
	Implement {{ .WorkID }}.
`)
	if err := os.WriteFile(filepath.Join(factoryDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	factoryCfg, err := loadFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("loadFactoryConfig: %v", err)
	}
	inlined, err := InlineRuntimeDefinitions(factoryDir, factoryCfg, InlineRuntimeDefinitionOptions{
		RequireSplitDefinitions: true,
	})
	if err != nil {
		t.Fatalf("InlineRuntimeDefinitions: %v", err)
	}

	if inlined.Workers[0].Name != "executor" || inlined.Workers[0].Command != "go" {
		t.Fatalf("expected worker definition to be inlined, got %#v", inlined.Workers[0])
	}
	if inlined.Workstations[0].Type != "MODEL_WORKSTATION" {
		t.Fatalf("expected workstation runtime type to be inlined, got %#v", inlined.Workstations[0])
	}
	if inlined.Workstations[0].PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("expected prompt file content to be inlined, got %q", inlined.Workstations[0].PromptTemplate)
	}
}

func TestInlineRuntimeDefinitions_MatchesFactoryConfigWithLoadedRuntimeDefinitions(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"resources": []map[string]any{},
		"workers": []map[string]any{{
			"name":      "executor",
			"type":      "MODEL_WORKER",
			"model":     "canonical-model",
			"stopToken": "CANONICAL_STOP",
		}},
		"workstations": []map[string]any{{
			"name":      "execute-story",
			"worker":    "executor",
			"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
			"stopWords": []string{"CANONICAL"},
		}},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
---
Run tests.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
stopWords: ["DONE"]
limits:
  maxRetries: 2
---
Fallback body.
`)
	if err := os.WriteFile(filepath.Join(factoryDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	factoryCfg, err := loadFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("loadFactoryConfig: %v", err)
	}
	runtimeDefs, err := loadRuntimeDefinitionLookupMapsFromFactoryConfig(factoryDir, factoryCfg, InlineRuntimeDefinitionOptions{
		RequireSplitDefinitions: true,
	})
	if err != nil {
		t.Fatalf("loadRuntimeDefinitionLookupMapsFromFactoryConfig: %v", err)
	}

	inlined, err := InlineRuntimeDefinitions(factoryDir, factoryCfg, InlineRuntimeDefinitionOptions{
		RequireSplitDefinitions: true,
	})
	if err != nil {
		t.Fatalf("InlineRuntimeDefinitions: %v", err)
	}
	merged, err := FactoryConfigWithRuntimeDefinitions(factoryCfg, runtimeDefs)
	if err != nil {
		t.Fatalf("FactoryConfigWithRuntimeDefinitions: %v", err)
	}

	if !reflect.DeepEqual(inlined, merged) {
		t.Fatalf("inline and lookup-merged factory configs differ\ninline: %#v\nlookup: %#v", inlined, merged)
	}
}
