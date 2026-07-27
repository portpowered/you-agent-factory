package rootobservation

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func sampleRootObservationSnapshot() *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus:          interfaces.RuntimeStatusActive,
		InFlightCount:          1,
		TickCount:              7,
		FactoryState:           string(interfaces.FactoryStateRunning),
		LifecycleControlStatus: "RUNNING",
		StreamGenerationID:     "gen-1",
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d1": {
				DispatchID:      "d1",
				WorkstationName: "desk",
				ConsumedTokens: []workerexecution.Token{
					{Color: workerexecution.Color{WorkID: "work-1"}},
					{Color: workerexecution.Color{WorkID: ""}},
				},
			},
			"nil": nil,
		},
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				DispatchID: "done-1",
				Outcome:    workerexecution.WorkOutcome("SUCCESS"),
				ConsumedTokens: []workerexecution.Token{
					{Color: workerexecution.Color{WorkID: "work-done"}},
				},
			},
		},
		Topology: &state.Net{Resources: map[string]*state.ResourceDef{
			"gpu": {ID: "gpu", Name: "GPU slot", Capacity: 2},
		}},
		Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			"gpu-1": {
				PlaceID: "gpu:available",
				Color: factorytoken.Color{
					DataType: factorytoken.DataTypeResource,
				},
			},
		}},
	}
}

func TestProject_NilSnapshotIsEmpty(t *testing.T) {
	empty := Project(nil, factory.ObservationScopeFull)
	if empty.Status != "" || empty.Progress != (factory.ObservationProgress{}) || len(empty.InFlightDispatches) != 0 || len(empty.Results) != 0 {
		t.Fatalf("Project(nil) = %#v, want empty", empty)
	}
}

func TestProject_FullProjectionOmitsNilDispatch(t *testing.T) {
	full := Project(sampleRootObservationSnapshot(), factory.ObservationScopeFull)
	if full.Status != factory.ObservationStatusActive {
		t.Fatalf("status = %q, want ACTIVE", full.Status)
	}
	if full.Progress.InFlightDispatchCount != 1 || full.Progress.TickCount != 7 {
		t.Fatalf("progress = %#v, want in-flight=1 tick=7", full.Progress)
	}
	if len(full.InFlightDispatches) != 1 {
		t.Fatalf("in-flight count = %d, want 1 (nil entry skipped)", len(full.InFlightDispatches))
	}
	if len(full.InFlightDispatches[0].WorkIDs) != 1 || full.InFlightDispatches[0].WorkIDs[0] != "work-1" {
		t.Fatalf("in-flight work ids = %#v, want [work-1]", full.InFlightDispatches[0].WorkIDs)
	}
	if len(full.Results) != 1 || full.Results[0].WorkID != "work-done" {
		t.Fatalf("results = %#v, want work-done", full.Results)
	}
	if full.Health.StreamGenerationID != "gen-1" {
		t.Fatalf("health = %#v, want gen-1", full.Health)
	}
	if len(full.Resources) != 1 || full.Resources[0] != (factory.ObservationResourceView{
		ResourceID: "gpu", ResourceName: "GPU slot", ResourceType: "RUNTIME", InUseCount: 1, AvailableCount: 1,
	}) {
		t.Fatalf("resources = %#v, want detached gpu usage", full.Resources)
	}
}

func TestProject_ScopeFilters(t *testing.T) {
	snap := sampleRootObservationSnapshot()

	statusOnly := Project(snap, factory.ObservationScopeStatus)
	if statusOnly.Status != factory.ObservationStatusActive || statusOnly.Progress != (factory.ObservationProgress{}) {
		t.Fatalf("STATUS scope = %#v, want status-only", statusOnly)
	}

	progressOnly := Project(snap, factory.ObservationScopeProgress)
	if progressOnly.Progress.TickCount != 7 || progressOnly.Status != "" {
		t.Fatalf("PROGRESS scope = %#v, want progress-only", progressOnly)
	}

	dispatchOnly := Project(snap, factory.ObservationScopeDispatches)
	if len(dispatchOnly.InFlightDispatches) != 1 || dispatchOnly.Status != "" {
		t.Fatalf("DISPATCHES scope = %#v, want dispatches-only", dispatchOnly)
	}

	resultsOnly := Project(snap, factory.ObservationScopeResults)
	if len(resultsOnly.Results) != 1 || resultsOnly.Status != "" {
		t.Fatalf("RESULTS scope = %#v, want results-only", resultsOnly)
	}

	resourcesOnly := Project(snap, factory.ObservationScopeResources)
	if resourcesOnly.Status != "" || len(resourcesOnly.Resources) != 1 || resourcesOnly.Resources[0].ResourceID != "gpu" {
		t.Fatalf("RESOURCES scope = %#v, want resources-only gpu view", resourcesOnly)
	}

	healthOnly := Project(snap, factory.ObservationScopeHealth)
	if healthOnly.Health.FactoryState == "" || healthOnly.Status != "" {
		t.Fatalf("HEALTH scope = %#v, want health-only", healthOnly)
	}

	unknownScope := Project(snap, factory.ObservationScope("OTHER"))
	if unknownScope.Status != factory.ObservationStatusActive {
		t.Fatalf("unknown scope = %#v, want full observation fallback", unknownScope)
	}
}

func TestProject_MapsRuntimeStatus(t *testing.T) {
	idleSnap := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatusIdle}
	if got := Project(idleSnap, ""); got.Status != factory.ObservationStatusIdle {
		t.Fatalf("idle status = %q, want IDLE", got.Status)
	}

	finishedSnap := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatusFinished}
	if got := Project(finishedSnap, factory.ObservationScopeFull); got.Status != factory.ObservationStatusFinished {
		t.Fatalf("finished status = %q, want FINISHED", got.Status)
	}

	unknownStatus := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatus("weird")}
	if got := Project(unknownStatus, factory.ObservationScopeFull); got.Status != factory.ObservationStatusIdle {
		t.Fatalf("unknown runtime status = %q, want IDLE default", got.Status)
	}
}

func TestProject_FactoryStatusMatchesSnapshotProjection(t *testing.T) {
	t.Parallel()
	snap := sampleRootObservationSnapshot()
	fromSnapshot := factory.NewFactoryStatusProjector().ProjectFactoryStatus(snap)
	fromObservation := factory.FactoryStatusFromObservation(Project(snap, factory.ObservationScopeFull))
	if fromObservation.RuntimeStatus != fromSnapshot.RuntimeStatus ||
		fromObservation.FactoryState != fromSnapshot.FactoryState ||
		fromObservation.TotalTokens != fromSnapshot.TotalTokens ||
		fromObservation.Categories != fromSnapshot.Categories ||
		len(fromObservation.Resources) != len(fromSnapshot.Resources) {
		t.Fatalf("observation status = %#v, want snapshot parity %#v", fromObservation, fromSnapshot)
	}
	for i := range fromObservation.Resources {
		if fromObservation.Resources[i] != fromSnapshot.Resources[i] {
			t.Fatalf("resource[%d] = %#v, want %#v", i, fromObservation.Resources[i], fromSnapshot.Resources[i])
		}
	}
}
