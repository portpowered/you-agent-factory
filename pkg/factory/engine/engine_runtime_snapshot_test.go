package engine

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

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
	eng := NewFactoryEngine(
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
	eng := NewFactoryEngine(
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
