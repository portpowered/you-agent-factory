package subsystems

import (
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

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
	outcome interfaces.WorkOutcome,
	consumedTokens []interfaces.Token,
	outputArcs []petri.Arc,
	now time.Time,
) []interfaces.MarkingMutation {
	if marking == nil || outcome != interfaces.OutcomeAccepted {
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

func executorReviewLineageFromConsumed(consumed []interfaces.Token) (traceID string, laneName string) {
	traceID = interfaces.CurrentChainingTraceIDFromTokens(consumed)
	for i := range consumed {
		if consumed[i].Color.DataType == interfaces.DataTypeResource {
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

func consumedTokenIDs(consumed []interfaces.Token) map[string]struct{} {
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

func canonicalExecutorReviewTraceID(color interfaces.TokenColor) string {
	if color.CurrentChainingTraceID != "" {
		return color.CurrentChainingTraceID
	}
	return color.TraceID
}
