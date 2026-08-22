package protocol

import (
	acpsdk "github.com/coder/acp-go-sdk"
)

// TerminalOutcome is the closed set of Factory turn terminal outcomes this
// transport recognizes when mapping to an ACP stop reason
// (final-proposal.md §6.3). Any other string value — including a future or
// unmapped outcome — falls through MapStopReason's default case rather than
// being added here first.
type TerminalOutcome string

const (
	TerminalCompleted       TerminalOutcome = "completed"
	TerminalCancelled       TerminalOutcome = "cancelled"
	TerminalTokensExhausted TerminalOutcome = "tokens_exhausted"
	TerminalRefused         TerminalOutcome = "refused"
	TerminalFailed          TerminalOutcome = "failed"
)

// StopResult is the closed, safe-to-serialize L1 V0 shape of a completed
// prompt turn: only the bounded ACP stop reason. It never carries the
// internal cause (if any) of a failed or unmapped terminal outcome.
type StopResult struct {
	StopReason acpsdk.StopReason `json:"stopReason"`
}

// MapStopReason is a total function from a Factory terminal outcome to the
// closed ACP stop-reason set (end_turn, cancelled, max_tokens, refusal).
// cause is accepted only so a caller can pass the internal failure it is
// mapping from; MapStopReason never inspects, formats, or otherwise
// surfaces it. Every terminal outcome this transport does not explicitly
// recognize — including TerminalFailed and any unknown/future outcome —
// maps to the documented safe fallback, end_turn, matching
// final-proposal.md §6.3's "Factory failed, or any unmapped terminal" row;
// the failure itself must already have been streamed as its own record
// before this terminal mapping runs, not serialized here.
func MapStopReason(outcome TerminalOutcome, cause error) StopResult {
	_ = cause // deliberately discarded: never inspected or serialized
	switch outcome {
	case TerminalCompleted:
		return StopResult{StopReason: acpsdk.StopReasonEndTurn}
	case TerminalCancelled:
		return StopResult{StopReason: acpsdk.StopReasonCancelled}
	case TerminalTokensExhausted:
		return StopResult{StopReason: acpsdk.StopReasonMaxTokens}
	case TerminalRefused:
		return StopResult{StopReason: acpsdk.StopReasonRefusal}
	default:
		return StopResult{StopReason: acpsdk.StopReasonEndTurn}
	}
}
