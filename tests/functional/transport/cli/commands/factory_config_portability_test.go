package commands_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryConfigPortability_ExpandThenFlattenPreservesSemanticConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("slow factory config portability")
	}

	dir := t.TempDir()
	original := portableExpandFactoryFixtureJSON()
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	writeFatFactoryJSON(t, dir, string(original))

	providerRunner := support.NewRecordingCommandRunner("expand/flatten must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: providerRunner}

	expandOutput := runFactoryConfigCommand(t, edges, "expand", factoryPath)
	assertExpandCommandOutput(t, expandOutput, "Expanded factory config into")

	loadedExpanded, err := support.LoadedFactory(t, dir)
	if err != nil {
		t.Fatalf("expanded factory should load through runtime config after split expansion: %v", err)
	}
	assertExpandedPortableFactoryLayout(t, dir, loadedExpanded)

	want := canonicalFactoryPayload(t, original)
	got := canonicalFactoryPayloadFromFlatten(t, edges, runFactoryConfigCommand(t, edges, "flatten", dir))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"expanded then flattened config changed semantics\nwant: %s\ngot:  %s",
			prettyJSON(t, want),
			prettyJSON(t, got),
		)
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for expand/flatten", providerRunner.CallCount())
	}
}

func TestFactoryConfigPortability_FlattenSplitLayoutExecutesStandalone(t *testing.T) {
	if testing.Short() {
		t.Skip("slow factory config portability")
	}

	splitDir := t.TempDir()
	writeFatFactoryJSON(t, splitDir, `{
  "name": "portable-split-layout-factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "resources": [{ "name": "agent-slot", "capacity": 1 }],
  "workers": [{ "name": "executor" }],
  "workstations": [
    {
      "name": "execute-task",
      "behavior": "STANDARD",
      "worker": "executor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    }
  ]
}`)
	writeFactoryTestFile(t, filepath.Join(splitDir, "workers", "executor", "AGENTS.md"), `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
executorProvider: script_wrap
resources:
  - name: agent-slot
    capacity: 1
stopToken: COMPLETE
---

You are the split factory executor.`)
	writeFactoryTestFile(t, filepath.Join(splitDir, "workstations", "execute-task", "AGENTS.md"), `---
type: MODEL_WORKSTATION
worker: executor
stopWords: ["DONE"]
---

Complete {{ (index .Inputs 0).WorkID }} from split config.`)

	flattenRunner := support.NewRecordingCommandRunner("flatten must not execute providers")
	flattened := runFactoryConfigCommand(t, serviceedges.Edges{ProviderCommandRunner: flattenRunner}, "flatten", splitDir)
	if flattenRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for split flatten", flattenRunner.CallCount())
	}

	flattenedCfg, err := support.DecodeFactoryDefinition(flattened)
	if err != nil {
		t.Fatalf("flattened split config should parse: %v", err)
	}
	if flattenedCfg.Workers == nil || len(*flattenedCfg.Workers) != 1 ||
		(*flattenedCfg.Workers)[0].ModelProvider == nil || string(*(*flattenedCfg.Workers)[0].ModelProvider) != "CLAUDE" {
		t.Fatalf("flattened worker definition missing split AGENTS.md fields: %#v", flattenedCfg.Workers)
	}
	if flattenedCfg.Workstations == nil || len(*flattenedCfg.Workstations) != 1 ||
		(*flattenedCfg.Workstations)[0].Type == nil ||
		(*flattenedCfg.Workstations)[0].Body == nil || *(*flattenedCfg.Workstations)[0].Body != "Complete {{ (index .Inputs 0).WorkID }} from split config." {
		t.Fatalf("flattened workstation runtime config missing split AGENTS.md fields: %#v", flattenedCfg.Workstations)
	}

	standaloneDir := t.TempDir()
	writeFatFactoryJSON(t, standaloneDir, string(flattened))
	testutil.WriteSeedFile(t, standaloneDir, "task", []byte(`{"title":"flattened split factory"}`))

	providerRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(
			`{"type":"result","subtype":"success","is_error":false,"result":"Finished from flattened split config. DONE COMPLETE","session_id":"portable-split"}` + "\n",
		)},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, standaloneDir, serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
	}, 10*time.Second)
	assertPortableTaskCompleted(t, listed)
	if providerRunner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want 1 for split standalone execution", providerRunner.CallCount())
	}
}

func TestFatFactory_StandaloneCanonicalFileExecutesWithInlineDefinitions(t *testing.T) {
	if testing.Short() {
		t.Skip("slow factory config portability")
	}

	dir := t.TempDir()
	writeFatFactoryJSON(t, dir, `{
  "name": "portable-standalone-factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "resources": [],
  "workers": [
    {
      "name": "executor",
      "type": "MODEL_WORKER",
      "model": "claude-sonnet-4-20250514",
      "modelProvider": "CLAUDE",
      "stopToken": "COMPLETE",
      "body": "You are the standalone factory executor."
    }
  ],
  "workstations": [
    {
      "name": "execute-task",
      "behavior": "STANDARD",
      "worker": "executor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "definition": {
        "type": "MODEL_WORKSTATION",
        "worker": "executor",
        "body": "Complete {{ (index .Inputs 0).WorkID }}.",
        "stopWords": ["DONE"]
      }
    }
  ]
}`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"standalone fat factory"}`))

	providerRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(
			`{"type":"result","subtype":"success","is_error":false,"result":"Finished from inline config. DONE COMPLETE","session_id":"portable-standalone"}` + "\n",
		)},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
	}, 10*time.Second)
	assertPortableTaskCompleted(t, listed)
	if providerRunner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want 1 for inline standalone execution", providerRunner.CallCount())
	}
}

func TestFatFactory_LoadOnlyStandaloneFileUsesSharedMappingPath(t *testing.T) {
	if testing.Short() {
		t.Skip("slow factory config portability")
	}

	dir := t.TempDir()
	writeFatFactoryJSON(t, dir, `{
  "name": "portable-load-only-factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "resources": [],
  "workers": [
    {
      "name": "executor",
      "type": "MODEL_WORKER",
      "modelProvider": "CLAUDE",
      "stopToken": "COMPLETE",
      "body": "You are loaded through the shared mapper."
    }
  ],
  "workstations": [
    {
      "name": "execute-task",
      "behavior": "STANDARD",
      "worker": "executor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "resources": [],
      "definition": {
        "type": "MODEL_WORKSTATION",
        "worker": "executor",
        "body": "Complete {{ (index .Inputs 0).WorkID }}.",
        "stopWords": ["DONE"]
      }
    }
  ]
}`)

	loaded, err := support.LoadedFactory(t, dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.WorkTypes == nil || len(*loaded.WorkTypes) != 1 || (*loaded.WorkTypes)[0].Name != "task" {
		t.Fatalf("expected mapped work type task, got %#v", loaded.WorkTypes)
	}

	worker, ok := support.FindFactoryWorker(loaded, "executor")
	if !ok {
		t.Fatal("expected inline worker definition to load")
	}
	if worker.ModelProvider == nil || string(*worker.ModelProvider) != "CLAUDE" {
		t.Fatalf("expected normalized model provider CLAUDE, got %v", worker.ModelProvider)
	}
	if worker.StopToken == nil || *worker.StopToken != "COMPLETE" {
		t.Fatalf("expected normalized stop token COMPLETE, got %q", stringPtrValue(worker.StopToken))
	}

	workstation, ok := support.FindFactoryWorkstation(loaded, "execute-task")
	if !ok {
		t.Fatal("expected inline workstation definition to load")
	}
	if workstation.Body == nil || *workstation.Body != "Complete {{ (index .Inputs 0).WorkID }}." {
		t.Fatalf("expected normalized prompt template, got %q", stringPtrValue(workstation.Body))
	}
	if workstation.StopWords == nil || len(*workstation.StopWords) != 1 || (*workstation.StopWords)[0] != "DONE" {
		t.Fatalf("expected normalized stop words, got %#v", workstation.StopWords)
	}
}

func TestFactoryConfigPortability_FlattenInlineScriptBackedFactoryExecutesStandalone(t *testing.T) {
	if testing.Short() {
		t.Skip("slow factory config portability")
	}

	authoredDir := writeInlineScriptBackedFactoryFixture(t)
	flattenRunner := support.NewRecordingCommandRunner("flatten must not execute providers")
	flattened := runFactoryConfigCommand(t, serviceedges.Edges{ProviderCommandRunner: flattenRunner}, "flatten", authoredDir)
	if flattenRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for inline-script flatten", flattenRunner.CallCount())
	}
	assertFlattenedInlineScriptBackedConfig(t, flattened)

	standaloneDir := writeFlattenedInlineScriptStandalone(t, flattened)
	assertLoadedInlineScriptBackedStandalone(t, standaloneDir)
	assertFlattenedInlineScriptStandaloneExecutes(t, standaloneDir)
}

func writeInlineScriptBackedFactoryFixture(t *testing.T) string {
	t.Helper()

	authoredDir := t.TempDir()
	writeFatFactoryJSON(t, authoredDir, `{
  "name": "portable-inline-script-factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "resources": [],
  "workers": [{ "name": "executor" }],
  "workstations": [
    {
      "name": "execute-story",
      "behavior": "STANDARD",
      "worker": "executor",
      "copyReferencedScripts": true,
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "type": "MODEL_WORKSTATION",
      "body": "Execute {{ (index .Inputs 0).Payload }}.",
      "workingDirectory": "/repo/{{ (index .Inputs 0).WorkID }}",
      "env": {
        "SCRIPT_MODE": "portable"
      }
    }
  ]
}`)
	writeFactoryTestFile(t, filepath.Join(authoredDir, "workers", "executor", "AGENTS.md"), `---
type: SCRIPT_WORKER
command: powershell
args:
  - -File
  - scripts/execute-story.ps1
timeout: 45m
---
Execute the story script.
`)
	writeFactoryTestFile(t, filepath.Join(authoredDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	return authoredDir
}

func assertFlattenedInlineScriptBackedConfig(t *testing.T, flattened []byte) {
	t.Helper()

	flattenedCfg, err := support.DecodeFactoryDefinition(flattened)
	if err != nil {
		t.Fatalf("flattened inline script-backed config should parse: %v", err)
	}
	if flattenedCfg.Workers == nil || flattenedCfg.Workstations == nil || len(*flattenedCfg.Workers) != 1 || len(*flattenedCfg.Workstations) != 1 {
		t.Fatalf("expected one worker/workstation after flatten: %#v", flattenedCfg)
	}
	worker := (*flattenedCfg.Workers)[0]
	workstation := (*flattenedCfg.Workstations)[0]
	if worker.Type == nil || string(*worker.Type) != string(interfaces.WorkerTypeScript) || worker.Command == nil || *worker.Command != "powershell" {
		t.Fatalf("flattened worker definition = %#v", worker)
	}
	if worker.Args == nil || len(*worker.Args) != 2 || (*worker.Args)[1] != "scripts/execute-story.ps1" {
		t.Fatalf("flattened worker args = %#v", worker.Args)
	}
	if workstation.Type == nil || string(*workstation.Type) != string(interfaces.WorkstationTypeScript) || workstation.CopyReferencedScripts == nil || !*workstation.CopyReferencedScripts {
		t.Fatalf("flattened workstation definition = %#v\nflattened payload: %s", workstation, flattened)
	}
	if workstation.WorkingDirectory == nil || *workstation.WorkingDirectory != "/repo/{{ (index .Inputs 0).WorkID }}" {
		t.Fatalf("flattened workstation working directory = %q", stringPtrValue(workstation.WorkingDirectory))
	}
}

func writeFlattenedInlineScriptStandalone(t *testing.T, flattened []byte) string {
	t.Helper()

	standaloneDir := t.TempDir()
	writeFatFactoryJSON(t, standaloneDir, string(flattened))
	testutil.WriteSeedFile(t, standaloneDir, "task", []byte(`{"title":"inline script-backed flatten"}`))
	return standaloneDir
}

func assertLoadedInlineScriptBackedStandalone(t *testing.T, standaloneDir string) {
	t.Helper()

	loaded, err := support.LoadedFactory(t, standaloneDir)
	if err != nil {
		t.Fatalf("flattened standalone config should load: %v", err)
	}

	worker, ok := support.FindFactoryWorker(loaded, "executor")
	if !ok {
		t.Fatal("expected flattened script worker definition to load")
	}
	if worker.Type == nil || string(*worker.Type) != string(interfaces.WorkerTypeScript) || worker.Command == nil || *worker.Command != "powershell" || worker.Timeout == nil || *worker.Timeout != "45m" {
		t.Fatalf("loaded worker = %#v", worker)
	}

	workstation, ok := support.FindFactoryWorkstation(loaded, "execute-story")
	if !ok {
		t.Fatal("expected flattened inline workstation definition to load")
	}
	if workstation.Type == nil || string(*workstation.Type) != string(interfaces.WorkstationTypeScript) || workstation.WorkingDirectory == nil || *workstation.WorkingDirectory != "/repo/{{ (index .Inputs 0).WorkID }}" {
		t.Fatalf("loaded workstation = %#v", workstation)
	}
}

func assertFlattenedInlineScriptStandaloneExecutes(t *testing.T, standaloneDir string) {
	t.Helper()

	scriptRunner := support.NewRecordingCommandRunner("flattened inline script accepted")
	providerRunner := support.NewRecordingCommandRunner("inline script execution must not call providers")
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, standaloneDir, serviceedges.Edges{
		ScriptCommandRunner:   scriptRunner,
		ProviderCommandRunner: providerRunner,
	}, 10*time.Second)
	assertPortableTaskCompleted(t, listed)
	if scriptRunner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want 1 for inline script standalone execution", scriptRunner.CallCount())
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for script-only execution", providerRunner.CallCount())
	}
}

func assertPortableTaskCompleted(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	for placeID, want := range map[string]int{
		"task:complete": 1,
		"task:init":     0,
		"task:failed":   0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func portableExpandFactoryFixtureJSON() []byte {
	return []byte(`{
  "name": "portable-expand-factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "resources": [{ "name": "agent-slot", "capacity": 1 }],
  "workers": [
    {
      "name": "executor",
      "type": "MODEL_WORKER",
      "model": "claude-sonnet-4-20250514",
      "modelProvider": "CLAUDE",
      "resources": [{ "name": "agent-slot", "capacity": 1 }],
      "stopToken": "COMPLETE",
      "body": "You are the portable factory executor."
    }
  ],
  "workstations": [
    {
      "id": "execute-task-id",
      "name": "execute-task",
      "behavior": "STANDARD",
      "worker": "executor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "resources": [{ "name": "agent-slot", "capacity": 1 }],
      "definition": {
        "type": "MODEL_WORKSTATION",
        "worker": "executor",
        "body": "Complete {{ (index .Inputs 0).WorkID }}.",
        "stopWords": ["DONE"]
      }
    }
  ]
}`)
}

func runFactoryConfigCommand(t *testing.T, edges serviceedges.Edges, subcommand string, target string) []byte {
	t.Helper()

	inputs := support.FakeInputs(context.Background(), []string{"you", "factory", "config", subcommand, target})
	inputs.Input.Env = os.Environ()
	inputs.WorkingDirectory = filepath.Dir(target)
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		inputs.WorkingDirectory = target
	}
	if err := support.BuildProcess(t, edges).Execute(inputs.Input); err != nil {
		t.Fatalf("execute config %s: %v", subcommand, err)
	}
	return []byte(inputs.Stdout())
}

func assertExpandCommandOutput(t *testing.T, output []byte, expected string) {
	t.Helper()
	if !strings.Contains(string(output), expected) {
		t.Fatalf("expected expand result output containing %q, got %q", expected, string(output))
	}
}

func assertExpandedPortableFactoryLayout(t *testing.T, dir string, loadedExpanded factoryapi.Factory) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(dir, "workers", "executor", "AGENTS.md")); err != nil {
		t.Fatalf("expected expand to create worker AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "workstations", "execute-task", "AGENTS.md")); err != nil {
		t.Fatalf("expected expand to create workstation AGENTS.md: %v", err)
	}
	workerAgents, err := os.ReadFile(filepath.Join(dir, "workers", "executor", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read expanded worker AGENTS.md: %v", err)
	}
	if got := string(workerAgents); got != "You are the portable factory executor." {
		t.Fatalf("expanded worker AGENTS.md = %q, want body-only worker content", got)
	}

	workerDef, ok := support.FindFactoryWorker(loadedExpanded, "executor")
	if !ok {
		t.Fatal("expected expanded fat-factory worker definition to load")
	}
	if workerDef.Model == nil || *workerDef.Model != "claude-sonnet-4-20250514" || workerDef.ModelProvider == nil || string(*workerDef.ModelProvider) != "CLAUDE" || workerDef.StopToken == nil || *workerDef.StopToken != "COMPLETE" {
		t.Fatalf("expanded worker definition did not preserve canonical fields: %#v", workerDef)
	}
	if workerDef.Resources == nil || len(*workerDef.Resources) != 1 || (*workerDef.Resources)[0].Name != "agent-slot" || (*workerDef.Resources)[0].Capacity != 1 {
		t.Fatalf("expanded worker resources = %#v, want agent-slot capacity 1", workerDef.Resources)
	}
	if workerDef.Body == nil || *workerDef.Body != "You are the portable factory executor." {
		t.Fatalf("expanded worker body = %q", stringPtrValue(workerDef.Body))
	}

	expandedWorkstation, ok := support.FindFactoryWorkstation(loadedExpanded, "execute-task")
	if !ok {
		t.Fatal("expected expanded fat-factory workstation definition to load")
	}
	if expandedWorkstation.Worker != "executor" || expandedWorkstation.Body == nil || *expandedWorkstation.Body != "Complete {{ (index .Inputs 0).WorkID }}." {
		t.Fatalf("expanded workstation definition did not preserve canonical fields: %#v", expandedWorkstation)
	}
}

func canonicalFactoryPayload(t *testing.T, data []byte) any {
	t.Helper()

	dir := t.TempDir()
	writeFatFactoryJSON(t, dir, string(data))
	flattenRunner := support.NewRecordingCommandRunner("canonical flatten must not execute providers")
	flattened := runFactoryConfigCommand(t, serviceedges.Edges{ProviderCommandRunner: flattenRunner}, "flatten", dir)
	if flattenRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for canonical flatten", flattenRunner.CallCount())
	}
	return canonicalFactoryPayloadFromFlatten(t, serviceedges.Edges{}, flattened)
}

func canonicalFactoryPayloadFromFlatten(t *testing.T, _ serviceedges.Edges, flattened []byte) any {
	t.Helper()

	var payload any
	if err := json.Unmarshal(flattened, &payload); err != nil {
		t.Fatalf("unmarshal canonical factory payload: %v\n%s", err, string(flattened))
	}
	return payload
}

func prettyJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal pretty JSON: %v", err)
	}
	return string(data)
}

func writeFatFactoryJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), []byte(content), 0o644); err != nil {
		t.Fatalf("write fat factory.json: %v", err)
	}
}

func writeFactoryTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
