//go:build functionallong

package runtime_api

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestEndToEndTopologyProjectionSmoke_LiveEventsAndReplayConfigMatch(t *testing.T) {
	support.SkipLongFunctional(t, "slow topology projection live-vs-replay sweep")

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{
			{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}}},
			{"name": "task-retry", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "task-followup", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "task-triage", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "task-backlog", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}},
			{"name": "task-failed", "states": []map[string]string{{"name": "failed", "type": "FAILED"}}},
			{"name": "task-abandoned", "states": []map[string]string{{"name": "failed", "type": "FAILED"}}},
		},
		"resources": []map[string]any{{"name": "executor-slot", "capacity": 2}},
		"workers":   []map[string]string{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id": "process-task-id", "name": "process-task", "worker": "executor",
			"inputs":      []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":     []map[string]string{{"workType": "task", "state": "complete"}},
			"onContinue":  []map[string]string{{"workType": "task-retry", "state": "init"}, {"workType": "task-followup", "state": "init"}},
			"onRejection": []map[string]string{{"workType": "task-triage", "state": "init"}, {"workType": "task-backlog", "state": "init"}},
			"onFailure":   []map[string]string{{"workType": "task-failed", "state": "failed"}, {"workType": "task-abandoned", "state": "failed"}},
			"resources":   []map[string]any{{"name": "executor-slot", "capacity": 1}},
			"guards":      []map[string]any{{"type": "VISIT_COUNT", "workstation": "process-task", "maxVisits": 3}},
			"stopWords":   []string{"BLOCKED"},
		}},
	})
	writeAgentConfig(t, dir, "executor", `---
type: MODEL_WORKER
executorProvider: codex-cli
modelProvider: openai
model: gpt-5.4
timeout: 30m
stopToken: COMPLETE
---
Process the input task.
`)
	writeWorkstationConfig(t, dir, "process-task", `---
type: MODEL_WORKSTATION
worker: executor
limits:
  maxRetries: 2
  maxExecutionTime: 10m
stopWords: ["DONE"]
---
Process {{ (index .Inputs 0).WorkID }}.
`)

	server := startFunctionalServer(t, dir, false, factory.WithServiceMode())
	stream := openFactoryEventHTTPStream(t, server.URL()+"/events")
	requireFunctionalEventStreamPrelude(t, stream)
	events, err := server.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one factory event")
	}
	liveWorld, err := projections.ReconstructFactoryWorldState(events, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	liveProjection := liveWorld.Topology

	replayProjection := projectReplayInitialStructureFromEmbeddedConfig(t, dir)

	assertTopologyWorker(t, liveProjection, interfaces.FactoryWorker{ID: "executor", Name: "executor", Provider: "SCRIPT_WRAP", ModelProvider: "CODEX", Model: "gpt-5.4", Config: map[string]string{"type": interfaces.WorkerTypeModel}})
	assertTopologyWorkstation(t, liveProjection, "process-task", "executor", []string{"task-retry:init", "task-followup:init"}, []string{"task-triage:init", "task-backlog:init"}, []string{"task-failed:failed", "task-abandoned:failed"})
	assertTopologyResource(t, liveProjection, "executor-slot", 2)
	assertTopologyWorker(t, replayProjection, interfaces.FactoryWorker{ID: "executor", Name: "executor", Provider: "script_wrap", ModelProvider: "codex", Model: "gpt-5.4", Config: map[string]string{"type": interfaces.WorkerTypeModel}})
	assertTopologyWorkstation(t, replayProjection, "process-task", "executor", []string{"task-retry:init", "task-followup:init"}, []string{"task-triage:init", "task-backlog:init"}, []string{"task-failed:failed", "task-abandoned:failed"})
	assertTopologyResource(t, replayProjection, "executor-slot", 2)
}

func projectReplayInitialStructureFromEmbeddedConfig(t *testing.T, dir string) interfaces.InitialStructurePayload {
	t.Helper()
	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generatedFactory, err := replay.GeneratedFactoryFromLoadedConfig(loaded, replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	replayRuntimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generatedFactory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	mapper := factoryconfig.ConfigMapper{}
	replayNet, err := mapper.Map(context.Background(), replayRuntimeCfg.Factory)
	if err != nil {
		t.Fatalf("Map replay factory: %v", err)
	}
	return projections.ProjectInitialStructure(replayNet, replayRuntimeCfg)
}

func writeWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()
	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertTopologyWorker(t *testing.T, payload interfaces.InitialStructurePayload, want interfaces.FactoryWorker) {
	t.Helper()
	for _, worker := range payload.Workers {
		if reflect.DeepEqual(worker, want) {
			return
		}
	}
	t.Fatalf("topology workers = %#v, want %#v", payload.Workers, want)
}

func assertTopologyWorkstation(t *testing.T, payload interfaces.InitialStructurePayload, id, workerID string, wantContinue []string, wantRejection []string, wantFailure []string) {
	t.Helper()
	for _, workstation := range payload.Workstations {
		if workstation.ID == id && workstation.WorkerID == workerID {
			if workstation.Config["type"] != interfaces.WorkstationTypeModel {
				t.Fatalf("workstation %q config = %#v, want model workstation type", id, workstation.Config)
			}
			if !reflect.DeepEqual(workstation.ContinuePlaceIDs, wantContinue) {
				t.Fatalf("workstation %q continue routes = %#v, want %#v", id, workstation.ContinuePlaceIDs, wantContinue)
			}
			if !reflect.DeepEqual(workstation.RejectionPlaceIDs, wantRejection) {
				t.Fatalf("workstation %q rejection routes = %#v, want %#v", id, workstation.RejectionPlaceIDs, wantRejection)
			}
			if !reflect.DeepEqual(workstation.FailurePlaceIDs, wantFailure) {
				t.Fatalf("workstation %q failure routes = %#v, want %#v", id, workstation.FailurePlaceIDs, wantFailure)
			}
			return
		}
	}
	t.Fatalf("topology workstations = %#v, want %s with worker %s", payload.Workstations, id, workerID)
}

func assertTopologyResource(t *testing.T, payload interfaces.InitialStructurePayload, id string, capacity int) {
	t.Helper()
	for _, resource := range payload.Resources {
		if resource.ID == id && resource.Capacity == capacity {
			return
		}
	}
	t.Fatalf("topology resources = %#v, want %s capacity %d", payload.Resources, id, capacity)
}
