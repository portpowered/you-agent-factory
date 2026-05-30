package testutil

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestAssertNoTransitionExhaustion_PassesWithoutExhaustion(t *testing.T) {
	AssertNoTransitionExhaustion(t, map[string]*petri.Transition{
		"work": {Type: petri.TransitionNormal},
	}, PetriTransitionAssertOptions{ExhaustionContext: "customer-authored mapping"})
}

func TestAssertNoTransitionExhaustion_IgnoresNilTransitions(t *testing.T) {
	AssertNoTransitionExhaustion(t, map[string]*petri.Transition{
		"nil-entry": nil,
		"work":      {Type: petri.TransitionNormal},
	}, PetriTransitionAssertOptions{ExhaustionContext: "replay-mapped customer config"})
}

func TestAssertGuardedLoopBreakerTransition_ValidPasses(t *testing.T) {
	transition := &petri.Transition{
		Type: petri.TransitionNormal,
		InputArcs: []petri.Arc{{
			PlaceID: "task:init",
			Guard: &petri.VisitCountGuard{
				TransitionID: "reviewer",
				MaxVisits:    3,
			},
		}},
		OutputArcs: []petri.Arc{{PlaceID: "task:failed"}},
	}

	AssertGuardedLoopBreakerTransition(t, transition, "task:init", "task:failed", "reviewer", 3)
}
