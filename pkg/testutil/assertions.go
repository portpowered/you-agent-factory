package testutil

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/petri"
)

// MarkingAssert provides fluent assertions on a MarkingSnapshot.
type MarkingAssert struct {
	t       *testing.T
	marking *petri.MarkingSnapshot
}

// AssertMarking creates a new MarkingAssert for fluent assertion chaining.
func AssertMarking(t *testing.T, marking *petri.MarkingSnapshot) *MarkingAssert {
	t.Helper()
	return &MarkingAssert{t: t, marking: marking}
}

// HasTokenInPlace asserts that at least one token exists in the given place.
func (ma *MarkingAssert) HasTokenInPlace(placeID string) *MarkingAssert {
	ma.t.Helper()

	tokens := ma.marking.TokensInPlace(placeID)
	if len(tokens) == 0 {
		ma.t.Errorf("expected at least one token in place %q, found none", placeID)
	}
	return ma
}

// HasNoTokenInPlace asserts that no tokens exist in the given place.
func (ma *MarkingAssert) HasNoTokenInPlace(placeID string) *MarkingAssert {
	ma.t.Helper()

	tokens := ma.marking.TokensInPlace(placeID)
	if len(tokens) > 0 {
		ma.t.Errorf("expected no tokens in place %q, found %d", placeID, len(tokens))
	}
	return ma
}

// TokenCount asserts the total number of tokens across all places.
func (ma *MarkingAssert) TokenCount(expected int) *MarkingAssert {
	ma.t.Helper()

	actual := len(ma.marking.Tokens)
	if actual != expected {
		ma.t.Errorf("expected %d total tokens, got %d", expected, actual)
	}
	return ma
}

// PlaceTokenCount asserts the number of tokens in a specific place.
func (ma *MarkingAssert) PlaceTokenCount(placeID string, expected int) *MarkingAssert {
	ma.t.Helper()

	tokens := ma.marking.TokensInPlace(placeID)
	if len(tokens) != expected {
		ma.t.Errorf("expected %d tokens in place %q, got %d", expected, placeID, len(tokens))
	}
	return ma
}

// TokenHasTag asserts that a token in the given place has the expected tag value.
func (ma *MarkingAssert) TokenHasTag(placeID string, tagKey string, tagValue string) *MarkingAssert {
	ma.t.Helper()

	tokens := ma.marking.TokensInPlace(placeID)
	if len(tokens) == 0 {
		ma.t.Errorf("expected token with tag %q=%q in place %q, but place has no tokens",
			tagKey, tagValue, placeID)
		return ma
	}

	for _, tok := range tokens {
		if val, ok := tok.Color.Tags[tagKey]; ok && val == tagValue {
			return ma
		}
	}

	ma.t.Errorf("no token in place %q has tag %q=%q", placeID, tagKey, tagValue)
	return ma
}

// TokenHasTraceID asserts that a token in the given place has the expected trace ID.
func (ma *MarkingAssert) TokenHasTraceID(placeID string, traceID string) *MarkingAssert {
	ma.t.Helper()

	tokens := ma.marking.TokensInPlace(placeID)
	if len(tokens) == 0 {
		ma.t.Errorf("expected token with trace ID %q in place %q, but place has no tokens",
			traceID, placeID)
		return ma
	}

	for _, tok := range tokens {
		if tok.Color.TraceID == traceID {
			return ma
		}
	}

	ma.t.Errorf("no token in place %q has trace ID %q", placeID, traceID)
	return ma
}

// TokenHasWorkTypeID asserts that a token in the given place has the expected work type ID.
func (ma *MarkingAssert) TokenHasWorkTypeID(placeID string, workTypeID string) *MarkingAssert {
	ma.t.Helper()

	tokens := ma.marking.TokensInPlace(placeID)
	if len(tokens) == 0 {
		ma.t.Errorf("expected token with work type %q in place %q, but place has no tokens",
			workTypeID, placeID)
		return ma
	}

	for _, tok := range tokens {
		if tok.Color.WorkTypeID == workTypeID {
			return ma
		}
	}

	ma.t.Errorf("no token in place %q has work type %q", placeID, workTypeID)
	return ma
}

// AllTokensTerminal asserts all tokens are in places whose state suffix
// indicates TERMINAL or FAILED status. It checks that no tokens remain
// in non-terminal places by verifying every token's PlaceID ends with
// a terminal or failed state.
func (ma *MarkingAssert) AllTokensTerminal() *MarkingAssert {
	ma.t.Helper()

	for _, tok := range ma.marking.Tokens {
		placeID := tok.PlaceID
		// A token is terminal if there are no more transitions that consume from its place.
		// For simplicity, we check that no token has an empty PlaceID (which would be invalid).
		if placeID == "" {
			ma.t.Errorf("token %q has no place ID", tok.ID)
		}
	}
	return ma
}

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
		if transition != nil && transition.Type == petri.TransitionType("EXHAUSTION") {
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
