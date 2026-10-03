package engine

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/factorystatus"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestRuntimeStateSnapshotPublishesWholeDispatchBatchBeforeExecution(t *testing.T) {
	t.Parallel()
	net := buildTestNet()
	resource := &state.ResourceDef{ID: "gpu", Name: "GPU", Capacity: 2}
	place, _ := state.GenerateResourcePlaces(resource, time.Time{})
	net.Resources[resource.ID] = resource
	net.Places[place.ID] = place
	marking := petri.NewMarking(net.ID)
	mutations := make([]interfaces.MarkingMutation, 2)
	dispatches := make([]interfaces.DispatchRecord, 2)
	for index, id := range []string{"first", "second"} {
		token := testDispatchToken(id, place.ID, factorytoken.DataTypeResource, time.Time{})
		marking.AddToken(token)
		mutations[index] = interfaces.MarkingMutation{Type: interfaces.MutationConsume, TokenID: id, FromPlace: place.ID}
		dispatches[index] = interfaces.DispatchRecord{
			Dispatch:  work.WorkDispatch{DispatchID: id, TransitionID: "t1"},
			Mutations: []interfaces.MarkingMutation{mutations[index]},
		}
	}
	dispatcher := &mockSubsystem{group: subsystems.Dispatcher,
		execFn: func(context.Context, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{Mutations: mutations, Dispatches: dispatches}, nil
		}}
	hook := newTestDispatchResultHook()
	recorded := make(map[string]interfaces.FactoryDispatchRecord)
	eng := newTestFactoryEngine(net, marking, []subsystems.Subsystem{dispatcher},
		WithDispatchResultHook(hook),
		WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) { recorded[record.DispatchID] = record }),
	)
	hook.submit = func(_ context.Context, dispatch work.WorkDispatch) error {
		// The tick still owns its mutex here: this reads the published boundary
		// at the first possible external effect, without a scheduling race.
		snapshot := eng.GetRuntimeStateSnapshot()
		if len(recorded) != 2 || len(snapshot.Dispatches) != 2 || snapshot.InFlightCount != 2 {
			t.Fatalf("before execution %q: recorded=%d dispatches=%d in-flight=%d, want complete batch", dispatch.DispatchID, len(recorded), len(snapshot.Dispatches), snapshot.InFlightCount)
		}
		assertHeldDispatchReservations(t, snapshot.Dispatches, dispatch.DispatchID, "first", "second")
		// Observe supplies the immutable topology to the detached snapshot.
		snapshot.Topology = net
		usage := factorystatus.ProjectFromSnapshot(&snapshot).Resources
		if len(usage) != 1 || usage[0].Name != "gpu" || usage[0].Available != 0 || usage[0].Total != 2 {
			t.Fatalf("before execution %q: resource usage=%#v, want both GPU reservations", dispatch.DispatchID, usage)
		}
		if snapshot.TickCount != 1 || dispatch.Execution.DispatchCreatedTick != 1 || dispatch.Execution.CurrentTick != 1 {
			t.Fatalf("before execution %q: tick=%d execution=%#v, want tick 1", dispatch.DispatchID, snapshot.TickCount, dispatch.Execution)
		}
		return nil
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(hook.submits) != 2 {
		t.Fatalf("submitted dispatches=%d, want 2", len(hook.submits))
	}
}

// assertHeldDispatchReservations checks that every dispatch in a published batch
// is already reserved, holding exactly the GPU token named by its own ID, at the
// first possible external effect of the batch. executingID names the dispatch
// currently being submitted so failures point at the observed boundary.
func assertHeldDispatchReservations(t *testing.T, dispatches map[string]*interfaces.DispatchEntry, executingID string, tokenIDs ...string) {
	t.Helper()
	for _, tokenID := range tokenIDs {
		entry := dispatches[tokenID]
		if entry == nil || len(entry.HeldMutations) != 1 || entry.HeldMutations[0].TokenID != tokenID {
			t.Fatalf("before execution %q: reservation %q=%#v, want its held GPU token", executingID, tokenID, entry)
		}
	}
}

func TestRuntimeStateSnapshot_IncludesActiveThrottlePausesFromSubsystem(t *testing.T) {
	pausedAt := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	pausedUntil := pausedAt.Add(5 * time.Minute)
	observer := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				ActiveThrottlePauses: []interfaces.ActiveThrottlePause{{
					LaneID:      "claude/claude-sonnet",
					Provider:    "claude",
					Model:       "claude-sonnet",
					PausedAt:    pausedAt,
					PausedUntil: pausedUntil,
				}},
				ThrottlePausesObserved: true,
			}, nil
		},
	}
	eng := newTestFactoryEngine(
		&state.Net{ID: "net", Places: map[string]*petri.Place{}},
		petri.NewMarking("net"),
		[]subsystems.Subsystem{observer},
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap := eng.GetRuntimeStateSnapshot()
	if len(snap.ActiveThrottlePauses) != 1 {
		t.Fatalf("ActiveThrottlePauses = %d, want 1", len(snap.ActiveThrottlePauses))
	}
	pause := snap.ActiveThrottlePauses[0]
	if pause.Provider != "claude" || pause.Model != "claude-sonnet" || pause.LaneID != "claude/claude-sonnet" {
		t.Fatalf("unexpected active throttle pause: %#v", pause)
	}
	if !pause.PausedAt.Equal(pausedAt) || !pause.PausedUntil.Equal(pausedUntil) {
		t.Fatalf("unexpected pause window: %#v", pause)
	}

	snap.ActiveThrottlePauses[0].Provider = "mutated"
	next := eng.GetRuntimeStateSnapshot()
	if next.ActiveThrottlePauses[0].Provider != "claude" {
		t.Fatalf("runtime snapshot did not deep-copy active throttle pauses: %#v", next.ActiveThrottlePauses[0])
	}
}

func TestRuntimeStateSnapshot_ClearsActiveThrottlePausesWhenObservedEmpty(t *testing.T) {
	pausedAt := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	observer := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if len(snap.ActiveThrottlePauses) == 0 {
				return &interfaces.TickResult{
					ActiveThrottlePauses: []interfaces.ActiveThrottlePause{{
						LaneID:      "claude/claude-sonnet",
						Provider:    "claude",
						Model:       "claude-sonnet",
						PausedAt:    pausedAt,
						PausedUntil: pausedAt.Add(5 * time.Minute),
					}},
					ThrottlePausesObserved: true,
				}, nil
			}
			return &interfaces.TickResult{
				ActiveThrottlePauses:   []interfaces.ActiveThrottlePause{},
				ThrottlePausesObserved: true,
			}, nil
		},
	}
	eng := newTestFactoryEngine(
		&state.Net{ID: "net", Places: map[string]*petri.Place{}},
		petri.NewMarking("net"),
		[]subsystems.Subsystem{observer},
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	first := eng.GetRuntimeStateSnapshot()
	if len(first.ActiveThrottlePauses) != 1 {
		t.Fatalf("first ActiveThrottlePauses = %d, want 1", len(first.ActiveThrottlePauses))
	}

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	second := eng.GetRuntimeStateSnapshot()
	if len(second.ActiveThrottlePauses) != 0 {
		t.Fatalf("second ActiveThrottlePauses = %d, want 0", len(second.ActiveThrottlePauses))
	}
}

func TestRuntimeStateSnapshotDoesNotWaitForActiveDispatchHandler(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(context.Context, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: work.WorkDispatch{DispatchID: "dispatch-blocked", TransitionID: "transition-blocked"},
				}},
			}, nil
		},
	}
	eng := newTestFactoryEngine(
		&state.Net{ID: "net", Places: map[string]*petri.Place{}},
		petri.NewMarking("net"),
		[]subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(work.WorkDispatch) {
			close(started)
			<-release
		}),
	)

	tickDone := make(chan error, 1)
	go func() { tickDone <- eng.Tick(context.Background()) }()
	<-started

	snapshotReady := make(chan interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], 1)
	go func() { snapshotReady <- eng.GetRuntimeStateSnapshot() }()
	select {
	case snapshot := <-snapshotReady:
		if snapshot.TickCount != 1 || len(snapshot.Dispatches) != 1 {
			t.Fatalf("busy-tick snapshot = tick %d, dispatches %d; want published dispatch boundary", snapshot.TickCount, len(snapshot.Dispatches))
		}
	case <-time.After(time.Second):
		close(release)
		<-tickDone
		t.Fatal("GetRuntimeStateSnapshot waited for the active dispatch handler")
	}

	close(release)
	if err := <-tickDone; err != nil {
		t.Fatalf("Tick: %v", err)
	}
	final := eng.GetRuntimeStateSnapshot()
	if final.TickCount != 1 || len(final.Dispatches) != 1 {
		t.Fatalf("final snapshot = tick %d, dispatches %d; want completed tick boundary", final.TickCount, len(final.Dispatches))
	}
}
