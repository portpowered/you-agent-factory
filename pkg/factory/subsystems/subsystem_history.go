package subsystems

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
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

	histories := make([]interfaces.TokenHistory, len(snapshot.Results))
	for i := range snapshot.Results {
		histories[i] = buildHistory(consumedTokensForResult(snapshot, &snapshot.Results[i]), &snapshot.Results[i])
	}

	h.logger.Debug("history: computed token histories", "count", len(histories))
	return &interfaces.TickResult{Histories: histories}, nil
}

// buildHistory creates a TokenHistory with updated TotalVisits and ConsecutiveFailures.
// Consumed token histories are merged from the runtime dispatch snapshot.
func buildHistory(consumedTokens []interfaces.Token, result *interfaces.WorkResult) interfaces.TokenHistory {
	history := interfaces.TokenHistory{
		TotalVisits:         make(map[string]int),
		ConsecutiveFailures: make(map[string]int),
		PlaceVisits:         make(map[string]int),
	}

	// Merge input token histories so visit counts accumulate.
	for _, consumed := range consumedTokens {
		ih := consumed.History
		for tid, v := range ih.TotalVisits {
			history.TotalVisits[tid] += v
		}
		for tid, v := range ih.ConsecutiveFailures {
			if v > history.ConsecutiveFailures[tid] {
				history.ConsecutiveFailures[tid] = v
			}
		}
		for pid, v := range ih.PlaceVisits {
			history.PlaceVisits[pid] += v
		}
		if ih.LastError != "" {
			history.LastError = ih.LastError
		}
		history.FailureLog = append(history.FailureLog, ih.FailureLog...)
	}

	// Increment TotalVisits for the current transition.
	history.TotalVisits[result.TransitionID]++

	switch result.Outcome {
	case interfaces.OutcomeAccepted, interfaces.OutcomeContinue, interfaces.OutcomeRejected:
		// Reset consecutive failures — the worker didn't fail.
		history.ConsecutiveFailures[result.TransitionID] = 0
	case interfaces.OutcomeFailed:
		// Increment consecutive failures.
		history.ConsecutiveFailures[result.TransitionID]++
	}

	return history
}

func consumedTokensForResult(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], result *interfaces.WorkResult) []interfaces.Token {
	if snapshot == nil || snapshot.Dispatches == nil {
		return nil
	}

	entry, ok := snapshot.Dispatches[result.DispatchID]
	if !ok || entry == nil {
		return nil
	}

	return entry.ConsumedTokens
}

// evaluateStopWords checks whether the executor output contains any of the
// configured stop words. Returns ACCEPTED if a stop word is found, FAILED otherwise.
func evaluateStopWords(stopWords []string, output string) interfaces.WorkOutcome {
	for _, sw := range stopWords {
		if strings.Contains(output, sw) {
			return interfaces.OutcomeAccepted
		}
	}
	return interfaces.OutcomeFailed
}
