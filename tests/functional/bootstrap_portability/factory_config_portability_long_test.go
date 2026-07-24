//go:build functionallong

package bootstrap_portability

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
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=legacy-config-portability-fixture review=2026-07-18 removal=split-expand-flatten-fixture-builders-before-next-portability-change
func TestFactoryConfigPortability_ExpandThenFlattenPreservesSemanticConfig(t *testing.T) {
	support.SkipLongFunctional(t, "slow config-portability expand-flatten sweep")
	dir := t.TempDir()
	original := portableExpandFactoryFixtureJSON()
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	writeFatFactoryJSON(t, dir, string(original))

	assertExpandCommandOutput(t, runFactoryConfigCommand(t, "expand", factoryPath), "Expanded factory config into")
	loadedExpanded, err := support.LoadedFactory(t, dir)
	if err != nil {
		t.Fatalf("expanded factory should load through runtime config after split expansion: %v", err)
	}
	assertExpandedPortableFactoryLayout(t, dir, loadedExpanded)

	want := canonicalFactoryPayload(t, original)
	got := canonicalFactoryPayload(t, runFactoryConfigCommand(t, "flatten", dir))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded then flattened config changed semantics\nwant: %s\ngot:  %s", prettyJSON(t, want), prettyJSON(t, got))
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-config-portability-fixture review=2026-07-18 removal=split-split-layout-execution-fixture-before-next-portability-change
func TestFactoryConfigPortability_FlattenSplitLayoutExecutesStandalone(t *testing.T) {
	support.SkipLongFunctional(t, "slow config-portability split-layout sweep")
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

	flattened := runFactoryConfigCommand(t, "flatten", splitDir)

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

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(
			`{"type":"result","subtype":"success","is_error":false,"result":"Finished from flattened split config. DONE COMPLETE","session_id":"portable-split"}` + "\n",
		)},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, standaloneDir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertPortableTaskCompleted(t, listed)
}

func TestFatFactory_StandaloneCanonicalFileExecutesWithInlineDefinitions(t *testing.T) {
	support.SkipLongFunctional(t, "slow fat-factory standalone-execution sweep")
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

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Finished from inline config. DONE COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertPortableTaskCompleted(t, listed)

	if provider.CallCount() != 1 {
		t.Fatalf("expected provider called once, got %d", provider.CallCount())
	}
}

func TestFatFactory_LoadOnlyStandaloneFileUsesSharedMappingPath(t *testing.T) {
	support.SkipLongFunctional(t, "slow fat-factory shared-mapping sweep")
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
		t.Fatalf("expected normalized model provider claude, got %v", worker.ModelProvider)
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
	support.SkipLongFunctional(t, "slow config-portability inline-script sweep")
	authoredDir := writeInlineScriptBackedFactoryFixture(t)
	flattened := flattenFactoryDir(t, authoredDir)
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

func flattenFactoryDir(t *testing.T, dir string) []byte {
	t.Helper()
	return runFactoryConfigCommand(t, "flatten", dir)
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

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, standaloneDir, serviceedges.Edges{
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("flattened inline script accepted"),
	}, 10*time.Second)
	assertPortableTaskCompleted(t, listed)
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

func runFactoryConfigCommand(t *testing.T, subcommand string, target string) []byte {
	t.Helper()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(
		context.Background(),
		[]string{"you", "factory", "config", subcommand, target},
	)
	inputs.WorkingDirectory = filepath.Dir(target)
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		inputs.WorkingDirectory = target
	}
	if err := process.Execute(inputs.Input); err != nil {
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

// NOTE: this shouldn't fail as is.
// func TestFatFactory_PartialCanonicalFileReturnsValidationError(t *testing.T) {
// 	dir := t.TempDir()
// 	writeFatFactoryJSON(t, dir, `{
//   "workTypes": [
//     {
//       "name": "task",
//       "states": [
//         { "name": "init", "type": "INITIAL" },
//         { "name": "complete", "type": "TERMINAL" }
//       ]
//     }
//   ],
//   "workers": [
//     {
//       "name": "executor",
//       "type": "MODEL_WORKER"
//     }
//   ],
//   "workstations": [
//     {
//       "name": "execute-task",
//       "worker": "executor",
//       "inputs": [{ "workType": "task", "state": "init" }],
//       "outputs": [{ "workType": "task", "state": "complete" }]
//     }
//   ]
// }`)

// 	_, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
// 		Dir:    dir,
// 		Logger: zap.NewNop(),
// 	})
// 	if err == nil {
// 		t.Fatal("expected partial standalone factory config to fail")
// 	}
// 	if !strings.Contains(err.Error(), "inline factory definition is incomplete") {
// 		t.Fatalf("expected clear inline factory validation error, got %v", err)
// 	}
// 	if !strings.Contains(err.Error(), "workstation \"execute-task\"") {
// 		t.Fatalf("expected error to identify missing workstation definition, got %v", err)
// 	}
// }

func canonicalFactoryPayload(t *testing.T, data []byte) any {
	t.Helper()

	dir := t.TempDir()
	writeFatFactoryJSON(t, dir, string(data))
	flattened := runFactoryConfigCommand(t, "flatten", dir)

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
