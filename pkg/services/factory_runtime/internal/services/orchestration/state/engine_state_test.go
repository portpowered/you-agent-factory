package state

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func TestSnapshotHasActiveWork(t *testing.T) {
	t.Parallel()

	snap := engineStateSnapshotFixture()
	if !SnapshotHasActiveWork(&snap) {
		t.Fatal("fixture snapshot should report active work")
	}
	if SnapshotHasActiveWork(nil) {
		t.Fatal("nil snapshot should not report active work")
	}

	dispatched := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{"d1": {}},
	}
	if !SnapshotHasActiveWork(&dispatched) {
		t.Fatal("dispatch map should report active work")
	}

	topology := &Net{
		Places: map[string]*petri.Place{
			PlaceID("task", "complete"): {
				ID:     PlaceID("task", "complete"),
				TypeID: "task",
				State:  "complete",
			},
			PlaceID("task", "init"): {
				ID:     PlaceID("task", "init"),
				TypeID: "task",
				State:  "init",
			},
		},
		WorkTypes: map[string]*WorkType{
			"task": {
				ID: "task",
				States: []StateDefinition{
					{Value: "complete", Category: StateCategoryTerminal},
					{Value: "failed", Category: StateCategoryFailed},
					{Value: "init", Category: StateCategoryInitial},
				},
			},
		},
	}
	terminalOnly := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]{
		Topology: topology,
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*factorytoken.Token{
				"tok-terminal": {
					ID:      "tok-terminal",
					PlaceID: PlaceID("task", "complete"),
					Color:   factorytoken.Color{WorkTypeID: "task"},
				},
			},
		},
	}
	if SnapshotHasActiveWork(&terminalOnly) {
		t.Fatal("terminal-only marking should not report active work")
	}

	processing := terminalOnly
	processing.Marking.Tokens["tok-active"] = &factorytoken.Token{
		ID:      "tok-active",
		PlaceID: PlaceID("task", "init"),
		Color:   factorytoken.Color{WorkTypeID: "task"},
	}
	if !SnapshotHasActiveWork(&processing) {
		t.Fatal("non-terminal work token should report active work")
	}

	resourceOnly := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]{
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*factorytoken.Token{
				"tok-resource": {
					ID:      "tok-resource",
					PlaceID: "gpu:available",
					Color:   factorytoken.Color{DataType: factorytoken.DataTypeResource},
				},
			},
		},
	}
	if SnapshotHasActiveWork(&resourceOnly) {
		t.Fatal("resource tokens should not report active work")
	}

	noTopology := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]{
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*factorytoken.Token{
				"tok-work": {
					ID:      "tok-work",
					PlaceID: PlaceID("task", "init"),
					Color:   factorytoken.Color{WorkTypeID: "task"},
				},
			},
		},
	}
	if !SnapshotHasActiveWork(&noTopology) {
		t.Fatal("work token without topology should report active work")
	}
}

func TestEngineStateSnapshot_AllFieldsAccessible(t *testing.T) {
	snap := engineStateSnapshotFixture()

	t.Run("runtime fields", func(t *testing.T) {
		assertSnapshotRuntimeFields(t, snap)
	})
	t.Run("factory metadata", func(t *testing.T) {
		assertSnapshotFactoryMetadata(t, snap)
	})
	t.Run("runtime projection", func(t *testing.T) {
		assertRuntimeStateSnapshot(t, snap)
	})
	t.Run("aggregate construction", func(t *testing.T) {
		runtime := snap.RuntimeStateSnapshot()
		assertNewEngineStateSnapshot(t, runtime, "RUNNING", time.Minute, snap.Topology)
	})
}

func assertSnapshotRuntimeFields(t *testing.T, snap interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]) {
	t.Helper()

	if len(snap.Marking.Tokens) != 1 {
		t.Errorf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	if len(snap.Dispatches) != 1 {
		t.Errorf("expected 1 dispatch, got %d", len(snap.Dispatches))
	}
	if len(snap.Dispatches["t1"].ConsumedTokens) != 1 {
		t.Errorf("expected 1 consumed token, got %d", len(snap.Dispatches["t1"].ConsumedTokens))
	}
	if snap.InFlightCount != 1 {
		t.Errorf("expected InFlightCount=1, got %d", snap.InFlightCount)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusActive {
		t.Errorf("expected RuntimeStatus=ACTIVE, got %s", snap.RuntimeStatus)
	}
	if snap.TickCount != 42 {
		t.Errorf("expected TickCount=42, got %d", snap.TickCount)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Errorf("expected 1 completed dispatch, got %d", len(snap.DispatchHistory))
	}
	if len(snap.DispatchHistory[0].ConsumedTokens) != 1 {
		t.Errorf("expected 1 completed dispatch consumed token, got %d", len(snap.DispatchHistory[0].ConsumedTokens))
	}
	if len(snap.DispatchHistory[0].OutputMutations) != 1 {
		t.Errorf("expected 1 completed dispatch output mutation, got %d", len(snap.DispatchHistory[0].OutputMutations))
	}
	if len(snap.ActiveThrottlePauses) != 1 {
		t.Fatalf("expected 1 active throttle pause, got %d", len(snap.ActiveThrottlePauses))
	}
	if snap.ActiveThrottlePauses[0].LaneID != "claude/claude-sonnet" {
		t.Fatalf("active throttle pause lane = %q, want claude/claude-sonnet", snap.ActiveThrottlePauses[0].LaneID)
	}
}

func assertRuntimeStateSnapshot(t *testing.T, snap interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]) {
	t.Helper()

	runtime := snap.RuntimeStateSnapshot()
	if runtime.RuntimeStatus != snap.RuntimeStatus {
		t.Errorf("runtime status = %q, want %q", runtime.RuntimeStatus, snap.RuntimeStatus)
	}
	if runtime.TickCount != snap.TickCount {
		t.Errorf("runtime tick count = %d, want %d", runtime.TickCount, snap.TickCount)
	}
	if len(runtime.ActiveThrottlePauses) != len(snap.ActiveThrottlePauses) {
		t.Fatalf("runtime active throttle pause count = %d, want %d", len(runtime.ActiveThrottlePauses), len(snap.ActiveThrottlePauses))
	}
}

func assertNewEngineStateSnapshot(
	t *testing.T,
	runtime interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net],
	factoryState string,
	uptime time.Duration,
	topology *Net,
) {
	t.Helper()

	aggregate := NewEngineStateSnapshot(runtime, factoryState, uptime, topology)
	if len(aggregate.ActiveThrottlePauses) != len(runtime.ActiveThrottlePauses) {
		t.Errorf("aggregate active throttle pause count = %d, want %d", len(aggregate.ActiveThrottlePauses), len(runtime.ActiveThrottlePauses))
	}
	if aggregate.FactoryState != factoryState {
		t.Errorf("aggregate factory state = %q, want %q", aggregate.FactoryState, factoryState)
	}
	if aggregate.Uptime != uptime {
		t.Errorf("aggregate uptime = %v, want %v", aggregate.Uptime, uptime)
	}
	if aggregate.Topology != topology {
		t.Fatalf("aggregate topology = %#v, want %#v", aggregate.Topology, topology)
	}
}

func assertSnapshotFactoryMetadata(t *testing.T, snap interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]) {
	t.Helper()

	if snap.FactoryState != "RUNNING" {
		t.Errorf("expected FactoryState=RUNNING, got %s", snap.FactoryState)
	}
	if snap.Uptime != 10*time.Minute {
		t.Errorf("expected Uptime=10m, got %v", snap.Uptime)
	}
	if snap.Topology == nil || snap.Topology.ID != "snapshot-topology" {
		t.Fatalf("expected topology snapshot-topology, got %#v", snap.Topology)
	}
}

func engineStateSnapshotFixture() interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net] {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	topology := &Net{ID: "snapshot-topology"}
	return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*factorytoken.Token{
				"tok-1": {ID: "tok-1", PlaceID: "task:init", Color: factorytoken.Color{WorkTypeID: "task"}},
			},
		},
		Dispatches: map[string]*interfaces.DispatchEntry{
			"t1": {
				DispatchID:      "dispatch-1",
				TransitionID:    "t1",
				WorkstationName: "review",
				StartTime:       now,
				ConsumedTokens: []factorytoken.Token{{
					ID:        "tok-2",
					PlaceID:   "task:processing",
					CreatedAt: now.Add(-time.Minute),
					Color:     factorytoken.Color{WorkID: "work-1", WorkTypeID: "task", TraceID: "trace-1"},
				}},
				HeldMutations: []interfaces.MarkingMutation{{
					Type: interfaces.MutationConsume, TokenID: "tok-2", FromPlace: "task:processing",
				}},
			},
		},
		InFlightCount: 1,
		DispatchHistory: []interfaces.CompletedDispatch{{
			DispatchID:      "dispatch-0",
			TransitionID:    "t0",
			WorkstationName: "plan",
			Outcome:         "ACCEPTED",
			Duration:        5 * time.Second,
			ConsumedTokens: []factorytoken.Token{{
				ID: "tok-0", PlaceID: "task:init", Color: factorytoken.Color{WorkID: "work-0", WorkTypeID: "task", TraceID: "trace-1"},
			}},
			OutputMutations: []interfaces.TokenMutationRecord{{
				DispatchID:   "dispatch-0",
				TransitionID: "t0",
				Outcome:      "ACCEPTED",
				Type:         interfaces.MutationCreate,
				TokenID:      "work-0",
				ToPlace:      "task:complete",
				Token: &factorytoken.Token{
					ID:      "work-0",
					PlaceID: "task:complete",
					Color:   factorytoken.Color{WorkID: "work-0", WorkTypeID: "task", TraceID: "trace-1"},
				},
			}},
		}},
		ActiveThrottlePauses: []interfaces.ActiveThrottlePause{{
			LaneID:      "claude/claude-sonnet",
			Provider:    "claude",
			Model:       "claude-sonnet",
			PausedAt:    now,
			PausedUntil: now.Add(5 * time.Minute),
		}},
		TickCount:    42,
		FactoryState: "RUNNING",
		Uptime:       10 * time.Minute,
		Topology:     topology,
	}
}
