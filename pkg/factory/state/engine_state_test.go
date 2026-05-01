package state

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestEngineStateSnapshot_AllFieldsAccessible(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	topology := &Net{ID: "snapshot-topology"}
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]{
		// Runtime state fields.
		RuntimeStatus: interfaces.RuntimeStatusActive,
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-1": {ID: "tok-1", PlaceID: "task:init", Color: interfaces.TokenColor{WorkTypeID: "task"}},
			},
		},
		Dispatches: map[string]*interfaces.DispatchEntry{
			"t1": {
				DispatchID:      "dispatch-1",
				TransitionID:    "t1",
				WorkstationName: "review",
				StartTime:       now,
				ConsumedTokens: []interfaces.Token{
					{
						ID:        "tok-2",
						PlaceID:   "task:processing",
						CreatedAt: now.Add(-time.Minute),
						Color:     interfaces.TokenColor{WorkID: "work-1", WorkTypeID: "task", TraceID: "trace-1"},
					},
				},
				HeldMutations: []interfaces.MarkingMutation{
					{Type: interfaces.MutationConsume, TokenID: "tok-2", FromPlace: "task:processing"},
				},
			},
		},
		InFlightCount: 1,
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				DispatchID:      "dispatch-0",
				TransitionID:    "t0",
				WorkstationName: "plan",
				Outcome:         "ACCEPTED",
				Duration:        5 * time.Second,
				ConsumedTokens: []interfaces.Token{
					{ID: "tok-0", PlaceID: "task:init", Color: interfaces.TokenColor{WorkID: "work-0", WorkTypeID: "task", TraceID: "trace-1"}},
				},
				OutputMutations: []interfaces.TokenMutationRecord{
					{
						DispatchID:   "dispatch-0",
						TransitionID: "t0",
						Outcome:      "ACCEPTED",
						Type:         interfaces.MutationCreate,
						TokenID:      "work-0",
						ToPlace:      "task:complete",
						Token: &interfaces.Token{
							ID:      "work-0",
							PlaceID: "task:complete",
							Color:   interfaces.TokenColor{WorkID: "work-0", WorkTypeID: "task", TraceID: "trace-1"},
						},
					},
				},
			},
		},
		ActiveThrottlePauses: []interfaces.ActiveThrottlePause{
			{
				LaneID:      "claude/claude-sonnet",
				Provider:    "claude",
				Model:       "claude-sonnet",
				PausedAt:    now,
				PausedUntil: now.Add(5 * time.Minute),
			},
		},
		TickCount: 42,

		// Factory lifecycle.
		FactoryState: "RUNNING",

		// Uptime.
		Uptime: 10 * time.Minute,

		// Topology.
		Topology: topology,
	}

	// Runtime state assertions.
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

	// Factory state.
	if snap.FactoryState != "RUNNING" {
		t.Errorf("expected FactoryState=RUNNING, got %s", snap.FactoryState)
	}

	// Uptime.
	if snap.Uptime != 10*time.Minute {
		t.Errorf("expected Uptime=10m, got %v", snap.Uptime)
	}
	if snap.Topology == nil || snap.Topology.ID != "snapshot-topology" {
		t.Fatalf("expected topology snapshot-topology, got %#v", snap.Topology)
	}

	runtime := snap.RuntimeStateSnapshot()
	if runtime.RuntimeStatus != snap.RuntimeStatus {
		t.Fatalf("runtime status = %q, want %q", runtime.RuntimeStatus, snap.RuntimeStatus)
	}
	if runtime.TickCount != snap.TickCount {
		t.Fatalf("runtime tick count = %d, want %d", runtime.TickCount, snap.TickCount)
	}
	if len(runtime.ActiveThrottlePauses) != 1 {
		t.Fatalf("runtime active throttle pause count = %d, want 1", len(runtime.ActiveThrottlePauses))
	}
	aggregate := NewEngineStateSnapshot(runtime, "RUNNING", time.Minute, topology)
	if len(aggregate.ActiveThrottlePauses) != 1 {
		t.Fatalf("aggregate active throttle pause count = %d, want 1", len(aggregate.ActiveThrottlePauses))
	}
}
