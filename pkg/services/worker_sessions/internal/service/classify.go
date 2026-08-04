package service

import (
	"errors"
	"fmt"
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
		// dispatch error alone. No WorkResult exists yet, so there is no
		// FailureMetadata to derive a safe Detail from.
		return failedTerminal(workersessions.FailureCauseStartFailure, safeDetail(workersessions.FailureCauseStartFailure, nil))
	}

	switch workResult.Outcome {
	case workers.OutcomeAccepted, workers.OutcomeContinue:
		return workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	default: // workers.OutcomeRejected, workers.OutcomeFailed
		kind := workersessions.FailureCauseWorkersExecutionFailure
		switch {
		case isExecutorPanicEvidence(dispatchErr, workResult):
			kind = workersessions.FailureCauseExecutorPanic
		case dispatchErr != nil:
			kind = workersessions.FailureCauseAdapterFailure
		}
		return failedTerminal(kind, safeDetail(kind, workResult.FailureMetadata))
	}
}

// isExecutorPanicEvidence reports whether either the adapter error or the
// WorkResult itself carries the Workers-established executor-panic
// evidence. The WorkResult text is checked independently of the adapter
// error so panic evidence is recognized even when the adapter error is nil.
// Classification is the only place Worker Sessions inspects this raw,
// free-form text: the result only ever selects a FailureCauseKind, and that
// raw text itself is never attached to the public FailureCause.Detail (see
// safeDetail).
func isExecutorPanicEvidence(dispatchErr error, workResult workers.WorkResult) bool {
	var panicErr *workers.WorkerExecutorPanicError
	if errors.As(dispatchErr, &panicErr) {
		return true
	}
	return isExecutorPanicText(workResult.Error) || isExecutorPanicText(rawDetail(dispatchErr))
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

func rawDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// genericFailureDetail is the fixed, per-Kind placeholder attached to a
// FailureCause when no Workers-documented structured classification is
// available. FailureCause.Kind already identifies the failure category
// (including EXECUTOR_PANIC), so a fixed generic Detail never hides the
// classification itself.
var genericFailureDetail = map[workersessions.FailureCauseKind]string{
	workersessions.FailureCauseStartFailure:            "the attempt could not be handed off to Workers",
	workersessions.FailureCauseWorkersExecutionFailure: "the Workers execution result was not successful",
	workersessions.FailureCauseAdapterFailure:          "the Workers adapter reported a failure",
	workersessions.FailureCauseExecutorPanic:           "the Workers executor reported a panic",
	workersessions.FailureCauseEventPublicationFailure: "the Worker Session opening record could not be published",
}

// knownFailureFamilies whitelists the exact WorkFailureFamily constants
// Workers documents (execution_contracts.go). WorkFailureFamily is an
// exported Go string type, not a runtime-validated enum, so a
// WorkFailureMetadata value can be constructed with any string, including
// attacker-controlled prompt/command/credential text; only a value present in
// this set is ever echoed into the public FailureCause.Detail.
var knownFailureFamilies = map[workers.WorkFailureFamily]bool{
	workers.WorkFailureFamilyTerminal:  true,
	workers.WorkFailureFamilyRetryable: true,
	workers.WorkFailureFamilyThrottle:  true,
}

// knownFailureTypes whitelists the exact WorkFailureType constants Workers
// documents. See knownFailureFamilies for why this whitelist exists.
var knownFailureTypes = map[workers.WorkFailureType]bool{
	workers.WorkFailureTypeAuthFailure:         true,
	workers.WorkFailureTypePermanentBadRequest: true,
	workers.WorkFailureTypeThrottled:           true,
	workers.WorkFailureTypeInternalServerError: true,
	workers.WorkFailureTypeTimeout:             true,
	workers.WorkFailureTypeUnknown:             true,
	workers.WorkFailureTypeMisconfigured:       true,
	workers.WorkFailureTypeCommandLineTooLong:  true,
	workers.WorkFailureTypeMissingExecutable:   true,
}

// safeDetail derives the public FailureCause.Detail for kind exclusively
// from Workers-documented, closed-vocabulary structured fields
// (WorkResult.FailureMetadata's Family/Type, whitelisted against the exact
// constants Workers defines as its stable customer-facing normalized failure
// vocabulary) or a fixed generic placeholder. safeDetail never reads
// WorkResult.Error or an adapter error's message: neither Workers nor the
// adapter boundary establishes that free-form text as free of payloads,
// credentials, environment values, prompts, or raw provider commands, so it
// is never attached to Detail in any form, redacted or otherwise.
// classifyTerminal still inspects that raw text for executor-panic evidence,
// but only to choose kind, never to build Detail. A Family or Type value
// that is not blank and not one of the whitelisted constants falls back to
// the fixed generic placeholder for kind rather than being echoed, so an
// unrecognized (potentially attacker-controlled) string can never reach
// Detail.
func safeDetail(kind workersessions.FailureCauseKind, metadata *workers.WorkFailureMetadata) string {
	if metadata == nil {
		return genericFailureDetail[kind]
	}
	family, familyKnown := safeFamily(metadata.Family)
	typ, typeKnown := safeType(metadata.Type)
	if !familyKnown || !typeKnown {
		return genericFailureDetail[kind]
	}
	if family == "" && typ == "" {
		return genericFailureDetail[kind]
	}
	return fmt.Sprintf("family=%s type=%s", orUnknown(family), orUnknown(typ))
}

// safeFamily returns (value, true) when family is blank or a whitelisted
// WorkFailureFamily constant. Any other value returns ("", false), which
// tells safeDetail to fall back to the fixed generic placeholder instead of
// echoing an unrecognized value.
func safeFamily(family workers.WorkFailureFamily) (string, bool) {
	if family == "" {
		return "", true
	}
	if knownFailureFamilies[family] {
		return string(family), true
	}
	return "", false
}

// safeType returns (value, true) when typ is blank or a whitelisted
// WorkFailureType constant. Any other value returns ("", false); see
// safeFamily.
func safeType(typ workers.WorkFailureType) (string, bool) {
	if typ == "" {
		return "", true
	}
	if knownFailureTypes[typ] {
		return string(typ), true
	}
	return "", false
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
