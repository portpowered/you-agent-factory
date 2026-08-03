package service

import (
	"errors"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// classifyTerminal derives the Worker Session terminal outcome from the
// Workers WorkResult first and the adapter (dispatch) error second, so a
// failed result carrying executor-panic evidence can never present as
// success, and can never be misclassified, because its adapter error is
// nil, absent, or disagrees with the result.
func classifyTerminal(
	dispatchErr error,
	dispatchResult workers.WorkstationDispatchResult,
) workersessions.TerminalResult {
	workResult := dispatchResult.Result

	if workResult.Outcome == "" {
		// The injected Workers boundary never produced a WorkResult: the
		// attempt could not be handed off (for example, an unavailable or
		// misconfigured workstation pool). This is the only case that never
		// reached the executor, so it is the only case classified from the
		// dispatch error alone.
		return failedTerminal(workersessions.FailureCauseStartFailure, safeDetail(dispatchErr))
	}

	switch workResult.Outcome {
	case workers.OutcomeAccepted, workers.OutcomeContinue:
		return workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	default: // workers.OutcomeRejected, workers.OutcomeFailed
		detail := safeFailureDetail(workResult, dispatchErr)
		if isExecutorPanicEvidence(dispatchErr, workResult) {
			return failedTerminal(workersessions.FailureCauseExecutorPanic, detail)
		}
		if dispatchErr != nil {
			return failedTerminal(workersessions.FailureCauseAdapterFailure, detail)
		}
		return failedTerminal(workersessions.FailureCauseWorkersExecutionFailure, detail)
	}
}

// isExecutorPanicEvidence reports whether either the adapter error or the
// WorkResult itself carries the Workers-established executor-panic
// evidence. The WorkResult text is checked independently of the adapter
// error so panic evidence is recognized even when the adapter error is nil.
func isExecutorPanicEvidence(dispatchErr error, workResult workers.WorkResult) bool {
	var panicErr *workers.WorkerExecutorPanicError
	if errors.As(dispatchErr, &panicErr) {
		return true
	}
	return isExecutorPanicText(workResult.Error) || isExecutorPanicText(safeDetail(dispatchErr))
}

// isExecutorPanicText matches the Workers-owned executor-panic
// compatibility text established across both Workers dispatch paths:
// "executor panic: <cause>" and "workstation executor panic: <cause>".
func isExecutorPanicText(text string) bool {
	lowered := strings.ToLower(text)
	return strings.HasPrefix(lowered, "executor panic:") ||
		strings.HasPrefix(lowered, "workstation executor panic:")
}

func failedTerminal(kind workersessions.FailureCauseKind, detail string) workersessions.TerminalResult {
	return workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeFailed,
		Cause:   &workersessions.FailureCause{Kind: kind, Detail: detail},
	}
}

// safeFailureDetail prefers the Workers-owned WorkResult.Error text, which
// Workers already documents as safe result text, and falls back to the
// adapter error's message only when WorkResult carries none.
func safeFailureDetail(workResult workers.WorkResult, dispatchErr error) string {
	if workResult.Error != "" {
		return workResult.Error
	}
	return safeDetail(dispatchErr)
}

func safeDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
