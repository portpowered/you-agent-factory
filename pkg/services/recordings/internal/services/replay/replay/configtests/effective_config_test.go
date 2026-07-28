package replay_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/replay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func mustFactorySnapshot(t testing.TB, value any) *interfaces.FactorySnapshot {
	t.Helper()
	snapshot, err := interfaces.NewFactorySnapshot(value)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	return snapshot
}

func TestGeneratedFactoryFromLoadedConfig_EmbedsLoadedFactoryAndRuntimeConfig(t *testing.T) {
	factoryDir := t.TempDir()
	writeEmbeddedFactoryFixture(t, factoryDir)

	loaded, err := loadedFactoryValue(factoryDir, embeddedFactoryDefinitions()...)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	generated, err := generatedFactoryFromLoadedConfig(
		scriptedLoadedFactorySnapshotCapturer(factoryDir),
		loaded,
		factoryDir,
		map[string]string{"code_version": "test-sha"},
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}

	assertEmbeddedGeneratedFactory(t, generated, factoryDir)
	runtimeCfg, err := runtimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	assertEmbeddedRuntimeConfig(t, runtimeCfg, factoryDir)
}

func TestRuntimeConfigFromGeneratedFactory_RebuildsWithoutOriginalFiles(t *testing.T) {
	factoryDir := t.TempDir()
	writeRebuildGeneratedFactoryFixture(t, factoryDir)

	loaded, err := loadedFactoryValue(
		factoryDir,
		withWorkerDefinition("executor", func(worker *interfaces.FactoryWorkerConfig) {
			worker.Type = interfaces.WorkerTypeScript
			worker.Command = "echo"
			worker.Args = []string{"ok"}
			worker.Body = "Script worker body."
		}),
		withWorkstationDefinition("execute-story", func(workstation *interfaces.FactoryWorkstationConfig) {
			workstation.Type = interfaces.WorkstationTypeLogical
			workstation.PromptTemplate = "Move the token."
		}),
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := generatedFactoryFromLoadedConfig(
		scriptedLoadedFactorySnapshotCapturer(factoryDir),
		loaded,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.replay.json")
	artifact, err := replay.NewEventLogArtifact(time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC), mustFactorySnapshot(t, generated), nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifact: %v", err)
	}
	if err := replay.Save(testReplayStorage(), artifactPath, artifact); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := os.RemoveAll(factoryDir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}
	loadedArtifact, err := replay.Load(
		testReplayStorage(),
		artifactPath,
		configTestFactorySnapshotDecoder,
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	runtimeCfg, err := runtimeConfigFromFactorySnapshot(loadedArtifact.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	assertRebuiltRuntimeConfig(t, runtimeCfg, factoryDir)
}

func TestRuntimeConfigFromGeneratedFactory_KeepsCanonicalRelativeExecutionPath(t *testing.T) {
	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, map[string]any{
		"name": "agent-factory",
		"id":   "agent-factory",
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

	loaded, err := loadedFactoryValue(
		factoryDir,
		withWorkerDefinition("worker-a", func(worker *interfaces.FactoryWorkerConfig) {
			worker.Type = interfaces.WorkerTypeModel
			worker.Model = "gpt-5.4"
			worker.Body = "System prompt."
		}),
		withWorkstationDefinition("standard", func(workstation *interfaces.FactoryWorkstationConfig) {
			workstation.Type = interfaces.WorkstationTypeModel
			workstation.WorkingDirectory = "workspace"
			workstation.PromptTemplate = "Work from {{ .Context.WorkDir }}"
		}),
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := generatedFactoryFromLoadedConfig(
		scriptedLoadedFactorySnapshotCapturer(factoryDir),
		loaded,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	runtimeCfg, err := runtimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	if runtimeCfg.FactoryDir() != factoryDir {
		t.Fatalf("Factory directory = %q, want %q", runtimeCfg.FactoryDir(), factoryDir)
	}
	workstation := runtimeWorkstationByName(t, runtimeCfg.FactoryConfig(), "standard")
	if workstation.WorkingDirectory != "workspace" {
		t.Fatalf("canonical working directory = %q, want relative workspace", workstation.WorkingDirectory)
	}
	if workstation.PromptTemplate != "Work from {{ .Context.WorkDir }}" {
		t.Fatalf("canonical prompt = %q, want authored work-directory template", workstation.PromptTemplate)
	}
	// Workers owns resolution of this relative path and prompt at execution
	// time in TestWorkstationExecutor_ResolvesRelativeWorkingDirectoryAgainstRuntimeConfigFactoryDirectory.
}

func TestRuntimeConfigFromGeneratedFactory_ProjectsReplayInitialTopologyFromFactory(t *testing.T) {
	factoryDir := t.TempDir()
	writeReplayInitialTopologyFixture(t, factoryDir)

	loaded, liveProjection := loadLiveReplayInitialProjection(t, factoryDir)
	replayProjection := loadReplayInitialProjectionFromArtifact(t, factoryDir, loaded)

	assertReplayInitialTopologyProjection(t, replayProjection, liveProjection)
}

func writeReplayInitialTopologyFixture(t *testing.T, factoryDir string) {
	t.Helper()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
			{"name": "story-retry", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "story-followup", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "story-triage", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "story-backlog", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "story-failed", "states": []map[string]string{{"name": "failed", "type": "FAILED"}}},
			{"name": "story-abandoned", "states": []map[string]string{{"name": "failed", "type": "FAILED"}}},
		},
		"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
		"workers":   []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":       "execute-story-id",
			"name":     "execute-story",
			"behavior": "STANDARD",
			"worker":   "executor",
			"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "story", "state": "complete"}},
			"onContinue": []map[string]string{
				{"workType": "story-retry", "state": "init"},
				{"workType": "story-followup", "state": "init"},
			},
			"onRejection": []map[string]string{
				{"workType": "story-triage", "state": "init"},
				{"workType": "story-backlog", "state": "init"},
			},
			"onFailure": []map[string]string{
				{"workType": "story-failed", "state": "failed"},
				{"workType": "story-abandoned", "state": "failed"},
			},
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
}

func loadLiveReplayInitialProjection(
	t *testing.T,
	factoryDir string,
) (interfaces.MutableLoadedFactorySource, *interfaces.FactoryConfig) {
	t.Helper()
	loaded, err := loadedFactoryValue(
		factoryDir,
		withWorkerDefinition("executor", func(worker *interfaces.FactoryWorkerConfig) {
			worker.Type = interfaces.WorkerTypeModel
			worker.ExecutorProvider = "script_wrap"
			worker.ModelProvider = "codex"
			worker.Model = "gpt-5.4"
			worker.Timeout = "30m"
			worker.Body = "Implement the story."
		}),
		withWorkstationDefinition("execute-story", func(workstation *interfaces.FactoryWorkstationConfig) {
			workstation.Type = interfaces.WorkstationTypeModel
			workstation.PromptFile = "prompt.md"
			workstation.PromptTemplate = "Implement {{ .WorkID }}."
			workstation.StopWords = append(workstation.StopWords, "DONE")
			workstation.Limits = interfaces.WorkstationLimits{MaxRetries: 2, MaxExecutionTime: "20m"}
		}),
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	return loaded, loaded.FactoryConfig()
}

func loadReplayInitialProjectionFromArtifact(
	t *testing.T,
	factoryDir string,
	loaded interfaces.MutableLoadedFactorySource,
) *interfaces.FactoryConfig {
	t.Helper()
	generated, err := generatedFactoryFromLoadedConfig(
		scriptedLoadedFactorySnapshotCapturer(factoryDir),
		loaded,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.replay.json")
	artifact, err := replay.NewEventLogArtifact(time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC), mustFactorySnapshot(t, generated), nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifact: %v", err)
	}
	if err := replay.Save(testReplayStorage(), artifactPath, artifact); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.RemoveAll(factoryDir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}
	loadedArtifact, err := replay.Load(
		testReplayStorage(),
		artifactPath,
		configTestFactorySnapshotDecoder,
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	replayRuntimeCfg, err := runtimeConfigFromFactorySnapshot(loadedArtifact.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	return replayRuntimeCfg.FactoryConfig()
}

func assertReplayInitialTopologyProjection(t *testing.T, replayProjection *interfaces.FactoryConfig, liveProjection *interfaces.FactoryConfig) {
	t.Helper()
	if replayProjection.Name != liveProjection.Name ||
		!reflect.DeepEqual(replayProjection.WorkTypes, liveProjection.WorkTypes) ||
		!reflect.DeepEqual(replayProjection.Resources, liveProjection.Resources) {
		t.Fatalf("replay Factory topology mismatch\n got: %#v\nwant: %#v", replayProjection, liveProjection)
	}
	workstation := runtimeWorkstationByName(t, replayProjection, "execute-story")
	liveWorkstation := runtimeWorkstationByName(t, liveProjection, "execute-story")
	if !reflect.DeepEqual(workstation.Inputs, liveWorkstation.Inputs) ||
		!reflect.DeepEqual(workstation.Outputs, liveWorkstation.Outputs) ||
		!reflect.DeepEqual(workstation.Guards, liveWorkstation.Guards) {
		t.Fatalf("replay workstation topology = %#v, want %#v", workstation, liveWorkstation)
	}
	assertRuntimeRoutes(t, workstation.OnContinue, []interfaces.IOConfig{{WorkTypeName: "story-retry", StateName: "init"}, {WorkTypeName: "story-followup", StateName: "init"}})
	assertRuntimeRoutes(t, workstation.OnRejection, []interfaces.IOConfig{{WorkTypeName: "story-triage", StateName: "init"}, {WorkTypeName: "story-backlog", StateName: "init"}})
	assertRuntimeRoutes(t, workstation.OnFailure, []interfaces.IOConfig{{WorkTypeName: "story-failed", StateName: "failed"}, {WorkTypeName: "story-abandoned", StateName: "failed"}})
	if workstation.Limits.MaxRetries != 2 || workstation.Limits.MaxExecutionTime != "20m" {
		t.Fatalf("replay workstation limits = %#v, want retries=2 execution=20m", workstation.Limits)
	}
	if !reflect.DeepEqual(workstation.StopWords, []string{"BLOCKED", "DONE"}) {
		t.Fatalf("replay workstation stop words = %#v, want BLOCKED,DONE", workstation.StopWords)
	}
	worker := runtimeWorkerByName(t, replayProjection, "executor")
	if worker.ExecutorProvider != "script_wrap" || worker.ModelProvider != "codex" || worker.Model != "gpt-5.4" {
		t.Fatalf("replay worker = %#v, want script_wrap/codex/gpt-5.4", worker)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this generated-factory contract test keeps the canonical workstation projection assertions in one readable flow.
func TestGeneratedFactoryFromLoadedConfig_EmitsCanonicalPublicWorkstationKind(t *testing.T) {
	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":       "retry-story-id",
			"name":     "retry-story",
			"behavior": "REPEATER",
			"worker":   "executor",
			"inputs":   []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "story", "state": "complete"}},
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

	loaded, err := loadedFactoryValue(factoryDir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := generatedFactoryFromLoadedConfig(
		scriptedLoadedFactorySnapshotCapturer(factoryDir),
		loaded,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one", generated.Workstations)
	}
	if (*generated.Workstations)[0].Behavior == nil || *(*generated.Workstations)[0].Behavior != factoryapi.WorkstationKindRepeater {
		t.Fatalf("generated workstation behavior = %#v, want REPEATER", (*generated.Workstations)[0].Behavior)
	}

	replayRuntimeCfg, err := runtimeConfigFromGeneratedFactory(generated)
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

	loaded, err := loadedFactoryValue(factoryDir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := generatedFactoryFromLoadedConfig(
		scriptedLoadedFactorySnapshotCapturer(factoryDir),
		loaded,
		"",
		nil,
	)
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

	runtimeCfg, err := runtimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	assertRuntimeInputGuard(t, runtimeWorkstationByName(t, runtimeCfg.FactoryConfig(), "chapter-complete"), "page", interfaces.GuardTypeAllChildrenComplete)
	assertRuntimeInputGuard(t, runtimeWorkstationByName(t, runtimeCfg.FactoryConfig(), "chapter-failed"), "page", interfaces.GuardTypeAnyChildFailed)

}

func TestRuntimeConfigFromGeneratedFactory_PreservesGuardedLoopBreakerRoundTrip(t *testing.T) {
	factoryDir := t.TempDir()
	writeGuardedLoopBreakerFactoryJSON(t, factoryDir)

	generated := loadGeneratedFactoryWithoutRetiredExhaustionRules(t, factoryDir)
	replayRuntimeCfg := roundTripGeneratedFactoryThroughReplayArtifact(t, factoryDir, generated)

	loopBreaker := runtimeWorkstationByName(t, replayRuntimeCfg.FactoryConfig(), "review-story-loop-breaker")
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

}

func writeGuardedLoopBreakerFactoryJSON(t *testing.T, factoryDir string) {
	t.Helper()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
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
				"onRejection": []map[string]string{{"workType": "story", "state": "init"}},
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
	loaded, err := loadedFactoryValue(factoryDir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := generatedFactoryFromLoadedConfig(
		scriptedLoadedFactorySnapshotCapturer(factoryDir),
		loaded,
		"",
		nil,
	)
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

func roundTripGeneratedFactoryThroughReplayArtifact(
	t *testing.T,
	factoryDir string,
	generated factoryapi.Factory,
) interfaces.ReplayRuntimeConfig {
	t.Helper()
	artifactPath := filepath.Join(t.TempDir(), "guarded-loop-breaker.replay.json")
	artifact, err := replay.NewEventLogArtifact(time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC), mustFactorySnapshot(t, generated), nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifact: %v", err)
	}
	if err := replay.Save(testReplayStorage(), artifactPath, artifact); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.RemoveAll(factoryDir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}
	loadedArtifact, err := replay.Load(
		testReplayStorage(),
		artifactPath,
		configTestFactorySnapshotDecoder,
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	replayRuntimeCfg, err := runtimeConfigFromFactorySnapshot(loadedArtifact.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	return replayRuntimeCfg
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

func runtimeWorkerByName(t *testing.T, cfg *interfaces.FactoryConfig, name string) interfaces.FactoryWorkerConfig {
	t.Helper()
	for _, worker := range cfg.Workers {
		if worker.Name == name {
			return worker
		}
	}
	t.Fatalf("runtime Factory config has no worker %q", name)
	return interfaces.FactoryWorkerConfig{}
}

func assertRuntimeRoutes(t *testing.T, got, want []interfaces.IOConfig) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime routes = %#v, want %#v", got, want)
	}
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
) (*interfaces.FactoryWorkerConfig, *interfaces.FactoryWorkstationConfig) {
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
	factoryfixtures.WriteFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
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

func writeAgentsMD(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}
