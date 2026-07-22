package subsystems

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TerminationCheckSubsystem detects when the runtime snapshot has no active
// work left: no dispatches are in flight, all resource tokens have been
// returned, and every visible token is already terminal or failed. It is
// intentionally snapshot-driven: it does not query transition enablement and
// it does not retain its own lifecycle state.
type TerminationCheckSubsystem struct {
	state       *state.Net
	logger      logging.Logger
	runtimeMode interfaces.RuntimeMode
}

// NewTerminationCheck creates a new TerminationCheckSubsystem.
func NewTerminationCheck(n *state.Net, logger logging.Logger, mode interfaces.RuntimeMode) *TerminationCheckSubsystem {
	if mode == "" {
		mode = interfaces.RuntimeModeBatch
	}
	return &TerminationCheckSubsystem{
		state:       n,
		logger:      logging.EnsureLogger(logger),
		runtimeMode: mode,
	}
}

var _ Subsystem = (*TerminationCheckSubsystem)(nil)

// TickGroup returns TerminationCheck (40).
func (tc *TerminationCheckSubsystem) TickGroup() TickGroup {
	return TerminationCheck
}

// Execute checks if the snapshot shows a fully terminated workflow.
func (tc *TerminationCheckSubsystem) Execute(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	if tc.runtimeMode != interfaces.RuntimeModeBatch {
		return nil, nil
	}
	if !tc.shouldTerminate(snapshot) {
		return nil, nil
	}

	tc.logger.Info("termination-check: no active work remains in the snapshot",
		"tokens", len(snapshot.Marking.Tokens),
		"in_flight", snapshot.InFlightCount)

	return &interfaces.TickResult{ShouldTerminate: true}, nil
}

func (tc *TerminationCheckSubsystem) shouldTerminate(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.InFlightCount > 0 {
		return false
	}
	if !tc.allResourcesReturned(&snapshot.Marking) {
		return false
	}

	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		if !tc.isTerminalOrFailed(token) {
			return false
		}
	}

	return true
}

// isTerminalOrFailed returns true if the token is in a TERMINAL or FAILED place.
func (tc *TerminationCheckSubsystem) isTerminalOrFailed(token *factorytoken.Token) bool {
	place, ok := tc.state.Places[token.PlaceID]
	if !ok {
		return false
	}
	wt, ok := tc.state.WorkTypes[place.TypeID]
	if !ok {
		_, isResource := tc.state.Resources[place.TypeID]
		return isResource
	}
	for _, s := range wt.States {
		if s.Value == place.State {
			return s.Category == state.StateCategoryTerminal || s.Category == state.StateCategoryFailed
		}
	}
	return false
}

// allResourcesReturned checks that each resource place has at least its initial
// capacity of tokens (i.e., consumed resources have been returned).
func (tc *TerminationCheckSubsystem) allResourcesReturned(snapshot *petri.MarkingSnapshot) bool {
	for _, res := range tc.state.Resources {
		placeID := state.PlaceID(res.ID, interfaces.ResourceStateAvailable)
		tokensInPlace := snapshot.TokensInPlace(placeID)
		if len(tokensInPlace) < res.Capacity {
			return false
		}
	}
	return true
}

const (
	executorReviewWorkstationProcess = "process"
	executorReviewWorkstationReview  = "review"
)

// executorReviewReconcileMutations consumes superseded same-trace review and task
// residue after process or review workstation completion so one recovery trace
// converges to a single durable queue outcome.
func executorReviewReconcileMutations(
	marking *petri.MarkingSnapshot,
	workstationName string,
	outcome workerexecution.WorkOutcome,
	consumedTokens []factorytoken.Token,
	outputArcs []petri.Arc,
	now time.Time,
) []interfaces.MarkingMutation {
	if marking == nil || outcome != workerexecution.OutcomeAccepted {
		return nil
	}

	traceID, laneName := executorReviewLineageFromConsumed(consumedTokens)
	if traceID == "" {
		return nil
	}

	outputPlaces := outputPlaceIDs(outputArcs)
	switch workstationName {
	case executorReviewWorkstationProcess:
		if !containsPlace(outputPlaces, state.PlaceID("review", "init")) || laneName == "" {
			return nil
		}
		return consumeTokensAtPlaceForTrace(
			marking,
			state.PlaceID("review", "init"),
			traceID,
			laneName,
			consumedTokenIDs(consumedTokens),
			now,
			"executor-review reconcile: supersede duplicate review:init before process output",
		)
	case executorReviewWorkstationReview:
		if !containsPlace(outputPlaces, state.PlaceID("task", "to-complete")) &&
			!containsPlace(outputPlaces, state.PlaceID("task", "complete")) {
			return nil
		}
		excluded := consumedTokenIDs(consumedTokens)
		var mutations []interfaces.MarkingMutation
		if laneName != "" {
			mutations = consumeTokensAtPlaceForTrace(
				marking,
				state.PlaceID("review", "init"),
				traceID,
				laneName,
				excluded,
				now,
				"executor-review reconcile: remove duplicate review:init after review completion",
			)
		}
		mutations = append(mutations, consumeTokensAtPlaceForTrace(
			marking,
			state.PlaceID("task", "init"),
			traceID,
			laneName,
			excluded,
			now,
			"executor-review reconcile: remove stale task:init after review completion",
		)...)
		mutations = append(mutations, consumeTokensAtPlaceForTrace(
			marking,
			state.PlaceID("task", "failed"),
			traceID,
			laneName,
			excluded,
			now,
			"executor-review reconcile: remove stale task:failed after review completion",
		)...)
		return mutations
	default:
		return nil
	}
}

func executorReviewLineageFromConsumed(consumed []factorytoken.Token) (traceID string, laneName string) {
	traceID = factorytoken.CurrentChainingTraceID(consumed, interfaces.SystemTimeWorkTypeID)
	for i := range consumed {
		if consumed[i].Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if consumed[i].Color.Name != "" {
			laneName = consumed[i].Color.Name
			break
		}
	}
	return traceID, laneName
}

func outputPlaceIDs(arcs []petri.Arc) []string {
	places := make([]string, 0, len(arcs))
	for i := range arcs {
		if arcs[i].PlaceID == "" {
			continue
		}
		places = append(places, arcs[i].PlaceID)
	}
	return places
}

func containsPlace(places []string, want string) bool {
	for _, placeID := range places {
		if placeID == want {
			return true
		}
	}
	return false
}

func consumedTokenIDs(consumed []factorytoken.Token) map[string]struct{} {
	if len(consumed) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(consumed))
	for i := range consumed {
		if consumed[i].ID == "" {
			continue
		}
		ids[consumed[i].ID] = struct{}{}
	}
	return ids
}

func consumeTokensAtPlaceForTrace(
	marking *petri.MarkingSnapshot,
	placeID string,
	traceID string,
	laneName string,
	excluded map[string]struct{},
	now time.Time,
	reason string,
) []interfaces.MarkingMutation {
	if marking == nil || traceID == "" || placeID == "" {
		return nil
	}

	mutations := make([]interfaces.MarkingMutation, 0)
	for _, token := range marking.TokensInPlace(placeID) {
		if excluded != nil {
			if _, skip := excluded[token.ID]; skip {
				continue
			}
		}
		if canonicalExecutorReviewTraceID(token.Color) != traceID {
			continue
		}
		if laneName != "" && token.Color.Name != laneName {
			continue
		}
		mutations = append(mutations, interfaces.MarkingMutation{
			Type:      interfaces.MutationConsume,
			TokenID:   token.ID,
			FromPlace: placeID,
			Reason:    fmt.Sprintf("%s (%s)", reason, token.ID),
		})
	}
	return mutations
}

func canonicalExecutorReviewTraceID(color factorytoken.Color) string {
	if color.CurrentChainingTraceID != "" {
		return color.CurrentChainingTraceID
	}
	return color.TraceID
}
