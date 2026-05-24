package config

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestLoadRuntimeConfig_LoadsCronWorkstationConfig(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "ready", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"resources": []map[string]any{},
		"workers":   []map[string]any{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"id":       "daily-refresh",
				"name":     "daily-refresh",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron": map[string]any{
					"schedule":       "*/5 * * * *",
					"triggerAtStart": true,
					"jitter":         "5s",
					"expiryWindow":   "45s",
				},
				"inputs": []map[string]string{
					{"workType": "task", "state": "ready"},
				},
				"outputs": []map[string]string{
					{"workType": "task", "state": "init"},
				},
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	cronCfg, ok := loaded.Workstation("daily-refresh")
	if !ok {
		t.Fatal("expected daily-refresh workstation config")
	}
	if cronCfg.Kind != "cron" {
		t.Fatalf("expected cron kind, got %q", cronCfg.Kind)
	}
	if cronCfg.Cron == nil || cronCfg.Cron.Schedule != "*/5 * * * *" {
		t.Fatalf("expected cron schedule to load intact, got %+v", cronCfg.Cron)
	}
	if !cronCfg.Cron.TriggerAtStart {
		t.Fatalf("expected cron triggerAtStart to load intact, got %+v", cronCfg.Cron)
	}
	if cronCfg.Cron.Jitter != "5s" {
		t.Fatalf("expected cron jitter to load intact, got %+v", cronCfg.Cron)
	}
	if cronCfg.Cron.ExpiryWindow != "45s" {
		t.Fatalf("expected cron expiry window to load intact, got %+v", cronCfg.Cron)
	}
	if len(cronCfg.Inputs) != 1 || cronCfg.Inputs[0].WorkTypeName != "task" || cronCfg.Inputs[0].StateName != "ready" {
		t.Fatalf("expected cron input requirement to load intact, got %+v", cronCfg.Inputs)
	}
	if len(cronCfg.Outputs) != 1 || cronCfg.Outputs[0].WorkTypeName != "task" || cronCfg.Outputs[0].StateName != "init" {
		t.Fatalf("expected cron output mapping to load intact, got %+v", cronCfg.Outputs)
	}
}

func TestLoadRuntimeConfig_DecodesOmittedTriggerAtStartAsFalse(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"resources": []map[string]any{},
		"workers":   []map[string]any{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "daily-refresh",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     map[string]string{"schedule": "0 * * * *"},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	cronCfg, ok := loaded.Workstation("daily-refresh")
	if !ok {
		t.Fatal("expected daily-refresh workstation config")
	}
	if cronCfg.Cron == nil {
		t.Fatal("expected cron config")
	}
	if cronCfg.Cron.TriggerAtStart {
		t.Fatalf("expected omitted triggerAtStart to decode as false, got %+v", cronCfg.Cron)
	}
}

func TestLoadRuntimeConfig_RejectsRetiredLegacyAliasesAtGeneratedBoundary(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "ready", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name":           "executor",
				"type":           "MODEL_WORKER",
				"model_provider": "anthropic",
			},
		},
		"workstations": []map[string]any{
			{
				"name":     "scheduled-story",
				"behavior": "CRON",
				"worker":   "executor",
				"cron": map[string]any{
					"schedule":         "*/5 * * * *",
					"trigger_at_start": true,
				},
				"outputs": []map[string]string{
					{"workType": "story", "state": "complete"},
				},
			},
		},
	})

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected retired generated-boundary aliases to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workers[0].model_provider is not supported; use modelProvider") {
		t.Fatalf("expected legacy model_provider rejection, got %v", err)
	}
}

func TestLoadRuntimeConfig_UsesCanonicalResourcesCapacity(t *testing.T) {
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
		"resources": []map[string]any{
			{"name": "agent-slot", "capacity": 2},
		},
		"workers": []map[string]any{
			{"name": "executor"},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
				"resources": []map[string]any{
					{"name": "agent-slot", "capacity": 2},
				},
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if len(loaded.FactoryConfig().Workstations) != 1 {
		t.Fatalf("expected one workstation, got %d", len(loaded.FactoryConfig().Workstations))
	}
	if loaded.FactoryConfig().Workstations[0].Resources[0].Capacity != 2 {
		t.Fatalf("expected resources capacity 2, got %d", loaded.FactoryConfig().Workstations[0].Resources[0].Capacity)
	}
}

func TestLoadRuntimeConfig_RejectsLegacyResourceUsageAliasAtGeneratedBoundary(t *testing.T) {
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
		"resources": []map[string]any{
			{"name": "agent-slot", "capacity": 2},
		},
		"workers": []map[string]any{},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
				"resource_usage": []map[string]any{
					{"name": "agent-slot", "capacity": 2},
				},
			},
		},
	})

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected legacy resource_usage alias to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].resource_usage is not supported; use resources") {
		t.Fatalf("expected resource_usage retirement guidance, got %v", err)
	}
}

func TestLoadRuntimeConfig_RejectsUnsupportedGeneratedBoundaryField(t *testing.T) {
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
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{
			{
				"name":             "execute-story",
				"worker":           "executor",
				"inputs":           []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":          []map[string]string{{"workType": "story", "state": "complete"}},
				"unsupportedField": true,
			},
		},
	})

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected unsupported factory.json boundary field to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), `json: unknown field "unsupportedField"`) {
		t.Fatalf("expected generated boundary unknown-field error, got %v", err)
	}
}

func TestLoadRuntimeConfig_RejectsRetiredCronIntervalAtGeneratedBoundary(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "ready", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{
			{
				"name":     "daily-refresh",
				"behavior": "CRON",
				"worker":   "executor",
				"outputs":  []map[string]string{{"workType": "task", "state": "complete"}},
				"cron":     map[string]any{"interval": "5m"},
			},
		},
	})

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected retired cron.interval factory.json boundary field to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].cron.interval is not supported; use cron.schedule") {
		t.Fatalf("expected retired cron.interval error, got %v", err)
	}
}

func TestLoadRuntimeConfig_RejectsMissingRequiredToolInResourceManifest(t *testing.T) {
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
		"workers": []map[string]any{
			{"name": "executor"},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
		"supportingFiles": map[string]any{
			"requiredTools": []map[string]any{
				{
					"name":    "missing helper",
					"command": "portos-missing-helper-for-runtime-validation",
				},
			},
		},
	})

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected missing required tool to fail")
	}
	if !containsAll(err.Error(), "required-tool-missing", `resourceManifest.requiredTools[0].command`, `command "portos-missing-helper-for-runtime-validation" was not found on PATH`) {
		t.Fatalf("expected required-tool validation error, got %v", err)
	}
}

func TestLoadRuntimeConfig_LoadsInlineRuntimeDefinitionsWithoutAgentsFiles(t *testing.T) {
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
		"workers": []map[string]any{
			{
				"name":          "executor",
				"type":          "MODEL_WORKER",
				"model":         "claude-sonnet-4-20250514",
				"modelProvider": "CLAUDE",
				"stopToken":     "COMPLETE",
				"body":          "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"id":        "execute-story",
				"name":      "execute-story",
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
				"type":      "MODEL_WORKSTATION",
				"stopWords": []string{"DONE"},
				"body":      "Implement {{ .WorkID }}.",
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected inline executor worker definition")
	}
	if workerDef.Type != "MODEL_WORKER" {
		t.Fatalf("expected worker type MODEL_WORKER, got %q", workerDef.Type)
	}
	if workerDef.ModelProvider != "claude" {
		t.Fatalf("expected model provider claude, got %q", workerDef.ModelProvider)
	}
	if workerDef.StopToken != "COMPLETE" {
		t.Fatalf("expected stop token COMPLETE, got %q", workerDef.StopToken)
	}
	if workerDef.Body != "You are the executor." {
		t.Fatalf("unexpected worker body %q", workerDef.Body)
	}

	workstationDef, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected inline execute-story workstation definition")
	}
	if workstationDef.Type != "MODEL_WORKSTATION" {
		t.Fatalf("expected workstation type MODEL_WORKSTATION, got %q", workstationDef.Type)
	}
	if workstationDef.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("unexpected prompt template %q", workstationDef.PromptTemplate)
	}
	if len(workstationDef.StopWords) != 1 || workstationDef.StopWords[0] != "DONE" {
		t.Fatalf("expected stop words [DONE], got %#v", workstationDef.StopWords)
	}
}

func TestLoadRuntimeConfig_PreservesInlineWorkerOperationsWithoutAgentsFiles(t *testing.T) {
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
		"workers": []map[string]any{
			{
				"name":          "executor",
				"type":          "MODEL_WORKER",
				"model":         "claude-sonnet-4-20250514",
				"modelProvider": "CLAUDE",
				"body":          "You are the executor.",
				"operations": []map[string]any{
					{
						"name": "TTS",
						"inputs": []map[string]any{
							{"name": "text", "contentTypes": []string{"TEXT"}, "required": true},
						},
						"outputs": []map[string]any{
							{"name": "audio", "contentTypes": []string{"AUDIO"}},
						},
					},
				},
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "Implement {{ .WorkID }}.",
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected inline executor worker definition")
	}
	if len(workerDef.Operations) != 1 {
		t.Fatalf("expected one inline operation, got %#v", workerDef.Operations)
	}
	operation := workerDef.Operations[0]
	if operation.Name != "TTS" {
		t.Fatalf("expected TTS operation, got %#v", operation)
	}
	if len(operation.Inputs) != 1 || operation.Inputs[0].Name != "text" || !operation.Inputs[0].Required {
		t.Fatalf("expected text input slot to load intact, got %#v", operation.Inputs)
	}
	if !reflect.DeepEqual(operation.Inputs[0].ContentTypes, []string{"TEXT"}) {
		t.Fatalf("expected text input content types [TEXT], got %#v", operation.Inputs[0].ContentTypes)
	}
	if len(operation.Outputs) != 1 || operation.Outputs[0].Name != "audio" {
		t.Fatalf("expected audio output slot to load intact, got %#v", operation.Outputs)
	}
	if !reflect.DeepEqual(operation.Outputs[0].ContentTypes, []string{"AUDIO"}) {
		t.Fatalf("expected audio output content types [AUDIO], got %#v", operation.Outputs[0].ContentTypes)
	}
}

func TestLoadRuntimeConfig_NormalizesInlineWorkstationRuntimeFieldsIntoCanonicalEntry(t *testing.T) {
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
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"id":               "execute-story-id",
				"name":             "execute-story",
				"behavior":         "STANDARD",
				"worker":           "executor",
				"inputs":           []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":          []map[string]string{{"workType": "story", "state": "complete"}},
				"type":             "MODEL_WORKSTATION",
				"promptFile":       "prompt.md",
				"outputSchema":     "schema.json",
				"limits":           map[string]any{"maxRetries": 2, "maxExecutionTime": "30m"},
				"stopWords":        []string{"DONE"},
				"body":             "Implement {{ .WorkID }}.",
				"workingDirectory": "/repo/{{ .WorkID }}",
				"worktree":         "worktrees/{{ .WorkID }}",
				"env":              map[string]string{"PROJECT": "{{ .Project }}"},
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	assertCanonicalInlineWorkstation(t, loaded)
}

func TestLoadRuntimeConfig_MergesSplitRuntimeWorkstationOverInlineRuntimeFields(t *testing.T) {
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
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "SCRIPT_WORKER",
			},
		},
		"workstations": []map[string]any{
			{
				"name":             "execute-story",
				"worker":           "executor",
				"inputs":           []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":          []map[string]string{{"workType": "story", "state": "complete"}},
				"type":             "MODEL_WORKSTATION",
				"body":             "Inline prompt {{ (index .Inputs 0).Name }}.",
				"workingDirectory": "/inline/{{ (index .Inputs 0).Name }}",
				"env":              map[string]string{"SHARED": "inline", "INLINE_ONLY": "true"},
			},
		},
	})
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
workingDirectory: "/runtime/{{ (index .Inputs 0).Name }}"
env:
  SHARED: runtime
  RUNTIME_ONLY: "true"
---
Runtime prompt {{ (index .Inputs 0).Name }}.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	workstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected execute-story workstation definition")
	}
	if workstation.PromptTemplate != "Runtime prompt {{ (index .Inputs 0).Name }}." {
		t.Fatalf("split runtime prompt was ignored, got %q", workstation.PromptTemplate)
	}
	if workstation.Body != "Runtime prompt {{ (index .Inputs 0).Name }}." {
		t.Fatalf("split runtime body was ignored, got %q", workstation.Body)
	}
	if workstation.WorkingDirectory != "/runtime/{{ (index .Inputs 0).Name }}" {
		t.Fatalf("split runtime workingDirectory was ignored, got %q", workstation.WorkingDirectory)
	}
	if workstation.Env["SHARED"] != "runtime" || workstation.Env["INLINE_ONLY"] != "true" || workstation.Env["RUNTIME_ONLY"] != "true" {
		t.Fatalf("expected inline env merged with split runtime override, got %#v", workstation.Env)
	}
}

func TestLoadRuntimeConfig_DerivesCanonicalWorkstationTypeFromWorkerAcrossInlineAndSplitDefinitions(t *testing.T) {
	inlineDir := t.TempDir()
	splitDir := t.TempDir()

	topology := map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "parent",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
				},
			},
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name":    "executor",
				"type":    "SCRIPT_WORKER",
				"command": "echo",
				"args":    []string{"ok"},
			},
		},
		"workstations": []map[string]any{
			{
				"id":      "execute-story-id",
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
	}

	inlineConfig := cloneJSONMap(t, topology)
	inlineConfig["workstations"].([]any)[0].(map[string]any)["body"] = "Inline fallback prompt."
	inlineConfig["workstations"].([]any)[0].(map[string]any)["limits"] = map[string]any{"maxExecutionTime": "15m"}
	inlineConfig["workstations"].([]any)[0].(map[string]any)["env"] = map[string]string{"SHARED": "inline"}

	writeRuntimeFactoryJSON(t, inlineDir, inlineConfig)
	splitConfig := cloneJSONMap(t, topology)
	splitConfig["workstations"].([]any)[0].(map[string]any)["limits"] = map[string]any{"maxExecutionTime": "15m"}
	splitConfig["workstations"].([]any)[0].(map[string]any)["env"] = map[string]string{"SHARED": "inline"}
	writeRuntimeFactoryJSON(t, splitDir, splitConfig)
	writeRuntimeWorkstationAgentsMD(t, splitDir, "execute-story", "Inline fallback prompt.\n")

	inlineLoaded, err := LoadRuntimeConfig(inlineDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(inline): %v", err)
	}
	splitLoaded, err := LoadRuntimeConfig(splitDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(split): %v", err)
	}

	inlineWorkstation, ok := inlineLoaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected inline execute-story workstation")
	}
	splitWorkstation, ok := splitLoaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected split execute-story workstation")
	}
	if inlineWorkstation.Type != interfaces.WorkstationTypeModel || splitWorkstation.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("expected canonical worker-backed model workstation type, got inline=%q split=%q", inlineWorkstation.Type, splitWorkstation.Type)
	}
	if !reflect.DeepEqual(inlineWorkstation, splitWorkstation) {
		t.Fatalf("inline and split worker-backed defaults differ\ninline: %#v\nsplit:  %#v", inlineWorkstation, splitWorkstation)
	}
}

// portos:func-length-exception owner=agent-factory reason=inline-split-workstation-equivalence-fixture review=2026-07-18 removal=split-workstation-equivalence-builders-before-next-runtime-config-change
func TestLoadRuntimeConfig_InlineAndSplitWorkstationsNormalizeToEquivalentCanonicalEntry(t *testing.T) {
	inlineDir := t.TempDir()
	splitDir := t.TempDir()

	topology := inlineSplitCanonicalTopology()
	inlineConfig := configureInlineCanonicalWorkstation(t, topology)
	writeRuntimeFactoryJSON(t, inlineDir, inlineConfig)

	splitConfig := configureSplitCanonicalWorkstation(t, topology)
	writeRuntimeFactoryJSON(t, splitDir, splitConfig)
	writeRuntimeWorkstationAgentsMD(t, splitDir, "execute-story", "Implement {{ .WorkID }}.\n")
	if err := os.WriteFile(filepath.Join(splitDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write split prompt file: %v", err)
	}

	assertEquivalentLoadedWorkstations(t, inlineDir, splitDir, "execute-story")
}

func inlineSplitCanonicalTopology() map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "parent",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
				},
			},
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "child-done", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"resources": []map[string]any{{"name": "agent-slot", "capacity": 2}},
		"workers": []map[string]any{
			{
				"name":            "executor",
				"type":            "SCRIPT_WORKER",
				"command":         "echo",
				"args":            []string{"ok"},
				"timeout":         "10m",
				"stopToken":       "COMPLETE",
				"body":            "Worker context body.",
				"skipPermissions": true,
			},
		},
		"workstations": []map[string]any{
			{
				"id":       "execute-story-id",
				"name":     "execute-story",
				"behavior": "CRON",
				"worker":   "executor",
				"cron":     map[string]any{"schedule": "*/5 * * * *", "triggerAtStart": true, "jitter": "5s", "expiryWindow": "45s"},
				"inputs": []map[string]any{
					{
						"workType": "parent",
						"state":    "init",
					},
					{
						"workType": "story",
						"state":    "init",
						"guards": []map[string]string{{
							"type":        "ALL_CHILDREN_COMPLETE",
							"parentInput": "parent",
						}},
					},
				},
				"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "story", "state": "failed"}},
				"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
				"guards": []map[string]any{
					{"type": "VISIT_COUNT", "workstation": "execute-story", "maxVisits": 3},
				},
			},
		},
	}
}

func configureInlineCanonicalWorkstation(t *testing.T, topology map[string]any) map[string]any {
	t.Helper()
	inlineConfig := cloneJSONMap(t, topology)
	inlineConfig["workstations"].([]any)[0].(map[string]any)["type"] = "MODEL_WORKSTATION"
	inlineConfig["workstations"].([]any)[0].(map[string]any)["promptFile"] = "prompt.md"
	inlineConfig["workstations"].([]any)[0].(map[string]any)["outputSchema"] = "schema.json"
	inlineConfig["workstations"].([]any)[0].(map[string]any)["limits"] = map[string]any{"maxRetries": 2, "maxExecutionTime": "30m"}
	inlineConfig["workstations"].([]any)[0].(map[string]any)["stopWords"] = []string{"DONE"}
	inlineConfig["workstations"].([]any)[0].(map[string]any)["body"] = "Implement {{ .WorkID }}."
	inlineConfig["workstations"].([]any)[0].(map[string]any)["workingDirectory"] = "/repo/{{ .WorkID }}"
	inlineConfig["workstations"].([]any)[0].(map[string]any)["worktree"] = "worktrees/{{ .WorkID }}"
	inlineConfig["workstations"].([]any)[0].(map[string]any)["env"] = map[string]string{"PROJECT": "{{ .Project }}"}
	return inlineConfig
}

func configureSplitCanonicalWorkstation(t *testing.T, topology map[string]any) map[string]any {
	t.Helper()
	splitConfig := cloneJSONMap(t, topology)
	splitConfig["workstations"].([]any)[0].(map[string]any)["type"] = "MODEL_WORKSTATION"
	splitConfig["workstations"].([]any)[0].(map[string]any)["promptFile"] = "prompt.md"
	splitConfig["workstations"].([]any)[0].(map[string]any)["outputSchema"] = "schema.json"
	splitConfig["workstations"].([]any)[0].(map[string]any)["limits"] = map[string]any{"maxRetries": 2, "maxExecutionTime": "30m"}
	splitConfig["workstations"].([]any)[0].(map[string]any)["stopWords"] = []string{"DONE"}
	splitConfig["workstations"].([]any)[0].(map[string]any)["workingDirectory"] = "/repo/{{ .WorkID }}"
	splitConfig["workstations"].([]any)[0].(map[string]any)["worktree"] = "worktrees/{{ .WorkID }}"
	splitConfig["workstations"].([]any)[0].(map[string]any)["env"] = map[string]string{"PROJECT": "{{ .Project }}"}
	return splitConfig
}

func assertEquivalentLoadedWorkstations(t *testing.T, inlineDir, splitDir, workstationName string) {
	t.Helper()
	inlineLoaded, err := LoadRuntimeConfig(inlineDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(inline): %v", err)
	}
	splitLoaded, err := LoadRuntimeConfig(splitDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(split): %v", err)
	}

	inlineWorkstation, ok := inlineLoaded.Workstation(workstationName)
	if !ok {
		t.Fatalf("expected inline %s workstation", workstationName)
	}
	splitWorkstation, ok := splitLoaded.Workstation(workstationName)
	if !ok {
		t.Fatalf("expected split %s workstation", workstationName)
	}
	if !reflect.DeepEqual(inlineWorkstation, splitWorkstation) {
		t.Fatalf("inline and split workstations differ\ninline: %#v\nsplit:  %#v", inlineWorkstation, splitWorkstation)
	}
}

func TestNewLoadedFactoryConfig_MergesRuntimeDefinitionsOntoCanonicalConfig(t *testing.T) {
	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), canonicalMergeRuntimeDefinitions())
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	assertMergedWorker(t, loaded)
	assertMergedWorkstation(t, loaded)
}

func TestFactoryConfigWithRuntimeDefinitions_MergesRuntimeDefinitionsOntoCanonicalConfig(t *testing.T) {
	inlined, err := FactoryConfigWithRuntimeDefinitions(canonicalMergeFactoryConfig(), canonicalMergeRuntimeDefinitions())
	if err != nil {
		t.Fatalf("FactoryConfigWithRuntimeDefinitions: %v", err)
	}

	assertMergedWorkerConfig(t, inlined)
	assertMergedWorkstationConfig(t, inlined)
}

func TestFactoryConfigWithRuntimeDefinitions_PreservesCanonicalWorkstationKindForMapping(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "ready", Type: interfaces.StateTypeProcessing},
				{Name: "approved", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{
			Name:  "executor",
			Type:  interfaces.WorkerTypeModel,
			Model: "canonical-model",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			Kind:           interfaces.WorkstationKindRepeater,
			Type:           interfaces.WorkstationTypeLogical,
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "approved"}},
		}},
	}
	runtimeDefs := newRuntimeDefinitionLookupMaps(0, 1)
	runtimeDefs.workstations["review"] = &interfaces.FactoryWorkstationConfig{
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "executor",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "ready"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "approved"}},
		Limits:         interfaces.WorkstationLimits{MaxRetries: 3},
	}

	inlined, err := FactoryConfigWithRuntimeDefinitions(cfg, runtimeDefs)
	if err != nil {
		t.Fatalf("FactoryConfigWithRuntimeDefinitions: %v", err)
	}
	if inlined.Workstations[0].Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("merged workstation kind = %q, want repeater", inlined.Workstations[0].Kind)
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), inlined)
	if err != nil {
		t.Fatalf("Map merged factory config: %v", err)
	}

	transition := net.Transitions["review"]
	if transition == nil {
		t.Fatal("expected review transition")
	}
	if len(transition.RejectionArcs) != 1 || transition.RejectionArcs[0].PlaceID != "story:ready" {
		t.Fatalf("merged repeater rejection arcs = %+v, want loopback to story:ready", transition.RejectionArcs)
	}
	if len(transition.FailureArcs) != 1 || transition.FailureArcs[0].PlaceID != "story:failed" {
		t.Fatalf("merged repeater failure arcs = %+v, want story:failed", transition.FailureArcs)
	}
}

func TestFactoryConfigWithRuntimeDefinitions_UsesCanonicalDefinitionsWhenRuntimeDefinitionsAreMissing(t *testing.T) {
	inlined, err := FactoryConfigWithRuntimeDefinitions(canonicalMergeFactoryConfig(), newRuntimeDefinitionLookupMaps(0, 0))
	if err != nil {
		t.Fatalf("FactoryConfigWithRuntimeDefinitions: %v", err)
	}

	assertCanonicalMergeWorkerConfig(t, inlined)
	assertCanonicalMergeWorkstationConfig(t, inlined)
}

func TestNewLoadedFactoryConfig_UsesCanonicalDefinitionsWhenRuntimeDefinitionsAreMissing(t *testing.T) {
	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), newRuntimeDefinitionLookupMaps(0, 0))
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	var lookup interfaces.RuntimeDefinitionLookup = loaded

	worker, ok := lookup.Worker("executor")
	if !ok {
		t.Fatal("expected canonical worker")
	}
	if worker.Type != interfaces.WorkerTypeModel || worker.Model != "canonical-model" {
		t.Fatalf("canonical worker fields were not preserved: %#v", worker)
	}
	assertCanonicalMergeWorkstation(t, lookup)
}

func TestNewLoadedFactoryConfig_LoadsCanonicalConfigWithoutRuntimeConfig(t *testing.T) {
	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	var lookup interfaces.RuntimeConfigLookup = loaded

	assertCanonicalRuntimeConfigLookupFactoryDir(t, lookup, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, lookup, "factory-dir")
	assertCanonicalRuntimeDefinitionLookupByName(t, lookup, "executor", "review")
	assertRuntimeDefinitionLookupMissesByName(t, lookup, "missing-worker", "missing-workstation")
	assertCanonicalMergeWorkstation(t, lookup)
}

func TestLoadedFactoryConfig_RuntimeBaseDirOverrideAndFallbackKeepsCanonicalLookupContract(t *testing.T) {
	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	var lookup interfaces.RuntimeConfigLookup = loaded

	assertCanonicalRuntimeConfigLookupFactoryDir(t, lookup, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, lookup, "factory-dir")

	loaded.SetRuntimeBaseDir(" runtime-base/child/.. ")

	assertCanonicalRuntimeConfigLookupFactoryDir(t, lookup, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, lookup, "runtime-base")

	loaded.SetRuntimeBaseDir(" \t ")

	assertCanonicalRuntimeConfigLookupFactoryDir(t, lookup, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, lookup, "factory-dir")
}

func TestLoadedFactoryConfig_SetRuntimeBaseDirNilReceiverNoops(t *testing.T) {
	var loaded *LoadedFactoryConfig
	var lookup interfaces.RuntimeConfigLookup = loaded

	loaded.SetRuntimeBaseDir("runtime-base")

	assertCanonicalRuntimeConfigLookupFactoryDir(t, lookup, "")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, lookup, "")
}

func TestLoadedFactoryConfig_RuntimeLookupNilReceiverReturnsMisses(t *testing.T) {
	var loaded *LoadedFactoryConfig
	var lookup interfaces.RuntimeDefinitionLookup = loaded

	assertRuntimeDefinitionLookupMissesByName(t, lookup, "executor", "review")
}
