package service

import workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"

// observationTurnUsage derives per-turn input context from cumulative usage
// counters already retained by Provider Sessions. A decreasing counter makes
// the sequence unsupported because its baseline cannot be interpreted as a
// cumulative total, so the optional projection is omitted.
func observationTurnUsage(cumulativeInputTokens []int) *workersessions.TurnUsage {
	if len(cumulativeInputTokens) == 0 {
		return nil
	}

	previous := 0
	final := 0
	peak := 0
	for _, cumulative := range cumulativeInputTokens {
		if cumulative < previous {
			return nil
		}
		perTurn := cumulative - previous
		if perTurn > peak {
			peak = perTurn
		}
		final = perTurn
		previous = cumulative
	}

	return &workersessions.TurnUsage{
		TurnCount:          len(cumulativeInputTokens),
		FinalContextTokens: final,
		PeakContextTokens:  peak,
	}
}
