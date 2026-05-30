package engine

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestTickCallsSubsystem(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	if _, err := submitWorkRequests(context.Background(), engine, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-1"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if sub.callCount != 1 {
		t.Errorf("expected subsystem called once, got %d", sub.callCount)
	}
	if sub.lastSnap == nil {
		t.Fatal("subsystem did not receive a marking snapshot")
	}

	tokensInInit := sub.lastSnap.Marking.TokensInPlace("task:init")
	if len(tokensInInit) != 1 {
		t.Fatalf("expected 1 token in task:init, got %d", len(tokensInInit))
	}
	if tokensInInit[0].Color.WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", tokensInInit[0].Color.WorkTypeID)
	}
	if tokensInInit[0].Color.TraceID != "trace-1" {
		t.Errorf("expected TraceID 'trace-1', got %q", tokensInInit[0].Color.TraceID)
	}
}

func TestTickNRunsMultipleTicks(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	if err := engine.TickN(context.Background(), 3); err != nil {
		t.Fatalf("TickN() error: %v", err)
	}
	if sub.callCount != 3 {
		t.Errorf("expected 3 calls, got %d", sub.callCount)
	}
}

func TestTickUntilStopsOnPredicate(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	err := engine.TickUntil(context.Background(), func(snap *petri.MarkingSnapshot) bool {
		return snap.TickCount >= 2
	}, 10)
	if err != nil {
		t.Fatalf("TickUntil() error: %v", err)
	}
	if sub.callCount != 2 {
		t.Errorf("expected 2 calls, got %d", sub.callCount)
	}
}

func TestTickUntilReturnsErrorOnMaxTicks(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	err := engine.TickUntil(context.Background(), func(_ *petri.MarkingSnapshot) bool {
		return false
	}, 3)
	if err == nil {
		t.Fatal("expected error when predicate never satisfied")
	}
}

func TestSubsystemsSortedByTickGroup(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	var order []subsystems.TickGroup
	makeSub := func(g subsystems.TickGroup) *mockSubsystem {
		return &mockSubsystem{
			group: g,
			execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
				order = append(order, g)
				return &interfaces.TickResult{}, nil
			},
		}
	}

	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{
		makeSub(subsystems.TerminationCheck),
		makeSub(subsystems.CircuitBreaker),
		makeSub(subsystems.Scheduler),
	})
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	expected := []subsystems.TickGroup{subsystems.CircuitBreaker, subsystems.Scheduler, subsystems.TerminationCheck}
	if len(order) != len(expected) {
		t.Fatalf("expected %d subsystems called, got %d", len(expected), len(order))
	}
	for i, g := range expected {
		if order[i] != g {
			t.Errorf("position %d: expected TickGroup %d, got %d", i, g, order[i])
		}
	}
}

func TestTickWhileAutomaticTicksPaused_SkipsSubsystemExecution(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	paused := true
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if sub.callCount != 0 {
		t.Fatalf("subsystem callCount = %d, want 0 while automatic ticks are paused", sub.callCount)
	}

	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() after resume error: %v", err)
	}
	if sub.callCount != 1 {
		t.Fatalf("subsystem callCount = %d, want 1 after automatic ticks resume", sub.callCount)
	}
}

func TestTickWhileAutomaticTicksPaused_SkipsCascadeMutations(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(&interfaces.Token{
		ID:      "parent-tok",
		PlaceID: "task:failed",
		Color:   interfaces.TokenColor{WorkID: "parent-work", WorkTypeID: "task"},
		History: newTestTokenHistory(),
	})
	marking.AddToken(&interfaces.Token{
		ID:      "child-tok",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			WorkID:     "child-work",
			WorkTypeID: "task",
			Relations: []interfaces.Relation{{
				Type:          interfaces.RelationDependsOn,
				TargetWorkID:  "parent-work",
				RequiredState: "complete",
			}},
		},
		History: newTestTokenHistory(),
	})

	engine := NewFactoryEngine(
		n,
		marking,
		[]subsystems.Subsystem{subsystems.NewCascadingFailure(n, nil)},
		WithAutomaticTicksPaused(func() bool { return true }),
	)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	child, ok := engine.GetMarking().Tokens["child-tok"]
	if !ok {
		t.Fatal("child token missing from marking")
	}
	if child.PlaceID != "task:init" {
		t.Fatalf("child place = %q, want task:init while paused (no cascade)", child.PlaceID)
	}
}

func newTestTokenHistory() interfaces.TokenHistory {
	return interfaces.TokenHistory{
		TotalVisits:         make(map[string]int),
		ConsecutiveFailures: make(map[string]int),
		PlaceVisits:         make(map[string]int),
	}
}

func TestMutationsAppliedBetweenSubsystems(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(&interfaces.Token{
		ID:      "tok-1",
		PlaceID: "task:init",
		Color:   interfaces.TokenColor{WorkTypeID: "task"},
		History: interfaces.TokenHistory{
			TotalVisits:         make(map[string]int),
			ConsecutiveFailures: make(map[string]int),
			PlaceVisits:         make(map[string]int),
		},
	})

	mover := &mockSubsystem{
		group: subsystems.Scheduler,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Mutations: []interfaces.MarkingMutation{{
					Type:      interfaces.MutationMove,
					TokenID:   "tok-1",
					FromPlace: "task:init",
					ToPlace:   "task:complete",
				}},
			}, nil
		},
	}
	var observedPlace string
	observer := &mockSubsystem{
		group: subsystems.Tracer,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if tok, ok := snap.Marking.Tokens["tok-1"]; ok {
				observedPlace = tok.PlaceID
			}
			return &interfaces.TickResult{}, nil
		},
	}

	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{mover, observer})
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if observedPlace != "task:complete" {
		t.Errorf("expected observer to see token in 'task:complete', got %q", observedPlace)
	}
}
