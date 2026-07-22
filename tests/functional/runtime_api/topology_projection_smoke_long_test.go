//go:build functionallong

package runtime_api

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	support.WriteAgentConfig(t, dir, "executor", `---
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

	artifactPath := filepath.Join(t.TempDir(), "topology-projection.replay.json")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges:      serviceedges.Edges{},
	})
	stream := openDefaultSessionFactoryEventHTTPStream(t, server.URL())
	requireFunctionalEventStreamPrelude(t, stream)
	events := server.GetFactoryEvents(t)
	if len(events) == 0 {
		t.Fatal("expected at least one factory event")
	}
	liveFactory := requireGeneratedSchemaRunStartedPayload(t, events).Factory
	server.Stop(t)

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	replayEvents := replayServer.GetFactoryEvents(t)
	replayFactory := requireGeneratedSchemaRunStartedPayload(t, replayEvents).Factory
	replayServer.Stop(t)

	for _, publicFactory := range []factoryapi.Factory{liveFactory, replayFactory} {
		assertTopologyWorker(t, publicFactory, "executor", "gpt-5.4")
		assertTopologyWorkstation(t, publicFactory, "process-task", "executor", []string{"task-retry:init", "task-followup:init"}, []string{"task-triage:init", "task-backlog:init"}, []string{"task-failed:failed", "task-abandoned:failed"})
		assertTopologyResource(t, publicFactory, "executor-slot", 2)
	}
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

func assertTopologyWorker(t *testing.T, factory factoryapi.Factory, name, model string) {
	t.Helper()
	if factory.Workers == nil {
		t.Fatal("public RUN_REQUEST Factory has no workers")
	}
	for _, worker := range *factory.Workers {
		if worker.Name == name && stringPointerValue(worker.Model) == model {
			return
		}
	}
	t.Fatalf("public RUN_REQUEST workers = %#v, want %s model %s", *factory.Workers, name, model)
}

func assertTopologyWorkstation(t *testing.T, factory factoryapi.Factory, name, workerName string, wantContinue []string, wantRejection []string, wantFailure []string) {
	t.Helper()
	if factory.Workstations == nil {
		t.Fatal("public RUN_REQUEST Factory has no workstations")
	}
	for _, workstation := range *factory.Workstations {
		if workstation.Name == name && workstation.Worker == workerName {
			if !reflect.DeepEqual(topologyRouteIDs(workstation.OnContinue), wantContinue) {
				t.Fatalf("workstation %q continue routes = %#v, want %#v", name, topologyRouteIDs(workstation.OnContinue), wantContinue)
			}
			if !reflect.DeepEqual(topologyRouteIDs(workstation.OnRejection), wantRejection) {
				t.Fatalf("workstation %q rejection routes = %#v, want %#v", name, topologyRouteIDs(workstation.OnRejection), wantRejection)
			}
			if !reflect.DeepEqual(topologyRouteIDs(workstation.OnFailure), wantFailure) {
				t.Fatalf("workstation %q failure routes = %#v, want %#v", name, topologyRouteIDs(workstation.OnFailure), wantFailure)
			}
			return
		}
	}
	t.Fatalf("public RUN_REQUEST workstations = %#v, want %s with worker %s", *factory.Workstations, name, workerName)
}

func assertTopologyResource(t *testing.T, factory factoryapi.Factory, name string, capacity int) {
	t.Helper()
	if factory.Resources == nil {
		t.Fatal("public RUN_REQUEST Factory has no resources")
	}
	for _, resource := range *factory.Resources {
		if resource.Name == name && resource.Capacity == capacity {
			return
		}
	}
	t.Fatalf("public RUN_REQUEST resources = %#v, want %s capacity %d", *factory.Resources, name, capacity)
}

func topologyRouteIDs(routes *[]factoryapi.WorkstationIO) []string {
	if routes == nil {
		return nil
	}
	ids := make([]string, 0, len(*routes))
	for _, route := range *routes {
		ids = append(ids, route.WorkType+":"+route.State)
	}
	return ids
}
