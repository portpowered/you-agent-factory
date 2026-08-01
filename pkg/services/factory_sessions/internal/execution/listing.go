package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Listing helpers remain available from this compatibility package while their
// canonical implementations live on the Factory Sessions service root.
var (
	IsPersistedListCandidate           = factorysessions.IsPersistedListCandidate
	LiveListSummaryFromSessionRead     = factorysessions.LiveListSummaryFromSessionRead
	DurableListSummaryFromSessionRead  = factorysessions.DurableListSummaryFromSessionRead
	MatchesDurableSessionListFilters   = factorysessions.MatchesDurableSessionListFilters
	FilterDurableSessionSummaries      = factorysessions.FilterDurableSessionSummaries
	FilterPersistedSessionSummaries    = factorysessions.FilterPersistedSessionSummaries
	DeduplicateLiveSessionsForAllScope = factorysessions.DeduplicateLiveSessionsForAllScope
	SortDurableSessionSummaries        = factorysessions.SortDurableSessionSummaries
	SortLiveSessionSummaries           = factorysessions.SortLiveSessionSummaries
	DeriveSessionActionAvailability    = factorysessions.DeriveSessionActionAvailability
	ApplySessionListScope              = factorysessions.ApplySessionListScope
	IsRecoverableSession               = factorysessions.IsRecoverableSession
)

func containsLifecycleStatus(values []LifecycleStatus, target LifecycleStatus) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func firstLifecycleTimestamp(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func latestLifecycleTimestamp(lifecycle *LifecycleTimestamps) *time.Time {
	if lifecycle == nil {
		return nil
	}
	var latest *time.Time
	for _, candidate := range []*time.Time{
		lifecycle.UpdatedAt,
		lifecycle.FinishedAt,
		lifecycle.InterruptedAt,
		lifecycle.TerminatedAt,
		lifecycle.ResumedAt,
		lifecycle.PausedAt,
		lifecycle.StartedAt,
		lifecycle.AwaitingApprovalAt,
		lifecycle.QueuedAt,
	} {
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			latest = candidate
		}
	}
	return latest
}

func uniqueRuntimeRecords(records []workflowsource.JavaScriptRuntimeRecord) []workflowsource.JavaScriptRuntimeRecord {
	unique := make([]workflowsource.JavaScriptRuntimeRecord, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			continue
		}
		key := string(encoded)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, cloneRuntimeRecord(record))
	}
	return unique
}

func appendCanonicalOrchestratorRuntimeRecordEvents(events []json.RawMessage, session SessionReadResult, input RuntimeDispatchEventInput, source string) []json.RawMessage {
	if len(input.RuntimeRecords) == 0 {
		return events
	}
	eventTime := canonicalSessionEventTime(session)
	var dialect *string
	if value := strings.TrimSpace(session.Dialect); value != "" {
		dialect = &value
	}
	checkpointByID := make(map[string]RuntimeCheckpointEventProjection, len(input.CheckpointEvents))
	for _, checkpoint := range input.CheckpointEvents {
		checkpointByID[strings.TrimSpace(checkpoint.CheckpointID)] = checkpoint
	}
	projected := make([]json.RawMessage, 0, len(input.RuntimeRecords)*2)
	var activePhase *orchestratorPhaseTransition
	for index, record := range input.RuntimeRecords {
		switch record.Kind {
		case workflowsource.JavaScriptRecordKindPhase:
			if record.Phase == nil || strings.TrimSpace(record.Phase.Name) == "" {
				continue
			}
			if activePhase != nil {
				projected = append(projected, canonicalOrchestratorPhaseTransitionEvent(events, session, source, dialect, *activePhase, "COMPLETED", index, len(projected)))
			}
			activePhase = &orchestratorPhaseTransition{index: index, name: strings.TrimSpace(record.Phase.Name)}
			projected = append(projected, canonicalOrchestratorPhaseTransitionEvent(events, session, source, dialect, *activePhase, "ACTIVE", index, len(projected)))
		case workflowsource.JavaScriptRecordKindCheckpoint:
			if record.Checkpoint == nil || strings.TrimSpace(record.Checkpoint.ID) == "" {
				continue
			}
			checkpointID := strings.TrimSpace(record.Checkpoint.ID)
			checkpoint := checkpointByID[checkpointID]
			checkpoint.CheckpointID = checkpointID
			if checkpoint.Label == "" {
				checkpoint.Label = strings.TrimSpace(record.Checkpoint.Label)
			}
			projected = append(projected, canonicalOrchestratorCheckpointRecordEvent(events, session, source, dialect, activePhase, checkpoint, eventTime, index, len(projected)))
		}
	}
	if activePhase != nil && IsTerminalLifecycleStatus(session.Status) {
		projected = append(projected, canonicalOrchestratorPhaseTransitionEvent(events, session, source, dialect, *activePhase, "COMPLETED", len(input.RuntimeRecords), len(projected)))
	}
	return insertEventsBeforeSessionCompleted(events, projected)
}

type orchestratorPhaseTransition struct {
	index int
	name  string
}

func canonicalOrchestratorPhaseTransitionEvent(events []json.RawMessage, session SessionReadResult, source string, dialect *string, phase orchestratorPhaseTransition, status string, transitionIndex, projectedIndex int) json.RawMessage {
	phaseName := phase.name
	builder := canonicalSessionEventBuilder{
		sessionID: session.SessionID, orchestratorKind: string(session.OrchestratorKind),
		orchestratorDialect: dialect, phaseID: &phaseName, phaseName: &phaseName,
		source: source, eventTime: canonicalSessionEventTime(session),
	}
	return builder.event(
		"ORCHESTRATOR_PHASE_CHANGED",
		fmt.Sprintf("orchestrator-phase-changed/%s/%d/%s/%s/%d", session.SessionID, phase.index, phase.name, strings.ToLower(status), transitionIndex),
		nextCanonicalSessionEventSequence(events)+projectedIndex,
		mustMarshalPayload(map[string]any{"phaseStatus": status}),
	)
}

func canonicalOrchestratorCheckpointRecordEvent(events []json.RawMessage, session SessionReadResult, source string, dialect *string, activePhase *orchestratorPhaseTransition, checkpoint RuntimeCheckpointEventProjection, eventTime time.Time, recordIndex, projectedIndex int) json.RawMessage {
	var phaseID *string
	if activePhase != nil {
		phase := activePhase.name
		phaseID = &phase
	}
	builder := canonicalSessionEventBuilder{
		sessionID: session.SessionID, orchestratorKind: string(session.OrchestratorKind),
		orchestratorDialect: dialect, phaseID: phaseID, phaseName: phaseID,
		source: source, eventTime: eventTime,
	}
	payload := map[string]any{"label": checkpoint.Label, "resumabilityStatus": checkpoint.ResumabilityStatus}
	if summary := strings.TrimSpace(checkpoint.Summary); summary != "" {
		payload["summary"] = summary
	}
	if sourceHash := strings.TrimSpace(checkpoint.SourceHash); sourceHash != "" {
		payload["sourceHash"] = sourceHash
	}
	timestamp := checkpoint.Timestamp
	if timestamp.IsZero() {
		timestamp = eventTime.Add(time.Duration(recordIndex+1) * time.Second)
	}
	payload["timestamp"] = timestamp.UTC().Format(time.RFC3339)
	checkpointID := checkpoint.CheckpointID
	return builder.eventWithCheckpoint(
		"ORCHESTRATOR_CHECKPOINT_WRITTEN",
		fmt.Sprintf("orchestrator-checkpoint-written/%s/%d/%s", session.SessionID, recordIndex, checkpointID),
		nextCanonicalSessionEventSequence(events)+projectedIndex,
		&checkpointID,
		mustMarshalPayload(payload),
	)
}

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
	CheckpointID        *string   `json:"checkpointId,omitempty"`
	DispatchID          *string   `json:"dispatchId,omitempty"`
	Source              *string   `json:"source,omitempty"`
}

type canonicalFactoryEvent struct {
	SchemaVersion string                       `json:"schemaVersion"`
	ID            string                       `json:"id"`
	Type          string                       `json:"type"`
	Context       canonicalFactoryEventContext `json:"context"`
	Payload       json.RawMessage              `json:"payload"`
}

const (
	canonicalEventSourceFakeService    = "fake-service"
	canonicalEventSourceRuntimeService = "runtime-service"
)

// BuildCanonicalSessionEvents synthesizes canonical FactoryEvent documents for one
// durable session read and result projection pair.
func BuildCanonicalSessionEvents(session SessionReadResult, result ResultReadResult) []json.RawMessage {
	return buildCanonicalSessionEvents(session, result, canonicalEventSourceFakeService)
}

// BuildCanonicalRuntimeSessionEvents synthesizes canonical FactoryEvent documents
// for one runtime-backed durable session read and result projection pair. When
// dispatch projection input is provided, runtime-backed child dispatches also
// emit DISPATCH_QUEUED and terminal DISPATCH_RECONCILED lifecycle events.
func BuildCanonicalRuntimeSessionEvents(
	session SessionReadResult,
	result ResultReadResult,
	dispatch ...RuntimeDispatchEventInput,
) []json.RawMessage {
	events := buildCanonicalSessionEvents(session, result, canonicalEventSourceRuntimeService)
	if len(dispatch) == 0 {
		if len(session.PhaseSummaries) > 0 {
			events = appendCanonicalOrchestratorPhaseEvents(events, session, canonicalEventSourceRuntimeService)
		}
		return events
	}
	input := dispatch[0]
	if len(input.RuntimeRecords) > 0 {
		events = appendCanonicalOrchestratorRuntimeRecordEvents(events, session, input, canonicalEventSourceRuntimeService)
	} else {
		if len(session.PhaseSummaries) > 0 {
			events = appendCanonicalOrchestratorPhaseEvents(events, session, canonicalEventSourceRuntimeService)
		}
		if len(input.CheckpointEvents) > 0 {
			events = appendCanonicalOrchestratorCheckpointEvents(events, session, input.CheckpointEvents, canonicalEventSourceRuntimeService)
		}
	}
	if len(input.Dispatches) == 0 {
		return events
	}
	return appendCanonicalRuntimeDispatchLifecycleEvents(
		events,
		session,
		input,
		canonicalEventSourceRuntimeService,
	)
}

// MapCanonicalRuntimeSessionEvents validates shared runtime facts before mapping
// them to canonical Factory Events. Callers at ingestion boundaries should use
// this function so malformed facts cannot become a partial public event stream.
func MapCanonicalRuntimeSessionEvents(
	session SessionReadResult,
	result ResultReadResult,
	input RuntimeDispatchEventInput,
) ([]json.RawMessage, error) {
	if err := validateCanonicalRuntimeFacts(session, result, input); err != nil {
		return nil, err
	}
	return BuildCanonicalRuntimeSessionEvents(session, result, input), nil
}

func validateCanonicalRuntimeFacts(
	session SessionReadResult,
	result ResultReadResult,
	input RuntimeDispatchEventInput,
) error {
	if err := validateCanonicalSessionFact(session, result); err != nil {
		return err
	}
	for index, dispatch := range input.Dispatches {
		if err := validateCanonicalDispatchFact(session.SessionID, index, dispatch); err != nil {
			return err
		}
	}
	for index, artifact := range input.Artifacts {
		if strings.TrimSpace(artifact.ID) == "" || strings.TrimSpace(artifact.Kind) == "" || strings.TrimSpace(artifact.Visibility) == "" {
			return fmt.Errorf("map canonical runtime facts for session %q: artifact %d requires ID, kind, and visibility", session.SessionID, index)
		}
	}
	for index, checkpoint := range input.CheckpointEvents {
		if strings.TrimSpace(checkpoint.CheckpointID) == "" {
			return fmt.Errorf("map canonical runtime facts for session %q: checkpoint %d ID is required", session.SessionID, index)
		}
	}
	return nil
}

func validateCanonicalSessionFact(session SessionReadResult, result ResultReadResult) error {
	if strings.TrimSpace(session.SessionID) == "" {
		return fmt.Errorf("map canonical runtime facts: session ID is required")
	}
	if strings.TrimSpace(session.OrchestratorKind) == "" {
		return fmt.Errorf("map canonical runtime facts for session %q: orchestrator kind is required", session.SessionID)
	}
	if result.SessionID != "" && result.SessionID != session.SessionID {
		return fmt.Errorf("map canonical runtime facts for session %q: result session ID %q does not match", session.SessionID, result.SessionID)
	}
	return nil
}

func validateCanonicalDispatchFact(sessionID string, index int, dispatch DispatchSummary) error {
	if strings.TrimSpace(dispatch.ID) == "" {
		return fmt.Errorf("map canonical runtime facts for session %q: dispatch %d ID is required", sessionID, index)
	}
	if strings.TrimSpace(string(dispatch.Status)) == "" {
		return fmt.Errorf("map canonical runtime facts for session %q dispatch %q: status is required", sessionID, dispatch.ID)
	}
	if strings.TrimSpace(dispatch.DispatchKind) == "" {
		return fmt.Errorf("map canonical runtime facts for session %q dispatch %q: dispatch kind is required", sessionID, dispatch.ID)
	}
	for refIndex, ref := range dispatch.ProviderSessionRefs {
		if strings.TrimSpace(ref.Provider) == "" || strings.TrimSpace(ref.Kind) == "" || strings.TrimSpace(ref.ID) == "" {
			return fmt.Errorf("map canonical runtime facts for session %q dispatch %q: provider session ref %d requires provider, kind, and ID", sessionID, dispatch.ID, refIndex)
		}
	}
	return nil
}

func buildCanonicalSessionEvents(session SessionReadResult, result ResultReadResult, source string) []json.RawMessage {
	if strings.TrimSpace(session.SessionID) == "" {
		return nil
	}
	eventTime := canonicalSessionEventTime(session)
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
	sessionSequence := 1

	if result.ResultStatus != "" {
		events = append(events, builder.event(
			"SESSION_RESULT_UPDATED",
			"session-result-updated/"+sessionID,
			sessionSequence,
			mustMarshalPayload(canonicalSessionResultUpdatedPayload(session, result)),
		))
		sessionSequence++
	}

	events, sessionSequence = appendCanonicalPauseResumeSessionEvents(events, builder, sessionID, session.Lifecycle, sessionSequence)
	events = synthesizeLifecycleControlEventsFromState(session, events, source)

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
		completedSequence := nextCanonicalSessionEventSequence(events)
		events = append(events, builder.event("SESSION_COMPLETED", "session-completed/"+sessionID, completedSequence, mustMarshalPayload(payload)))
	}

	return events
}

func appendCanonicalPauseResumeSessionEvents(
	events []json.RawMessage,
	builder canonicalSessionEventBuilder,
	sessionID string,
	lifecycle *LifecycleTimestamps,
	sessionSequence int,
) ([]json.RawMessage, int) {
	if lifecycle == nil {
		return events, sessionSequence
	}
	if lifecycle.PausedAt != nil {
		events = append(events, builder.event(
			"SESSION_PAUSED",
			"session-paused/"+sessionID,
			sessionSequence,
			mustMarshalPayload(map[string]any{
				"status":   string(LifecycleStatusPaused),
				"pausedAt": lifecycle.PausedAt.UTC().Format(time.RFC3339),
			}),
		))
		sessionSequence++
	}
	if lifecycle.ResumedAt != nil {
		resumeStatus := string(LifecycleStatusRunning)
		if lifecycle.InterruptedAt != nil && lifecycle.InterruptedAt.Before(lifecycle.ResumedAt.UTC()) {
			resumeStatus = string(LifecycleStatusResuming)
		}
		events = append(events, builder.event(
			"SESSION_RESUMED",
			"session-resumed/"+sessionID,
			sessionSequence,
			mustMarshalPayload(map[string]any{
				"status":    resumeStatus,
				"resumedAt": lifecycle.ResumedAt.UTC().Format(time.RFC3339),
			}),
		))
		sessionSequence++
	}
	return events, sessionSequence
}

func canonicalSessionResultUpdatedPayload(session SessionReadResult, result ResultReadResult) map[string]any {
	payload := map[string]any{
		"resultStatus": string(result.ResultStatus),
	}
	if primaryResult := canonicalPrimaryResultPayload(result.PrimaryResult); primaryResult != nil {
		payload["resultSummary"] = primaryResult
	} else if session.ResultSummary != nil {
		if summary := strings.TrimSpace(session.ResultSummary.Summary); summary != "" {
			payload["resultSummary"] = []map[string]any{
				{"type": "text", "text": summary},
			}
		}
	}
	if len(result.ArtifactIDs) > 0 {
		payload["artifactIds"] = append([]string(nil), result.ArtifactIDs...)
	}
	if availability := canonicalResultAvailabilityPayload(result.Availability); availability != nil {
		payload["availability"] = availability
	}
	return payload
}

func canonicalPrimaryResultPayload(primaryResult json.RawMessage) any {
	if len(primaryResult) == 0 {
		return nil
	}
	var payload []work.WorkContentPart
	if err := json.Unmarshal(primaryResult, &payload); err != nil || payload == nil {
		return nil
	}
	return append(json.RawMessage(nil), primaryResult...)
}

func canonicalResultAvailabilityPayload(availability *ResultAvailabilityDetail) map[string]any {
	if availability == nil {
		return nil
	}
	reason := strings.TrimSpace(availability.Reason)
	message := strings.TrimSpace(availability.Message)
	if reason == "" && message == "" {
		return nil
	}
	payload := map[string]any{
		"retryable": availability.Retryable,
	}
	if reason != "" {
		payload["reason"] = reason
	}
	if message != "" {
		payload["message"] = message
	}
	return payload
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
	return b.eventWithCheckpoint(eventType, id, sessionSequence, nil, payload)
}

func (b canonicalSessionEventBuilder) eventWithCheckpoint(
	eventType, id string,
	sessionSequence int,
	checkpointID *string,
	payload json.RawMessage,
) json.RawMessage {
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
	if checkpointID != nil && strings.TrimSpace(*checkpointID) != "" {
		context.CheckpointID = checkpointID
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
