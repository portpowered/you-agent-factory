package subsystems

import (
	"context"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// HistorySubsystem reads WorkResults from the RuntimeStateSnapshot and computes
// token visit histories (TotalVisits, ConsecutiveFailures, FailureLog, etc.)
// from the consumed-token snapshots stored on each dispatch entry. It runs at
// TickGroup 11 before the Transitioner. Callers can use the computed histories
// directly without persisting them in runtime state.
type HistorySubsystem struct {
	logger logging.Logger
}

var _ Subsystem = (*HistorySubsystem)(nil)

// NewHistory creates a HistorySubsystem.
func NewHistory(logger logging.Logger) *HistorySubsystem {
	return &HistorySubsystem{
		logger: logging.EnsureLogger(logger),
	}
}

// TickGroup returns History (11).
func (h *HistorySubsystem) TickGroup() TickGroup {
	return History
}

// Execute computes a TokenHistory for each result in the snapshot by resolving
// its DispatchID back to the dispatch entry's consumed token snapshots.
func (h *HistorySubsystem) Execute(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	if len(snapshot.Results) == 0 {
		return nil, nil
	}

	histories := make([]factorytoken.History, len(snapshot.Results))
	for i := range snapshot.Results {
		result := &snapshot.Results[i]
		consumedTokens := consumedTokensForResult(snapshot, result)
		histories[i] = buildHistory(consumedTokens, result, candidateWorkID(snapshot.Topology, result.TransitionID, consumedTokens))
	}

	h.logger.Debug("history: computed token histories", "count", len(histories))
	return &interfaces.TickResult{Histories: histories}, nil
}

// buildHistory creates a TokenHistory with updated TotalVisits and ConsecutiveFailures.
// Consumed token histories are merged from the candidate work lineage stored in
// the runtime dispatch snapshot.
func buildHistory(consumedTokens []factorytoken.Token, result *workerexecution.WorkResult, candidateID string) factorytoken.History {
	history := factorytoken.History{
		TotalVisits:         make(map[string]int),
		ConsecutiveFailures: make(map[string]int),
		PlaceVisits:         make(map[string]int),
	}

	// Merge input token histories. Co-consumed tokens from the same dispatch often
	// carry copies of the same lineage visit counts (for example task + review
	// companions in executor/review loops). Use max per key so visit counts reflect
	// one work lineage instead of summing duplicate counters.
	for _, consumed := range candidateLineageTokens(consumedTokens, candidateID) {
		ih := consumed.History
		for tid, v := range ih.TotalVisits {
			if v > history.TotalVisits[tid] {
				history.TotalVisits[tid] = v
			}
		}
		for tid, v := range ih.ConsecutiveFailures {
			if v > history.ConsecutiveFailures[tid] {
				history.ConsecutiveFailures[tid] = v
			}
		}
		for pid, v := range ih.PlaceVisits {
			if v > history.PlaceVisits[pid] {
				history.PlaceVisits[pid] = v
			}
		}
		if ih.LastError != "" {
			history.LastError = ih.LastError
		}
		if ih.FailureLogDroppedCount > history.FailureLogDroppedCount {
			// Companion tokens can carry the same compacted lineage history;
			// retain the greatest omitted prefix rather than counting it twice.
			history.FailureLogDroppedCount = ih.FailureLogDroppedCount
		}
		history.FailureLog = append(history.FailureLog, ih.FailureLog...)
	}

	// Increment TotalVisits for the current transition.
	history.TotalVisits[result.TransitionID]++

	switch result.Outcome {
	case workerexecution.OutcomeAccepted, workerexecution.OutcomeContinue, workerexecution.OutcomeRejected, workerexecution.OutcomeCanceled:
		// Reset consecutive failures — the worker didn't fail.
		history.ConsecutiveFailures[result.TransitionID] = 0
	case workerexecution.OutcomeFailed:
		// Increment consecutive failures.
		history.ConsecutiveFailures[result.TransitionID]++
	}

	return history
}

// candidateWorkID identifies the durable work item whose history should be
// propagated by a transition. Authored input order is canonical, so the first
// non-resource input arc is the candidate while later inputs may be generated
// companions or other supporting work.
func candidateWorkID(net *state.Net, transitionID string, consumedTokens []factorytoken.Token) string {
	if net != nil {
		if transition := net.Transitions[transitionID]; transition != nil {
			for _, arc := range transition.InputArcs {
				for _, token := range consumedTokens {
					if token.PlaceID == arc.PlaceID && token.Color.DataType != factorytoken.DataTypeResource && token.Color.WorkID != "" {
						return token.Color.WorkID
					}
				}
			}
		}
	}

	for _, token := range consumedTokens {
		if token.Color.DataType != factorytoken.DataTypeResource && token.Color.WorkID != "" {
			return token.Color.WorkID
		}
	}
	return ""
}

func candidateLineageTokens(consumedTokens []factorytoken.Token, candidateID string) []factorytoken.Token {
	if candidateID == "" {
		return consumedTokens
	}

	lineage := make([]factorytoken.Token, 0, len(consumedTokens))
	for _, token := range consumedTokens {
		if token.Color.WorkID == candidateID || token.Color.ParentID == candidateID {
			lineage = append(lineage, token)
		}
	}
	return lineage
}

func consumedTokensForResult(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], result *workerexecution.WorkResult) []factorytoken.Token {
	if snapshot == nil || snapshot.Dispatches == nil {
		return nil
	}

	entry, ok := snapshot.Dispatches[result.DispatchID]
	if !ok || entry == nil {
		return nil
	}

	return factorytoken.FromWorkerSlice(entry.ConsumedTokens)
}

// evaluateStopWords checks whether the executor output contains any of the
// configured stop words. Returns ACCEPTED if a stop word is found, FAILED otherwise.
func evaluateStopWords(stopWords []string, output string) workerexecution.WorkOutcome {
	for _, sw := range stopWords {
		if strings.Contains(output, sw) {
			return workerexecution.OutcomeAccepted
		}
	}
	return workerexecution.OutcomeFailed
}

func firstNonResourceInput(inputs []factorytoken.Color) *factorytoken.Color {
	for i := range inputs {
		if inputs[i].DataType != factorytoken.DataTypeResource && inputs[i].WorkTypeID != interfaces.SystemTimeWorkTypeID {
			return &inputs[i]
		}
	}
	for i := range inputs {
		if inputs[i].DataType != factorytoken.DataTypeResource {
			return &inputs[i]
		}
	}
	return nil
}

func cloneHistoryForIntermittentFailureRequeue(
	history factorytoken.History,
	result resolvedWorkResult,
	now time.Time,
) factorytoken.History {
	cloned := factorytoken.CloneHistory(history)
	cloned.LastError = result.err
	cloned.FailureLog = append(cloned.FailureLog, factorytoken.Failure{
		TransitionID: result.transitionID,
		Timestamp:    now,
		Error:        result.err,
		Attempt:      history.TotalVisits[result.transitionID],
	})
	return cloned
}
