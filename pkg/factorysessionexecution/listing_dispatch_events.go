package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

const (
	dispatchQueuedEventIDPrefix                   = "factory-event/dispatch-queued"
	dispatchReconciledEventIDPrefix               = "factory-event/dispatch-reconciled"
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
	CheckpointEvents          []RuntimeCheckpointEventProjection
}

// RuntimeCheckpointEventProjection carries replay-safe checkpoint lineage for one
// ORCHESTRATOR_CHECKPOINT_WRITTEN event.
type RuntimeCheckpointEventProjection struct {
	CheckpointID       string
	Label              string
	Summary            string
	SourceHash         string
	ResumabilityStatus string
	Timestamp          time.Time
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
		CheckpointEvents:          checkpointEventsFromRuntimeState(state),
	}
}

func checkpointEventsFromRuntimeState(state *runtimeSessionState) []RuntimeCheckpointEventProjection {
	if state == nil {
		return nil
	}
	resumability := "UNKNOWN"
	if state.checkpointSummary != nil {
		resumability = "RESUMABLE"
	}
	sourceHash := strings.TrimSpace(state.session.SourceHash)
	events := make([]RuntimeCheckpointEventProjection, 0)
	for _, record := range state.runtimeRecords {
		if record.Kind != workflowruntime.RecordKindCheckpoint || record.Checkpoint == nil {
			continue
		}
		checkpoint := record.Checkpoint
		projection := RuntimeCheckpointEventProjection{
			CheckpointID:       strings.TrimSpace(checkpoint.ID),
			Label:              strings.TrimSpace(checkpoint.Label),
			Summary:            strings.TrimSpace(checkpoint.Summary),
			SourceHash:         sourceHash,
			ResumabilityStatus: resumability,
		}
		if state.checkpointSummary != nil && !state.checkpointSummary.CreatedAt.IsZero() {
			projection.Timestamp = state.checkpointSummary.CreatedAt.UTC()
		}
		events = append(events, projection)
	}
	return events
}

func appendCanonicalOrchestratorCheckpointEvents(
	events []json.RawMessage,
	session SessionReadResult,
	checkpoints []RuntimeCheckpointEventProjection,
	source string,
) []json.RawMessage {
	if len(checkpoints) == 0 {
		return events
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
	checkpointEvents := make([]json.RawMessage, 0, len(checkpoints))
	for index, checkpoint := range checkpoints {
		checkpointID := strings.TrimSpace(checkpoint.CheckpointID)
		if checkpointID == "" {
			continue
		}
		payload := map[string]any{
			"label":              checkpoint.Label,
			"resumabilityStatus": checkpoint.ResumabilityStatus,
		}
		if summary := strings.TrimSpace(checkpoint.Summary); summary != "" {
			payload["summary"] = summary
		}
		if sourceHash := strings.TrimSpace(checkpoint.SourceHash); sourceHash != "" {
			payload["sourceHash"] = sourceHash
		}
		timestamp := checkpoint.Timestamp
		if timestamp.IsZero() {
			timestamp = eventTime.Add(time.Duration(index+1) * time.Second)
		}
		payload["timestamp"] = timestamp.UTC().Format(time.RFC3339)
		sequence := nextCanonicalSessionEventSequence(events) + len(checkpointEvents)
		id := fmt.Sprintf("orchestrator-checkpoint-written/%s/%s", sessionID, checkpointID)
		checkpointEvents = append(checkpointEvents, builder.eventWithCheckpoint(
			"ORCHESTRATOR_CHECKPOINT_WRITTEN",
			id,
			sequence,
			&checkpointID,
			mustMarshalPayload(payload),
		))
	}
	if len(checkpointEvents) == 0 {
		return events
	}
	return insertEventsBeforeSessionCompleted(events, checkpointEvents)
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
