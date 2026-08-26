package subsystems

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TerminationCheckSubsystem detects when the runtime snapshot has no active
// work left. A finite runtime completes when all customer Work is terminal;
// when non-terminal Work is drained but no transition is immediately runnable,
// it returns an explicit incomplete classification. It is intentionally
// snapshot-driven and does not retain lifecycle state.
type TerminationCheckSubsystem struct {
	state       *state.Net
	logger      logging.Logger
	runtimeMode interfaces.RuntimeMode
	evaluator   *scheduler.EnablementEvaluator
}

// NewTerminationCheck creates a new TerminationCheckSubsystem.
func NewTerminationCheck(n *state.Net, logger logging.Logger, mode interfaces.RuntimeMode) *TerminationCheckSubsystem {
	return NewTerminationCheckWithRuntime(n, logger, mode, nil, time.Now)
}

// NewTerminationCheckWithRuntime creates a termination checker using the same
// runtime definition lookup and clock as the Dispatcher enablement boundary.
func NewTerminationCheckWithRuntime(
	n *state.Net,
	logger logging.Logger,
	mode interfaces.RuntimeMode,
	runtimeConfig interfaces.RuntimeDefinitionLookup,
	now func() time.Time,
) *TerminationCheckSubsystem {
	if mode == "" {
		mode = interfaces.RuntimeModeBatch
	}
	if now == nil {
		now = time.Now
	}
	l := logging.EnsureLogger(logger)
	return &TerminationCheckSubsystem{
		state:       n,
		logger:      l,
		runtimeMode: mode,
		evaluator:   scheduler.NewEnablementEvaluator(l, now, runtimeConfig),
	}
}

var _ Subsystem = (*TerminationCheckSubsystem)(nil)

// TickGroup returns TerminationCheck (40).
func (tc *TerminationCheckSubsystem) TickGroup() TickGroup {
	return TerminationCheck
}

// Execute classifies a finite runtime only after dispatches and resources have
// quiesced. Service mode deliberately never emits a finite termination result.
func (tc *TerminationCheckSubsystem) Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	if tc.runtimeMode != interfaces.RuntimeModeBatch {
		return nil, nil
	}
	termination := tc.classify(ctx, snapshot)
	if termination == nil {
		return nil, nil
	}

	tc.logger.Info("termination-check: finite runtime classified",
		"classification", termination.Classification,
		"non_terminal_work_items", termination.NonTerminalWorkCount,
		"tokens", len(snapshot.Marking.Tokens),
		"in_flight", snapshot.InFlightCount)

	return &interfaces.TickResult{
		ShouldTerminate: true,
		Termination:     termination,
	}, nil
}

func (tc *TerminationCheckSubsystem) classify(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) *interfaces.TerminationResult {
	if snapshot == nil {
		return nil
	}
	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 {
		return nil
	}
	if !tc.allResourcesReturned(&snapshot.Marking) {
		return nil
	}

	nonTerminalWork := tc.nonTerminalWorkIDs(snapshot.Marking.Tokens)
	if len(nonTerminalWork) == 0 {
		return &interfaces.TerminationResult{
			Classification: interfaces.TerminationClassificationComplete,
		}
	}

	if tc.hasImmediatelyRunnableActivity(ctx, snapshot) {
		return nil
	}

	return &interfaces.TerminationResult{
		Classification:       interfaces.TerminationClassificationIncomplete,
		NonTerminalWorkCount: len(nonTerminalWork),
	}
}

func (tc *TerminationCheckSubsystem) nonTerminalWorkIDs(tokens map[string]*factorytoken.Token) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, token := range tokens {
		if !tc.isCustomerWorkToken(token) || tc.isTerminalOrFailed(token) {
			continue
		}
		if token.Color.WorkID != "" {
			ids[token.Color.WorkID] = struct{}{}
		}
	}
	return ids
}

func (tc *TerminationCheckSubsystem) isCustomerWorkToken(token *factorytoken.Token) bool {
	if tc.state == nil || token == nil || token.Color.WorkID == "" {
		return false
	}
	if !factoryruntime.IsPublicWorkToken(token) {
		return false
	}
	place, ok := tc.state.Places[token.PlaceID]
	if !ok || place == nil {
		return false
	}
	if _, isResource := tc.state.Resources[place.TypeID]; isResource {
		return false
	}
	_, isWorkType := tc.state.WorkTypes[place.TypeID]
	return isWorkType
}

func (tc *TerminationCheckSubsystem) hasImmediatelyRunnableActivity(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if tc.state == nil || tc.evaluator == nil || len(tc.state.Transitions) == 0 {
		return false
	}
	return len(tc.evaluator.FindEnabledTransitionsWithSnapshot(ctx, tc.state, snapshot)) > 0
}

// isTerminalOrFailed returns true if the token is in a TERMINAL or FAILED place.
func (tc *TerminationCheckSubsystem) isTerminalOrFailed(token *factorytoken.Token) bool {
	if tc.state == nil || token == nil {
		return false
	}
	place, ok := tc.state.Places[token.PlaceID]
	if !ok || place == nil {
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
	if tc.state == nil || snapshot == nil {
		return false
	}
	for _, res := range tc.state.Resources {
		placeID := state.PlaceID(res.ID, interfaces.ResourceStateAvailable)
		tokensInPlace := snapshot.TokensInPlace(placeID)
		if len(tokensInPlace) < res.Capacity {
			return false
		}
	}
	return true
}

func terminalMutationFacts(topology *state.Net, mutation interfaces.MarkingMutation) (bool, bool) {
	if topology == nil {
		return false, false
	}
	placeID := mutation.ToPlace
	if mutation.Type == interfaces.MutationConsume {
		placeID = mutation.FromPlace
	}
	if placeID == "" {
		return false, false
	}
	category := topology.StateCategoryForPlace(placeID)
	if category != state.StateCategoryTerminal && category != state.StateCategoryFailed {
		return false, false
	}
	if mutation.Type == interfaces.MutationConsume {
		return true, false
	}
	return true, terminalPlaceHasLiveInput(topology, placeID)
}

func terminalPlaceHasLiveInput(topology *state.Net, placeID string) bool {
	place, ok := topology.Places[placeID]
	if !ok || place == nil {
		return true
	}
	for _, transition := range topology.Transitions {
		if transition == nil {
			continue
		}
		for _, arc := range transition.InputArcs {
			if arc.PlaceID == placeID {
				return true
			}
			if arc.Cardinality.Mode != petri.CardinalityAllTerminal {
				continue
			}
			arcPlace, exists := topology.Places[arc.PlaceID]
			if exists && arcPlace != nil && arcPlace.TypeID == place.TypeID {
				return true
			}
		}
	}
	return false
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
