//go:build functionallong

package bootstrap_portability

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/cli"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=legacy-config-portability-fixture review=2026-07-18 removal=split-expand-flatten-fixture-builders-before-next-portability-change
func TestFactoryConfigPortability_ExpandThenFlattenPreservesSemanticConfig(t *testing.T) {
	support.SkipLongFunctional(t, "slow config-portability expand-flatten sweep")
	dir := t.TempDir()
	original := []byte(`{
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
      "onFailure": { "workType": "task", "state": "failed" },
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
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	writeFatFactoryJSON(t, dir, string(original))

	var expandOut bytes.Buffer
	expandCmd := cli.NewRootCommand()
	expandCmd.SetOut(&expandOut)
	expandCmd.SetErr(&bytes.Buffer{})
	expandCmd.SetArgs([]string{"config", "expand", factoryPath})
	if err := expandCmd.Execute(); err != nil {
		t.Fatalf("execute config expand: %v", err)
	}
	if !strings.Contains(expandOut.String(), "Expanded factory config into") {
		t.Fatalf("expected expand result output, got %q", expandOut.String())
	}

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
	if got := string(workerAgents); got != "You are the portable factory executor.\n" {
		t.Fatalf("expanded worker AGENTS.md = %q, want body-only worker content", got)
	}
	expandedWorkstation, err := factoryconfig.LoadWorkstationConfig(filepath.Join(dir, "workstations", "execute-task"))
	if err != nil {
		t.Fatalf("expanded workstation AGENTS.md should load: %v", err)
	}
	if expandedWorkstation.WorkerTypeName != "executor" || expandedWorkstation.PromptTemplate != "Complete {{ (index .Inputs 0).WorkID }}." {
		t.Fatalf("expanded workstation definition did not preserve canonical fields: %#v", expandedWorkstation)
	}

	var flattenOut bytes.Buffer
	flattenCmd := cli.NewRootCommand()
	flattenCmd.SetOut(&flattenOut)
	flattenCmd.SetErr(&bytes.Buffer{})
	flattenCmd.SetArgs([]string{"config", "flatten", dir})
	if err := flattenCmd.Execute(); err != nil {
		t.Fatalf("execute config flatten: %v", err)
	}

	want := canonicalFactoryPayload(t, original)
	got := canonicalFactoryPayload(t, flattenOut.Bytes())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded then flattened config changed semantics\nwant: %s\ngot:  %s", prettyJSON(t, want), prettyJSON(t, got))
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("expanded factory should load through runtime config: %v", err)
	}
	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected expanded fat-factory worker definition to load")
	}
	if workerDef.Model != "claude-sonnet-4-20250514" || workerDef.ModelProvider != "claude" || workerDef.StopToken != "COMPLETE" {
		t.Fatalf("expanded worker definition did not preserve canonical fields: %#v", workerDef)
	}
	if len(workerDef.Resources) != 1 || workerDef.Resources[0].Name != "agent-slot" || workerDef.Resources[0].Capacity != 1 {
		t.Fatalf("expanded worker resources = %#v, want agent-slot capacity 1", workerDef.Resources)
	}
	if workerDef.Body != "You are the portable factory executor." {
		t.Fatalf("expanded worker body = %q", workerDef.Body)
	}
	if _, ok := loaded.Workstation("execute-task"); !ok {
		t.Fatal("expected expanded fat-factory workstation definition to load")
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
      "onFailure": { "workType": "task", "state": "failed" },
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

	var flattenOut bytes.Buffer
	flattenCmd := cli.NewRootCommand()
	flattenCmd.SetOut(&flattenOut)
	flattenCmd.SetErr(&bytes.Buffer{})
	flattenCmd.SetArgs([]string{"config", "flatten", splitDir})
	if err := flattenCmd.Execute(); err != nil {
		t.Fatalf("execute config flatten: %v", err)
	}

	flattenedCfg, err := factoryconfig.NewFactoryConfigMapper().Expand(flattenOut.Bytes())
	if err != nil {
		t.Fatalf("flattened split config should parse: %v", err)
	}
	if flattenedCfg.Workers[0].ModelProvider != "claude" {
		t.Fatalf("flattened worker definition missing split AGENTS.md fields: %#v", flattenedCfg.Workers[0])
	}
	if flattenedCfg.Workstations[0].Type == "" || flattenedCfg.Workstations[0].PromptTemplate != "Complete {{ (index .Inputs 0).WorkID }} from split config." {
		t.Fatalf("flattened workstation runtime config missing split AGENTS.md fields: %#v", flattenedCfg.Workstations[0])
	}

	standaloneDir := t.TempDir()
	writeFatFactoryJSON(t, standaloneDir, flattenOut.String())
	testutil.WriteSeedFile(t, standaloneDir, "task", []byte(`{"title":"flattened split factory"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Finished from flattened split config. DONE COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, standaloneDir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)
	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
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
      "onFailure": { "workType": "task", "state": "failed" },
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
		interfaces.InferenceResponse{Content: "Finished from inline config. DONE COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)

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
      "onFailure": { "workType": "task", "state": "failed" },
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

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if len(loaded.FactoryConfig().WorkTypes) != 1 || loaded.FactoryConfig().WorkTypes[0].Name != "task" {
		t.Fatalf("expected mapped work type task, got %#v", loaded.FactoryConfig().WorkTypes)
	}

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected inline worker definition to load")
	}
	if worker.ModelProvider != "claude" {
		t.Fatalf("expected normalized model provider claude, got %q", worker.ModelProvider)
	}
	if worker.StopToken != "COMPLETE" {
		t.Fatalf("expected normalized stop token COMPLETE, got %q", worker.StopToken)
	}

	workstation, ok := loaded.Workstation("execute-task")
	if !ok {
		t.Fatal("expected inline workstation definition to load")
	}
	if workstation.PromptTemplate != "Complete {{ (index .Inputs 0).WorkID }}." {
		t.Fatalf("expected normalized prompt template, got %q", workstation.PromptTemplate)
	}
	if len(workstation.StopWords) != 1 || workstation.StopWords[0] != "DONE" {
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
      "onFailure": { "workType": "task", "state": "failed" },
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

	var flattenOut bytes.Buffer
	flattenCmd := cli.NewRootCommand()
	flattenCmd.SetOut(&flattenOut)
	flattenCmd.SetErr(&bytes.Buffer{})
	flattenCmd.SetArgs([]string{"config", "flatten", dir})
	if err := flattenCmd.Execute(); err != nil {
		t.Fatalf("execute config flatten: %v", err)
	}
	return flattenOut.Bytes()
}

func assertFlattenedInlineScriptBackedConfig(t *testing.T, flattened []byte) {
	t.Helper()

	flattenedCfg, err := factoryconfig.NewFactoryConfigMapper().Expand(flattened)
	if err != nil {
		t.Fatalf("flattened inline script-backed config should parse: %v", err)
	}
	if len(flattenedCfg.Workers) != 1 || len(flattenedCfg.Workstations) != 1 {
		t.Fatalf("expected one worker/workstation after flatten, got %d/%d", len(flattenedCfg.Workers), len(flattenedCfg.Workstations))
	}
	if flattenedCfg.Workers[0].Type != interfaces.WorkerTypeScript || flattenedCfg.Workers[0].Command != "powershell" {
		t.Fatalf("flattened worker definition = %#v", flattenedCfg.Workers[0])
	}
	if len(flattenedCfg.Workers[0].Args) != 2 || flattenedCfg.Workers[0].Args[1] != "scripts/execute-story.ps1" {
		t.Fatalf("flattened worker args = %#v", flattenedCfg.Workers[0].Args)
	}
	if flattenedCfg.Workstations[0].Type != interfaces.WorkstationTypeModel || !flattenedCfg.Workstations[0].CopyReferencedScripts {
		t.Fatalf("flattened workstation definition = %#v", flattenedCfg.Workstations[0])
	}
	if flattenedCfg.Workstations[0].WorkingDirectory != "/repo/{{ (index .Inputs 0).WorkID }}" {
		t.Fatalf("flattened workstation working directory = %q", flattenedCfg.Workstations[0].WorkingDirectory)
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

	loaded, err := factoryconfig.LoadRuntimeConfig(standaloneDir, nil)
	if err != nil {
		t.Fatalf("flattened standalone config should load: %v", err)
	}

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected flattened script worker definition to load")
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "powershell" || worker.Timeout != "45m" {
		t.Fatalf("loaded worker = %#v", worker)
	}

	workstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected flattened inline workstation definition to load")
	}
	if workstation.Type != interfaces.WorkstationTypeModel || workstation.WorkingDirectory != "/repo/{{ (index .Inputs 0).WorkID }}" {
		t.Fatalf("loaded workstation = %#v", workstation)
	}
}

func assertFlattenedInlineScriptStandaloneExecutes(t *testing.T, standaloneDir string) {
	t.Helper()

	h := testutil.NewServiceTestHarness(t, standaloneDir,
		testutil.WithCommandRunner(successRunner("flattened inline script accepted")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)
	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)
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

	mapper := factoryconfig.NewFactoryConfigMapper()
	cfg, err := mapper.Expand(data)
	if err != nil {
		t.Fatalf("expand canonical factory payload: %v\n%s", err, string(data))
	}
	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("flatten canonical factory payload: %v", err)
	}

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
