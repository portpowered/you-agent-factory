package subsystems

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// CascadingFailureSubsystem propagates failure from parent/dependency tokens
// to their dependents. When a token enters a FAILED place, all tokens that
// declare a DEPENDS_ON relation targeting it are automatically moved to their
// own FAILED place. Propagation is transitive within a single tick via BFS.
type CascadingFailureSubsystem struct {
	state  *state.Net
	logger logging.Logger
	now    func() time.Time
}

// NewCascadingFailure creates a new CascadingFailureSubsystem.
func NewCascadingFailure(n *state.Net, logger logging.Logger) *CascadingFailureSubsystem {
	return &CascadingFailureSubsystem{
		state:  n,
		logger: logging.EnsureLogger(logger),
		now:    time.Now,
	}
}

var _ Subsystem = (*CascadingFailureSubsystem)(nil)

// TickGroup returns CascadingFailure (15), after Transitioner so newly failed
// tokens are visible, before the Tracer.
func (cf *CascadingFailureSubsystem) TickGroup() TickGroup {
	return CascadingFailure
}

// Execute scans the marking for failed tokens and cascades failure to any
// dependent tokens that are not yet in a terminal or failed state.
// Propagation is transitive: if P fails → C1 fails → C2 fails, all within
// a single Execute call via BFS.
func (cf *CascadingFailureSubsystem) Execute(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	// Build reverse index: WorkID → tokens that DEPEND_ON that WorkID.
	dependents := make(map[string][]*interfaces.Token)
	for _, tok := range snapshot.Marking.Tokens {
		for _, rel := range tok.Color.Relations {
			if rel.Type == interfaces.RelationDependsOn {
				dependents[rel.TargetWorkID] = append(dependents[rel.TargetWorkID], tok)
			}
		}
	}

	// No dependency relations at all — nothing to cascade.
	if len(dependents) == 0 {
		return nil, nil
	}

	// Seed the BFS queue with all currently failed tokens.
	var queue []string // WorkIDs to process
	for _, tok := range snapshot.Marking.Tokens {
		if cf.isInFailedPlace(tok) {
			queue = append(queue, tok.Color.WorkID)
		}
	}

	// BFS: cascade failure transitively.
	var mutations []interfaces.MarkingMutation
	cascaded := make(map[string]bool) // token IDs already moved

	now := cf.now()
	for len(queue) > 0 {
		currentWorkID := queue[0]
		queue = queue[1:]

		for _, dep := range dependents[currentWorkID] {
			if cascaded[dep.ID] {
				continue
			}
			if cf.isTerminalOrFailed(dep) {
				continue
			}

			failedPlace := cf.failedPlaceForToken(dep)
			if failedPlace == "" {
				continue
			}

			cascaded[dep.ID] = true
			cf.logger.Info("cascading-failure: propagating failure",
				"token", dep.ID, "dependency", currentWorkID, "to_place", failedPlace)

			mutations = append(mutations, interfaces.MarkingMutation{
				Type:      interfaces.MutationMove,
				TokenID:   dep.ID,
				FromPlace: dep.PlaceID,
				ToPlace:   failedPlace,
				Reason:    fmt.Sprintf("cascading failure: dependency %s failed", currentWorkID),
				FailureRecords: []interfaces.FailureRecord{{
					TransitionID: "",
					Timestamp:    now,
					Error:        fmt.Sprintf("cascading failure: dependency %s failed", currentWorkID),
					Attempt:      0,
				}},
			})

			// Queue newly-failed token for transitive cascading.
			queue = append(queue, dep.Color.WorkID)
		}
	}

	if len(mutations) == 0 {
		return nil, nil
	}

	return &interfaces.TickResult{Mutations: mutations}, nil
}

// isInFailedPlace returns true if the token is in a FAILED-category place.
func (cf *CascadingFailureSubsystem) isInFailedPlace(token *interfaces.Token) bool {
	place, ok := cf.state.Places[token.PlaceID]
	if !ok {
		return false
	}
	wt, ok := cf.state.WorkTypes[place.TypeID]
	if !ok {
		return false
	}
	for _, s := range wt.States {
		if s.Value == place.State {
			return s.Category == state.StateCategoryFailed
		}
	}
	return false
}

// isTerminalOrFailed returns true if the token is in a TERMINAL or FAILED place.
func (cf *CascadingFailureSubsystem) isTerminalOrFailed(token *interfaces.Token) bool {
	place, ok := cf.state.Places[token.PlaceID]
	if !ok {
		return false
	}
	wt, ok := cf.state.WorkTypes[place.TypeID]
	if !ok {
		return false
	}
	for _, s := range wt.States {
		if s.Value == place.State {
			return s.Category == state.StateCategoryTerminal || s.Category == state.StateCategoryFailed
		}
	}
	return false
}

// failedPlaceForToken returns the FAILED place ID for the token's work type.
func (cf *CascadingFailureSubsystem) failedPlaceForToken(token *interfaces.Token) string {
	wt, ok := cf.state.WorkTypes[token.Color.WorkTypeID]
	if !ok {
		return ""
	}
	for _, s := range wt.States {
		if s.Category == state.StateCategoryFailed {
			return state.PlaceID(wt.ID, s.Value)
		}
	}
	return ""
}
