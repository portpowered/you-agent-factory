package runtimetests

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func legacyEncodedNamedFactorySegment(name string) string {
	return strings.NewReplacer("%", "%25", "/", "%2F").Replace(name)
}

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

func ownerFactoryDefinitionValidator() factorydefinitions.Validator {
	return factoryvalidation.New(nil)
}

func ownerFactoryDefinitionPersistence() factorydefinitions.Persistence {
	validator := ownerFactoryDefinitionValidator()
	mapper := factorymapping.NewFactoryConfigMapper()
	fileSystem := platformfilesystem.Local{}
	writer := factoryauthoredlayout.NewWriter(
		authoredmapping.RenderWorkerAgentsMarkdown,
		authoredmapping.RenderWorkstationAgentsMarkdown,
		authoredmapping.RenderAgentsBody,
		factoryauthoredlayout.NewAgentsFileWriter(fileSystem),
		authoredmapping.SafeFactoryLayoutSegment,
		authoredmapping.SafePromptFilePath,
		fileSystem,
		inboxgitkeep.NewLocal(fileSystem),
	)
	persistence, err := factorypersistence.New(
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, factorydefinitioncomposition.LoadCanonicalJSON)
		},
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return factoryauthoredlayout.Prepare(
				ctx,
				segment,
				payload,
				validator,
				mapper.Expand,
				authoredmapping.AuthoredFactoryConfigForExpandedLayout,
				mapper.Flatten,
			)
		},
		func(
			targetDir string,
			prepared *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			return writer.WritePrepared(
				targetDir,
				prepared,
				sourcePath,
				portableconfig.NewMaterializer(platformfilesystem.Local{}),
				factorydefinitioncomposition.PruneRemovedDocs,
			)
		},
		func(targetDir string) error {
			_, err := factorydefinitioncomposition.LoadDirectory(targetDir, nil)
			return err
		},
		nil,
		nil,
		nil,
		platformfilesystem.Local{},
		factorydefinitioncomposition.NamedPaths().RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		panic(err)
	}
	return persistence
}

func assertCanonicalRuntimeConfigLookupFactoryDir(t *testing.T, lookup factorydefinitions.RuntimeConfigLookup, want string) {
	t.Helper()
	if lookup.FactoryDir() != want {
		t.Fatalf("FactoryDir = %q, want %q", lookup.FactoryDir(), want)
	}
}

func assertCanonicalRuntimeConfigLookupRuntimeBaseDir(t *testing.T, lookup factorydefinitions.RuntimeConfigLookup, want string) {
	t.Helper()
	if lookup.RuntimeBaseDir() != want {
		t.Fatalf("RuntimeBaseDir = %q, want %q", lookup.RuntimeBaseDir(), want)
	}
}

func assertCanonicalRuntimeDefinitionLookupByName(t *testing.T, lookup factorydefinitions.RuntimeDefinitionLookup, workerName string, workstationName string) {
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

func assertRuntimeDefinitionLookupMissesByName(t *testing.T, lookup factorydefinitions.RuntimeDefinitionLookup, workerName string, workstationName string) {
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

func canonicalMergeFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "story",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "ready", Type: factorydefinitions.StateTypeProcessing},
				{Name: "approved", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name:      "executor",
			Type:      factorydefinitions.WorkerTypeModel,
			Model:     "canonical-model",
			StopToken: "CANONICAL_STOP",
			Timeout:   "20m",
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{canonicalMergeWorkstation()},
	}
}

func canonicalMergeRuntimeDefinitions() factorydefinitions.RuntimeDefinitionLookup {
	runtimeDefs := testRuntimeDefinitionLookup{
		workers:      map[string]*factorydefinitions.FactoryWorkerConfig{},
		workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{},
	}
	runtimeDefs.workers["executor"] = &factorydefinitions.FactoryWorkerConfig{
		Type:        factorydefinitions.WorkerTypeScript,
		Command:     "go",
		Args:        []string{"test", "./..."},
		Concurrency: 3,
		Body:        "runtime worker body",
	}
	runtimeDefs.workstations["review"] = &factorydefinitions.FactoryWorkstationConfig{
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "runtime-worker",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "ready"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "approved"}},
		Timeout:        "5m",
		Limits:         factorydefinitions.WorkstationLimits{MaxRetries: 3},
		StopWords:      []string{"RUNTIME"},
		PromptTemplate: "Runtime prompt.",
		Env:            map[string]string{"SHARED": "runtime", "RUNTIME_ONLY": "true"},
	}
	return runtimeDefs
}

func emptyRuntimeDefinitionLookup() factorydefinitions.RuntimeDefinitionLookup {
	return testRuntimeDefinitionLookup{
		workers:      map[string]*factorydefinitions.FactoryWorkerConfig{},
		workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{},
	}
}

type testRuntimeDefinitionLookup struct {
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
}

func (lookup testRuntimeDefinitionLookup) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	worker, ok := lookup.workers[name]
	return worker, ok
}

func (lookup testRuntimeDefinitionLookup) Workstation(name string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	workstation, ok := lookup.workstations[name]
	return workstation, ok
}

func canonicalMergeWorkstation() factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		ID:               "review-id",
		Name:             "review",
		Kind:             factorydefinitions.WorkstationKindCron,
		Type:             factorydefinitions.WorkstationTypeLogical,
		WorkerTypeName:   "executor",
		Cron:             &factorydefinitions.CronConfig{Schedule: "*/5 * * * *"},
		Inputs:           []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "init"}},
		Outputs:          []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		OnFailure:        []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		Resources:        []factorydefinitions.ResourceConfig{{Name: "agent-slot", Capacity: 1}},
		StopWords:        []string{"CANONICAL"},
		PromptTemplate:   "Canonical prompt.",
		Timeout:          "30m",
		Limits:           factorydefinitions.WorkstationLimits{MaxRetries: 1, MaxExecutionTime: "40m"},
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
	if worker.Type != factorydefinitions.WorkerTypeScript || worker.Command != "go" || worker.Concurrency != 3 {
		t.Fatalf("runtime worker fields did not override canonical fields: %#v", worker)
	}
	if worker.Model != "canonical-model" || worker.StopToken != "CANONICAL_STOP" || worker.Timeout != "20m" {
		t.Fatalf("canonical worker fields without runtime equivalents were not preserved: %#v", worker)
	}
}

func assertMergedWorkerConfig(t *testing.T, cfg *factorydefinitions.FactoryConfig) {
	t.Helper()
	if cfg == nil || len(cfg.Workers) == 0 {
		t.Fatalf("expected merged worker config, got %#v", cfg)
	}
	worker := cfg.Workers[0]
	if worker.Type != factorydefinitions.WorkerTypeScript || worker.Command != "go" || worker.Concurrency != 3 {
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
	if workstation.ID != "review-id" || workstation.Kind != factorydefinitions.WorkstationKindCron || workstation.Cron.Schedule != "*/5 * * * *" {
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

func assertMergedWorkstationConfig(t *testing.T, cfg *factorydefinitions.FactoryConfig) {
	t.Helper()
	if cfg == nil || len(cfg.Workstations) == 0 {
		t.Fatalf("expected merged workstation config, got %#v", cfg)
	}
	workstation := cfg.Workstations[0]
	if workstation.Inputs[0].StateName != "ready" || workstation.Outputs[0].StateName != "approved" {
		t.Fatalf("runtime workstation states did not override canonical states: %#v", workstation)
	}
	if workstation.ID != "review-id" || workstation.Kind != factorydefinitions.WorkstationKindCron || workstation.Cron.Schedule != "*/5 * * * *" {
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

func assertCanonicalMergeWorkerConfig(t *testing.T, cfg *factorydefinitions.FactoryConfig) {
	t.Helper()
	if cfg == nil || len(cfg.Workers) == 0 {
		t.Fatalf("expected canonical worker config, got %#v", cfg)
	}
	worker := cfg.Workers[0]
	if worker.Type != factorydefinitions.WorkerTypeModel || worker.Model != "canonical-model" {
		t.Fatalf("canonical worker fields were not preserved: %#v", worker)
	}
}

func assertCanonicalMergeWorkstation(t *testing.T, lookup factorydefinitions.RuntimeDefinitionLookup) {
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

func assertCanonicalMergeWorkstationConfig(t *testing.T, cfg *factorydefinitions.FactoryConfig) {
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
	secondLookup, ok := loaded.Workstation("execute-story")
	if !ok || secondLookup != workstation {
		t.Fatal("expected runtime lookup to return a stable canonical workstation entry")
	}
	if workstation.ID != "execute-story-id" || workstation.Kind != factorydefinitions.WorkstationKindStandard {
		t.Fatalf("expected topology fields on canonical workstation, got %#v", workstation)
	}
	assertCanonicalInlineRuntimeFields(t, workstation)
}

func assertCanonicalInlineRuntimeFields(t *testing.T, workstation *factorydefinitions.FactoryWorkstationConfig) {
	t.Helper()
	if workstation.Type != factorydefinitions.WorkstationTypeModel {
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
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), data, 0o644); err != nil {
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
					{"name": "failed", "type": "FAILED"},
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
				"onFailure": []map[string]string{
					{"workType": "task", "state": "failed"},
				},
				"type": "MODEL_WORKSTATION",
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
					{"name": "failed", "type": "FAILED"},
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
				"onFailure": []map[string]string{
					{"workType": "task", "state": "failed"},
				},
				"type": "MODEL_WORKSTATION",
				"body": "Implement {{ .WorkID }}.",
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
	if runtime.GOOS == "windows" {
		return
	}
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

func runtimeFactoryWithoutLayout() map[string]any {
	return map[string]any{
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
			"id":      "execute-story",
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			"type":    "MODEL_WORKSTATION",
		}},
	}
}

func portableLayoutFixture(nodeID, edgeID string) map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"nodes": []map[string]any{{
			"id": nodeID,
			"position": map[string]any{
				"x": 128,
				"y": 256,
			},
			"size": map[string]any{
				"width":  320,
				"height": 180,
			},
			"locked": true,
		}},
		"edges": []map[string]any{{
			"id": edgeID,
			"waypoints": []map[string]any{{
				"x": 180,
				"y": 220,
			}},
			"labelPosition": map[string]any{
				"x": 200,
				"y": 210,
			},
		}},
		"groups": []map[string]any{{
			"id":      "group-1",
			"label":   "Main lane",
			"nodeIds": []string{nodeID},
			"bounds": map[string]any{
				"x":      100,
				"y":      200,
				"width":  400,
				"height": 240,
			},
		}},
		"viewport": map[string]any{
			"x":    40,
			"y":    60,
			"zoom": 0.9,
		},
		"preferences": map[string]any{
			"direction": "RIGHT",
		},
	}
}

func namedFactoryPayloadWithPortableLayout(t *testing.T, project string) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(namedFactoryPayload(t, project), &payload); err != nil {
		t.Fatalf("Unmarshal(namedFactoryPayload): %v", err)
	}
	payload["layout"] = portableLayoutFixture(
		"workstation:execute-"+project,
		"workstation-output:workstation:execute-"+project+"->work-state:task:complete",
	)
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayload with layout): %v", err)
	}
	return updated
}

func assertLoadedPortableLayoutConfig(t *testing.T, cfg *factorydefinitions.FactoryConfig, wantNodeID string) {
	t.Helper()

	layout := requirePortableLayoutFixture(t, cfg)
	assertPortableLayoutFixtureNode(t, layout, wantNodeID)
	assertPortableLayoutFixtureEdge(t, layout)
	assertPortableLayoutFixtureGroup(t, layout, wantNodeID)
	assertPortableLayoutFixtureViewportPreferences(t, layout)
}

func requirePortableLayoutFixture(t *testing.T, cfg *factorydefinitions.FactoryConfig) *factorydefinitions.FactoryLayoutConfig {
	t.Helper()

	if cfg == nil || cfg.Layout == nil {
		t.Fatalf("expected portable layout on loaded factory config, got %#v", cfg)
	}
	if cfg.Layout.SchemaVersion != 1 {
		t.Fatalf("layout schemaVersion = %d, want 1", cfg.Layout.SchemaVersion)
	}
	return cfg.Layout
}

func assertPortableLayoutFixtureNode(t *testing.T, layout *factorydefinitions.FactoryLayoutConfig, wantNodeID string) {
	t.Helper()

	if len(layout.Nodes) != 1 || layout.Nodes[0].ID != wantNodeID {
		t.Fatalf("layout nodes = %#v, want %s", layout.Nodes, wantNodeID)
	}
	if layout.Nodes[0].Position.X != 128 || layout.Nodes[0].Position.Y != 256 {
		t.Fatalf("layout node position = %#v, want x=128 y=256", layout.Nodes[0].Position)
	}
}

func assertPortableLayoutFixtureEdge(t *testing.T, layout *factorydefinitions.FactoryLayoutConfig) {
	t.Helper()

	if len(layout.Edges) != 1 || layout.Edges[0].ID == "" {
		t.Fatalf("layout edges = %#v, want one edge", layout.Edges)
	}
	if len(layout.Edges[0].Waypoints) != 1 || layout.Edges[0].Waypoints[0].X != 180 {
		t.Fatalf("layout edge waypoints = %#v, want one waypoint at x=180", layout.Edges[0].Waypoints)
	}
	if layout.Edges[0].LabelPosition == nil || layout.Edges[0].LabelPosition.X != 200 {
		t.Fatalf("layout edge labelPosition = %#v, want x=200", layout.Edges[0].LabelPosition)
	}
}

func assertPortableLayoutFixtureGroup(t *testing.T, layout *factorydefinitions.FactoryLayoutConfig, wantNodeID string) {
	t.Helper()

	if len(layout.Groups) != 1 || layout.Groups[0].ID != "group-1" {
		t.Fatalf("layout groups = %#v, want group-1", layout.Groups)
	}
	if len(layout.Groups[0].NodeIDs) != 1 || layout.Groups[0].NodeIDs[0] != wantNodeID {
		t.Fatalf("layout group nodeIds = %#v, want %s", layout.Groups[0].NodeIDs, wantNodeID)
	}
}

func assertPortableLayoutFixtureViewportPreferences(t *testing.T, layout *factorydefinitions.FactoryLayoutConfig) {
	t.Helper()

	if layout.Viewport == nil || math.Abs(layout.Viewport.Zoom-0.9) > 1e-6 {
		t.Fatalf("layout viewport = %#v, want zoom 0.9", layout.Viewport)
	}
	if layout.Preferences == nil || layout.Preferences.Direction != "RIGHT" {
		t.Fatalf("layout preferences = %#v, want RIGHT direction", layout.Preferences)
	}
}

func assertPortableLayoutJSONPayload(t *testing.T, value any, wantNodeID string) {
	t.Helper()

	layout, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("persisted layout = %#v, want object", value)
	}
	if got := layout["schemaVersion"]; got != float64(1) {
		t.Fatalf("persisted layout schemaVersion = %#v, want 1", got)
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("persisted layout nodes = %#v, want one node", layout["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok || node["id"] != wantNodeID {
		t.Fatalf("persisted layout node = %#v, want %s", nodes[0], wantNodeID)
	}
	edges, ok := layout["edges"].([]any)
	if !ok || len(edges) != 1 {
		t.Fatalf("persisted layout edges = %#v, want one edge", layout["edges"])
	}
	groups, ok := layout["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("persisted layout groups = %#v, want one group", layout["groups"])
	}
	viewport, ok := layout["viewport"].(map[string]any)
	if !ok || viewport["zoom"] != 0.9 {
		t.Fatalf("persisted layout viewport = %#v, want zoom 0.9", layout["viewport"])
	}
	preferences, ok := layout["preferences"].(map[string]any)
	if !ok || preferences["direction"] != "RIGHT" {
		t.Fatalf("persisted layout preferences = %#v, want RIGHT", layout["preferences"])
	}
}

func assertPortableLayoutJSONBytes(t *testing.T, payload []byte, wantNodeID string) {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal(flattened payload): %v", err)
	}
	assertPortableLayoutJSONPayload(t, decoded["layout"], wantNodeID)
}
