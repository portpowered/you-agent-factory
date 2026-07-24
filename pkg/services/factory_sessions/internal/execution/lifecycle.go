package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func EmptySessionUsage() SessionUsage {
	return factorysessions.EmptySessionUsage()
}

func EvaluateLifecycleControl(operation LifecycleControlKind, status LifecycleStatus) LifecycleControlOutcome {
	return factorysessions.EvaluateLifecycleControl(operation, status)
}

func LifecycleStatusFromFactoryRuntimeState(factoryState string) LifecycleStatus {
	return factorysessions.LifecycleStatusFromFactoryRuntimeState(factoryState)
}

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

// IsTerminalLifecycleStatus reports whether status is terminal and therefore
// immutable except for explicitly allowed inspection or retry behaviors.
func IsTerminalLifecycleStatus(status LifecycleStatus) bool {
	return factorysessions.IsTerminalLifecycleStatus(status)
}

// AllowsRetryDispatchOnTerminal reports whether retry-dispatch remains permitted
// after the session reaches a terminal status. Failed sessions may still accept
// retry-dispatch for failed child dispatches.
func AllowsRetryDispatchOnTerminal(status LifecycleStatus) bool {
	return factorysessions.AllowsRetryDispatchOnTerminal(status)
}

// AllowsInterruptDispatchOnSession reports whether interrupt-dispatch remains
// permitted while the session is actively running goal work.
func AllowsInterruptDispatchOnSession(status LifecycleStatus) bool {
	return factorysessions.AllowsInterruptDispatchOnSession(status)
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
func ObservedCancellationStatusForInterrupt(priorStatus DispatchStatus) DispatchStatus {
	switch priorStatus {
	case DispatchStatusRunning:
		return DispatchStatusRunning
	default:
		return DispatchStatusInterrupted
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
	interruptedAt time.Time,
) []json.RawMessage {
	if strings.TrimSpace(session.SessionID) == "" || strings.TrimSpace(dispatch.ID) == "" {
		return events
	}
	reason := dispatchInterruptionReason(interrupt)
	observedStatus := ObservedCancellationStatusForInterrupt(priorStatus)
	sequence, sessionSequence := nextCanonicalEventSequence(events)
	eventTime := canonicalSessionEventTime(session).Add(time.Duration(sessionSequence) * time.Second)
	if !interruptedAt.IsZero() && interruptedAt.After(eventTime) {
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
	case "DISPATCH_QUEUED":
		return applyDispatchQueuedProjection(dispatches, envelope)
	case "DISPATCH_RECONCILED":
		return applyDispatchReconciledProjection(dispatches, envelope)
	case "DISPATCH_INTERRUPTED":
		return applyDispatchInterruptedProjection(dispatches, envelope)
	default:
		return nil
	}
}

func applyDispatchQueuedProjection(dispatches map[string]DispatchSummary, envelope canonicalFactoryEvent) error {
	dispatchID := stringValuePtr(envelope.Context.DispatchID)
	if dispatchID == "" {
		return fmt.Errorf("DISPATCH_QUEUED missing dispatchId")
	}
	var payload dispatchQueuedEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal DISPATCH_QUEUED payload: %w", err)
	}
	dispatches[dispatchID] = DispatchSummary{
		ID:           dispatchID,
		Status:       DispatchStatusQueued,
		DispatchKind: strings.TrimSpace(payload.DispatchKind),
		Phase:        stringValuePtr(envelope.Context.PhaseID),
		Label:        strings.TrimSpace(payload.Label),
		RunnerID:     strings.TrimSpace(payload.RunnerID),
		Model:        strings.TrimSpace(payload.Model),
		Provider:     strings.TrimSpace(payload.Provider),
	}
	return nil
}

func applyDispatchReconciledProjection(dispatches map[string]DispatchSummary, envelope canonicalFactoryEvent) error {
	dispatchID := stringValuePtr(envelope.Context.DispatchID)
	if dispatchID == "" {
		return fmt.Errorf("DISPATCH_RECONCILED missing dispatchId")
	}
	var payload dispatchReconciledEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal DISPATCH_RECONCILED payload: %w", err)
	}
	dispatch := dispatches[dispatchID]
	dispatch.ID = dispatchID
	dispatch.Status = DispatchStatus(strings.TrimSpace(payload.ReconciledStatus))
	dispatch.ProviderSessionRefs = cloneProviderSessionRefs(payload.ProviderSessionRefs)
	dispatch.OutputArtifactIDs = append([]string(nil), payload.ArtifactIDs...)
	if payload.FailureDetail != nil {
		dispatch.FailureDetail = &DispatchFailureDetail{
			Reason:  strings.TrimSpace(payload.FailureDetail.Reason),
			Message: strings.TrimSpace(payload.FailureDetail.Message),
		}
	}
	dispatches[dispatchID] = dispatch
	return nil
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
	return resequenceCanonicalEvents(merged)
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

type parsedCanonicalEvent struct {
	ID              string
	Sequence        int
	SessionSequence *int
}

// FilterEventsAfterReconnect returns only events after the requested reconnect cursor.
// When both AfterEventID and AfterSequence are set, AfterEventID wins.
func FilterEventsAfterReconnect(events []json.RawMessage, req EventReconnectRequest, sessionID string) ([]json.RawMessage, error) {
	if len(events) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(req.AfterEventID) == "" && req.AfterSequence == nil {
		return append([]json.RawMessage(nil), events...), nil
	}

	parsed := make([]parsedCanonicalEvent, len(events))
	for index, raw := range events {
		event, err := parseCanonicalEvent(raw)
		if err != nil {
			return nil, fmt.Errorf("parse event %d: %w", index, err)
		}
		parsed[index] = event
	}

	if afterID := strings.TrimSpace(req.AfterEventID); afterID != "" {
		return filterEventsAfterEventID(events, parsed, afterID)
	}
	if req.AfterSequence == nil {
		return append([]json.RawMessage(nil), events...), nil
	}
	return filterEventsAfterSequence(events, parsed, *req.AfterSequence, sessionID)
}

func filterEventsAfterEventID(events []json.RawMessage, parsed []parsedCanonicalEvent, afterID string) ([]json.RawMessage, error) {
	for index, event := range parsed {
		if event.ID == afterID {
			return append([]json.RawMessage(nil), events[index+1:]...), nil
		}
	}
	return nil, fmt.Errorf("%w: after_event_id %q", ErrReconnectCursorNotFound, afterID)
}

func filterEventsAfterSequence(events []json.RawMessage, parsed []parsedCanonicalEvent, ackSequence int, sessionID string) ([]json.RawMessage, error) {
	if sessionID != "" {
		for index := len(parsed) - 1; index >= 0; index-- {
			event := parsed[index]
			sequence := event.Sequence
			if event.SessionSequence != nil {
				sequence = *event.SessionSequence
			}
			if sequence == ackSequence {
				return append([]json.RawMessage(nil), events[index+1:]...), nil
			}
		}
		return nil, fmt.Errorf("%w: after_sequence %d for session %q", ErrReconnectCursorNotFound, ackSequence, sessionID)
	}
	for index := len(parsed) - 1; index >= 0; index-- {
		if parsed[index].Sequence == ackSequence {
			return append([]json.RawMessage(nil), events[index+1:]...), nil
		}
	}
	return nil, fmt.Errorf("%w: after_sequence %d", ErrReconnectCursorNotFound, ackSequence)
}

func parseCanonicalEvent(raw json.RawMessage) (parsedCanonicalEvent, error) {
	var envelope struct {
		ID      string `json:"id"`
		Context struct {
			Sequence        int  `json:"sequence"`
			SessionSequence *int `json:"sessionSequence"`
		} `json:"context"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return parsedCanonicalEvent{}, err
	}
	if strings.TrimSpace(envelope.ID) == "" {
		return parsedCanonicalEvent{}, fmt.Errorf("event id is required")
	}
	return parsedCanonicalEvent{
		ID:              envelope.ID,
		Sequence:        envelope.Context.Sequence,
		SessionSequence: envelope.Context.SessionSequence,
	}, nil
}
