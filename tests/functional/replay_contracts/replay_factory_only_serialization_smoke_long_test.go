//go:build functionallong

package replay_contracts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayFactoryOnlySerializationSmoke_RecordReplayUsesRunStartedFactoryPayload(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay record/serialization smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_resource"))
	artifactPath := filepath.Join(t.TempDir(), "factory-only-serialization.replay.json")
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "factory-only serialization smoke",
		WorkID:     "work-factory-only-serialization-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-factory-only-serialization-smoke",
		Payload:    []byte(`{"title":"factory-only serialization smoke"}`),
	})
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"exec-worker": {
			{Content: "first pass needs another iteration"},
			{Content: "second pass needs another iteration"},
			{Content: "Done. COMPLETE"},
		},
		"finish-worker": {
			{Content: "Finalized. COMPLETE"},
		},
	})
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)

	harness.RunUntilComplete(t, 15*time.Second)
	harness.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	runStarted := requireFactoryOnlyRunStartedPayload(t, artifact.Events)
	assertFactoryOnlyArtifactJSON(t, artifactPath)
	assertFactoryOnlyPayloadCoversRepresentativeConfig(t, runStarted.Factory)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original factory dir: %v", err)
	}

	assertFactoryOnlyPayloadProjectsInitialTopology(t, runStarted.Factory)
	replayHarness := testutil.AssertReplaySucceeds(t, artifactPath, 15*time.Second)
	replayHarness.Service.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
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

func assertFactoryOnlyPayloadProjectsInitialTopology(t *testing.T, factory factoryapi.Factory) {
	t.Helper()

	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), runtimeCfg.Factory)
	if err != nil {
		t.Fatalf("map generated factory config: %v", err)
	}
	topology := projections.ProjectInitialStructure(net, runtimeCfg)
	assertProjectedResource(t, topology.Resources, "slot", 1)
	assertProjectedWorker(t, topology.Workers, "exec-worker")
	assertProjectedWorkstation(t, topology.Workstations, "executor", "exec-worker")
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
		if worker.Name == name && stringPointerValue(worker.Type) == interfaces.WorkerTypeModel && stringPointerValue(worker.StopToken) == "COMPLETE" {
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
		if stringPointerValue(workstation.Type) != interfaces.WorkstationTypeModel {
			t.Fatalf("workstation %q runtime type = %#v, want MODEL_WORKSTATION", name, workstation.Type)
		}
		if wantResource && !factoryOnlyHasResourceUsage(workstation.Resources, "slot", 1) {
			t.Fatalf("workstation %q resources = %#v, want slot total 1", name, workstation.Resources)
		}
		return
	}
	t.Fatalf("generated workstations = %#v, want %s using worker %s", workstations, name, worker)
}

func assertProjectedResource(t *testing.T, resources []interfaces.FactoryResource, name string, capacity int) {
	t.Helper()

	for _, resource := range resources {
		if resource.Name == name && resource.Capacity == capacity {
			return
		}
	}
	t.Fatalf("projected resources = %#v, want %s capacity %d", resources, name, capacity)
}

func assertProjectedWorker(t *testing.T, workers []interfaces.FactoryWorker, id string) {
	t.Helper()

	for _, worker := range workers {
		if worker.ID == id && worker.Config["type"] == interfaces.WorkerTypeModel {
			return
		}
	}
	t.Fatalf("projected workers = %#v, want %s", workers, id)
}

func assertProjectedWorkstation(t *testing.T, workstations []interfaces.FactoryWorkstation, name, workerID string) {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Name == name && workstation.WorkerID == workerID {
			return
		}
	}
	t.Fatalf("projected workstations = %#v, want %s using worker %s", workstations, name, workerID)
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
