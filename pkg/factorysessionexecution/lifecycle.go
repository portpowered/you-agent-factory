package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ProjectedLifecycleControlStatus returns the lifecycle-control status that live
// session status reads should expose. Canonical SESSION_PAUSED and SESSION_RESUMED
// replay reconstruct the same PAUSED or RUNNING value; when no control events are
// present yet, the current factory runtime state is mapped into the shared
// lifecycle vocabulary.
func ProjectedLifecycleControlStatus(lifecycleControlStatus string, factoryState string) LifecycleStatus {
	if trimmed := LifecycleStatus(strings.TrimSpace(lifecycleControlStatus)); trimmed != "" {
		return trimmed
	}
	return LifecycleStatusFromFactoryRuntimeState(factoryState)
}

// LifecycleStatusFromFactoryRuntimeState maps one live Petri factory runtime state
// into the shared Factory Session lifecycle vocabulary used by control surfaces.
func LifecycleStatusFromFactoryRuntimeState(factoryState string) LifecycleStatus {
	switch strings.ToUpper(strings.TrimSpace(factoryState)) {
	case "RUNNING", "IDLE":
		return LifecycleStatusRunning
	case "PAUSED":
		return LifecycleStatusPaused
	case "COMPLETED":
		return LifecycleStatusSucceeded
	case "FAILED":
		return LifecycleStatusFailed
	default:
		return ""
	}
}

// IsTerminalLifecycleStatus reports whether status is terminal and therefore
// immutable except for explicitly allowed inspection or retry behaviors.
func IsTerminalLifecycleStatus(status LifecycleStatus) bool {
	switch status {
	case LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated:
		return true
	default:
		return false
	}
}

// AllowsRetryDispatchOnTerminal reports whether retry-dispatch remains permitted
// after the session reaches a terminal status. Failed sessions may still accept
// retry-dispatch for failed child dispatches.
func AllowsRetryDispatchOnTerminal(status LifecycleStatus) bool {
	return status == LifecycleStatusFailed
}

// AllowsInterruptDispatchOnSession reports whether interrupt-dispatch remains
// permitted while the session is actively running goal work.
func AllowsInterruptDispatchOnSession(status LifecycleStatus) bool {
	switch status {
	case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming:
		return true
	default:
		return false
	}
}

// EvaluateInterruptDispatchControl classifies one interrupt-dispatch request
// against the current durable session and dispatch status.
func EvaluateInterruptDispatchControl(sessionStatus LifecycleStatus, dispatchStatus DispatchStatus) LifecycleControlOutcome {
	if sessionStatus == "" {
		return LifecycleControlOutcomeInvalidState
	}
	if IsTerminalLifecycleStatus(sessionStatus) {
		return LifecycleControlOutcomeTerminalSession
	}
	if !AllowsInterruptDispatchOnSession(sessionStatus) {
		return LifecycleControlOutcomeInvalidState
	}
	switch dispatchStatus {
	case DispatchStatusRunning:
		return LifecycleControlOutcomeAccepted
	case DispatchStatusInterrupted:
		return LifecycleControlOutcomeNoOp
	case DispatchStatusQueued, DispatchStatusCompleted, DispatchStatusFailed, DispatchStatusCanceled, DispatchStatusTimedOut, DispatchStatusSkipped:
		return LifecycleControlOutcomeInvalidState
	default:
		return LifecycleControlOutcomeInvalidState
	}
}

// EvaluateLifecycleControl classifies one lifecycle control request against the
// current durable session status without runtime-specific dispatch context.
//
// pkgmaintcheck:ignore-cyclomatic-complexity this transition classifier keeps durable lifecycle control outcomes explicit across terminal and active states.
func EvaluateLifecycleControl(operation LifecycleControlKind, status LifecycleStatus) LifecycleControlOutcome {
	if status == "" {
		return LifecycleControlOutcomeInvalidState
	}
	if IsTerminalLifecycleStatus(status) {
		switch operation {
		case LifecycleControlRetryDispatch:
			if status == LifecycleStatusFailed {
				return LifecycleControlOutcomeAccepted
			}
			return LifecycleControlOutcomeTerminalSession
		case LifecycleControlCancel, LifecycleControlTerminate:
			if status == LifecycleStatusCanceled && operation == LifecycleControlCancel {
				return LifecycleControlOutcomeNoOp
			}
			if status == LifecycleStatusTerminated && operation == LifecycleControlTerminate {
				return LifecycleControlOutcomeNoOp
			}
			return LifecycleControlOutcomeTerminalSession
		default:
			return LifecycleControlOutcomeTerminalSession
		}
	}

	switch operation {
	case LifecycleControlPause:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusPaused:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlResume:
		switch status {
		case LifecycleStatusPaused:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusResuming, LifecycleStatusRunning:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlCancel:
		switch status {
		case LifecycleStatusCanceling:
			return LifecycleControlOutcomeNoOp
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlTerminate:
		switch status {
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming,
			LifecycleStatusCanceling:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlApprove:
		if status == LifecycleStatusAwaitingApproval {
			return LifecycleControlOutcomeAccepted
		}
		return LifecycleControlOutcomeInvalidState
	case LifecycleControlRetryDispatch:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	default:
		return LifecycleControlOutcomeInvalidState
	}
}

// LifecycleStatus is the durable factory-session lifecycle status shared by read
// and control surfaces.
type LifecycleStatus string

const (
	LifecycleStatusQueued           LifecycleStatus = "QUEUED"
	LifecycleStatusAwaitingApproval LifecycleStatus = "AWAITING_APPROVAL"
	LifecycleStatusRunning          LifecycleStatus = "RUNNING"
	LifecycleStatusPaused           LifecycleStatus = "PAUSED"
	LifecycleStatusResuming         LifecycleStatus = "RESUMING"
	LifecycleStatusSucceeded        LifecycleStatus = "SUCCEEDED"
	LifecycleStatusFailed           LifecycleStatus = "FAILED"
	LifecycleStatusCanceling        LifecycleStatus = "CANCELING"
	LifecycleStatusCanceled         LifecycleStatus = "CANCELED"
	LifecycleStatusTimedOut         LifecycleStatus = "TIMED_OUT"
	LifecycleStatusInterrupted      LifecycleStatus = "INTERRUPTED"
	LifecycleStatusTerminated       LifecycleStatus = "TERMINATED"
)

// LifecycleControlKind identifies one durable session lifecycle control operation.
type LifecycleControlKind string

const (
	LifecycleControlPause             LifecycleControlKind = "PAUSE"
	LifecycleControlResume            LifecycleControlKind = "RESUME"
	LifecycleControlCancel            LifecycleControlKind = "CANCEL"
	LifecycleControlTerminate         LifecycleControlKind = "TERMINATE"
	LifecycleControlApprove           LifecycleControlKind = "APPROVE"
	LifecycleControlRetryDispatch     LifecycleControlKind = "RETRY_DISPATCH"
	LifecycleControlInterruptDispatch LifecycleControlKind = "INTERRUPT_DISPATCH"
)

// LifecycleControlOutcome reports how one lifecycle control request was evaluated.
type LifecycleControlOutcome string

const (
	LifecycleControlOutcomeAccepted        LifecycleControlOutcome = "ACCEPTED"
	LifecycleControlOutcomeNoOp            LifecycleControlOutcome = "NO_OP"
	LifecycleControlOutcomeInvalidState    LifecycleControlOutcome = "INVALID_STATE"
	LifecycleControlOutcomeTerminalSession LifecycleControlOutcome = "TERMINAL_SESSION"
	LifecycleControlOutcomeConflict        LifecycleControlOutcome = "CONFLICT"
)

// ControlRequest is optional metadata shared by pause, resume, cancel, and terminate.
type ControlRequest struct {
	RequestID string
	Reason    string
}

// ApproveRequest approves one durable session awaiting policy approval.
type ApproveRequest struct {
	ControlRequest
	ApprovalPreviewID string
	ApprovedPolicy    map[string]any
}

// RetryDispatchRequest retries one durable session dispatch.
type RetryDispatchRequest struct {
	ControlRequest
	DispatchID        string
	ForceNewAttempt   bool
	ResetAttemptCount bool
}

// InterruptDispatchRequest interrupts one active durable session dispatch.
type InterruptDispatchRequest struct {
	ControlRequest
	DispatchID string
}

// PhaseSummary summarizes dispatch progress for one workflow phase.
type PhaseSummary struct {
	Phase                  string
	Label                  string
	DispatchCount          int
	CompletedDispatchCount int
	FailedDispatchCount    int
}

const (
	defaultDispatchInterruptionReason     = "Operator interrupted active dispatch"
	dispatchInterruptionFailureReasonCode = "DISPATCH_INTERRUPTED"
)

type dispatchInterruptedEventPayload struct {
	Reason         string `json:"reason"`
	ObservedStatus string `json:"observedStatus"`
	InterruptedAt  string `json:"interruptedAt"`
	RetryPlanned   bool   `json:"retryPlanned"`
}

type interruptedDispatchPreservation struct {
	summary              DispatchSummary
	statusTransitions    []DispatchStatus
	javaScriptProjection *DispatchJavaScriptProjection
}

// ObservedCancellationStatusForInterrupt returns the provider or process cancellation
// status observed when one dispatch interruption is recorded.
func ObservedCancellationStatusForInterrupt(priorStatus DispatchStatus) factoryapi.FactoryDispatchStatus {
	switch priorStatus {
	case DispatchStatusRunning:
		return factoryapi.FactoryDispatchStatusRUNNING
	default:
		return factoryapi.FactoryDispatchStatusINTERRUPTED
	}
}

func dispatchInterruptionReason(interrupt InterruptDispatchRequest) string {
	if reason := strings.TrimSpace(interrupt.Reason); reason != "" {
		return reason
	}
	return defaultDispatchInterruptionReason
}

func dispatchInterruptionFailureDetail(reason string) *DispatchFailureDetail {
	return &DispatchFailureDetail{
		Reason:  dispatchInterruptionFailureReasonCode,
		Message: reason,
	}
}

// MarkDispatchInterrupted updates one dispatch summary and transition history for interruption.
func MarkDispatchInterrupted(
	dispatches []DispatchSummary,
	dispatchStatusTransitions map[string][]DispatchStatus,
	dispatchID string,
	interrupt InterruptDispatchRequest,
) ([]DispatchSummary, map[string][]DispatchStatus) {
	reason := dispatchInterruptionReason(interrupt)
	failureDetail := dispatchInterruptionFailureDetail(reason)
	for index, dispatch := range dispatches {
		if dispatch.ID != dispatchID {
			continue
		}
		dispatch.Status = DispatchStatusInterrupted
		dispatch.FailureDetail = failureDetail
		dispatches[index] = dispatch
		if dispatchStatusTransitions != nil {
			if transitions, ok := dispatchStatusTransitions[dispatchID]; ok {
				dispatchStatusTransitions[dispatchID] = append(transitions, DispatchStatusInterrupted)
			} else {
				dispatchStatusTransitions[dispatchID] = []DispatchStatus{DispatchStatusInterrupted}
			}
		}
		break
	}
	return dispatches, dispatchStatusTransitions
}

// AppendDispatchInterruptedEvent appends one canonical DISPATCH_INTERRUPTED event.
func AppendDispatchInterruptedEvent(
	events []json.RawMessage,
	session SessionReadResult,
	dispatch DispatchSummary,
	interrupt InterruptDispatchRequest,
	priorStatus DispatchStatus,
	source string,
) []json.RawMessage {
	if strings.TrimSpace(session.SessionID) == "" || strings.TrimSpace(dispatch.ID) == "" {
		return events
	}
	interruptedAt := time.Now().UTC()
	reason := dispatchInterruptionReason(interrupt)
	observedStatus := ObservedCancellationStatusForInterrupt(priorStatus)
	sequence, sessionSequence := nextCanonicalEventSequence(events)
	eventTime := canonicalSessionEventTime(session).Add(time.Duration(sessionSequence) * time.Second)
	if interruptedAt.After(eventTime) {
		eventTime = interruptedAt
	}

	sessionID := session.SessionID
	orchestratorKind := strings.ToUpper(strings.TrimSpace(session.OrchestratorKind))
	var orchestratorDialect *string
	if dialect := strings.TrimSpace(session.Dialect); dialect != "" {
		orchestratorDialect = &dialect
	}
	var phaseID *string
	var phaseName *string
	if phase := strings.TrimSpace(dispatch.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	} else if phase := strings.TrimSpace(session.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	}
	dispatchID := dispatch.ID

	payload, err := json.Marshal(dispatchInterruptedEventPayload{
		Reason:         reason,
		ObservedStatus: string(observedStatus),
		InterruptedAt:  interruptedAt.Format(time.RFC3339),
		RetryPlanned:   false,
	})
	if err != nil {
		return events
	}

	context := canonicalFactoryEventContext{
		Sequence:        sequence,
		Tick:            sequence,
		EventTime:       eventTime,
		SessionID:       &sessionID,
		SessionSequence: intPtr(sessionSequence),
		Source:          &source,
		DispatchID:      &dispatchID,
	}
	if orchestratorKind != "" {
		context.OrchestratorKind = &orchestratorKind
	}
	if orchestratorDialect != nil {
		context.OrchestratorDialect = orchestratorDialect
	}
	if phaseID != nil {
		context.PhaseID = phaseID
	}
	if phaseName != nil {
		context.PhaseName = phaseName
	}

	encoded, err := json.Marshal(canonicalFactoryEvent{
		SchemaVersion: canonicalFactoryEventSchemaVersion,
		ID:            fmt.Sprintf("dispatch-interrupted/%s/%d", dispatchID, sequence),
		Type:          "DISPATCH_INTERRUPTED",
		Context:       context,
		Payload:       payload,
	})
	if err != nil {
		return events
	}
	return append(events, encoded)
}

func nextCanonicalEventSequence(events []json.RawMessage) (sequence int, sessionSequence int) {
	maxSequence := 0
	maxSessionSequence := -1
	for _, raw := range events {
		parsed, err := parseCanonicalEvent(raw)
		if err != nil {
			continue
		}
		if parsed.Sequence > maxSequence {
			maxSequence = parsed.Sequence
		}
		if parsed.SessionSequence != nil && *parsed.SessionSequence > maxSessionSequence {
			maxSessionSequence = *parsed.SessionSequence
		}
	}
	return maxSequence + 1, maxSessionSequence + 1
}

// ReplayDispatchProjection reconstructs durable dispatch summaries from canonical session events.
func ReplayDispatchProjection(events []json.RawMessage) ([]DispatchSummary, error) {
	dispatches := make(map[string]DispatchSummary)
	for index, raw := range events {
		if err := applyDispatchProjectionEvent(dispatches, raw); err != nil {
			return nil, fmt.Errorf("apply event %d: %w", index, err)
		}
	}
	if len(dispatches) == 0 {
		return nil, nil
	}
	ordered := make([]DispatchSummary, 0, len(dispatches))
	for _, dispatch := range dispatches {
		ordered = append(ordered, dispatch)
	}
	return ordered, nil
}

func applyDispatchProjectionEvent(dispatches map[string]DispatchSummary, raw json.RawMessage) error {
	var envelope canonicalFactoryEvent
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("unmarshal event envelope: %w", err)
	}
	switch strings.TrimSpace(envelope.Type) {
	case "DISPATCH_INTERRUPTED":
		return applyDispatchInterruptedProjection(dispatches, envelope)
	default:
		return nil
	}
}

func applyDispatchInterruptedProjection(dispatches map[string]DispatchSummary, envelope canonicalFactoryEvent) error {
	dispatchID := stringValuePtr(envelope.Context.DispatchID)
	if dispatchID == "" {
		return fmt.Errorf("DISPATCH_INTERRUPTED missing dispatchId")
	}
	var payload dispatchInterruptedEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal DISPATCH_INTERRUPTED payload: %w", err)
	}
	reason := strings.TrimSpace(payload.Reason)
	if reason == "" {
		reason = defaultDispatchInterruptionReason
	}
	dispatches[dispatchID] = DispatchSummary{
		ID:     dispatchID,
		Status: DispatchStatusInterrupted,
		Phase:  stringValuePtr(envelope.Context.PhaseID),
		FailureDetail: &DispatchFailureDetail{
			Reason:  dispatchInterruptionFailureReasonCode,
			Message: reason,
		},
	}
	return nil
}

func snapshotInterruptedDispatches(state *runtimeSessionState) map[string]interruptedDispatchPreservation {
	if state == nil || len(state.dispatches) == 0 {
		return nil
	}
	preserved := make(map[string]interruptedDispatchPreservation)
	for _, dispatch := range state.dispatches {
		if dispatch.Status != DispatchStatusInterrupted {
			continue
		}
		preservation := interruptedDispatchPreservation{
			summary:           cloneDispatchSummary(dispatch),
			statusTransitions: cloneDispatchStatusSlice(state.dispatchStatusTransitions[dispatch.ID]),
		}
		if js, ok := state.dispatchJavaScript[dispatch.ID]; ok {
			projection := js
			preservation.javaScriptProjection = &projection
		}
		preserved[dispatch.ID] = preservation
	}
	if len(preserved) == 0 {
		return nil
	}
	return preserved
}

func restoreInterruptedDispatchResultSuppression(
	state *runtimeSessionState,
	preserved map[string]interruptedDispatchPreservation,
) {
	if state == nil || len(preserved) == 0 {
		return
	}
	projectedByID := make(map[string]DispatchSummary, len(state.dispatches))
	for _, dispatch := range state.dispatches {
		projectedByID[dispatch.ID] = dispatch
	}
	for index, dispatch := range state.dispatches {
		preservation, ok := preserved[dispatch.ID]
		if !ok {
			continue
		}
		state.dispatches[index] = enrichInterruptedDispatchDiagnostics(
			preservation.summary,
			projectedByID[dispatch.ID],
		)
	}
	for dispatchID, preservation := range preserved {
		if _, ok := projectedByID[dispatchID]; ok {
			continue
		}
		state.dispatches = append(state.dispatches, cloneDispatchSummary(preservation.summary))
	}
	if state.dispatchStatusTransitions == nil {
		state.dispatchStatusTransitions = make(map[string][]DispatchStatus, len(preserved))
	}
	for dispatchID, preservation := range preserved {
		if len(preservation.statusTransitions) > 0 {
			state.dispatchStatusTransitions[dispatchID] = cloneDispatchStatusSlice(preservation.statusTransitions)
		}
		if preservation.javaScriptProjection != nil {
			if state.dispatchJavaScript == nil {
				state.dispatchJavaScript = make(map[string]DispatchJavaScriptProjection)
			}
			state.dispatchJavaScript[dispatchID] = *preservation.javaScriptProjection
		}
	}
	state.artifacts = filterArtifactsSuppressingInterruptedLateResults(state.artifacts, preserved)
	recalculateSessionProgressFromDispatches(state)
}

func enrichInterruptedDispatchDiagnostics(preserved, projected DispatchSummary) DispatchSummary {
	enriched := cloneDispatchSummary(preserved)
	if enriched.Label == "" {
		enriched.Label = projected.Label
	}
	if enriched.RunnerID == "" {
		enriched.RunnerID = projected.RunnerID
	}
	if enriched.Model == "" {
		enriched.Model = projected.Model
	}
	if enriched.Provider == "" {
		enriched.Provider = projected.Provider
	}
	if len(enriched.ProviderSessionRefs) == 0 && len(projected.ProviderSessionRefs) > 0 {
		enriched.ProviderSessionRefs = cloneProviderSessionRefs(projected.ProviderSessionRefs)
	}
	return enriched
}

func filterArtifactsSuppressingInterruptedLateResults(
	artifacts []ArtifactSummary,
	preserved map[string]interruptedDispatchPreservation,
) []ArtifactSummary {
	if len(artifacts) == 0 || len(preserved) == 0 {
		return artifacts
	}
	filtered := make([]ArtifactSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		dispatchID := strings.TrimSpace(artifact.DispatchID)
		if dispatchID == "" {
			filtered = append(filtered, artifact)
			continue
		}
		if _, interrupted := preserved[dispatchID]; interrupted && artifact.Kind == "CHILD_RESULT" {
			continue
		}
		filtered = append(filtered, artifact)
	}
	return filtered
}

func recalculateSessionProgressFromDispatches(state *runtimeSessionState) {
	if state == nil {
		return
	}
	phaseCount := 0
	if state.session.Progress != nil {
		phaseCount = state.session.Progress.PhaseCount
	}
	progress := progressCountsFromDispatches(state.dispatches, phaseCount)
	state.session.Progress = &progress
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.session.ArtifactCount = len(state.session.ArtifactRefs)
}

func extractDispatchInterruptedEvents(events []json.RawMessage) []json.RawMessage {
	if len(events) == 0 {
		return nil
	}
	interrupted := make([]json.RawMessage, 0)
	for _, raw := range events {
		var envelope factoryEventEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if strings.TrimSpace(envelope.Type) != "DISPATCH_INTERRUPTED" {
			continue
		}
		interrupted = append(interrupted, append(json.RawMessage(nil), raw...))
	}
	return interrupted
}

func mergePreservedDispatchInterruptedEvents(projected, preserved []json.RawMessage) []json.RawMessage {
	if len(preserved) == 0 {
		return projected
	}
	merged := make([]json.RawMessage, 0, len(projected)+len(preserved))
	seen := make(map[string]struct{}, len(preserved))
	for _, raw := range projected {
		var envelope factoryEventEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			merged = append(merged, raw)
			continue
		}
		if strings.TrimSpace(envelope.Type) == "DISPATCH_INTERRUPTED" {
			seen[eventIdentityKey(raw)] = struct{}{}
		}
		merged = append(merged, raw)
	}
	for _, raw := range preserved {
		key := eventIdentityKey(raw)
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, raw)
		seen[key] = struct{}{}
	}
	return merged
}

func eventIdentityKey(raw json.RawMessage) string {
	var envelope struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return string(raw)
	}
	if id := strings.TrimSpace(envelope.ID); id != "" {
		return id
	}
	return strings.TrimSpace(envelope.Type)
}

func cloneDispatchStatusSlice(transitions []DispatchStatus) []DispatchStatus {
	if len(transitions) == 0 {
		return nil
	}
	return append([]DispatchStatus(nil), transitions...)
}

func cloneProviderSessionRefs(refs []ProviderSessionRef) []ProviderSessionRef {
	if len(refs) == 0 {
		return nil
	}
	return append([]ProviderSessionRef(nil), refs...)
}

func cloneDispatchSummary(dispatch DispatchSummary) DispatchSummary {
	cloned := dispatch
	if len(dispatch.OutputArtifactIDs) > 0 {
		cloned.OutputArtifactIDs = append([]string(nil), dispatch.OutputArtifactIDs...)
	}
	if len(dispatch.ProviderSessionRefs) > 0 {
		cloned.ProviderSessionRefs = cloneProviderSessionRefs(dispatch.ProviderSessionRefs)
	}
	if dispatch.FailureDetail != nil {
		detail := *dispatch.FailureDetail
		cloned.FailureDetail = &detail
	}
	return cloned
}

// ProgressCounts summarizes durable dispatch progress for one session.
type ProgressCounts struct {
	TotalDispatches     int
	CompletedDispatches int
	FailedDispatches    int
	InFlightDispatches  int
	PhaseCount          int
}

// ResultSummary exposes customer-visible result readiness for one session read.
type ResultSummary struct {
	ResultStatus string
	Summary      string
}

// FailureSummary exposes customer-visible durable session failure details.
type FailureSummary struct {
	Reason                 string
	Message                string
	ErrorClass             string
	PartialResultAvailable bool
}

// LifecycleTimestamps exposes durable session lifecycle timestamps.
type LifecycleTimestamps struct {
	QueuedAt           *time.Time
	AwaitingApprovalAt *time.Time
	StartedAt          *time.Time
	PausedAt           *time.Time
	ResumedAt          *time.Time
	FinishedAt         *time.Time
	InterruptedAt      *time.Time
	TerminatedAt       *time.Time
	UpdatedAt          *time.Time
}

// ArtifactRefSummary is a customer-visible artifact reference on session reads.
type ArtifactRefSummary struct {
	ID          string
	Kind        string
	Visibility  string
	ContentHash string
	SizeBytes   int64
}

// SessionBudgets exposes effective orchestrator policy budgets for one session read.
type SessionBudgets struct {
	MaxAgents int
}

// ResourceUsage summarizes one named resource pool for session runtime usage.
type ResourceUsage struct {
	Name      string
	Available int
	Total     int
}

// SessionUsage exposes resource availability and consumption for one session read.
type SessionUsage struct {
	Resources []ResourceUsage
}

// EmptySessionUsage returns the stable zero usage projection for sessions without
// runtime consumption data.
func EmptySessionUsage() SessionUsage {
	return SessionUsage{Resources: []ResourceUsage{}}
}

// SessionReadResult is the shared durable session read projection consumed by API,
// CLI, MCP, and UI transports.
type SessionReadResult struct {
	SessionID        string
	Status           LifecycleStatus
	OrchestratorKind string
	Dialect          string
	ResolvedSource   ResolvedSource
	SourceHash       string
	Policy           PolicyProjection
	Phase            string
	PhaseSummaries   []PhaseSummary
	Progress         *ProgressCounts
	Budgets          *SessionBudgets
	Usage            SessionUsage
	ResultSummary    *ResultSummary
	ArtifactRefs     []ArtifactRefSummary
	ArtifactCount    int
	Failure          *FailureSummary
	Lifecycle        *LifecycleTimestamps
	StaleLease       bool
	Links            InspectionLinks
}

// LifecycleControlLinks are API-relative links for post-control inspection.
type LifecycleControlLinks struct {
	Session    string
	Results    string
	Dispatches string
	Artifacts  string
	Events     string
	Status     string
}

// LifecycleControlResult is the shared durable lifecycle control outcome.
type LifecycleControlResult struct {
	SessionID           string
	Operation           LifecycleControlKind
	Outcome             LifecycleControlOutcome
	Status              LifecycleStatus
	Session             *SessionReadResult
	EffectivePolicyHash string
	ApprovalPreviewID   string
	DispatchID          string
	RetryDispatchID     string
	Detail              string
	Links               LifecycleControlLinks
}

const (
	dispatchQueuedEventIDPrefix      = "factory-event/dispatch-queued"
	dispatchReconciledEventIDPrefix  = "factory-event/dispatch-reconciled"
	dispatchReconciliationSourceProviderSession   = "PROVIDER_SESSION"
	dispatchReconciliationSourceRuntimeReconciler = "RUNTIME_RECONCILER"
)

// RuntimeDispatchEventInput carries durable dispatch projection inputs used to
// synthesize canonical DISPATCH_* events for runtime-backed sessions.
type RuntimeDispatchEventInput struct {
	Dispatches                []DispatchSummary
	DispatchStatusTransitions map[string][]DispatchStatus
	DispatchJavaScript        map[string]DispatchJavaScriptProjection
	Artifacts                 []ArtifactSummary
}

func runtimeDispatchEventInputFromState(state *runtimeSessionState) RuntimeDispatchEventInput {
	if state == nil {
		return RuntimeDispatchEventInput{}
	}
	return RuntimeDispatchEventInput{
		Dispatches:                state.dispatches,
		DispatchStatusTransitions: state.dispatchStatusTransitions,
		DispatchJavaScript:        state.dispatchJavaScript,
		Artifacts:                 state.artifacts,
	}
}

func rebuildRuntimeSessionCanonicalEvents(state *runtimeSessionState) []json.RawMessage {
	if state == nil {
		return nil
	}
	preserved := extractDispatchInterruptedEvents(state.events)
	projected := BuildCanonicalRuntimeSessionEvents(
		state.session,
		state.result,
		runtimeDispatchEventInputFromState(state),
	)
	return mergePreservedDispatchInterruptedEvents(projected, preserved)
}

type dispatchQueuedEventPayload struct {
	DispatchKind  string `json:"dispatchKind"`
	Label         string `json:"label,omitempty"`
	RunnerID      string `json:"runnerId,omitempty"`
	Model         string `json:"model,omitempty"`
	Provider      string `json:"provider,omitempty"`
	QueuePosition *int   `json:"queuePosition,omitempty"`
}

type dispatchReconciledEventPayload struct {
	ReconciledStatus     string                       `json:"reconciledStatus"`
	ReconciliationSource string                       `json:"reconciliationSource"`
	Replayed             bool                         `json:"replayed"`
	ArtifactIDs          []string                     `json:"artifactIds,omitempty"`
	FailureDetail        *dispatchFailureEventPayload `json:"failureDetail,omitempty"`
}

type dispatchFailureEventPayload struct {
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	ErrorClass string `json:"errorClass,omitempty"`
}

func appendCanonicalRuntimeDispatchLifecycleEvents(
	events []json.RawMessage,
	session SessionReadResult,
	input RuntimeDispatchEventInput,
	source string,
) []json.RawMessage {
	if len(input.Dispatches) == 0 {
		return events
	}
	dispatchEvents := make([]json.RawMessage, 0, len(input.Dispatches)*2)
	for index, dispatch := range input.Dispatches {
		if strings.TrimSpace(dispatch.ID) == "" {
			continue
		}
		if dispatch.Status == DispatchStatusInterrupted {
			continue
		}
		dispatchEvents = append(dispatchEvents, buildDispatchQueuedEvent(events, dispatchEvents, session, dispatch, source, index)...)
		if isReconciledDispatchStatus(dispatch.Status) {
			dispatchEvents = append(dispatchEvents, buildDispatchReconciledEvent(events, dispatchEvents, session, dispatch, source)...)
		}
	}
	if len(dispatchEvents) == 0 {
		return events
	}
	return insertEventsBeforeSessionCompleted(events, dispatchEvents)
}

func buildDispatchQueuedEvent(
	baseEvents []json.RawMessage,
	pending []json.RawMessage,
	session SessionReadResult,
	dispatch DispatchSummary,
	source string,
	queueIndex int,
) []json.RawMessage {
	dispatchKind := strings.TrimSpace(dispatch.DispatchKind)
	if dispatchKind == "" {
		dispatchKind = "JAVASCRIPT_AGENT"
	}
	payload := dispatchQueuedEventPayload{DispatchKind: dispatchKind}
	if label := strings.TrimSpace(dispatch.Label); label != "" {
		payload.Label = label
	}
	if runnerID := strings.TrimSpace(dispatch.RunnerID); runnerID != "" {
		payload.RunnerID = runnerID
	}
	if model := strings.TrimSpace(dispatch.Model); model != "" {
		payload.Model = model
	}
	if provider := strings.TrimSpace(dispatch.Provider); provider != "" {
		payload.Provider = provider
	}
	position := queueIndex
	payload.QueuePosition = &position

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return pending
	}
	return append(pending, dispatchLifecycleEvent(
		baseEvents,
		pending,
		"DISPATCH_QUEUED",
		fmt.Sprintf("%s/%s", dispatchQueuedEventIDPrefix, dispatch.ID),
		session,
		dispatch,
		source,
		encodedPayload,
	))
}

func buildDispatchReconciledEvent(
	baseEvents []json.RawMessage,
	pending []json.RawMessage,
	session SessionReadResult,
	dispatch DispatchSummary,
	source string,
) []json.RawMessage {
	payload := dispatchReconciledEventPayload{
		ReconciledStatus:     string(dispatch.Status),
		ReconciliationSource: dispatchReconciliationSource(dispatch),
		Replayed:             false,
	}
	if len(dispatch.OutputArtifactIDs) > 0 {
		payload.ArtifactIDs = append([]string(nil), dispatch.OutputArtifactIDs...)
	}
	if dispatch.FailureDetail != nil {
		payload.FailureDetail = &dispatchFailureEventPayload{
			Reason:     strings.TrimSpace(dispatch.FailureDetail.Reason),
			Message:    strings.TrimSpace(dispatch.FailureDetail.Message),
			ErrorClass: strings.TrimSpace(dispatch.FailureDetail.ErrorClass),
		}
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return pending
	}
	return append(pending, dispatchLifecycleEvent(
		baseEvents,
		pending,
		"DISPATCH_RECONCILED",
		fmt.Sprintf("%s/%s", dispatchReconciledEventIDPrefix, dispatch.ID),
		session,
		dispatch,
		source,
		encodedPayload,
	))
}

func dispatchReconciliationSource(dispatch DispatchSummary) string {
	if len(dispatch.ProviderSessionRefs) > 0 {
		return dispatchReconciliationSourceProviderSession
	}
	return dispatchReconciliationSourceRuntimeReconciler
}

func isReconciledDispatchStatus(status DispatchStatus) bool {
	switch status {
	case DispatchStatusCompleted,
		DispatchStatusFailed,
		DispatchStatusCanceled,
		DispatchStatusTimedOut,
		DispatchStatusSkipped:
		return true
	default:
		return false
	}
}

func dispatchLifecycleEvent(
	baseEvents []json.RawMessage,
	pending []json.RawMessage,
	eventType, id string,
	session SessionReadResult,
	dispatch DispatchSummary,
	source string,
	payload json.RawMessage,
) json.RawMessage {
	sequence, sessionSequence := nextCanonicalEventSequence(append(baseEvents, append(pending, json.RawMessage("{}"))...))
	eventTime := canonicalSessionEventTime(session).Add(time.Duration(sessionSequence) * time.Second)

	sessionID := session.SessionID
	orchestratorKind := strings.ToUpper(strings.TrimSpace(session.OrchestratorKind))
	var orchestratorDialect *string
	if dialect := strings.TrimSpace(session.Dialect); dialect != "" {
		orchestratorDialect = &dialect
	}
	var phaseID *string
	var phaseName *string
	if phase := strings.TrimSpace(dispatch.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	} else if phase := strings.TrimSpace(session.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	}
	dispatchID := dispatch.ID

	context := canonicalFactoryEventContext{
		Sequence:        sequence,
		Tick:            sequence,
		EventTime:       eventTime,
		SessionID:       &sessionID,
		SessionSequence: intPtr(sessionSequence),
		Source:          &source,
		DispatchID:      &dispatchID,
	}
	if orchestratorKind != "" {
		context.OrchestratorKind = &orchestratorKind
	}
	if orchestratorDialect != nil {
		context.OrchestratorDialect = orchestratorDialect
	}
	if phaseID != nil {
		context.PhaseID = phaseID
	}
	if phaseName != nil {
		context.PhaseName = phaseName
	}

	encoded, err := json.Marshal(canonicalFactoryEvent{
		SchemaVersion: canonicalFactoryEventSchemaVersion,
		ID:            id,
		Type:          eventType,
		Context:       context,
		Payload:       payload,
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func insertEventsBeforeSessionCompleted(events, insertion []json.RawMessage) []json.RawMessage {
	if len(insertion) == 0 {
		return events
	}
	completedIndex := len(events)
	for index, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if strings.TrimSpace(envelope.Type) == "SESSION_COMPLETED" {
			completedIndex = index
			break
		}
	}
	merged := make([]json.RawMessage, 0, len(events)+len(insertion))
	merged = append(merged, events[:completedIndex]...)
	merged = append(merged, insertion...)
	merged = append(merged, events[completedIndex:]...)
	return merged
}
