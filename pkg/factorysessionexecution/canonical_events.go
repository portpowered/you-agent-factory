package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const canonicalFactoryEventSchemaVersion = "agent-factory.event.v1"

type canonicalFactoryEventContext struct {
	Sequence            int       `json:"sequence"`
	Tick                int       `json:"tick"`
	EventTime           time.Time `json:"eventTime"`
	SessionID           *string   `json:"sessionId,omitempty"`
	SessionSequence     *int      `json:"sessionSequence,omitempty"`
	OrchestratorKind    *string   `json:"orchestratorKind,omitempty"`
	OrchestratorDialect *string   `json:"orchestratorDialect,omitempty"`
	PhaseID             *string   `json:"phaseId,omitempty"`
	PhaseName           *string   `json:"phaseName,omitempty"`
	Source              *string   `json:"source,omitempty"`
}

type canonicalFactoryEvent struct {
	SchemaVersion string                       `json:"schemaVersion"`
	ID            string                       `json:"id"`
	Type          string                       `json:"type"`
	Context       canonicalFactoryEventContext `json:"context"`
	Payload       json.RawMessage              `json:"payload"`
}

type parsedCanonicalEvent struct {
	ID              string
	Sequence        int
	SessionSequence *int
}

// BuildCanonicalSessionEvents synthesizes canonical FactoryEvent documents for one
// durable session read and result projection pair.
func BuildCanonicalSessionEvents(session SessionReadResult, result ResultReadResult) []json.RawMessage {
	if strings.TrimSpace(session.SessionID) == "" {
		return nil
	}
	eventTime := canonicalSessionEventTime(session)
	source := "fake-service"
	sessionID := session.SessionID
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

	builder := canonicalSessionEventBuilder{
		sessionID:           sessionID,
		orchestratorKind:    orchestratorKind,
		orchestratorDialect: orchestratorDialect,
		phaseID:             phaseID,
		phaseName:           phaseName,
		source:              source,
		eventTime:           eventTime,
	}

	events := []json.RawMessage{
		builder.event("SESSION_STARTED", "session-started/"+sessionID, 0, mustMarshalPayload(map[string]any{
			"sourceRef":  optionalString(session.ResolvedSource.SourceRef),
			"sourceHash": optionalString(session.SourceHash),
			"policyHash": optionalString(session.Policy.EffectiveHash),
			"startedAt":  eventTime.UTC().Format(time.RFC3339),
		})),
	}

	if result.ResultStatus != "" {
		payload := map[string]any{
			"resultStatus": string(result.ResultStatus),
		}
		if len(result.ArtifactIDs) > 0 {
			payload["artifactIds"] = append([]string(nil), result.ArtifactIDs...)
		}
		events = append(events, builder.event("SESSION_RESULT_UPDATED", "session-result-updated/"+sessionID, 1, mustMarshalPayload(payload)))
	}

	if IsTerminalLifecycleStatus(session.Status) {
		completedAt := eventTime
		if session.Lifecycle != nil {
			switch {
			case session.Lifecycle.FinishedAt != nil:
				completedAt = session.Lifecycle.FinishedAt.UTC()
			case session.Lifecycle.TerminatedAt != nil:
				completedAt = session.Lifecycle.TerminatedAt.UTC()
			case session.Lifecycle.InterruptedAt != nil:
				completedAt = session.Lifecycle.InterruptedAt.UTC()
			}
		}
		payload := map[string]any{
			"finalStatus": string(session.Status),
			"completedAt": completedAt.UTC().Format(time.RFC3339),
		}
		if result.ResultStatus != "" {
			payload["resultStatus"] = string(result.ResultStatus)
		}
		if len(result.ArtifactIDs) > 0 {
			payload["artifactIds"] = append([]string(nil), result.ArtifactIDs...)
		}
		events = append(events, builder.event("SESSION_COMPLETED", "session-completed/"+sessionID, 2, mustMarshalPayload(payload)))
	}

	return events
}

type canonicalSessionEventBuilder struct {
	sessionID           string
	orchestratorKind    string
	orchestratorDialect *string
	phaseID             *string
	phaseName           *string
	source              string
	eventTime           time.Time
}

func (b canonicalSessionEventBuilder) event(eventType, id string, sessionSequence int, payload json.RawMessage) json.RawMessage {
	sequence := sessionSequence + 1
	context := canonicalFactoryEventContext{
		Sequence:        sequence,
		Tick:            sequence,
		EventTime:       b.eventTime.Add(time.Duration(sessionSequence) * time.Second),
		SessionID:       &b.sessionID,
		SessionSequence: intPtr(sessionSequence),
		Source:          &b.source,
	}
	if b.orchestratorKind != "" {
		context.OrchestratorKind = &b.orchestratorKind
	}
	if b.orchestratorDialect != nil {
		context.OrchestratorDialect = b.orchestratorDialect
	}
	if b.phaseID != nil {
		context.PhaseID = b.phaseID
	}
	if b.phaseName != nil {
		context.PhaseName = b.phaseName
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

func canonicalSessionEventTime(session SessionReadResult) time.Time {
	if session.Lifecycle != nil {
		switch {
		case session.Lifecycle.StartedAt != nil:
			return session.Lifecycle.StartedAt.UTC()
		case session.Lifecycle.QueuedAt != nil:
			return session.Lifecycle.QueuedAt.UTC()
		case session.Lifecycle.AwaitingApprovalAt != nil:
			return session.Lifecycle.AwaitingApprovalAt.UTC()
		}
	}
	return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
}

func optionalString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func mustMarshalPayload(payload map[string]any) json.RawMessage {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func intPtr(value int) *int {
	return &value
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
			if event.SessionSequence != nil && *event.SessionSequence == ackSequence {
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
