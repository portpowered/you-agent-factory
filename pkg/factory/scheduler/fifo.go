package scheduler

import (
	"sort"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// FIFOScheduler selects transitions to fire in the order they appear in the enabled
// list (first-in, first-out). It greedily assigns tokens and skips transitions whose
// required tokens have already been claimed by an earlier decision.
type FIFOScheduler struct{}

// NewFIFOScheduler creates a new FIFOScheduler.
func NewFIFOScheduler() *FIFOScheduler {
	return &FIFOScheduler{}
}

// Select iterates enabled transitions in order, greedily claiming tokens. If any
// token required by a transition has already been claimed, that transition is skipped.
func (s *FIFOScheduler) Select(enabled []interfaces.EnabledTransition, _ /* snapshot not needed for FIFO */ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	var decisions []interfaces.FiringDecision
	claimed := make(map[string]bool) // token IDs already assigned to a firing

	for _, et := range enabled {
		// Collect all token IDs this transition needs to consume.
		// OBSERVE-mode arcs are checked for conflicts but not consumed.
		var tokenIDs []string
		inputBindings := make(map[string][]string)
		conflict := false

		// Sort binding keys for deterministic token ordering. Without this,
		// Go map iteration is random, causing non-deterministic ConsumeTokens
		// order which propagates through the dispatcher and transitioner.
		arcNames := make([]string, 0, len(et.Bindings))
		for arcName := range et.Bindings {
			arcNames = append(arcNames, arcName)
		}
		sort.Strings(arcNames)

		for _, arcName := range arcNames {
			tokens := et.Bindings[arcName]
			for i := range tokens {
				id := tokens[i].ID
				if claimed[id] {
					conflict = true
					break
				}
				// Only consume tokens from CONSUME-mode arcs.
				if et.ArcModes[arcName] != interfaces.ArcModeObserve {
					tokenIDs = append(tokenIDs, id)
					inputBindings[arcName] = append(inputBindings[arcName], id)
				}
			}
			if conflict {
				break
			}
		}
		if conflict {
			continue
		}

		// Claim all tokens for this firing.
		for _, id := range tokenIDs {
			claimed[id] = true
		}

		decisions = append(decisions, interfaces.FiringDecision{
			TransitionID:  et.TransitionID,
			ConsumeTokens: tokenIDs,
			WorkerType:    et.WorkerType,
			InputBindings: inputBindings,
		})
	}

	return decisions
}
