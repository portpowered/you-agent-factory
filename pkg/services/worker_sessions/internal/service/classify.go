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
		// A successful WorkResult cannot overrule a second, contradictory
		// failure signal. In particular, an adapter error may be returned
		// alongside a result after the provider/session ingestion boundary has
		// failed. Treating that combination as COMPLETED recreates the phantom
		// success path this supervisor owns.
		if dispatchErr != nil || dispatchResult.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeFailed {
			kind := workersessions.FailureCauseAdapterFailure
			if isExecutorPanicEvidence(dispatchErr, workResult) {
				kind = workersessions.FailureCauseExecutorPanic
			}
			detail := safeDetailWithDiagnostics(kind, workResult.FailureMetadata, workResult.Diagnostics)
			if kind != workersessions.FailureCauseExecutorPanic {
				detail = contradictorySuccessDetail(dispatchErr != nil, safeDiagnosticFailureContext(workResult.Diagnostics))
			}
			return terminalForWorkResult(kind, detail, workResult)
		}
		return workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	default: // workers.OutcomeRejected, workers.OutcomeFailed
		kind := workersessions.FailureCauseWorkersExecutionFailure
		switch {
		case isExecutorPanicEvidence(dispatchErr, workResult):
			kind = workersessions.FailureCauseExecutorPanic
		case dispatchErr != nil:
			kind = workersessions.FailureCauseAdapterFailure
		}
		return terminalForWorkResult(
			kind,
			safeDetailWithDiagnostics(kind, workResult.FailureMetadata, workResult.Diagnostics),
			workResult,
		)
	}
}

func terminalForWorkResult(
	kind workersessions.FailureCauseKind,
	detail string,
	workResult workers.WorkResult,
) workersessions.TerminalResult {
	terminal := failedTerminal(kind, detail)
	terminal.Cause.ProviderFailureKind,
		terminal.Cause.ProviderContinuationFailureKind,
		terminal.Cause.ProviderContinuationOutcome = workersessions.SanitizeProviderFailureClassification(
		workResult.ProviderFailureKind,
		workResult.ProviderContinuationFailureKind,
		workResult.ProviderContinuationOutcome,
	)
	return terminal
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
		Cause: &workersessions.FailureCause{
			Kind:   kind,
			Detail: boundedFailureDetail(kind, detail),
		},
	}
}

func contradictorySuccessDetail(adapterError bool, context string) string {
	detail := "the dispatch reported failure after a successful Workers result"
	if adapterError {
		detail = "the Workers adapter reported failure after a successful result"
	}
	if context != "" {
		return detail + " " + context
	}
	return detail
}

func boundedFailureDetail(kind workersessions.FailureCauseKind, detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = genericFailureDetail[kind]
	}
	if detail == "" {
		detail = "the Worker Session failed without a reported cause"
	}
	runes := []rune(detail)
	if len(runes) > workersessions.MaxFailureCauseDetailRunes {
		return string(runes[:workersessions.MaxFailureCauseDetailRunes])
	}
	return detail
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

func safeDetailWithDiagnostics(
	kind workersessions.FailureCauseKind,
	metadata *workers.WorkFailureMetadata,
	diagnostics *workers.WorkDiagnostics,
) string {
	detail := safeDetail(kind, metadata)
	context := safeDiagnosticFailureContext(diagnostics)
	if context == "" {
		return detail
	}
	return detail + " " + context
}

func safeDiagnosticFailureContext(diagnostics *workers.WorkDiagnostics) string {
	if diagnostics == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if operation := safeDiagnosticValue(diagnosticValue(diagnostics, "failure_operation"), knownFailureOperations); operation != "" {
		parts = append(parts, "operation="+operation)
	}
	if classification := safeDiagnosticValue(diagnosticValue(diagnostics, "failure_classification"), knownFailureClassifications); classification != "" {
		parts = append(parts, "classification="+classification)
	}
	if stage := safeDiagnosticValue(diagnosticValue(diagnostics, "failure_stage"), knownFailureStages); stage != "" {
		parts = append(parts, "stage="+stage)
	}
	return strings.Join(parts, " ")
}

func diagnosticValue(diagnostics *workers.WorkDiagnostics, key string) string {
	if value, ok := diagnostics.Metadata[key]; ok {
		return value
	}
	if diagnostics.Provider != nil {
		return diagnostics.Provider.ResponseMetadata[key]
	}
	return ""
}

func safeDiagnosticValue(value string, allowed map[string]struct{}) string {
	value = strings.TrimSpace(value)
	if len(value) > 64 {
		return ""
	}
	value = strings.ToLower(value)
	if _, ok := allowed[value]; !ok {
		return ""
	}
	return value
}

var knownFailureOperations = map[string]struct{}{
	"completion_validation":      {},
	"provider_inference":         {},
	"provider_session_ingestion": {},
	"worker_dispatch":            {},
}

var knownFailureClassifications = map[string]struct{}{
	"canceled":                    {},
	"contradictory_completion":    {},
	"missing_completion_evidence": {},
	"parse":                       {},
	"resource_limit":              {},
	"storage":                     {},
}

var knownFailureStages = map[string]struct{}{
	"cancellation": {},
	"decode":       {},
	"final_parse":  {},
	"flush":        {},
	"native":       {},
	"parse":        {},
	"storage":      {},
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
