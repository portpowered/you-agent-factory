package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AppendSessionLifecycleControlEvent records one accepted pause or resume control on
// the canonical session event stream without rebuilding earlier lifecycle events.
func AppendSessionLifecycleControlEvent(
	events []json.RawMessage,
	session SessionReadResult,
	previousStatus LifecycleStatus,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	occurredAt time.Time,
	source string,
	reason string,
) []json.RawMessage {
	if outcome != LifecycleControlOutcomeAccepted {
		return events
	}
	if operation != LifecycleControlPause && operation != LifecycleControlResume {
		return events
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		return events
	}
	sessionSequence := nextCanonicalSessionEventSequence(events)
	eventID := fmt.Sprintf("session-lifecycle-control/%s/%d", sessionID, sessionSequence)
	event := buildSessionLifecycleControlEvent(
		session,
		previousStatus,
		session.Status,
		operation,
		outcome,
		occurredAt.UTC(),
		source,
		reason,
		eventID,
		sessionSequence,
	)
	return append(append([]json.RawMessage(nil), events...), event)
}

func synthesizeLifecycleControlEventsFromState(
	session SessionReadResult,
	events []json.RawMessage,
	source string,
) []json.RawMessage {
	if session.Lifecycle == nil {
		return synthesizeLifecycleControlEventsFromStatus(session, events, source)
	}
	out := append([]json.RawMessage(nil), events...)
	if session.Lifecycle.PausedAt != nil {
		previousStatus := LifecycleStatusRunning
		if session.Lifecycle.ResumedAt != nil && session.Lifecycle.ResumedAt.After(*session.Lifecycle.PausedAt) {
			// Session was resumed; still synthesize the pause that preceded resume.
		}
		out = appendLifecycleControlEventIfAbsent(
			out,
			session,
			previousStatus,
			LifecycleStatusPaused,
			LifecycleControlPause,
			*session.Lifecycle.PausedAt,
			source,
			"",
		)
	}
	if session.Lifecycle.ResumedAt != nil && session.Lifecycle.PausedAt != nil &&
		!session.Lifecycle.ResumedAt.Before(*session.Lifecycle.PausedAt) {
		out = appendLifecycleControlEventIfAbsent(
			out,
			session,
			LifecycleStatusPaused,
			LifecycleStatusRunning,
			LifecycleControlResume,
			*session.Lifecycle.ResumedAt,
			source,
			"",
		)
	}
	if len(out) > len(events) {
		return out
	}
	return synthesizeLifecycleControlEventsFromStatus(session, events, source)
}

func synthesizeLifecycleControlEventsFromStatus(
	session SessionReadResult,
	events []json.RawMessage,
	source string,
) []json.RawMessage {
	if session.Status != LifecycleStatusPaused {
		return events
	}
	baseTime := canonicalSessionEventTime(session).UTC()
	pausedAt := baseTime.Add(2 * time.Second)
	return appendLifecycleControlEventIfAbsent(
		events,
		session,
		LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleControlPause,
		pausedAt,
		source,
		"",
	)
}

func appendLifecycleControlEventIfAbsent(
	events []json.RawMessage,
	session SessionReadResult,
	previousStatus LifecycleStatus,
	newStatus LifecycleStatus,
	operation LifecycleControlKind,
	occurredAt time.Time,
	source string,
	reason string,
) []json.RawMessage {
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type != "SESSION_LIFECYCLE_CONTROL" {
			continue
		}
		var payload sessionLifecycleControlEventPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			continue
		}
		if payload.Operation == string(operation) && payload.NewStatus == string(newStatus) {
			return events
		}
	}
	sessionSequence := nextCanonicalSessionEventSequence(events)
	eventID := fmt.Sprintf("session-lifecycle-control/%s/%d", session.SessionID, sessionSequence)
	event := buildSessionLifecycleControlEvent(
		session,
		previousStatus,
		newStatus,
		operation,
		LifecycleControlOutcomeAccepted,
		occurredAt.UTC(),
		source,
		reason,
		eventID,
		sessionSequence,
	)
	return append(append([]json.RawMessage(nil), events...), event)
}

type sessionLifecycleControlEventPayload struct {
	Operation      string `json:"operation"`
	Outcome        string `json:"outcome"`
	PreviousStatus string `json:"previousStatus"`
	NewStatus      string `json:"newStatus"`
	OccurredAt     string `json:"occurredAt"`
	Reason         string `json:"reason,omitempty"`
}

func buildSessionLifecycleControlEvent(
	session SessionReadResult,
	previousStatus LifecycleStatus,
	newStatus LifecycleStatus,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	occurredAt time.Time,
	source string,
	reason string,
	eventID string,
	sessionSequence int,
) json.RawMessage {
	sessionID := strings.TrimSpace(session.SessionID)
	orchestratorKind := string(session.OrchestratorKind)
	var orchestratorDialect *string
	if dialect := strings.TrimSpace(session.Dialect); dialect != "" {
		orchestratorDialect = &dialect
	}
	var phaseID *string
	var phaseName *string
	if phase := strings.TrimSpace(session.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	}
	payload := sessionLifecycleControlEventPayload{
		Operation:      string(operation),
		Outcome:        string(outcome),
		PreviousStatus: string(previousStatus),
		NewStatus:      string(newStatus),
		OccurredAt:     occurredAt.Format(time.RFC3339),
	}
	if trimmedReason := strings.TrimSpace(reason); trimmedReason != "" {
		payload.Reason = trimmedReason
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}

	sequence := sessionSequence + 1
	context := canonicalFactoryEventContext{
		Sequence:        sequence,
		Tick:            sequence,
		EventTime:       occurredAt,
		SessionID:       &sessionID,
		SessionSequence: intPtr(sessionSequence),
		Source:          &source,
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
		ID:            eventID,
		Type:          "SESSION_LIFECYCLE_CONTROL",
		Context:       context,
		Payload:       encodedPayload,
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func nextCanonicalSessionEventSequence(events []json.RawMessage) int {
	next := 0
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Context.SessionSequence == nil {
			continue
		}
		if *envelope.Context.SessionSequence >= next {
			next = *envelope.Context.SessionSequence + 1
		}
	}
	return next
}
