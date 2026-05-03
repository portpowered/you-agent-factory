package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestPersistNamedFactory_WritesCanonicalNamedLayout(t *testing.T) {
	rootDir := t.TempDir()

	factoryDir, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	wantDir := filepath.Join(rootDir, "alpha")
	if factoryDir != wantDir {
		t.Fatalf("factory dir = %q, want %q", factoryDir, wantDir)
	}
	for _, path := range []string{
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		filepath.Join(factoryDir, interfaces.InputsDir),
		filepath.Join(factoryDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-alpha", interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected persisted named-factory path %s: %v", path, err)
		}
	}

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(persisted named factory): %v", err)
	}
	if loaded.FactoryConfig().Project != "alpha" {
		t.Fatalf("project = %q, want alpha", loaded.FactoryConfig().Project)
	}
}

func TestPersistNamedFactory_RejectsDuplicateNames(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("first PersistNamedFactory: %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha-second")); err == nil {
		t.Fatal("expected duplicate named factory to fail")
	} else if !strings.Contains(err.Error(), `factory "alpha" already exists`) {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestPersistNamedFactory_RejectsInvalidNames(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "../alpha", namedFactoryPayload(t, "alpha")); err == nil {
		t.Fatal("expected invalid factory name to fail")
	} else if !strings.Contains(err.Error(), `factory name "../alpha" cannot contain path separators`) {
		t.Fatalf("expected invalid-name error, got %v", err)
	}
}

func TestPersistNamedFactory_RejectsInvalidPayloadWithoutChangingCurrentFactory(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	invalidPayload, err := json.Marshal(map[string]any{
		"id": "broken",
		"workTypes": []map[string]any{
			{
				"name": "task",
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
				"name":      "execute-broken",
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "failed"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
		"exhaustionRules": []map[string]any{
			{
				"name":             "broken-loop-cap",
				"watchWorkstation": "execute-broken",
				"maxVisits":        3,
				"source":           map[string]string{"workType": "task", "state": "init"},
				"target":           map[string]string{"workType": "task", "state": "failed"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(invalid named factory payload): %v", err)
	}

	if _, err := PersistNamedFactory(rootDir, "broken", invalidPayload); err == nil {
		t.Fatal("expected invalid named factory payload to fail")
	} else if got := err.Error(); !containsAll(got, generatedFactoryBoundaryErrorPrefix, "exhaustion_rules is retired") {
		t.Fatalf("expected generated-boundary validation error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "broken")); !os.IsNotExist(err) {
		t.Fatalf("expected rejected factory directory to be absent, got stat err=%v", err)
	}

	loaded, err := LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(current after invalid persist): %v", err)
	}
	if loaded.FactoryDir() != filepath.Join(rootDir, "alpha") {
		t.Fatalf("FactoryDir after invalid persist = %q, want %q", loaded.FactoryDir(), filepath.Join(rootDir, "alpha"))
	}
	if loaded.FactoryConfig().Project != "alpha" {
		t.Fatalf("project after invalid persist = %q, want alpha", loaded.FactoryConfig().Project)
	}
}

func TestPersistNamedFactory_RollsBackStagedLayoutWhenLoadRuntimeConfigFails(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	_, err := persistNamedFactory(rootDir, "broken", namedFactoryPayload(t, "broken"), namedFactoryPersistHooks{
		afterWrite: func(stagingDir string) error {
			path := filepath.Join(stagingDir, interfaces.WorkstationsDir, "execute-broken", interfaces.FactoryAgentsFileName)
			return os.WriteFile(path, []byte("---\ntype: [\n"), 0o644)
		},
	})
	if err == nil {
		t.Fatal("expected staged named-factory validation failure")
	}
	if got := err.Error(); !containsAll(got, `validate factory "broken" config`, "load workstation", "AGENTS.md missing closing frontmatter delimiter") {
		t.Fatalf("expected load-time validation error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "broken")); !os.IsNotExist(err) {
		t.Fatalf("expected failed staged factory directory to be absent, got stat err=%v", err)
	}

	loaded, err := LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(current after staged failure): %v", err)
	}
	if loaded.FactoryDir() != filepath.Join(rootDir, "alpha") {
		t.Fatalf("FactoryDir after staged failure = %q, want %q", loaded.FactoryDir(), filepath.Join(rootDir, "alpha"))
	}
	if loaded.FactoryConfig().Project != "alpha" {
		t.Fatalf("project after staged failure = %q, want alpha", loaded.FactoryConfig().Project)
	}
}

func TestLoadRuntimeConfig_UsesCurrentFactoryPointerFromNamedLayout(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	loaded, err := LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(named root): %v", err)
	}
	if loaded.FactoryDir() != filepath.Join(rootDir, "beta") {
		t.Fatalf("FactoryDir = %q, want %q", loaded.FactoryDir(), filepath.Join(rootDir, "beta"))
	}
	if loaded.FactoryConfig().Project != "beta" {
		t.Fatalf("project = %q, want beta", loaded.FactoryConfig().Project)
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
				"id":             "execute-story",
				"name":           "execute-story",
				"worker":         "executor",
				"inputs":         []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":        []map[string]string{{"workType": "story", "state": "complete"}},
				"type":           "MODEL_WORKSTATION",
				"stopWords":      []string{"DONE"},
				"body": "Implement {{ .WorkID }}.",
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
				"onFailure": map[string]string{"workType": "story", "state": "failed"},
				"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
				"guards": []map[string]any{
					{"type": "VISIT_COUNT", "workstation": "execute-story", "maxVisits": 3},
				},
			},
		},
	}

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

	writeRuntimeFactoryJSON(t, inlineDir, inlineConfig)
	splitConfig := cloneJSONMap(t, topology)
	splitConfig["workstations"].([]any)[0].(map[string]any)["type"] = "MODEL_WORKSTATION"
	splitConfig["workstations"].([]any)[0].(map[string]any)["promptFile"] = "prompt.md"
	splitConfig["workstations"].([]any)[0].(map[string]any)["outputSchema"] = "schema.json"
	splitConfig["workstations"].([]any)[0].(map[string]any)["limits"] = map[string]any{"maxRetries": 2, "maxExecutionTime": "30m"}
	splitConfig["workstations"].([]any)[0].(map[string]any)["stopWords"] = []string{"DONE"}
	splitConfig["workstations"].([]any)[0].(map[string]any)["workingDirectory"] = "/repo/{{ .WorkID }}"
	splitConfig["workstations"].([]any)[0].(map[string]any)["worktree"] = "worktrees/{{ .WorkID }}"
	splitConfig["workstations"].([]any)[0].(map[string]any)["env"] = map[string]string{"PROJECT": "{{ .Project }}"}
	writeRuntimeFactoryJSON(t, splitDir, splitConfig)
	writeRuntimeWorkstationAgentsMD(t, splitDir, "execute-story", "Implement {{ .WorkID }}.\n")
	if err := os.WriteFile(filepath.Join(splitDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write split prompt file: %v", err)
	}

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
	if !reflect.DeepEqual(inlineWorkstation, splitWorkstation) {
		t.Fatalf("inline and split workstations differ\ninline: %#v\nsplit:  %#v", inlineWorkstation, splitWorkstation)
	}
}

func TestNewLoadedFactoryConfig_MergesRuntimeDefinitionsOntoCanonicalConfig(t *testing.T) {
	runtimeDefs := newRuntimeDefinitionConfig(1, 1)
	runtimeDefs.workers["executor"] = &interfaces.WorkerConfig{
		Type:        interfaces.WorkerTypeScript,
		Command:     "go",
		Args:        []string{"test", "./..."},
		Concurrency: 3,
		Body:        "runtime worker body",
	}
	runtimeDefs.workstations["review"] = &interfaces.FactoryWorkstationConfig{
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "runtime-worker",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "ready"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "approved"}},
		Timeout:        "5m",
		Limits:         interfaces.WorkstationLimits{MaxRetries: 3},
		StopWords:      []string{"RUNTIME"},
		PromptTemplate: "Runtime prompt.",
		Env:            map[string]string{"SHARED": "runtime", "RUNTIME_ONLY": "true"},
	}

	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), runtimeDefs)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	assertMergedWorker(t, loaded)
	assertMergedWorkstation(t, loaded)
}

func TestNewLoadedFactoryConfig_UsesCanonicalDefinitionsWhenRuntimeDefinitionsAreMissing(t *testing.T) {
	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), newRuntimeDefinitionConfig(0, 0))
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected canonical worker")
	}
	if worker.Type != interfaces.WorkerTypeModel || worker.Model != "canonical-model" {
		t.Fatalf("canonical worker fields were not preserved: %#v", worker)
	}
	assertCanonicalMergeWorkstation(t, loaded)
}

func TestNewLoadedFactoryConfig_LoadsCanonicalConfigWithoutRuntimeConfig(t *testing.T) {
	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	assertCanonicalRuntimeConfigLookupFactoryDir(t, loaded, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, loaded, "factory-dir")
	assertCanonicalRuntimeDefinitionLookupByName(t, loaded, "executor", "review")
	assertRuntimeDefinitionLookupMissesByName(t, loaded, "missing-worker", "missing-workstation")
	assertCanonicalMergeWorkstation(t, loaded)
}

func TestLoadedFactoryConfig_RuntimeBaseDirOverrideAndFallbackKeepsCanonicalLookupContract(t *testing.T) {
	loaded, err := NewLoadedFactoryConfig("factory-dir", canonicalMergeFactoryConfig(), nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	assertCanonicalRuntimeConfigLookupFactoryDir(t, loaded, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, loaded, "factory-dir")

	loaded.SetRuntimeBaseDir(" runtime-base/child/.. ")

	assertCanonicalRuntimeConfigLookupFactoryDir(t, loaded, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, loaded, "runtime-base")

	loaded.SetRuntimeBaseDir(" \t ")

	assertCanonicalRuntimeConfigLookupFactoryDir(t, loaded, "factory-dir")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, loaded, "factory-dir")
}

func TestLoadedFactoryConfig_SetRuntimeBaseDirNilReceiverNoops(t *testing.T) {
	var loaded *LoadedFactoryConfig

	loaded.SetRuntimeBaseDir("runtime-base")

	assertCanonicalRuntimeConfigLookupFactoryDir(t, loaded, "")
	assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t, loaded, "")
}

func TestLoadedFactoryConfig_RuntimeLookupNilReceiverReturnsMisses(t *testing.T) {
	var loaded *LoadedFactoryConfig

	assertRuntimeDefinitionLookupMissesByName(t, loaded, "executor", "review")
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

	worker, ok := loaded.Worker("executor")
	if !ok || worker == nil {
		t.Fatalf("Worker(executor) = %#v ok=%v, want runtime worker hit", worker, ok)
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "go" {
		t.Fatalf("effective worker lookup = %#v, want runtime-applied script worker", worker)
	}
	if loaded.FactoryConfig().Workers[0].Type != worker.Type || loaded.FactoryConfig().Workers[0].Command != worker.Command {
		t.Fatalf("factory worker = %#v, want canonical lookup worker %#v", loaded.FactoryConfig().Workers[0], worker)
	}

	workstation, ok := loaded.Workstation("execute-story")
	if !ok || workstation == nil {
		t.Fatalf("Workstation(execute-story) = %#v ok=%v, want runtime workstation hit", workstation, ok)
	}
	if workstation.PromptTemplate != "Runtime prompt." {
		t.Fatalf("effective workstation lookup prompt = %q, want runtime prompt", workstation.PromptTemplate)
	}
	if loaded.FactoryConfig().Workstations[0].PromptTemplate != workstation.PromptTemplate {
		t.Fatalf("factory workstation = %#v, want canonical lookup workstation %#v", loaded.FactoryConfig().Workstations[0], workstation)
	}

	assertRuntimeDefinitionLookupMissesByName(t, loaded, "missing-worker", "missing-workstation")
}

func assertCanonicalRuntimeConfigLookupFactoryDir(t *testing.T, lookup interfaces.RuntimeConfigLookup, want string) {
	t.Helper()
	if lookup.FactoryDir() != want {
		t.Fatalf("FactoryDir = %q, want %q", lookup.FactoryDir(), want)
	}
}

func assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t *testing.T, lookup interfaces.RuntimeConfigLookup, want string) {
	t.Helper()
	if lookup.RuntimeBaseDir() != want {
		t.Fatalf("RuntimeBaseDir = %q, want %q", lookup.RuntimeBaseDir(), want)
	}
}

func assertCanonicalRuntimeDefinitionLookupByName(t *testing.T, lookup interfaces.RuntimeDefinitionLookup, workerName string, workstationName string) {
	t.Helper()
	worker, ok := lookup.Worker(workerName)
	if !ok || worker == nil {
		t.Fatalf("canonical worker lookup %q = %#v ok=%v, want worker", workerName, worker, ok)
	}
	workstation, ok := lookup.Workstation(workstationName)
	if !ok || workstation == nil {
		t.Fatalf("canonical workstation lookup %q = %#v ok=%v, want workstation", workstationName, workstation, ok)
	}
}

func assertRuntimeDefinitionLookupMissesByName(t *testing.T, lookup interfaces.RuntimeDefinitionLookup, workerName string, workstationName string) {
	t.Helper()
	worker, ok := lookup.Worker(workerName)
	if ok || worker != nil {
		t.Fatalf("worker miss %q = %#v ok=%v, want nil false", workerName, worker, ok)
	}
	workstation, ok := lookup.Workstation(workstationName)
	if ok || workstation != nil {
		t.Fatalf("workstation miss %q = %#v ok=%v, want nil false", workstationName, workstation, ok)
	}
}

func canonicalMergeFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
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
			Name:      "executor",
			Type:      interfaces.WorkerTypeModel,
			Model:     "canonical-model",
			StopToken: "CANONICAL_STOP",
			Timeout:   "20m",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{canonicalMergeWorkstation()},
	}
}

func canonicalMergeWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		ID:               "review-id",
		Name:             "review",
		Kind:             interfaces.WorkstationKindCron,
		Type:             interfaces.WorkstationTypeLogical,
		WorkerTypeName:   "executor",
		Cron:             &interfaces.CronConfig{Schedule: "*/5 * * * *"},
		Inputs:           []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
		Outputs:          []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		OnFailure:        &interfaces.IOConfig{WorkTypeName: "story", StateName: "failed"},
		Resources:        []interfaces.ResourceConfig{{Name: "agent-slot", Capacity: 1}},
		StopWords:        []string{"CANONICAL"},
		PromptTemplate:   "Canonical prompt.",
		Timeout:          "30m",
		Limits:           interfaces.WorkstationLimits{MaxRetries: 1, MaxExecutionTime: "40m"},
		WorkingDirectory: "/repo/canonical",
		Env:              map[string]string{"CANONICAL_ONLY": "true", "SHARED": "canonical"},
	}
}

func assertMergedWorker(t *testing.T, loaded *LoadedFactoryConfig) {
	t.Helper()
	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected merged worker")
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "go" || worker.Concurrency != 3 {
		t.Fatalf("runtime worker fields did not override canonical fields: %#v", worker)
	}
	if worker.Model != "canonical-model" || worker.StopToken != "CANONICAL_STOP" || worker.Timeout != "20m" {
		t.Fatalf("canonical worker fields without runtime equivalents were not preserved: %#v", worker)
	}
}

func assertMergedWorkstation(t *testing.T, loaded *LoadedFactoryConfig) {
	t.Helper()
	workstation, ok := loaded.Workstation("review")
	if !ok {
		t.Fatal("expected merged workstation")
	}
	if workstation.Inputs[0].StateName != "ready" || workstation.Outputs[0].StateName != "approved" {
		t.Fatalf("runtime workstation states did not override canonical states: %#v", workstation)
	}
	if workstation.ID != "review-id" || workstation.Kind != interfaces.WorkstationKindCron || workstation.Cron.Schedule != "*/5 * * * *" {
		t.Fatalf("canonical workstation topology fields were not preserved: %#v", workstation)
	}
	if workstation.Limits.MaxRetries != 3 || workstation.Limits.MaxExecutionTime != "5m" {
		t.Fatalf("workstation limits were not merged: %#v", workstation.Limits)
	}
	if workstation.Timeout != "" {
		t.Fatalf("expected canonical workstation timeout alias to be cleared, got %#v", workstation)
	}
	if workstation.Env["CANONICAL_ONLY"] != "true" || workstation.Env["SHARED"] != "runtime" || workstation.Env["RUNTIME_ONLY"] != "true" {
		t.Fatalf("workstation env was not merged with runtime override: %#v", workstation.Env)
	}
}

func assertCanonicalMergeWorkstation(t *testing.T, loaded *LoadedFactoryConfig) {
	t.Helper()
	workstation, ok := loaded.Workstation("review")
	if !ok {
		t.Fatal("expected canonical workstation")
	}
	if workstation.Inputs[0].StateName != "init" || workstation.Outputs[0].StateName != "failed" {
		t.Fatalf("canonical workstation states were not preserved: %#v", workstation)
	}
	if workstation.PromptTemplate != "Canonical prompt." || workstation.Timeout != "" || workstation.Limits.MaxExecutionTime != "40m" {
		t.Fatalf("canonical workstation runtime fields were not preserved: %#v", workstation)
	}
}

func assertCanonicalInlineWorkstation(t *testing.T, loaded *LoadedFactoryConfig) {
	t.Helper()
	workstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected execute-story workstation definition")
	}
	if loaded.WorkstationConfigs()["execute-story"] != workstation {
		t.Fatal("expected runtime lookup to return the canonical workstation map entry")
	}
	if workstation.ID != "execute-story-id" || workstation.Kind != interfaces.WorkstationKindStandard {
		t.Fatalf("expected topology fields on canonical workstation, got %#v", workstation)
	}
	assertCanonicalInlineRuntimeFields(t, workstation)
}

func assertCanonicalInlineRuntimeFields(t *testing.T, workstation *interfaces.FactoryWorkstationConfig) {
	t.Helper()
	if workstation.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("expected type MODEL_WORKSTATION, got %q", workstation.Type)
	}
	if workstation.WorkerTypeName != "executor" {
		t.Fatalf("expected worker executor, got %q", workstation.WorkerTypeName)
	}
	if workstation.PromptFile != "prompt.md" || workstation.OutputSchema != "schema.json" {
		t.Fatalf("expected prompt file and output schema, got %#v", workstation)
	}
	if workstation.Timeout != "" || workstation.Limits.MaxRetries != 2 || workstation.Limits.MaxExecutionTime != "30m" {
		t.Fatalf("expected canonical execution limits, got %#v", workstation)
	}
	if len(workstation.StopWords) != 1 || workstation.StopWords[0] != "DONE" {
		t.Fatalf("expected stop words [DONE], got %#v", workstation.StopWords)
	}
	if workstation.Body != "Implement {{ .WorkID }}." || workstation.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("expected body and prompt template, got body=%q prompt=%q", workstation.Body, workstation.PromptTemplate)
	}
	if workstation.WorkingDirectory != "/repo/{{ .WorkID }}" || workstation.Worktree != "worktrees/{{ .WorkID }}" {
		t.Fatalf("expected execution paths, got %#v", workstation)
	}
	if workstation.Env["PROJECT"] != "{{ .Project }}" {
		t.Fatalf("expected env PROJECT template, got %#v", workstation.Env)
	}
}

// TODO: this should not fail.
// func TestLoadRuntimeConfig_RejectsPartialInlineRuntimeDefinitionsWithoutAgentsFiles(t *testing.T) {
// 	factoryDir := t.TempDir()

// 	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
// 		"workTypes": []map[string]any{
// 			{
// 				"name": "story",
// 				"states": []map[string]string{
// 					{"name": "init", "type": "INITIAL"},
// 					{"name": "complete", "type": "TERMINAL"},
// 				},
// 			},
// 		},
// 		"workers": []map[string]any{
// 			{
// 				"name": "executor",
// 				"type": "MODEL_WORKER",
// 			},
// 		},
// 		"workstations": []map[string]any{
// 			{
// 				"name":    "execute-story",
// 				"worker":  "executor",
// 				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
// 				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
// 			},
// 		},
// 	})

// 	_, err := LoadRuntimeConfig(factoryDir, nil)
// 	if err == nil {
// 		t.Fatal("expected partial inline factory config to fail")
// 	}
// 	if !strings.Contains(err.Error(), "inline factory definition is incomplete") {
// 		t.Fatalf("expected clear inline factory definition error, got %v", err)
// 	}
// 	if !strings.Contains(err.Error(), "workstation \"execute-story\"") {
// 		t.Fatalf("expected error to identify missing workstation definition, got %v", err)
// 	}
// }

func writeRuntimeFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
}

func cloneJSONMap(t *testing.T, cfg map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return cloned
}

func containsAll(value string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(value, substring) {
			return false
		}
	}
	return true
}

func namedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()

	cfg := map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":           "execute-" + project,
				"worker":         "executor",
				"inputs":         []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":        []map[string]string{{"workType": "task", "state": "complete"}},
				"type":           "MODEL_WORKSTATION",
				"body": "Implement {{ .WorkID }}.",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayload): %v", err)
	}
	return data
}

func writeRuntimeWorkerAgentsMD(t *testing.T, factoryDir, workerName, content string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workerDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(worker AGENTS.md): %v", err)
	}
}

func writeRuntimeWorkstationAgentsMD(t *testing.T, factoryDir, workstationName, content string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workstationDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(workstation AGENTS.md): %v", err)
	}
}
