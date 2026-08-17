package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// startTuple is the detached value compared for one asynchronous start replay.
type startTuple struct {
	SessionID   string
	Execution   workers.WorkstationDispatchRequest
	MaxAttempts int
}

// startReplay stores one accepted or deterministically rejected start result.
type startReplay struct {
	tuple     startTuple
	sessionID string
	done      chan struct{}
	result    workersessions.StartResult
	err       error
}

func normalizeStartRequest(req workersessions.StartRequest) workersessions.StartRequest {
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.Execution = cloneWorkstationDispatchRequest(req.Execution)
	req.Retry.MaxAttempts = req.Retry.Attempts()
	return req
}

func startTupleFor(req workersessions.StartRequest) startTuple {
	return startTuple{SessionID: req.ID, Execution: cloneWorkstationDispatchRequest(req.Execution), MaxAttempts: req.Retry.Attempts()}
}

type runtimeAttempt struct {
	registry   *registry
	workerID   string
	dispatchID string
	attemptID  string
	once       sync.Once
}

func runtimeAttemptContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func runtimeAttemptIDs(req workersessions.RuntimeAttemptRequest) (string, string) {
	logicalDispatchID := strings.TrimSpace(req.Execution.Execution.Dispatch.DispatchID)
	attemptID := strings.TrimSpace(req.AttemptID)
	if attemptID == "" {
		attemptID = logicalDispatchID
	}
	return logicalDispatchID, attemptID
}

func (r *registry) runtimeAttemptOwnedByOther(logicalDispatchID, workerID, attemptID string) bool {
	r.mu.RLock()
	ownerID, owned := r.dispatchOwners[logicalDispatchID]
	r.mu.RUnlock()
	return owned && ownerID != workerID && attemptID == logicalDispatchID
}

func (r *registry) claimRuntimeAttempt(logicalDispatchID, workerID, attemptID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runtimeAttempts == nil {
		r.runtimeAttempts = make(map[string]struct{})
	}
	if r.dispatchOwners == nil {
		r.dispatchOwners = make(map[string]string)
	}
	if ownerID, exists := r.dispatchOwners[logicalDispatchID]; exists && ownerID != workerID && attemptID == logicalDispatchID {
		return false
	}
	r.dispatchOwners[logicalDispatchID] = workerID
	r.runtimeAttempts[workerID] = struct{}{}
	return true
}

// Complete commits the one terminal Worker Session observation. Runtime has
// already normalized cancellation and execution outcomes before invoking this
// hook; Worker Sessions only classifies and durably publishes the detached
// lifecycle result. The handle is idempotent because a late duplicate callback
// must not rewrite the terminal record or close recording twice.
func (a *runtimeAttempt) Complete(
	ctx context.Context,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) error {
	if a == nil || a.registry == nil {
		return errors.New("worker sessions: runtime attempt is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.once.Do(func() {
		r := a.registry
		r.associateProviderSessionFromResult(a.workerID, a.dispatchID, result)
		state, terminal := dispatchedTerminal("", result, dispatchErr)
		final, committed := r.commitTerminal(a.workerID, state, terminal)
		if committed {
			r.logTerminal(a.workerID, a.attemptID, final)
			r.publishTerminalRecordOrLog(
				context.WithoutCancel(ctx),
				a.workerID,
				a.attemptID,
				state,
				*final.Result,
			)
		}
		r.mu.Lock()
		delete(r.runtimeAttempts, a.workerID)
		r.mu.Unlock()
	})
	return nil
}

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

	if kind := reconciliationFailureCause[dispatchResult.ReconciliationReason]; kind != "" {
		return terminalForDispatchResult(kind, genericFailureDetail[kind], workResult, dispatchErr)
	}

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
	case workers.OutcomeRejected:
		// A cleanly completed rejection is a normal business result. Keep the
		// Worker's rejection route and feedback untouched; Worker Sessions
		// records the bounded classification so inspection does not confuse it
		// with an execution failure.
		if dispatchErr == nil && dispatchResult.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed {
			return terminalForDispatchResult(
				workersessions.FailureCauseRejected,
				safeDetailForDispatchError(workersessions.FailureCauseRejected, workResult, nil, false),
				workResult,
				nil,
			)
		}
		fallthrough
	default: // workers.OutcomeFailed and contradictory/unknown outcomes
		kind := workersessions.FailureCauseWorkersExecutionFailure
		switch {
		case isExecutorPanicEvidence(dispatchErr, workResult):
			kind = workersessions.FailureCauseExecutorPanic
		case dispatchErr != nil:
			kind = workersessions.FailureCauseAdapterFailure
		case isIncompleteOutputEvidence(workResult):
			kind = workersessions.FailureCauseIncompleteOutput
		}
		return terminalForDispatchResult(
			kind,
			safeDetailForDispatchError(kind, workResult, dispatchErr, false),
			workResult,
			dispatchErr,
		)
	}
}

// isIncompleteOutputEvidence recognizes only the Workers-owned structured
// completion-validation facts. The raw WorkResult output and error are not
// inspected here: a readable output-contract failure is classified by the
// diagnostics emitted at the Worker boundary, while process and transcript
// failures retain the ordinary execution-failure kind.
func isIncompleteOutputEvidence(workResult workers.WorkResult) bool {
	if strings.ToLower(strings.TrimSpace(diagnosticValue(workResult.Diagnostics, "failure_operation"))) != "completion_validation" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(diagnosticValue(workResult.Diagnostics, "failure_classification"))) {
	case "contradictory_completion", "missing_completion_evidence", "missing_required_output":
		return true
	default:
		return false
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
	workersessions.FailureCauseRejected:                "the Workers result was rejected by the business review",
	workersessions.FailureCauseIncompleteOutput:        "the Workers result did not include the required final output",
	workersessions.FailureCauseAdapterFailure:          "the Workers adapter reported a failure",
	workersessions.FailureCauseExecutorPanic:           "the Workers executor reported a panic",
	workersessions.FailureCauseEventPublicationFailure: "the Worker Session opening record could not be published",
	workersessions.FailureCauseProcessGone:             "the worker process exited before dispatch completion",
	workersessions.FailureCauseTimeout:                 "the worker execution exceeded its hard deadline",
	workersessions.FailureCauseOperatorCanceled:        "an operator cancel control ended the Worker Session",
	workersessions.FailureCauseOperatorTerminated:      "an operator terminate control ended the Worker Session",
}

// controlTerminalCause maps the two absorbing control states to the operator
// control outcome that produced them.
var controlTerminalCause = map[workersessions.State]workersessions.FailureCauseKind{
	workersessions.StateCanceled:   workersessions.FailureCauseOperatorCanceled,
	workersessions.StateTerminated: workersessions.FailureCauseOperatorTerminated,
}

// observedTerminalCause names why a session ended, for the inspection surface
// only. A committed FailureCause is authoritative. A session ended by an
// operator control never has one: commitControlTerminal deliberately keeps
// Result nil to preserve the "cancel invents no result" invariant, and the
// boundary cancel path commits an empty TerminalResult. Deriving the control
// outcome from the absorbing state keeps that invariant while still naming the
// reason, so an operator no longer reads "unavailable" — which is
// indistinguishable from a failure whose cause was never recorded.
func observedTerminalCause(session workersessions.Session) *workersessions.FailureCause {
	if session.Result != nil && session.Result.Cause != nil {
		cause := *session.Result.Cause
		return &cause
	}
	kind, ok := controlTerminalCause[session.State]
	if !ok {
		return nil
	}
	return &workersessions.FailureCause{Kind: kind, Detail: boundedFailureDetail(kind, "")}
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
	"missing_required_output":     {},
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
func (r *registry) appendDraft(ctx context.Context, topic events.Topic, identity events.AppendIdentity, schemaID events.SchemaID, draft workers.Draft) (events.AppendResult, error) {
	if err := workers.ValidateDraft(draft); err != nil {
		return events.AppendResult{}, fmt.Errorf("worker sessions: invalid draft: %w", err)
	}
	// draft's fields are exhaustively strings and json.RawMessage, neither of
	// which json.Marshal can fail to encode, so the marshal error is
	// unreachable and intentionally discarded rather than defended against.
	envelope, _ := json.Marshal(draft)
	return r.events.Append(ctx, events.AppendRequest{
		Topic:          topic,
		SourceType:     identity.SourceType,
		SourceID:       identity.SourceID,
		SourceSequence: identity.SourceSequence,
		SourceEventID:  identity.SourceEventID,
		SchemaID:       schemaID,
		Payload:        envelope,
	}.Detached())
}

// Fixed identities Worker Sessions uses to commit its opening, optional
// provider-binding, and terminal lifecycle records. The binding has its own
// SourceID so the opening/terminal source keeps its historical sequence
// numbers while the publication lock still determines aggregate order.
const (
	lifecycleSourceType           events.SourceType     = "worker_session_lifecycle"
	openingSourceSequence         events.SourceSequence = 1
	openingSourceEventID          events.SourceEventID  = "started"
	providerBindingSourceSequence events.SourceSequence = 1
	providerBindingSourceEventID  events.SourceEventID  = "provider-bound"
	terminalSourceSequence        events.SourceSequence = 2
	terminalSourceEventID         events.SourceEventID  = "terminal"
	workerDraftSchemaID           events.SchemaID       = "workers.draft.v1"
)

func providerBindingSourceID(id string) events.SourceID {
	return events.SourceID(id + "/provider-binding")
}

func lifecycleProvenance(provider string) workers.Provenance {
	return workers.Provenance{
		Delivery:        workers.DeliverySynthesized,
		Fidelity:        workers.FidelityLifecycleOnly,
		NativeEventType: string(lifecycleSourceType),
		Provider:        providers.ID(provider).CanonicalSessionProvider(),
		Representation:  workers.RepresentationNotification,
	}
}

func isTerminalLifecycleRecord(record events.Record) bool {
	return record.SourceType == lifecycleSourceType &&
		record.SourceSequence >= terminalSourceSequence &&
		record.SourceEventID == terminalSourceEventID
}

// publishOpeningRecord commits the one opening KindSession/PhaseStarted
// workers.Draft onto workersessions.Topic(id), detached from any
// caller-owned backing array, before Start ever calls the injected
// workers.Service. It runs under
// id's publication lock, the same lock PublishRecord and
// publishTerminalRecord use, and opens id's publication window only once
// the append itself has committed: no PublishRecord call can be accepted for
// id until this succeeds. A non-nil return means no record was committed and
// the window stays closed: Start must not proceed to Workers handoff.
func (r *registry) publishOpeningRecord(
	ctx context.Context,
	id,
	attemptID string,
	payload workers.SessionPayload,
	provider string,
	recordingsForSession ...recordings.WorkerSessionRecording,
) error {
	pub := r.publicationFor(id)
	pub.mu.Lock()
	recording := firstWorkerRecording(recordingsForSession)

	// SessionPayload contains only JSON value fields, so json.Marshal cannot
	// fail here; the error is intentionally discarded rather than defended
	// against.
	draftPayload, _ := json.Marshal(payload)
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseStarted,
		Provenance: lifecycleProvenance(provider),
		Payload:    draftPayload,
		DispatchID: attemptID,
		TurnID:     payload.TurnID,
	}
	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: openingSourceSequence,
		SourceEventID:  openingSourceEventID,
	}
	if _, err := r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft); err != nil {
		openingErr := fmt.Errorf("%w: %v", recordings.ErrWorkerRecordingOpening, err)
		if recording != nil {
			if abortErr := recording.Abort(context.WithoutCancel(ctx), openingErr); abortErr != nil {
				r.logger.Info(
					"worker session recording opening cleanup failed",
					"sessionID", id,
					"attemptID", attemptID,
					"outcome", "cleanup_failed",
				)
			}
		}
		pub.mu.Unlock()
		return err
	}
	if recording != nil {
		if err := recording.AwaitOpening(ctx); err != nil {
			if abortErr := recording.Abort(context.WithoutCancel(ctx), err); abortErr != nil {
				r.logger.Info(
					"worker session recording opening cleanup failed",
					"sessionID", id,
					"attemptID", attemptID,
					"outcome", "cleanup_failed",
				)
			}
			pub.mu.Unlock()
			return err
		}
	}
	pub.open = true
	pub.recording = recording
	pub.recordingID = strings.TrimSpace(payload.RecordingID)
	pub.provider = providers.ID(provider).CanonicalSessionProvider()
	pub.turnID = strings.TrimSpace(payload.TurnID)
	pub.lastSequence = make(map[sourceKey]events.SourceSequence)
	pub.accepted = make(map[events.AppendIdentity]struct{})
	pub.mu.Unlock()
	return nil
}

func firstWorkerRecording(recordingsForSession []recordings.WorkerSessionRecording) recordings.WorkerSessionRecording {
	if len(recordingsForSession) == 0 {
		return nil
	}
	return recordingsForSession[0]
}

// publishTerminalRecord commits the one terminal KindSession workers.Draft
// onto workersessions.Topic(id), derived from state+result, through the same
// appendDraft helper publishOpeningRecord and PublishRecord already share.
// It runs under id's publication lock and closes id's publication window
// before attempting the append: once this call starts, no concurrent or
// later PublishRecord call can be accepted for id, and any PublishRecord call
// already holding the lock has fully committed or failed by the time this
// one acquires it. A non-nil return means no record was committed; the
// caller must not rewrite the already-committed canonical W2 terminal
// Session on this failure -- see publishTerminalRecordOrLog.
func (r *registry) publishTerminalRecord(ctx context.Context, id, attemptID string, state workersessions.State, result workersessions.TerminalResult) error {
	draft, err := terminalDraft(state, result, attemptID)
	if err != nil {
		return err
	}

	pub := r.publicationFor(id)
	if pub == nil {
		return workersessions.ErrSessionNotFound
	}
	pub.control.close()
	pub.mu.Lock()
	if !pub.open {
		pub.mu.Unlock()
		return workersessions.ErrPublicationNotOpen
	}
	pub.open = false
	recording := pub.recording
	pub.recording = nil
	draft.Provenance = lifecycleProvenance(pub.provider)

	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: terminalSourceSequence,
		SourceEventID:  terminalSourceEventID,
	}
	appendResult, err := r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft)
	pub.mu.Unlock()
	if recording != nil {
		closeErr := r.closeWorkerRecording(
			context.WithoutCancel(ctx),
			recording,
			state,
			appendResult.Record.ID.Position,
		)
		if err == nil {
			err = closeErr
		} else if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	return err
}

// publishTerminalRecordOrLog calls publishTerminalRecord and, on failure,
// logs it explicitly rather than propagating it: the caller already holds
// the real committed terminal Session (commitTerminal's return value) and
// must return that value unchanged. A terminal-record publication failure is
// never reported as success and never silently replaced with fabricated
// history.
func (r *registry) publishTerminalRecordOrLog(ctx context.Context, id, attemptID string, state workersessions.State, result workersessions.TerminalResult) {
	if err := r.publishTerminalRecord(ctx, id, attemptID, state, result); err != nil {
		r.logger.Info(
			"worker session terminal record publication failed",
			"sessionID", id,
			"attemptID", attemptID,
			"state", string(state),
			"outcome", "publish_failed",
		)
	}
}

// associateProviderSessionFromResult preserves the Provider Session reference
// returned by Workers before the terminal lifecycle record can close the
// session's publication window. A malformed or conflicting Worker result is
// visible in structured operation logs and never replaces an accepted exact
// reference; it also never invents a replacement from runner or model state.
func (r *registry) associateProviderSessionFromResult(
	id, dispatchID string,
	result workers.WorkstationDispatchResult,
) {
	continuation := result.Result.Continuation
	if continuation == nil {
		return
	}
	reference, err := continuation.ToSessionRef()
	if err != nil {
		r.logger.Info("worker session provider session association from result rejected", "sessionID", id, "attemptID", dispatchID, "outcome", "rejected")
		return
	}
	_, err = r.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: id,
		DispatchID:      dispatchID,
		Reference:       reference,
	})
	if err != nil {
		r.logger.Info("worker session provider session association from result rejected", "sessionID", id, "attemptID", dispatchID, "outcome", "rejected")
	}
}

var reconciliationFailureCause = map[workers.WorkstationDispatchReconciliationReason]workersessions.FailureCauseKind{
	workers.WorkstationDispatchReconciliationReasonProcessGone: workersessions.FailureCauseProcessGone,
	workers.WorkstationDispatchReconciliationReasonTimeout:     workersessions.FailureCauseTimeout,
}
