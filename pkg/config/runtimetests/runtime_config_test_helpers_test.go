package runtimetests

import (
	"encoding/json"
	. "github.com/portpowered/infinite-you/pkg/config"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

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

func canonicalMergeRuntimeDefinitions() interfaces.RuntimeDefinitionLookup {
	runtimeDefs := testRuntimeDefinitionLookup{
		workers:      map[string]*interfaces.WorkerConfig{},
		workstations: map[string]*interfaces.FactoryWorkstationConfig{},
	}
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
	return runtimeDefs
}

func emptyRuntimeDefinitionLookup() interfaces.RuntimeDefinitionLookup {
	return testRuntimeDefinitionLookup{
		workers:      map[string]*interfaces.WorkerConfig{},
		workstations: map[string]*interfaces.FactoryWorkstationConfig{},
	}
}

type testRuntimeDefinitionLookup struct {
	workers      map[string]*interfaces.WorkerConfig
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (lookup testRuntimeDefinitionLookup) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := lookup.workers[name]
	return worker, ok
}

func (lookup testRuntimeDefinitionLookup) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := lookup.workstations[name]
	return workstation, ok
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
		OnFailure:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
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

func assertMergedWorkerConfig(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if cfg == nil || len(cfg.Workers) == 0 {
		t.Fatalf("expected merged worker config, got %#v", cfg)
	}
	worker := cfg.Workers[0]
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

func assertMergedWorkstationConfig(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if cfg == nil || len(cfg.Workstations) == 0 {
		t.Fatalf("expected merged workstation config, got %#v", cfg)
	}
	workstation := cfg.Workstations[0]
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

func assertCanonicalMergeWorkerConfig(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if cfg == nil || len(cfg.Workers) == 0 {
		t.Fatalf("expected canonical worker config, got %#v", cfg)
	}
	worker := cfg.Workers[0]
	if worker.Type != interfaces.WorkerTypeModel || worker.Model != "canonical-model" {
		t.Fatalf("canonical worker fields were not preserved: %#v", worker)
	}
}

func assertCanonicalMergeWorkstation(t *testing.T, lookup interfaces.RuntimeDefinitionLookup) {
	t.Helper()
	workstation, ok := lookup.Workstation("review")
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

func assertCanonicalMergeWorkstationConfig(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if cfg == nil || len(cfg.Workstations) == 0 {
		t.Fatalf("expected canonical workstation config, got %#v", cfg)
	}
	workstation := cfg.Workstations[0]
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
				"name":    "execute-" + project,
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "Implement {{ .WorkID }}.",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayload): %v", err)
	}
	return data
}

func namedFactoryPayloadWithBundledFiles(t *testing.T, project string) []byte {
	t.Helper()

	cfg := map[string]any{
		"name": project,
		"id":   project,
		"supportingFiles": map[string]any{
			"bundledFiles": []map[string]any{
				{
					"type":       "ROOT_HELPER",
					"targetPath": "Makefile",
					"content": map[string]string{
						"encoding": "utf-8",
						"inline":   "test:\n\tgo test ./...\n",
					},
				},
				{
					"type":       "DOC",
					"targetPath": "factory/docs/README.md",
					"content": map[string]string{
						"encoding": "utf-8",
						"inline":   "# Portable factory\n",
					},
				},
				{
					"type":       "SCRIPT",
					"targetPath": "factory/scripts/execute-story.ps1",
					"content": map[string]string{
						"encoding": "utf-8",
						"inline":   "Write-Output 'portable script'\n",
					},
				},
				{
					"type":       "INPUT",
					"targetPath": "factory/inputs/task/default/starter.md",
					"content": map[string]string{
						"encoding": "utf-8",
						"inline":   "starter work\n",
					},
				},
			},
		},
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
				"name":    "execute-" + project,
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "Implement {{ .WorkID }}.",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayloadWithBundledFiles): %v", err)
	}
	return data
}

func assertRuntimeFactoryFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}

func assertRuntimeFactoryFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %v, want %v", path, got, want)
	}
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
