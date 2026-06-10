package runtime

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestTick_ExhaustionTransitionFiresThroughCircuitBreakerRuntimePath(t *testing.T) {
	f, err := New(
		factory.WithNet(buildReviewLoopWithExhaustionNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{
		WorkID:     "work-exhaust",
		WorkTypeID: "task",
		TraceID:    "trace-exhaust",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	tickable := tickableFactory(t, f)
	for attempt := 0; attempt < 3; attempt++ {
		if err := tickable.Tick(ctx); err != nil {
			t.Fatalf("Tick review loop attempt %d: %v", attempt+1, err)
		}
		if !markingContainsWorkAtPlace(snapshotMarking(t, f, ctx), "work-exhaust", "task:init") {
			t.Fatalf("attempt %d: expected work-exhaust to remain in task:init before exhaustion threshold", attempt+1)
		}
	}

	if err := tickable.Tick(ctx); err != nil {
		t.Fatalf("Tick exhaustion: %v", err)
	}
	if !markingContainsWorkAtPlace(snapshotMarking(t, f, ctx), "work-exhaust", "task:failed") {
		t.Fatalf("marking after exhaustion = %#v, want work-exhaust at task:failed", snapshotMarking(t, f, ctx).Tokens)
	}
	if markingContainsWorkAtPlace(snapshotMarking(t, f, ctx), "work-exhaust", "task:init") {
		t.Fatalf("marking after exhaustion = %#v, want work-exhaust removed from task:init", snapshotMarking(t, f, ctx).Tokens)
	}
}

func buildReviewLoopWithExhaustionNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}

	places := make(map[string]*petri.Place)
	for _, place := range wt.GeneratePlaces() {
		places[place.ID] = place
	}

	return &state.Net{
		ID:     "review-loop-with-exhaustion",
		Places: places,
		Transitions: map[string]*petri.Transition{
			"review": {
				ID:         "review",
				Name:       "Review",
				Type:       petri.TransitionNormal,
				WorkerType: "mock",
				InputArcs: []petri.Arc{{
					ID:          "review-in",
					Name:        "work",
					PlaceID:     "task:init",
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
				}},
				OutputArcs: []petri.Arc{{
					ID:          "review-out",
					Name:        "work",
					PlaceID:     "task:init",
					Direction:   petri.ArcOutput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
				}},
			},
			"review-exhausted": {
				ID:   "review-exhausted",
				Name: "review-exhausted",
				Type: petri.TransitionExhaustion,
				InputArcs: []petri.Arc{{
					ID:          "exhaust-in",
					Name:        "work",
					PlaceID:     "task:init",
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					Guard: &petri.VisitCountGuard{
						TransitionID: "review",
						MaxVisits:    3,
					},
				}},
				OutputArcs: []petri.Arc{{
					ID:        "exhaust-out",
					Name:      "failed",
					PlaceID:   "task:failed",
					Direction: petri.ArcOutput,
				}},
			},
		},
		WorkTypes: map[string]*state.WorkType{"task": wt},
		Resources: make(map[string]*state.ResourceDef),
	}
}

func snapshotMarking(t *testing.T, f factory.Factory, ctx context.Context) *petri.MarkingSnapshot {
	t.Helper()

	snap, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return &snap.Marking
}
