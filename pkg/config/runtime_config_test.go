package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-config-fixture review=2026-07-18 removal=split-runtime-config-fixture-before-next-runtime-config-change
func TestLoadRuntimeConfig_LoadsEffectiveRuntimeConfig(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
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
				"id":       "execute-story",
				"name":     "execute-story",
				"behavior": "REPEATER",
				"worker":   "executor",
				"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":  []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: </promise>
---
You are the executor worker.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
stopWords: ["DONE"]
---
Implement {{ .WorkID }}.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	if loaded.FactoryConfig() == nil {
		t.Fatal("expected Factory to be loaded")
	}
	if loaded.FactoryConfig().Workstations[0].ID != "execute-story" {
		t.Fatalf("expected workstation id execute-story, got %q", loaded.FactoryConfig().Workstations[0].ID)
	}
	if loaded.FactoryConfig().Resources[0].Capacity != 2 {
		t.Fatalf("expected resource capacity 2, got %d", loaded.FactoryConfig().Resources[0].Capacity)
	}

	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected executor worker definition")
	}
	if workerDef.StopToken != "</promise>" {
		t.Fatalf("expected stop token </promise>, got %q", workerDef.StopToken)
	}

	workstationDef, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected execute-story workstation definition")
	}
	if workstationDef.WorkerTypeName != "executor" {
		t.Fatalf("expected workstation worker executor, got %q", workstationDef.WorkerTypeName)
	}
	if workstationDef.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("unexpected prompt template %q", workstationDef.PromptTemplate)
	}

	workstationByName, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected workstation lookup by name")
	}
	if workstationByName.Type != "MODEL_WORKSTATION" {
		t.Fatalf("expected workstation type MODEL_WORKSTATION, got %q", workstationByName.Type)
	}
}

func TestLoadRuntimeConfig_PreservesFactoryInferenceThrottleGuards(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"guards": []map[string]any{{
			"type":          "INFERENCE_THROTTLE_GUARD",
			"modelProvider": "CLAUDE",
			"model":         "claude-sonnet-4-5-20250514",
			"refreshWindow": "3s",
		}},
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{
			"name": "claude-worker",
		}},
		"workstations": []map[string]any{{
			"name":    "process-claude",
			"worker":  "claude-worker",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "claude-worker", `---
type: MODEL_WORKER
model: claude-sonnet-4-5-20250514
modelProvider: claude
stopToken: COMPLETE
---
Claude worker.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "process-claude", `---
type: MODEL_WORKSTATION
worker: claude-worker
---
Process.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if len(loaded.FactoryConfig().Guards) != 1 {
		t.Fatalf("expected one preserved factory guard, got %#v", loaded.FactoryConfig().Guards)
	}
	guard := loaded.FactoryConfig().Guards[0]
	if guard.Type != interfaces.GuardTypeInferenceThrottle || guard.ModelProvider != "claude" || guard.Model != "claude-sonnet-4-5-20250514" || guard.RefreshWindow != "3s" {
		t.Fatalf("preserved factory guard = %#v", guard)
	}
}

func TestLoadRuntimeConfig_MergesInlineWorkerMetadataWithBodyOnlyAgentsFile(t *testing.T) {
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
				"name":             "executor",
				"type":             "MODEL_WORKER",
				"model":            "claude-sonnet-4-20250514",
				"modelProvider":    "CLAUDE",
				"executorProvider": "SCRIPT_WRAP",
				"stopToken":        "COMPLETE",
				"timeout":          "20m",
				"skipPermissions":  true,
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", "You are the body-only worker.\n")
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
---
Execute {{ .WorkID }}.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected executor worker definition")
	}
	if workerDef.Type != interfaces.WorkerTypeModel || workerDef.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("worker type/model = %#v", workerDef)
	}
	if workerDef.ModelProvider != "claude" || workerDef.ExecutorProvider != "script_wrap" {
		t.Fatalf("worker providers = %#v", workerDef)
	}
	if workerDef.StopToken != "COMPLETE" || workerDef.Timeout != "20m" || !workerDef.SkipPermissions {
		t.Fatalf("worker runtime fields = %#v", workerDef)
	}
	if workerDef.Body != "You are the body-only worker." {
		t.Fatalf("worker body = %q", workerDef.Body)
	}
}

func TestLoadRuntimeConfig_LoadsHostedLinearWorkerFrontmatter(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "queued", "type": "PROCESSING"},
			},
		}},
		"workers": []map[string]any{{"name": "linear-poller"}},
		"workstations": []map[string]any{{
			"name":     "poll-linear",
			"behavior": "POLLER",
			"worker":   "linear-poller",
			"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "story", "state": "queued"}},
		}},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "linear-poller", `---
type: HOSTED_WORKER
provider: LINEAR
auth:
  secretRef: secrets/linear-api-key
linear:
  pollInterval: 30s
  teamIds: ["team-a"]
  mapping:
    workType: story
    state: init
  claim:
    assigneeField: assignee.email
---
Hosted Linear poller.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "poll-linear", `---
worker: linear-poller
---
Poll Linear.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	workerDef, ok := loaded.Worker("linear-poller")
	if !ok {
		t.Fatal("expected linear-poller worker definition")
	}
	if workerDef.Type != interfaces.WorkerTypeHosted || workerDef.Provider != interfaces.HostedWorkerProviderLinear {
		t.Fatalf("hosted worker identity = %#v", workerDef)
	}
	if workerDef.Auth == nil || workerDef.Auth.SecretRef != "secrets/linear-api-key" {
		t.Fatalf("hosted worker auth = %#v", workerDef.Auth)
	}
	if workerDef.Linear == nil || workerDef.Linear.Mapping.WorkType != "story" || workerDef.Linear.Mapping.State != "init" {
		t.Fatalf("hosted worker linear mapping = %#v", workerDef.Linear)
	}
}

func TestLoadRuntimeConfig_RejectsMissingBodyOnlyWorkerAgentsFileForSplitAuthoredLayout(t *testing.T) {
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
				"stopToken":     "COMPLETE",
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
	})
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
---
Execute {{ .WorkID }}.
`)
	if err := os.MkdirAll(filepath.Join(factoryDir, "workers", "executor"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workerDir): %v", err)
	}

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected missing body-only worker AGENTS.md to fail")
	}
	if !strings.Contains(err.Error(), `load worker "executor" config`) {
		t.Fatalf("expected worker path context in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `worker "executor" is missing body-only AGENTS.md content required by the split authored layout`) {
		t.Fatalf("expected missing body-only worker error, got %v", err)
	}
}

func TestLoadRuntimeConfig_RejectsMissingBodyOnlyWorkstationAgentsFileForSplitAuthoredLayout(t *testing.T) {
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
				"name":  "executor",
				"type":  "MODEL_WORKER",
				"body":  "You are the inline worker.",
				"model": "claude-sonnet-4-20250514",
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"env":     map[string]string{"PROJECT": "demo"},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(factoryDir, "workstations", "execute-story"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workstationDir): %v", err)
	}

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected missing body-only workstation AGENTS.md to fail")
	}
	if !strings.Contains(err.Error(), `load workstation "execute-story" config`) {
		t.Fatalf("expected workstation path context in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstation "execute-story" is missing body-only AGENTS.md content required by the split authored layout`) {
		t.Fatalf("expected missing body-only workstation error, got %v", err)
	}
}

func TestLoadRuntimeConfig_RejectsRetiredSplitWorkerAliases(t *testing.T) {
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
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: MODEL_WORKER
model: gpt-5.4
executorProvider: codex-cli
provider: script_wrap
---
Rejected worker alias.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
---
Run the work.
`)

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected retired split worker alias to be rejected")
	}
	if got := err.Error(); got == "" || !containsAll(got, `load worker "executor" config`, "frontmatter.provider is not supported; use executorProvider") {
		t.Fatalf("expected provider retirement guidance, got %v", err)
	}
}

func TestLoadRuntimeConfig_RejectsMissingRequiredFactoryName(t *testing.T) {
	factoryDir := t.TempDir()

	writePortableNameOmittedFactoryJSON := map[string]any{
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
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
	}
	data, err := json.MarshalIndent(writePortableNameOmittedFactoryJSON, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	_, err = LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected missing factory.name to be rejected")
	}
	if !containsAll(err.Error(), generatedFactoryBoundaryErrorPrefix, "factory.name is required") {
		t.Fatalf("expected missing factory.name boundary error, got %v", err)
	}
}

func TestLoadRuntimeConfig_RejectsRetiredExhaustionRulesWithMigrationGuidance(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "failed", "type": "FAILED"},
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
				"outputs": []map[string]string{{"workType": "story", "state": "failed"}},
			},
		},
		"exhaustionRules": []map[string]any{
			{
				"name":             "review-loop-cap",
				"watchWorkstation": "execute-story",
				"maxVisits":        3,
				"source":           map[string]string{"workType": "story", "state": "init"},
				"target":           map[string]string{"workType": "story", "state": "failed"},
			},
		},
	})

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected retired exhaustion_rules field to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if got := err.Error(); got == "" || !containsAll(got, "exhaustion_rules is retired", "guarded LOGICAL_MOVE workstation", "visit_count guard") {
		t.Fatalf("expected migration guidance in error, got %v", err)
	}
}

func TestLoadRuntimeConfig_AllowsTopologyOnlyLogicalMoveLoopBreakersWithoutSplitDefinitions(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "failed", "type": "FAILED"},
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
				"outputs": []map[string]string{{"workType": "story", "state": "failed"}},
			},
			{
				"name":    "execute-story-loop-breaker",
				"type":    "LOGICAL_MOVE",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "failed"}},
				"guards": []map[string]any{{
					"type":        "VISIT_COUNT",
					"workstation": "execute-story",
					"maxVisits":   3,
				}},
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	if loaded == nil || loaded.FactoryConfig() == nil {
		t.Fatal("expected effective runtime config to load")
	}
	workstation, ok := loaded.Workstation("execute-story-loop-breaker")
	if !ok {
		t.Fatal("expected loop-breaker workstation to be present")
	}
	if workstation.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop-breaker type = %q, want %q", workstation.Type, interfaces.WorkstationTypeLogical)
	}
	if len(workstation.Guards) != 1 {
		t.Fatalf("loop-breaker guards = %#v, want one visit_count guard", workstation.Guards)
	}
	if workstation.Guards[0].Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("loop-breaker guard type = %q, want %q", workstation.Guards[0].Type, interfaces.GuardTypeVisitCount)
	}
	if workstation.Guards[0].Workstation != "execute-story" || workstation.Guards[0].MaxVisits != 3 {
		t.Fatalf("loop-breaker guard = %#v, want execute-story maxVisits=3", workstation.Guards[0])
	}
	if len(workstation.Outputs) != 1 || workstation.Outputs[0].StateName != "failed" || workstation.Outputs[0].WorkTypeName != "story" {
		t.Fatalf("loop-breaker outputs = %#v, want story:failed", workstation.Outputs)
	}
}

func TestLoadRuntimeConfig_ExposesEffectiveRuntimeDefinitionsThroughCanonicalLookup(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
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
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
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
stopWords: ["DONE"]
---
Runtime prompt.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	var lookup interfaces.RuntimeConfigLookup = loaded

	assertCanonicalRuntimeConfigLookupFactoryDir(t, lookup, factoryDir)
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, lookup, factoryDir)

	worker, ok := lookup.Worker("executor")
	if !ok || worker == nil {
		t.Fatalf("Worker(executor) = %#v ok=%v, want runtime worker hit", worker, ok)
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "go" {
		t.Fatalf("effective worker lookup = %#v, want runtime-applied script worker", worker)
	}
	if loaded.FactoryConfig().Workers[0].Type != worker.Type || loaded.FactoryConfig().Workers[0].Command != worker.Command {
		t.Fatalf("factory worker = %#v, want canonical lookup worker %#v", loaded.FactoryConfig().Workers[0], worker)
	}

	workstation, ok := lookup.Workstation("execute-story")
	if !ok || workstation == nil {
		t.Fatalf("Workstation(execute-story) = %#v ok=%v, want runtime workstation hit", workstation, ok)
	}
	if workstation.PromptTemplate != "Runtime prompt." {
		t.Fatalf("effective workstation lookup prompt = %q, want runtime prompt", workstation.PromptTemplate)
	}
	if loaded.FactoryConfig().Workstations[0].PromptTemplate != workstation.PromptTemplate {
		t.Fatalf("factory workstation = %#v, want canonical lookup workstation %#v", loaded.FactoryConfig().Workstations[0], workstation)
	}

	assertRuntimeDefinitionLookupMissesByName(t, lookup, "missing-worker", "missing-workstation")
}
