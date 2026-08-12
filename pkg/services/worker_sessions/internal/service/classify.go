package service

import (
	"encoding/json"
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
		return terminalForDispatchResult(
			workersessions.FailureCauseStartFailure,
			safeDetailForDispatchError(workersessions.FailureCauseStartFailure, workResult, dispatchErr, true),
			workResult,
			dispatchErr,
		)
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
			detail := safeDetailForDispatchError(kind, workResult, dispatchErr, true)
			if kind != workersessions.FailureCauseExecutorPanic {
				detail = contradictorySuccessDetail(
					dispatchErr != nil,
					strings.TrimSpace(strings.Join([]string{
						safeFailureClassificationForDispatch(kind, workResult, dispatchErr),
						diagnosticContextForDispatch(workResult, dispatchErr),
					}, " ")),
				)
			}
			return terminalForDispatchResult(kind, detail, workResult, dispatchErr)
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
		return terminalForDispatchResult(
			kind,
			safeDetailForDispatchError(kind, workResult, dispatchErr, false),
			workResult,
			dispatchErr,
		)
	}
}

func terminalForWorkResult(
	kind workersessions.FailureCauseKind,
	detail string,
	workResult workers.WorkResult,
) workersessions.TerminalResult {
	terminal := failedTerminal(kind, detail)
	if terminal.Cause != nil {
		terminal.Cause.AgentRunFailureClass = agentRunFailureClassFromWorkDiagnostics(workResult.Diagnostics)
	}
	terminal.Cause.ProviderFailureKind,
		terminal.Cause.ProviderContinuationFailureKind,
		terminal.Cause.ProviderContinuationOutcome = workersessions.SanitizeProviderFailureClassification(
		workResult.ProviderFailureKind,
		workResult.ProviderContinuationFailureKind,
		workResult.ProviderContinuationOutcome,
	)
	return terminal
}

func terminalForDispatchResult(
	kind workersessions.FailureCauseKind,
	detail string,
	workResult workers.WorkResult,
	dispatchErr error,
) workersessions.TerminalResult {
	terminal := terminalForWorkResult(kind, detail, workResult)
	providerErr := workers.NormalizeProviderExecutionError(dispatchErr)
	if providerErr == nil || terminal.Cause == nil {
		return terminal
	}
	terminal.Cause.ProviderFailureKind,
		terminal.Cause.ProviderContinuationFailureKind,
		terminal.Cause.ProviderContinuationOutcome = workersessions.SanitizeProviderFailureClassification(
		providerErr.ProviderFailureKind,
		providerErr.ProviderContinuationFailureKind,
		providerErr.ProviderContinuationOutcome,
	)
	return terminal
}

func safeDetailForDispatchError(
	kind workersessions.FailureCauseKind,
	workResult workers.WorkResult,
	dispatchErr error,
	preferDispatch bool,
) string {
	metadata := failureMetadataForDispatch(workResult, dispatchErr, preferDispatch)
	return safeDetailWithContext(
		kind,
		metadata,
		diagnosticContextForDispatch(workResult, dispatchErr),
	)
}

func safeFailureClassificationForDispatch(
	kind workersessions.FailureCauseKind,
	workResult workers.WorkResult,
	dispatchErr error,
) string {
	classification := safeDetail(kind, failureMetadataForDispatch(workResult, dispatchErr, true))
	if classification == genericFailureDetail[kind] {
		return ""
	}
	return classification
}

func failureMetadataForDispatch(
	workResult workers.WorkResult,
	dispatchErr error,
	preferDispatch bool,
) *workers.WorkFailureMetadata {
	metadata := workResult.FailureMetadata
	if providerErr := workers.NormalizeProviderExecutionError(dispatchErr); providerErr != nil {
		if preferDispatch || metadata == nil {
			metadata = workers.WorkFailureMetadataFromProviderError(providerErr)
		}
	}
	return metadata
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
	workers.WorkFailureTypeAuthFailure:                     true,
	workers.WorkFailureTypePermanentBadRequest:             true,
	workers.WorkFailureTypeThrottled:                       true,
	workers.WorkFailureTypeInternalServerError:             true,
	workers.WorkFailureTypeTimeout:                         true,
	workers.WorkFailureTypeUnknown:                         true,
	workers.WorkFailureTypeMisconfigured:                   true,
	workers.WorkFailureTypeCommandLineTooLong:              true,
	workers.WorkFailureTypeMissingExecutable:               true,
	workers.WorkFailureTypeStructuredOutputSchemaViolation: true,
	workers.WorkFailureTypeExpectedArtifactsUnsatisfied:    true,
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
	return safeDetailWithContext(kind, metadata, safeDiagnosticFailureContext(diagnostics))
}

func safeDetailWithContext(kind workersessions.FailureCauseKind, metadata *workers.WorkFailureMetadata, context string) string {
	detail := safeDetail(kind, metadata)
	if context == "" {
		return detail
	}
	return detail + " " + context
}

func diagnosticContextForDispatch(workResult workers.WorkResult, dispatchErr error) string {
	providerErr := workers.NormalizeProviderExecutionError(dispatchErr)
	if providerErr == nil {
		return safeDiagnosticFailureContext(workResult.Diagnostics)
	}
	return mergeDiagnosticContexts(workResult.Diagnostics, providerErr.Diagnostics)
}

func mergeDiagnosticContexts(primary, fallback *workers.WorkDiagnostics) string {
	parts := make([]string, 0, 3)
	operation := safeDiagnosticValue(diagnosticValue(primary, "failure_operation"), knownFailureOperations)
	if operation == "" {
		operation = safeDiagnosticValue(diagnosticValue(fallback, "failure_operation"), knownFailureOperations)
	}
	if operation != "" {
		parts = append(parts, "operation="+operation)
	}
	classification := safeDiagnosticValue(diagnosticValue(primary, "failure_classification"), knownFailureClassifications)
	if classification == "" {
		classification = safeDiagnosticValue(diagnosticValue(fallback, "failure_classification"), knownFailureClassifications)
	}
	if classification != "" {
		parts = append(parts, "classification="+classification)
	}
	stage := safeDiagnosticValue(diagnosticValue(primary, "failure_stage"), knownFailureStages)
	if stage == "" {
		stage = safeDiagnosticValue(diagnosticValue(fallback, "failure_stage"), knownFailureStages)
	}
	if stage != "" {
		parts = append(parts, "stage="+stage)
	}
	return strings.Join(parts, " ")
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
	if diagnostics == nil {
		return ""
	}
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

// normalizeCommittedTerminal is the last in-process guard before a terminal
// snapshot becomes durable. Normal classification already supplies a valid
// result, but this boundary also protects against an adapter or future caller
// constructing an empty/overlong cause: a FAILED session and its event must
// never be committed with a blank diagnostic.
func normalizeCommittedTerminal(state workersessions.State, result workersessions.TerminalResult) workersessions.TerminalResult {
	switch state {
	case workersessions.StateCompleted:
		return workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	case workersessions.StateFailed:
		if result.Outcome != workersessions.TerminalOutcomeFailed || result.Cause == nil || !result.Cause.Kind.Valid() {
			return failedTerminal(
				workersessions.FailureCauseWorkersExecutionFailure,
				"the Worker Session failed without a reported cause",
			)
		}
		cause := *result.Cause
		cause.Detail = boundedFailureDetail(cause.Kind, cause.Detail)
		cause.ProviderFailureKind,
			cause.ProviderContinuationFailureKind,
			cause.ProviderContinuationOutcome = workersessions.SanitizeProviderFailureClassification(
			cause.ProviderFailureKind,
			cause.ProviderContinuationFailureKind,
			cause.ProviderContinuationOutcome,
		)
		cause.AgentRunFailureClass = sanitizeAgentRunFailureClass(cause.AgentRunFailureClass)
		return workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause:   &cause,
		}
	default:
		return result
	}
}

func agentRunFailureClassFromWorkDiagnostics(diagnostics *workers.WorkDiagnostics) string {
	if diagnostics == nil {
		return ""
	}
	diagnostic := workers.SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics)
	if diagnostic == nil {
		return ""
	}
	return sanitizeAgentRunFailureClass(diagnostic.FailureClass)
}

func sanitizeAgentRunFailureClass(class string) string {
	switch class {
	case workers.AgentRunFailureClassProvider, workers.AgentRunFailureClassHarness:
		return class
	default:
		return ""
	}
}

// cloneSession returns a detached copy of session: mutating the returned
// value, or its Result, never affects registry-owned state.
func cloneSession(session workersessions.Session) workersessions.Session {
	session.Result = cloneTerminalResult(session.Result)
	if session.ProviderSessionAssociation != nil {
		association := session.ProviderSessionAssociation.Clone()
		session.ProviderSessionAssociation = &association
	}
	return session
}

func cloneTerminalResult(result *workersessions.TerminalResult) *workersessions.TerminalResult {
	if result == nil {
		return nil
	}
	clone := *result
	if result.Cause != nil {
		cause := *result.Cause
		clone.Cause = &cause
	}
	return &clone
}

func causeKindString(cause *workersessions.FailureCause) string {
	if cause == nil {
		return ""
	}
	return string(cause.Kind)
}

// terminalSessionPayload is the SESSION KindSession payload committed for the
// W3 terminal record. Failure fields are additive and carry only the already
// normalized Worker Sessions classification.
type terminalSessionPayload struct {
	Status               string `json:"status,omitempty"`
	FailureCause         string `json:"failureCause,omitempty"`
	FailureDetail        string `json:"failureDetail,omitempty"`
	AgentRunFailureClass string `json:"agentRunFailureClass,omitempty"`
}

// terminalPhase is the pure mapping from a committed Worker Session State to
// its terminal projection Phase. CANCELED and TERMINATED share the existing
// canceled phase because Worker Sessions has no separate terminal phase.
func terminalPhase(state workersessions.State) (workers.Phase, error) {
	switch state {
	case workersessions.StateCompleted:
		return workers.PhaseCompleted, nil
	case workersessions.StateFailed:
		return workers.PhaseFailed, nil
	case workersessions.StateCanceled, workersessions.StateTerminated:
		return workers.PhaseCanceled, nil
	default:
		return "", fmt.Errorf("worker sessions: state %q has no terminal projection phase", state)
	}
}

// terminalDraft maps a committed Worker Session State and TerminalResult to
// the one KindSession terminal draft emitted after prior Worker output.
func terminalDraft(state workersessions.State, result workersessions.TerminalResult, attemptID string) (workers.Draft, error) {
	phase, err := terminalPhase(state)
	if err != nil {
		return workers.Draft{}, err
	}
	if state == workersessions.StateCompleted || state == workersessions.StateFailed {
		if err := result.Validate(); err != nil {
			return workers.Draft{}, err
		}
	}
	payload := terminalSessionPayload{Status: string(state)}
	if result.Cause != nil {
		payload.FailureCause = string(result.Cause.Kind)
		payload.FailureDetail = result.Cause.Detail
		payload.AgentRunFailureClass = result.Cause.AgentRunFailureClass
	}
	payloadJSON, _ := json.Marshal(payload)
	return workers.Draft{
		Kind:       workers.KindSession,
		Phase:      phase,
		Provenance: lifecycleProvenance(""),
		Payload:    payloadJSON,
		DispatchID: attemptID,
	}, nil
}
