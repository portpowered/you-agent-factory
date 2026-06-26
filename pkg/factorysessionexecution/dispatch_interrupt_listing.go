package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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
	dispatch := dispatches[dispatchID]
	dispatch.ID = dispatchID
	dispatch.Status = DispatchStatusInterrupted
	dispatch.Phase = stringValuePtr(envelope.Context.PhaseName)
	if dispatch.Phase == "" {
		dispatch.Phase = stringValuePtr(envelope.Context.PhaseID)
	}
	dispatch.FailureDetail = dispatchInterruptionFailureDetail(reason)
	dispatches[dispatchID] = dispatch
	return nil
}
