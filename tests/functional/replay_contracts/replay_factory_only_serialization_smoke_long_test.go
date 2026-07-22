//go:build functionallong

package replay_contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayFactoryOnlySerializationSmoke_RecordReplayUsesRunStartedFactoryPayload(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay record/serialization smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_resource"))
	artifactPath := filepath.Join(t.TempDir(), "factory-only-serialization.replay.json")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "factory-only serialization smoke",
		WorkID:     "work-factory-only-serialization-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-factory-only-serialization-smoke",
		Payload:    []byte(`{"title":"factory-only serialization smoke"}`),
	})
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker": {
			{Content: "first pass needs another iteration"},
			{Content: "second pass needs another iteration"},
			{Content: "Done. COMPLETE"},
		},
		"finish-worker": {
			{Content: "Finalized. COMPLETE"},
		},
	})
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, recordServer.URL(), 15*time.Second)
	assertReplaySessionPlaces(t, support.GetDefaultSession(t, recordServer.URL()), map[string]int{
		"task:complete": 1, "task:init": 0, "task:failed": 0,
	})
	recordServer.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	runStarted := requireFactoryOnlyRunStartedPayload(t, testutil.GeneratedFactoryEvents(t, artifact.Events))
	assertFactoryOnlyArtifactJSON(t, artifactPath)
	assertFactoryOnlyPayloadCoversRepresentativeConfig(t, runStarted.Factory)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original factory dir: %v", err)
	}

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	support.WaitForTerminalStatus(t, replayServer.URL(), 15*time.Second)
	assertReplaySessionPlaces(t, support.GetDefaultSession(t, replayServer.URL()), map[string]int{
		"task:complete": 1, "task:init": 0, "task:failed": 0,
	})
	assertFactoryOnlyReplayProjectsInitialTopology(t, replayServer.GetFactoryEvents(t))
	replayServer.Stop(t)
}

func requireFactoryOnlyRunStartedPayload(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.RunRequestEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeRunRequest {
			continue
		}
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			t.Fatalf("decode run-request payload %q: %v", event.Id, err)
		}
		if payload.Factory.WorkTypes == nil || len(*payload.Factory.WorkTypes) == 0 {
			t.Fatalf("run-request payload factory missing work types: %#v", payload.Factory)
		}
		return payload
	}
	t.Fatalf("recorded events missing RUN_REQUEST: %#v", replayEventSummaries(events))
	return factoryapi.RunRequestEventPayload{}
}

func assertFactoryOnlyArtifactJSON(t *testing.T, artifactPath string) {
	t.Helper()

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read replay artifact %s: %v", artifactPath, err)
	}
	text := string(data)
	if !strings.Contains(text, `"factory"`) {
		t.Fatalf("replay artifact %s missing factory payload: %s", artifactPath, text)
	}
	for _, key := range factoryOnlyForbiddenConfigKeys() {
		if strings.Contains(text, key) {
			t.Fatalf("replay artifact %s contains legacy config key %q", artifactPath, key)
		}
	}
}

func assertFactoryOnlyPayloadCoversRepresentativeConfig(t *testing.T, factory factoryapi.Factory) {
	t.Helper()

	assertFactoryOnlyWorkType(t, generatedWorkTypes(factory), "task", []string{"init", "processing", "complete", "failed"})
	assertFactoryOnlyResource(t, generatedResources(factory), "slot", 1)
	assertFactoryOnlyWorker(t, generatedWorkers(factory), "exec-worker")
	assertFactoryOnlyWorker(t, generatedWorkers(factory), "finish-worker")
	assertFactoryOnlyWorkstation(t, generatedWorkstations(factory), "executor", "exec-worker", true)
	assertFactoryOnlyWorkstation(t, generatedWorkstations(factory), "finisher", "finish-worker", false)
}

func assertFactoryOnlyReplayProjectsInitialTopology(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	runRequest := requireFactoryOnlyRunStartedPayload(t, events)
	assertFactoryOnlyResource(t, generatedResources(runRequest.Factory), "slot", 1)
	assertFactoryOnlyWorker(t, generatedWorkers(runRequest.Factory), "exec-worker")
	assertFactoryOnlyWorkstation(t, generatedWorkstations(runRequest.Factory), "executor", "exec-worker", true)
}

func assertFactoryOnlyWorkType(t *testing.T, workTypes []factoryapi.WorkType, name string, states []string) {
	t.Helper()

	for _, workType := range workTypes {
		if workType.Name != name {
			continue
		}
		for _, state := range states {
			if !factoryOnlyHasState(workType.States, state) {
				t.Fatalf("work type %q states = %#v, want state %q", name, workType.States, state)
			}
		}
		return
	}
	t.Fatalf("generated work types = %#v, want %q", workTypes, name)
}

func assertFactoryOnlyResource(t *testing.T, resources []factoryapi.Resource, name string, capacity int) {
	t.Helper()

	for _, resource := range resources {
		if resource.Name == name && resource.Capacity == capacity {
			return
		}
	}
	t.Fatalf("generated resources = %#v, want %s capacity %d", resources, name, capacity)
}

func assertFactoryOnlyWorker(t *testing.T, workers []factoryapi.Worker, name string) {
	t.Helper()

	for _, worker := range workers {
		if worker.Name == name {
			if stringPointerValue(worker.Type) != interfaces.WorkerTypeAgent || stringPointerValue(worker.StopToken) != "COMPLETE" {
				t.Fatalf("worker %q type=%q stopToken=%q, want AGENT_WORKER/COMPLETE", name, stringPointerValue(worker.Type), stringPointerValue(worker.StopToken))
			}
			return
		}
	}
	t.Fatalf("generated workers = %#v, want runtime MODEL_WORKER %q", workers, name)
}

func assertFactoryOnlyWorkstation(t *testing.T, workstations []factoryapi.Workstation, name, worker string, wantResource bool) {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Name != name || workstation.Worker != worker {
			continue
		}
		if stringPointerValue(workstation.Type) != interfaces.WorkstationTypeAgent {
			t.Fatalf("workstation %q runtime type = %#v, want AGENT_RUN", name, workstation.Type)
		}
		if wantResource && !factoryOnlyHasResourceUsage(workstation.Resources, "slot", 1) {
			t.Fatalf("workstation %q resources = %#v, want slot total 1", name, workstation.Resources)
		}
		return
	}
	t.Fatalf("generated workstations = %#v, want %s using worker %s", workstations, name, worker)
}

func factoryOnlyForbiddenConfigKeys() []string {
	return []string{
		strings.Join([]string{"effective", "Config"}, ""),
		strings.Join([]string{"__replay", "Effective", "Config"}, ""),
		strings.Join([]string{"runtime", "Worker", "Config"}, ""),
	}
}

func factoryOnlyHasState(states []factoryapi.WorkState, name string) bool {
	for _, state := range states {
		if state.Name == name {
			return true
		}
	}
	return false
}

func factoryOnlyHasResourceUsage(resources *[]factoryapi.ResourceRequirement, name string, capacity int) bool {
	if resources == nil {
		return false
	}
	for _, resource := range *resources {
		if resource.Name == name && resource.Capacity == capacity {
			return true
		}
	}
	return false
}

func generatedWorkTypes(factory factoryapi.Factory) []factoryapi.WorkType {
	if factory.WorkTypes == nil {
		return nil
	}
	return *factory.WorkTypes
}

func generatedResources(factory factoryapi.Factory) []factoryapi.Resource {
	if factory.Resources == nil {
		return nil
	}
	return *factory.Resources
}

func generatedWorkers(factory factoryapi.Factory) []factoryapi.Worker {
	if factory.Workers == nil {
		return nil
	}
	return *factory.Workers
}

func generatedWorkstations(factory factoryapi.Factory) []factoryapi.Workstation {
	if factory.Workstations == nil {
		return nil
	}
	return *factory.Workstations
}

func replayEventSummaries(events []factoryapi.FactoryEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, string(event.Type)+"@"+event.Id)
	}
	return out
}

func assertReplaySessionPlaces(t *testing.T, session factoryapi.FactorySession, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.SessionPlaceTokenCount(session, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}
