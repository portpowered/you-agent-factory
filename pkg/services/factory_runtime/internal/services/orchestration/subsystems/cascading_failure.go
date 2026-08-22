package subsystems

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
func NewCascadingFailure(n *state.Net, logger logging.Logger, now func() time.Time) *CascadingFailureSubsystem {
	if now == nil {
		panic("Factory Runtime cascading-failure clock is required")
	}
	return &CascadingFailureSubsystem{
		state:  n,
		logger: logging.EnsureLogger(logger),
		now:    now,
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
	dependents := make(map[string][]*factorytoken.Token)
	for _, tok := range snapshot.Marking.Tokens {
		for _, rel := range tok.Color.Relations {
			if rel.Type == work.RelationDependsOn {
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
				FailureRecords: []factorytoken.Failure{{
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

func shouldRouteTerminalFailureToFailedState(result resolvedWorkResult) bool {
	if result.outcome != workerexecution.OutcomeFailed || result.failureMetadata == nil {
		return false
	}
	// Cancellation is a terminal dispatch lifecycle result, but its late-result
	// path is absorbed by the runtime rather than routed through the authored
	// failure place. Keep that established behavior while routing other terminal
	// normalized failures directly to FAILED. Explicit dependency, timeout, and
	// throttling failures remain on their existing intermittent retry paths.
	if result.err == workerexecution.ErrWorkstationDispatchCanceled.Error() {
		return false
	}
	return workerexecution.FailureDecisionFromMetadata(result.failureMetadata).Terminal
}

func (t *TransitionerSubsystem) terminalFailureArcs(
	transition *petri.Transition,
	consumedTokens []factorytoken.Token,
) []petri.Arc {
	if t.netDefinition == nil || transition == nil {
		return nil
	}

	workTypeIDs := make([]string, 0, len(consumedTokens))
	seenWorkTypes := make(map[string]struct{}, len(consumedTokens))
	appendWorkType := func(workTypeID string) {
		if workTypeID == "" {
			return
		}
		if _, seen := seenWorkTypes[workTypeID]; seen {
			return
		}
		if _, exists := t.netDefinition.WorkTypes[workTypeID]; !exists {
			return
		}
		seenWorkTypes[workTypeID] = struct{}{}
		workTypeIDs = append(workTypeIDs, workTypeID)
	}

	for _, token := range consumedTokens {
		appendWorkType(token.Color.WorkTypeID)
	}
	if len(workTypeIDs) == 0 {
		for _, arc := range transition.InputArcs {
			place, ok := t.netDefinition.Places[arc.PlaceID]
			if !ok {
				continue
			}
			appendWorkType(place.TypeID)
		}
	}

	arcs := make([]petri.Arc, 0, len(workTypeIDs))
	for _, workTypeID := range workTypeIDs {
		workType := t.netDefinition.WorkTypes[workTypeID]
		failedState := ""
		for _, stateDefinition := range workType.States {
			if stateDefinition.Category == state.StateCategoryFailed {
				failedState = stateDefinition.Value
				break
			}
		}
		if failedState == "" {
			continue
		}
		failedPlaceID := state.PlaceID(workTypeID, failedState)
		arcs = append(arcs, petri.Arc{
			ID:           fmt.Sprintf("%s:terminal-failure:%s", transition.ID, failedPlaceID),
			Name:         fmt.Sprintf("%s:terminal-failure:%s", transition.ID, failedPlaceID),
			PlaceID:      failedPlaceID,
			TransitionID: transition.ID,
			Direction:    petri.ArcOutput,
			Cardinality:  petri.ArcCardinality{Mode: petri.CardinalityOne},
		})
	}
	return arcs
}

func (t *TransitionerSubsystem) calculateArcsForResolvedResult(
	currentTransition *petri.Transition,
	resolved resolvedWorkResult,
	consumedTokens []factorytoken.Token,
) ([]petri.Arc, resolvedWorkResult, error) {
	if shouldRouteTerminalFailureToFailedState(resolved) {
		if arcs := t.terminalFailureArcs(currentTransition, consumedTokens); len(arcs) > 0 {
			return arcs, resolved, nil
		}
	}
	workstation, ok := runtimeWorkstation(currentTransition.Name, t.runtimeConfig)
	if ok &&
		workstation != nil &&
		t.decisionEnvelopes != nil &&
		t.decisionEnvelopes.UsesGoalRoutingDecisionEnvelope(workstation) {
		if resolved.outcome == workerexecution.OutcomeAccepted {
			return matchClassificationLabelArcs(currentTransition, resolved.selectedClassificationLabel, resolved, "decision %q did not match any authored routing route")
		}
		arcs, err := calculateArcs(currentTransition, resolved.outcome)
		return arcs, resolved, err
	}
	if !ok || workstation == nil || workstation.Type != interfaces.WorkstationTypeClassify || resolved.outcome != workerexecution.OutcomeAccepted {
		arcs, err := calculateArcs(currentTransition, resolved.outcome)
		return arcs, resolved, err
	}
	return matchClassificationLabelArcs(currentTransition, resolved.output, resolved, "classifier label %q did not match any authored classification route")
}

// isInFailedPlace returns true if the token is in a FAILED-category place.
func (cf *CascadingFailureSubsystem) isInFailedPlace(token *factorytoken.Token) bool {
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
func (cf *CascadingFailureSubsystem) isTerminalOrFailed(token *factorytoken.Token) bool {
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
func (cf *CascadingFailureSubsystem) failedPlaceForToken(token *factorytoken.Token) string {
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
