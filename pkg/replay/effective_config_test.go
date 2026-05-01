package replay_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/replay"
	"github.com/portpowered/agent-factory/pkg/workers"
)

// portos:func-length-exception owner=agent-factory reason=generated-factory-runtime-fixture review=2026-07-18 removal=split-loaded-factory-runtime-config-and-assertions-before-next-generated-factory-change
func TestGeneratedFactoryFromLoadedConfig_EmbedsLoadedFactoryAndRuntimeConfig(t *testing.T) {
	factoryDir := t.TempDir()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"name": "customer-facing-name",
		"id":   "internal-id",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
		"workers":   []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":        "execute-story-id",
			"name":      "execute-story",
			"worker":    "executor",
			"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
			"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
		}},
	})
	writeAgentsMD(t, filepath.Join(factoryDir, "workers", "executor"), `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
timeout: 30s
---
Run the test suite.
`)
	writeAgentsMD(t, filepath.Join(factoryDir, "workstations", "execute-story"), `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
stopWords: ["DONE"]
limits:
  maxExecutionTime: 20m
  maxRetries: 2
---
Fallback body.
`)
	if err := os.WriteFile(filepath.Join(factoryDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	generated, err := replay.GeneratedFactoryFromLoadedConfig(
		loaded,
		replay.WithGeneratedFactorySourceDirectory(factoryDir),
		replay.WithGeneratedFactoryWorkflowID("workflow-123"),
		replay.WithGeneratedFactoryMetadata(map[string]string{"code_version": "test-sha"}),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}

	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one", generated.Workstations)
	}
	if generated.Name != "customer-facing-name" {
		t.Fatalf("generated factory name = %q, want customer-facing-name", generated.Name)
	}
	if generated.Id == nil || *generated.Id != "internal-id" {
		t.Fatalf("generated factory id = %#v, want internal-id", generated.Id)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.Name != "execute-story" {
		t.Fatalf("generated workstation name = %q, want execute-story", workstation.Name)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("generated workers = %#v, want one", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.Command == nil || *worker.Command != "go" {
		t.Fatalf("generated worker command = %#v, want go", worker.Command)
	}
	if workstation.Type == nil || *workstation.Type != "MODEL_WORKSTATION" {
		t.Fatalf("generated workstation runtime type = %#v, want MODEL_WORKSTATION", workstation.Type)
	}
	if workstation.PromptTemplate == nil || *workstation.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("generated workstation prompt template = %#v, want prompt file content", workstation.PromptTemplate)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	assertCanonicalReplayFactoryDir(t, runtimeCfg, factoryDir)
	assertCanonicalReplayRuntimeBaseDir(t, runtimeCfg, factoryDir)
	workerDef, workstationDef := assertCanonicalRuntimeDefinitionLookupByName(t, runtimeCfg, "executor", "execute-story")
	if workerDef.Command != "go" {
		t.Fatalf("runtime worker = %#v, want command go", workerDef)
	}
	if workstationDef.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("runtime workstation = %#v, want prompt template", workstationDef)
	}
	if workstationDef.Timeout != "" {
		t.Fatalf("runtime workstation timeout alias = %q, want cleared", workstationDef.Timeout)
	}
	if workstationDef.Limits.MaxExecutionTime != "20m" {
		t.Fatalf("runtime workstation max execution time = %q, want 20m", workstationDef.Limits.MaxExecutionTime)
	}
	if workstationDef.ID != "execute-story-id" {
		t.Fatalf("runtime workstation ID = %q, want execute-story-id", workstationDef.ID)
	}
	if generated.SourceDirectory == nil || *generated.SourceDirectory != factoryDir {
		t.Fatalf("source directory = %#v, want %q", generated.SourceDirectory, factoryDir)
	}
	if generated.Metadata == nil {
		t.Fatal("expected generated metadata")
	}
	if !strings.HasPrefix((*generated.Metadata)["factory_hash"], "sha256:") {
		t.Fatalf("factory_hash metadata = %q, want sha256 prefix", (*generated.Metadata)["factory_hash"])
	}
	if !strings.HasPrefix((*generated.Metadata)["runtime_config_hash"], "sha256:") {
		t.Fatalf("runtime_config_hash metadata = %q, want sha256 prefix", (*generated.Metadata)["runtime_config_hash"])
	}
	if (*generated.Metadata)["code_version"] != "test-sha" {
		t.Fatalf("code_version metadata = %q, want test-sha", (*generated.Metadata)["code_version"])
	}
}

// portos:func-length-exception owner=agent-factory reason=generated-factory-rebuild-fixture review=2026-07-18 removal=split-embedded-config-fixture-and-rebuild-assertions-before-next-replay-config-change
func TestRuntimeConfigFromGeneratedFactory_RebuildsWithoutOriginalFiles(t *testing.T) {
	factoryDir := t.TempDir()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"resources": []map[string]any{},
		"workers":   []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":      "execute-story-id",
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
		}},
	})
	writeAgentsMD(t, filepath.Join(factoryDir, "workers", "executor"), `---
type: SCRIPT_WORKER
command: echo
args: ["ok"]
---
Script worker body.
`)
	writeAgentsMD(t, filepath.Join(factoryDir, "workstations", "execute-story"), `---
type: LOGICAL_MOVE
worker: executor
---
Move the token.
`)

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := replay.GeneratedFactoryFromLoadedConfig(loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.replay.json")
	artifact, err := replay.NewEventLogArtifactFromFactory(time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC), generated, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := os.RemoveAll(factoryDir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}
	loadedArtifact, err := replay.Load(artifactPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(loadedArtifact.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	assertCanonicalReplayFactoryDir(t, runtimeCfg, factoryDir)
	assertCanonicalReplayRuntimeBaseDir(t, runtimeCfg, factoryDir)

	if runtimeCfg.Factory.WorkTypes[0].Name != "story" {
		t.Fatalf("runtime factory work type = %q, want story", runtimeCfg.Factory.WorkTypes[0].Name)
	}
	workerDef, workstationDef := assertCanonicalRuntimeDefinitionLookupByName(t, runtimeCfg, "executor", "execute-story")
	if workerDef.Command != "echo" || len(workerDef.Args) != 1 || workerDef.Args[0] != "ok" {
		t.Fatalf("embedded worker command = %q args=%v, want echo [ok]", workerDef.Command, workerDef.Args)
	}
	if workstationDef.Type != "LOGICAL_MOVE" {
		t.Fatalf("embedded workstation runtime type = %q, want LOGICAL_MOVE", workstationDef.Type)
	}
	if workstationDef.ID != "execute-story-id" {
		t.Fatalf("embedded workstation ID = %q, want execute-story-id", workstationDef.ID)
	}
}

func TestRuntimeConfigFromGeneratedFactory_KeepsCanonicalRelativeExecutionPath(t *testing.T) {
	factoryDir := t.TempDir()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"id": "agent-factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":    "standard",
			"worker":  "worker-a",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
	writeAgentsMD(t, filepath.Join(factoryDir, "workers", "worker-a"), `---
type: MODEL_WORKER
model: gpt-5.4
---
System prompt.
`)
	writeAgentsMD(t, filepath.Join(factoryDir, "workstations", "standard"), `---
type: MODEL_WORKSTATION
worker: worker-a
workingDirectory: workspace
---
Work from {{ .Context.WorkDir }}
`)

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := replay.GeneratedFactoryFromLoadedConfig(loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	executor := &captureReplayWorkstationExecutor{
		result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"},
	}
	we := &workers.WorkstationExecutor{
		RuntimeConfig: runtimeCfg,
		Executor:      executor,
		Renderer:      &workers.DefaultPromptRenderer{},
	}

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-replay-runtime-base",
		TransitionID:    "t-replay-runtime-base",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID:    "tok-1",
			Color: interfaces.TokenColor{WorkID: "work-1"},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if executor.request.WorkingDirectory != filepath.Join(factoryDir, "workspace") {
		t.Fatalf("working directory = %q, want %q", executor.request.WorkingDirectory, filepath.Join(factoryDir, "workspace"))
	}
	if executor.request.UserMessage != "Work from "+filepath.Join(factoryDir, "workspace") {
		t.Fatalf("user message = %q", executor.request.UserMessage)
	}
}

// portos:func-length-exception owner=agent-factory reason=replay-topology-projection-fixture review=2026-07-18 removal=split-topology-fixture-and-projection-assertions-before-next-replay-topology-change
func TestRuntimeConfigFromGeneratedFactory_ProjectsReplayInitialTopologyFromFactory(t *testing.T) {
	factoryDir := t.TempDir()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
		"workers":   []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":        "execute-story-id",
			"name":      "execute-story",
			"behavior":  "STANDARD",
			"worker":    "executor",
			"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
			"onFailure": map[string]string{"workType": "story", "state": "failed"},
			"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
			"guards": []map[string]any{{
				"type":        "VISIT_COUNT",
				"workstation": "execute-story",
				"maxVisits":   3,
			}},
			"stopWords": []string{"BLOCKED"},
		}},
	})
	writeAgentsMD(t, filepath.Join(factoryDir, "workers", "executor"), `---
type: MODEL_WORKER
executorProvider: script_wrap
modelProvider: codex
model: gpt-5.4
timeout: 30m
---
Implement the story.
`)
	writeAgentsMD(t, filepath.Join(factoryDir, "workstations", "execute-story"), `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
limits:
  maxRetries: 2
  maxExecutionTime: 20m
stopWords: ["DONE"]
---
Fallback body.
`)
	if err := os.WriteFile(filepath.Join(factoryDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	mapper := config.ConfigMapper{}
	liveNet, err := mapper.Map(context.Background(), loaded.FactoryConfig())
	if err != nil {
		t.Fatalf("Map live factory: %v", err)
	}
	liveProjection := projections.ProjectInitialStructure(liveNet, loaded)

	generated, err := replay.GeneratedFactoryFromLoadedConfig(loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.replay.json")
	artifact, err := replay.NewEventLogArtifactFromFactory(time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC), generated, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.RemoveAll(factoryDir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}

	loadedArtifact, err := replay.Load(artifactPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	replayRuntimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(loadedArtifact.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	replayNet, err := mapper.Map(context.Background(), replayRuntimeCfg.Factory)
	if err != nil {
		t.Fatalf("Map replay factory: %v", err)
	}
	replayProjection := projections.ProjectInitialStructure(replayNet, replayRuntimeCfg)

	if !reflect.DeepEqual(replayProjection, liveProjection) {
		t.Fatalf("replay projection mismatch\n got: %#v\nwant: %#v", replayProjection, liveProjection)
	}
	assertProjectedWorker(t, replayProjection, interfaces.FactoryWorker{
		ID:            "executor",
		Name:          "executor",
		Provider:      "script_wrap",
		ModelProvider: "codex",
		Model:         "gpt-5.4",
		Config:        map[string]string{"type": interfaces.WorkerTypeModel},
	})
	assertProjectedConstraint(t, replayProjection, interfaces.FactoryConstraint{
		ID:    "workstation/execute-story/limits",
		Type:  "workstation_limit",
		Scope: "workstation:execute-story",
		Values: map[string]string{
			"max_execution_time": "20m",
			"max_retries":        "2",
		},
	})
	assertProjectedConstraint(t, replayProjection, interfaces.FactoryConstraint{
		ID:    "workstation/execute-story/stop-words",
		Type:  "stop_words",
		Scope: "workstation:execute-story",
		Values: map[string]string{
			"words": "BLOCKED,DONE",
		},
	})
}

func TestGeneratedFactoryFromLoadedConfig_EmitsCanonicalPublicWorkstationKind(t *testing.T) {
	factoryDir := t.TempDir()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":      "retry-story-id",
			"name":    "retry-story",
			"behavior": "REPEATER",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
		}},
	})
	writeAgentsMD(t, filepath.Join(factoryDir, "workers", "executor"), `---
type: MODEL_WORKER
modelProvider: openai
model: gpt-5.4
---
Execute the work.
`)
	writeAgentsMD(t, filepath.Join(factoryDir, "workstations", "retry-story"), `---
type: MODEL_WORKSTATION
worker: executor
---
Retry the work.
`)

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := replay.GeneratedFactoryFromLoadedConfig(loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one", generated.Workstations)
	}
	if (*generated.Workstations)[0].Behavior == nil || *(*generated.Workstations)[0].Behavior != factoryapi.WorkstationKindRepeater {
		t.Fatalf("generated workstation behavior = %#v, want REPEATER", (*generated.Workstations)[0].Behavior)
	}

	replayRuntimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	workstation, ok := replayRuntimeCfg.Workstation("retry-story")
	if !ok {
		t.Fatal("expected replay workstation definition")
	}
	if workstation.Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("replay workstation kind = %q, want repeater", workstation.Kind)
	}
}

func TestRuntimeConfigFromGeneratedFactory_PreservesPerInputGuardFanIn(t *testing.T) {
	factoryDir := t.TempDir()
	writePerInputGuardFanInFactoryJSON(t, factoryDir)

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := replay.GeneratedFactoryFromLoadedConfig(loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	assertGeneratedInputGuard(t, generatedWorkstationByName(t, generated, "chapter-complete"), "page", string(factoryapi.GuardTypeAllChildrenComplete))
	assertGeneratedInputGuard(t, generatedWorkstationByName(t, generated, "chapter-failed"), "page", string(factoryapi.GuardTypeAnyChildFailed))
	data, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated factory: %v", err)
	}
	if strings.Contains(string(data), `"join"`) {
		t.Fatalf("generated factory contains retired join field: %s", data)
	}

	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	assertRuntimeInputGuard(t, runtimeWorkstationByName(t, runtimeCfg.Factory, "chapter-complete"), "page", interfaces.GuardTypeAllChildrenComplete)
	assertRuntimeInputGuard(t, runtimeWorkstationByName(t, runtimeCfg.Factory, "chapter-failed"), "page", interfaces.GuardTypeAnyChildFailed)

	mapper := config.ConfigMapper{}
	net, err := mapper.Map(context.Background(), runtimeCfg.Factory)
	if err != nil {
		t.Fatalf("Map replay factory: %v", err)
	}
	assertDynamicFanInGuard(t, net.Transitions["chapter-complete"], "page:complete", &petri.FanoutCountGuard{})
	assertDynamicFanInGuard(t, net.Transitions["chapter-failed"], "page:failed", &petri.AnyWithParentGuard{})
	if net.FanoutGroups["parser"] != "parser:fanout-count" {
		t.Fatalf("fanout group for parser = %q, want parser:fanout-count", net.FanoutGroups["parser"])
	}
}

func TestRuntimeConfigFromGeneratedFactory_PreservesGuardedLoopBreakerRoundTrip(t *testing.T) {
	factoryDir := t.TempDir()
	writeGuardedLoopBreakerFactoryJSON(t, factoryDir)

	generated := loadGeneratedFactoryWithoutRetiredExhaustionRules(t, factoryDir)
	replayRuntimeCfg := roundTripGeneratedFactoryThroughReplayArtifact(t, factoryDir, generated)

	loopBreaker := runtimeWorkstationByName(t, replayRuntimeCfg.Factory, "review-story-loop-breaker")
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop-breaker type = %q, want %q", loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("loop-breaker guards = %#v, want one guard", loopBreaker.Guards)
	}
	if loopBreaker.Guards[0].Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("loop-breaker guard type = %q, want %q", loopBreaker.Guards[0].Type, interfaces.GuardTypeVisitCount)
	}
	if loopBreaker.Guards[0].Workstation != "review-story" || loopBreaker.Guards[0].MaxVisits != 3 {
		t.Fatalf("loop-breaker guard = %#v, want review-story maxVisits=3", loopBreaker.Guards[0])
	}
	if len(loopBreaker.Outputs) != 1 || loopBreaker.Outputs[0].WorkTypeName != "story" || loopBreaker.Outputs[0].StateName != "failed" {
		t.Fatalf("loop-breaker outputs = %#v, want story:failed", loopBreaker.Outputs)
	}

	mapper := config.ConfigMapper{}
	net, err := mapper.Map(context.Background(), replayRuntimeCfg.Factory)
	if err != nil {
		t.Fatalf("Map replay factory: %v", err)
	}
	assertReplayGuardedLoopBreakerTransition(t, net.Transitions["review-story-loop-breaker"], "story:init", "story:failed", "review-story", 3)
	assertReplayHasNoTransitionExhaustion(t, net.Transitions)
}

func writeGuardedLoopBreakerFactoryJSON(t *testing.T, factoryDir string) {
	t.Helper()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{"name": "reviewer-worker"}},
		"workstations": []map[string]any{
			{
				"name":        "review-story",
				"worker":      "reviewer-worker",
				"inputs":      []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":     []map[string]string{{"workType": "story", "state": "complete"}},
				"onRejection": map[string]string{"workType": "story", "state": "init"},
			},
			{
				"name":    "review-story-loop-breaker",
				"type":    "LOGICAL_MOVE",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "failed"}},
				"guards": []map[string]any{{
					"type":        "VISIT_COUNT",
					"workstation": "review-story",
					"maxVisits":   3,
				}},
			},
		},
	})
}

func loadGeneratedFactoryWithoutRetiredExhaustionRules(t *testing.T, factoryDir string) factoryapi.Factory {
	t.Helper()
	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := replay.GeneratedFactoryFromLoadedConfig(loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	generatedJSON, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated factory: %v", err)
	}
	if strings.Contains(string(generatedJSON), `"exhaustionRules"`) || strings.Contains(string(generatedJSON), `"exhaustion_rules"`) {
		t.Fatalf("generated factory must not serialize retired exhaustion rules: %s", generatedJSON)
	}
	return generated
}

func roundTripGeneratedFactoryThroughReplayArtifact(t *testing.T, factoryDir string, generated factoryapi.Factory) *replay.EmbeddedRuntimeConfig {
	t.Helper()
	artifactPath := filepath.Join(t.TempDir(), "guarded-loop-breaker.replay.json")
	artifact, err := replay.NewEventLogArtifactFromFactory(time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC), generated, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.RemoveAll(factoryDir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}
	loadedArtifact, err := replay.Load(artifactPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	replayRuntimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(loadedArtifact.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	return replayRuntimeCfg
}

func assertProjectedWorker(t *testing.T, payload interfaces.InitialStructurePayload, want interfaces.FactoryWorker) {
	t.Helper()
	for _, worker := range payload.Workers {
		if reflect.DeepEqual(worker, want) {
			return
		}
	}
	t.Fatalf("projected workers = %#v, want %#v", payload.Workers, want)
}

func assertProjectedConstraint(t *testing.T, payload interfaces.InitialStructurePayload, want interfaces.FactoryConstraint) {
	t.Helper()
	for _, constraint := range payload.Constraints {
		if reflect.DeepEqual(constraint, want) {
			return
		}
	}
	t.Fatalf("projected constraints = %#v, want %#v", payload.Constraints, want)
}

func assertGeneratedInputGuard(t *testing.T, workstation factoryapi.Workstation, workType string, guardType string) {
	t.Helper()
	for _, input := range workstation.Inputs {
		if input.WorkType != workType {
			continue
		}
		if input.Guards == nil || len(*input.Guards) != 1 {
			t.Fatalf("%s input %s generated guards = %#v, want one guard", workstation.Name, workType, input.Guards)
		}
		if got := string((*input.Guards)[0].Type); got != guardType {
			t.Fatalf("%s input %s generated guard = %q, want %q", workstation.Name, workType, got, guardType)
		}
		return
	}
	t.Fatalf("%s has no generated input for work type %q", workstation.Name, workType)
}

func assertRuntimeInputGuard(t *testing.T, workstation interfaces.FactoryWorkstationConfig, workType string, guardType interfaces.GuardType) {
	t.Helper()
	for _, input := range workstation.Inputs {
		if input.WorkTypeName != workType {
			continue
		}
		if input.Guard == nil {
			t.Fatalf("%s input %s runtime guard is nil", workstation.Name, workType)
		}
		if input.Guard.Type != guardType {
			t.Fatalf("%s input %s runtime guard = %q, want %q", workstation.Name, workType, input.Guard.Type, guardType)
		}
		if input.Guard.ParentInput != "chapter" || input.Guard.SpawnedBy != "parser" {
			t.Fatalf("%s input %s runtime guard context = %#v, want chapter/parser", workstation.Name, workType, input.Guard)
		}
		return
	}
	t.Fatalf("%s has no runtime input for work type %q", workstation.Name, workType)
}

func assertReplayGuardedLoopBreakerTransition(t *testing.T, transition *petri.Transition, inputPlace string, outputPlace string, watchedWorkstation string, maxVisits int) {
	t.Helper()
	if transition == nil {
		t.Fatal("expected guarded loop-breaker transition to exist")
	}
	if transition.Type != petri.TransitionNormal {
		t.Fatalf("loop-breaker transition type = %s, want %s", transition.Type, petri.TransitionNormal)
	}
	if len(transition.InputArcs) != 1 {
		t.Fatalf("loop-breaker input arcs = %#v, want one input arc", transition.InputArcs)
	}
	if transition.InputArcs[0].PlaceID != inputPlace {
		t.Fatalf("loop-breaker input place = %q, want %q", transition.InputArcs[0].PlaceID, inputPlace)
	}
	guard, ok := transition.InputArcs[0].Guard.(*petri.VisitCountGuard)
	if !ok {
		t.Fatalf("loop-breaker guard = %T, want *petri.VisitCountGuard", transition.InputArcs[0].Guard)
	}
	if guard.TransitionID != watchedWorkstation || guard.MaxVisits != maxVisits {
		t.Fatalf("loop-breaker visit guard = %#v, want transition=%q maxVisits=%d", guard, watchedWorkstation, maxVisits)
	}
	if len(transition.OutputArcs) != 1 {
		t.Fatalf("loop-breaker output arcs = %#v, want one output arc", transition.OutputArcs)
	}
	if transition.OutputArcs[0].PlaceID != outputPlace {
		t.Fatalf("loop-breaker output place = %q, want %q", transition.OutputArcs[0].PlaceID, outputPlace)
	}
}

func assertReplayHasNoTransitionExhaustion(t *testing.T, transitions map[string]*petri.Transition) {
	t.Helper()
	for name, transition := range transitions {
		if transition != nil && transition.Type == petri.TransitionExhaustion {
			t.Fatalf("unexpected TransitionExhaustion transition %q in replay-mapped customer config", name)
		}
	}
}

func assertDynamicFanInGuard(t *testing.T, transition *petri.Transition, childPlaceID string, want petri.Guard) {
	t.Helper()
	if transition == nil {
		t.Fatal("expected transition to exist")
	}
	for _, arc := range transition.InputArcs {
		if arc.PlaceID != childPlaceID {
			continue
		}
		if arc.Mode != interfaces.ArcModeObserve {
			t.Fatalf("%s child arc mode = %v, want observe", transition.Name, arc.Mode)
		}
		switch want.(type) {
		case *petri.FanoutCountGuard:
			guard, ok := arc.Guard.(*petri.FanoutCountGuard)
			if !ok {
				t.Fatalf("%s child arc guard = %T, want FanoutCountGuard", transition.Name, arc.Guard)
			}
			if guard.MatchBinding != "parent" || guard.CountBinding != "fanout-count" {
				t.Fatalf("%s fanout guard = %#v, want parent/fanout-count bindings", transition.Name, guard)
			}
		case *petri.AnyWithParentGuard:
			guard, ok := arc.Guard.(*petri.AnyWithParentGuard)
			if !ok {
				t.Fatalf("%s child arc guard = %T, want AnyWithParentGuard", transition.Name, arc.Guard)
			}
			if guard.MatchBinding != "parent" {
				t.Fatalf("%s any-child guard binding = %q, want parent", transition.Name, guard.MatchBinding)
			}
		default:
			t.Fatalf("unsupported expected guard type %T", want)
		}
		return
	}
	t.Fatalf("%s has no child observation arc for %q", transition.Name, childPlaceID)
}

type captureReplayWorkstationExecutor struct {
	request interfaces.WorkstationExecutionRequest
	result  interfaces.WorkResult
}

func (e *captureReplayWorkstationExecutor) Execute(_ context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	e.request = request
	return e.result, nil
}

func generatedWorkstationByName(t *testing.T, generated factoryapi.Factory, name string) factoryapi.Workstation {
	t.Helper()
	if generated.Workstations == nil {
		t.Fatal("generated factory has no workstations")
	}
	for _, workstation := range *generated.Workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("generated factory has no workstation %q", name)
	return factoryapi.Workstation{}
}

func runtimeWorkstationByName(t *testing.T, cfg *interfaces.FactoryConfig, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	if cfg == nil {
		t.Fatal("runtime factory config is nil")
	}
	for _, workstation := range cfg.Workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("runtime factory config has no workstation %q", name)
	return interfaces.FactoryWorkstationConfig{}
}

func assertCanonicalReplayFactoryDir(t *testing.T, lookup interfaces.RuntimeConfigLookup, want string) {
	t.Helper()
	if lookup.FactoryDir() != want {
		t.Fatalf("runtime FactoryDir = %q, want %q", lookup.FactoryDir(), want)
	}
}

func assertCanonicalReplayRuntimeBaseDir(t *testing.T, lookup interfaces.RuntimeConfigLookup, want string) {
	t.Helper()
	if lookup.RuntimeBaseDir() != want {
		t.Fatalf("runtime RuntimeBaseDir = %q, want %q", lookup.RuntimeBaseDir(), want)
	}
}

func assertCanonicalRuntimeDefinitionLookupByName(
	t *testing.T,
	lookup interfaces.RuntimeDefinitionLookup,
	workerName string,
	workstationName string,
) (*interfaces.WorkerConfig, *interfaces.FactoryWorkstationConfig) {
	t.Helper()
	worker, ok := lookup.Worker(workerName)
	if !ok || worker == nil {
		t.Fatalf("canonical worker lookup %q = %#v ok=%v, want worker", workerName, worker, ok)
	}
	workstation, ok := lookup.Workstation(workstationName)
	if !ok || workstation == nil {
		t.Fatalf("canonical workstation lookup %q = %#v ok=%v, want workstation", workstationName, workstation, ok)
	}
	return worker, workstation
}

func writePerInputGuardFanInFactoryJSON(t *testing.T, factoryDir string) {
	t.Helper()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "chapter",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "processing", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "page",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "parser"},
			{"name": "completion-worker"},
			{"name": "failure-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":    "parser",
				"worker":  "parser",
				"inputs":  []map[string]string{{"workType": "chapter", "state": "init"}},
				"outputs": []map[string]string{{"workType": "chapter", "state": "processing"}},
			},
			{
				"name":   "chapter-complete",
				"worker": "completion-worker",
				"inputs": []map[string]any{
					{"workType": "chapter", "state": "processing"},
					{
						"workType": "page",
						"state":    "complete",
						"guards": []map[string]string{{
							"type":        "ALL_CHILDREN_COMPLETE",
							"parentInput": "chapter",
							"spawnedBy":   "parser",
						}},
					},
				},
				"outputs": []map[string]string{{"workType": "chapter", "state": "complete"}},
			},
			{
				"name":   "chapter-failed",
				"worker": "failure-worker",
				"inputs": []map[string]any{
					{"workType": "chapter", "state": "processing"},
					{
						"workType": "page",
						"state":    "failed",
						"guards": []map[string]string{{
							"type":        "ANY_CHILD_FAILED",
							"parentInput": "chapter",
							"spawnedBy":   "parser",
						}},
					},
				},
				"outputs": []map[string]string{{"workType": "chapter", "state": "failed"}},
			},
		},
	})
}

func writeFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()
	ensureFactoryName(cfg, filepath.Base(factoryDir))
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func ensureFactoryName(cfg map[string]any, fallback string) {
	if cfg == nil {
		return
	}
	if name, ok := cfg["name"].(string); ok && strings.TrimSpace(name) != "" {
		return
	}
	if id, ok := cfg["id"].(string); ok && strings.TrimSpace(id) != "" {
		cfg["name"] = id
		return
	}
	if strings.TrimSpace(fallback) != "" {
		cfg["name"] = fallback
		return
	}
	cfg["name"] = "factory"
}

func writeAgentsMD(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}
