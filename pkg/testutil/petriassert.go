package testutil

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/petri"
)

// PetriTransitionAssertOptions configures Petri transition assertion failure messages.
type PetriTransitionAssertOptions struct {
	// ExhaustionContext is embedded in AssertNoTransitionExhaustion fatals
	// (for example "customer-authored mapping" or "replay-mapped customer config").
	ExhaustionContext string
}

// AssertNoTransitionExhaustion fails when any non-nil transition has type TransitionExhaustion.
func AssertNoTransitionExhaustion(t *testing.T, transitions map[string]*petri.Transition, opts PetriTransitionAssertOptions) {
	t.Helper()

	for name, transition := range transitions {
		if transition != nil && transition.Type == petri.TransitionExhaustion {
			t.Fatalf("unexpected TransitionExhaustion transition %q in %s", name, opts.ExhaustionContext)
		}
	}
}

// AssertGuardedLoopBreakerTransition asserts a normal loop-breaker transition with a single
// VisitCountGuard input arc and a single output arc to the expected places.
func AssertGuardedLoopBreakerTransition(
	t *testing.T,
	transition *petri.Transition,
	inputPlace string,
	outputPlace string,
	watchedTransitionID string,
	maxVisits int,
) {
	t.Helper()

	if transition == nil {
		t.Fatal("expected guarded loop-breaker transition to exist")
	}
	if transition.Type != petri.TransitionNormal {
		t.Fatalf("guarded loop-breaker type = %s, want %s", transition.Type, petri.TransitionNormal)
	}
	if len(transition.InputArcs) != 1 {
		t.Fatalf("guarded loop-breaker input arcs = %d, want 1", len(transition.InputArcs))
	}
	if transition.InputArcs[0].PlaceID != inputPlace {
		t.Fatalf("guarded loop-breaker input place = %q, want %q", transition.InputArcs[0].PlaceID, inputPlace)
	}
	guard, ok := transition.InputArcs[0].Guard.(*petri.VisitCountGuard)
	if !ok {
		t.Fatalf("expected VisitCountGuard on guarded loop breaker, got %T", transition.InputArcs[0].Guard)
	}
	if guard.TransitionID != watchedTransitionID {
		t.Fatalf("guarded loop-breaker guard transition = %q, want %q", guard.TransitionID, watchedTransitionID)
	}
	if guard.MaxVisits != maxVisits {
		t.Fatalf("guarded loop-breaker guard max visits = %d, want %d", guard.MaxVisits, maxVisits)
	}
	if len(transition.OutputArcs) != 1 {
		t.Fatalf("guarded loop-breaker output arcs = %d, want 1", len(transition.OutputArcs))
	}
	if transition.OutputArcs[0].PlaceID != outputPlace {
		t.Fatalf("guarded loop-breaker output place = %q, want %q", transition.OutputArcs[0].PlaceID, outputPlace)
	}
}
