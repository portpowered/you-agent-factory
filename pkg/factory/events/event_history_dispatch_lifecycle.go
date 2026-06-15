package events

import (
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	eventIDDispatchQueuedPrefix      = "factory-event/dispatch-queued"
	eventIDDispatchInterruptedPrefix = "factory-event/dispatch-interrupted"
	eventIDDispatchReconciledPrefix  = "factory-event/dispatch-reconciled"
	eventIDArtifactCreatedPrefix     = "factory-event/artifact-created"
)

// DispatchQueuedInput carries replay-safe facts for DISPATCH_QUEUED.
type DispatchQueuedInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	DispatchKind        factoryapi.FactoryDispatchKind
	Label               string
	CoordinationRef     string
	ModelProvider       string
	RunnerID            string
	Model               string
	Provider            string
	ParentDispatchID    string
	RetryOfDispatchID   string
	QueuePosition       *int
	PromptDigest        string
	SchemaDigest        string
	InputArtifactIDs    []string
	InputWorkIDs        []string
}

// DispatchInterruptedInput carries replay-safe facts for DISPATCH_INTERRUPTED.
type DispatchInterruptedInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	Reason              string
	ObservedStatus      factoryapi.FactoryDispatchStatus
	RetryPlanned        bool
	ProviderSessionRef  *factoryapi.LoadableProviderSessionRef
	CheckpointRef       *factoryapi.FactorySessionJavaScriptCheckpointRef
}

// DispatchReconciledInput carries replay-safe facts for DISPATCH_RECONCILED.
type DispatchReconciledInput struct {
	SessionID              string
	OrchestratorKind       factoryapi.FactoryOrchestratorKind
	OrchestratorDialect    string
	PhaseID                string
	PhaseName              string
	DispatchID             string
	Source                 string
	Tick                   int
	ReconciledStatus       factoryapi.FactoryDispatchStatus
	ReconciliationSource   factoryapi.DispatchReconciliationSource
	Replayed               bool
	Usage                  *factoryapi.FactoryDispatchUsage
	ResultArtifactRef      *factoryapi.FactoryArtifactRef
	ArtifactIDs            []string
	FailureDetail          *factoryapi.FactoryDispatchFailureDetail
}

// ArtifactCreatedInput carries replay-safe facts for ARTIFACT_CREATED.
type ArtifactCreatedInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	Artifact            factoryapi.FactoryArtifact
	CapturedAt          *time.Time
}

// RecordDispatchQueued records a canonical dispatch queue marker.
// pkgmaintcheck:ignore-cyclomatic-complexity dispatch queue emission keeps optional replay metadata on one canonical recorder.
func (h *FactoryEventHistory) RecordDispatchQueued(input DispatchQueuedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.DispatchID) == "" || input.DispatchKind == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.DispatchQueuedEventPayload{
		DispatchKind: input.DispatchKind,
	}
	if label := strings.TrimSpace(input.Label); label != "" {
		payload.Label = &label
	}
	if coordinationRef := strings.TrimSpace(input.CoordinationRef); coordinationRef != "" {
		payload.CoordinationRef = &coordinationRef
	}
	if modelProvider := dispatchQueuedEventModelProvider(input); modelProvider != nil {
		payload.ModelProvider = modelProvider
	}
	if model := strings.TrimSpace(input.Model); model != "" {
		payload.Model = &model
	}
	if provider := strings.TrimSpace(input.Provider); provider != "" {
		payload.Provider = &provider
	}
	if parentDispatchID := strings.TrimSpace(input.ParentDispatchID); parentDispatchID != "" {
		payload.ParentDispatchId = &parentDispatchID
	}
	if retryOfDispatchID := strings.TrimSpace(input.RetryOfDispatchID); retryOfDispatchID != "" {
		payload.RetryOfDispatchId = &retryOfDispatchID
	}
	if input.QueuePosition != nil {
		payload.QueuePosition = input.QueuePosition
	}
	if promptDigest := strings.TrimSpace(input.PromptDigest); promptDigest != "" {
		payload.PromptDigest = &promptDigest
	}
	if schemaDigest := strings.TrimSpace(input.SchemaDigest); schemaDigest != "" {
		payload.SchemaDigest = &schemaDigest
	}
	if len(input.InputArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), input.InputArtifactIDs...)
		payload.InputArtifactIds = &artifactIDs
	}
	if len(input.InputWorkIDs) > 0 {
		workIDs := append([]string(nil), input.InputWorkIDs...)
		payload.InputWorkIds = &workIDs
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchQueued,
		fmt.Sprintf("%s/%s", eventIDDispatchQueuedPrefix, input.DispatchID),
		context,
		payload,
	))
}

// RecordDispatchInterrupted records a canonical dispatch interruption marker.
func (h *FactoryEventHistory) RecordDispatchInterrupted(input DispatchInterruptedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.DispatchID) == "" || input.ObservedStatus == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.DispatchInterruptedEventPayload{
		Reason:         strings.TrimSpace(input.Reason),
		ObservedStatus: input.ObservedStatus,
		InterruptedAt:  eventTime,
		RetryPlanned:   input.RetryPlanned,
	}
	if input.ProviderSessionRef != nil {
		providerSessionRef := *input.ProviderSessionRef
		payload.ProviderSessionRef = &providerSessionRef
	}
	if input.CheckpointRef != nil {
		checkpointRef := *input.CheckpointRef
		payload.CheckpointRef = &checkpointRef
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchInterrupted,
		fmt.Sprintf("%s/%d", eventIDDispatchInterruptedPrefix, sequence),
		context,
		payload,
	))
}

// RecordDispatchReconciled records a canonical dispatch reconciliation marker.
func (h *FactoryEventHistory) RecordDispatchReconciled(input DispatchReconciledInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.DispatchID) == "" || input.ReconciledStatus == "" || input.ReconciliationSource == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.DispatchReconciledEventPayload{
		ReconciledStatus:     input.ReconciledStatus,
		ReconciliationSource: input.ReconciliationSource,
		Replayed:             input.Replayed,
	}
	if input.Usage != nil {
		usage := *input.Usage
		payload.Usage = &usage
	}
	if input.ResultArtifactRef != nil {
		resultArtifactRef := *input.ResultArtifactRef
		payload.ResultArtifactRef = &resultArtifactRef
	}
	if len(input.ArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), input.ArtifactIDs...)
		payload.ArtifactIds = &artifactIDs
	}
	if input.FailureDetail != nil {
		failureDetail := *input.FailureDetail
		payload.FailureDetail = &failureDetail
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchReconciled,
		fmt.Sprintf("%s/%s", eventIDDispatchReconciledPrefix, input.DispatchID),
		context,
		payload,
	))
}

// RecordArtifactCreated records a canonical artifact creation marker.
func (h *FactoryEventHistory) RecordArtifactCreated(input ArtifactCreatedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Artifact.Id) == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.ArtifactCreatedEventPayload{
		Artifact: input.Artifact,
	}
	if input.CapturedAt != nil {
		capturedAt := canonicalDispatchLifecycleEventTime(*input.CapturedAt)
		payload.CapturedAt = &capturedAt
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeArtifactCreated,
		fmt.Sprintf("%s/%s", eventIDArtifactCreatedPrefix, input.Artifact.Id),
		context,
		payload,
	))
}

func (h *FactoryEventHistory) dispatchLifecycleContext(
	sessionID string,
	orchestratorKind factoryapi.FactoryOrchestratorKind,
	orchestratorDialect string,
	phaseID string,
	phaseName string,
	dispatchID string,
	source string,
	tick int,
	eventTime time.Time,
	sessionSequence int,
) factoryapi.FactoryEventContext {
	context := h.sessionLifecycleContext(sessionID, orchestratorKind, orchestratorDialect, source, tick, eventTime, sessionSequence)
	if phaseID := strings.TrimSpace(phaseID); phaseID != "" {
		context.PhaseId = &phaseID
	}
	if phaseName := strings.TrimSpace(phaseName); phaseName != "" {
		context.PhaseName = &phaseName
	}
	if dispatchID := strings.TrimSpace(dispatchID); dispatchID != "" {
		context.DispatchId = &dispatchID
	}
	return context
}

func dispatchQueuedEventModelProvider(input DispatchQueuedInput) *factoryapi.WorkerModelProvider {
	if provider := strings.TrimSpace(input.ModelProvider); provider != "" {
		return interfaces.GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProviderPtr(provider)
	}
	return interfaces.GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProviderPtr(input.RunnerID)
}

func canonicalDispatchLifecycleEventTime(eventTime time.Time) time.Time {
	if eventTime.IsZero() {
		return eventTime
	}
	return eventTime.UTC()
}

// ErrReconnectCursorNotFound reports that the reconnect cursor did not match
// any recorded event.
var ErrReconnectCursorNotFound = fmt.Errorf("reconnect cursor not found in event history")

// BuildReconnectReplay returns the historical events a client should apply after
// reconnecting from a last acknowledged event id or sequence. When durable
// dispatch state advanced beyond the acknowledged projection, synthetic
// DISPATCH_RECONCILED facts are appended with replayed=true.
func BuildReconnectReplay(
	events []factoryapi.FactoryEvent,
	cursor interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) ([]factoryapi.FactoryEvent, error) {
	if cursor.AfterEventID == "" && cursor.AfterSequence == nil {
		replay := make([]factoryapi.FactoryEvent, len(events))
		copy(replay, events)
		return replay, nil
	}

	ackIndex, err := findAcknowledgedEventIndex(events, cursor, scope)
	if err != nil {
		return nil, err
	}

	missed := events[ackIndex+1:]
	reconciled, err := synthesizeDispatchReconciliationEvents(events, ackIndex, missed, scope)
	if err != nil {
		return nil, err
	}
	if len(reconciled) == 0 {
		replay := make([]factoryapi.FactoryEvent, len(missed))
		copy(replay, missed)
		return replay, nil
	}

	replay := make([]factoryapi.FactoryEvent, 0, len(missed)+len(reconciled))
	replay = append(replay, missed...)
	replay = append(replay, reconciled...)
	return replay, nil
}

func findAcknowledgedEventIndex(
	events []factoryapi.FactoryEvent,
	cursor interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) (int, error) {
	if afterID := strings.TrimSpace(cursor.AfterEventID); afterID != "" {
		for index, event := range events {
			if event.Id == afterID {
				return index, nil
			}
		}
		return -1, fmt.Errorf("%w: after_event_id %q", ErrReconnectCursorNotFound, afterID)
	}
	if cursor.AfterSequence == nil {
		return -1, nil
	}
	ackSequence := *cursor.AfterSequence
	if scope.SessionID != "" {
		for index := len(events) - 1; index >= 0; index-- {
			event := events[index]
			if !eventBelongsToSession(event, scope.SessionID) {
				continue
			}
			if event.Context.SessionSequence != nil && *event.Context.SessionSequence == ackSequence {
				return index, nil
			}
		}
		return -1, fmt.Errorf("%w: after_sequence %d for session %q", ErrReconnectCursorNotFound, ackSequence, scope.SessionID)
	}
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Context.Sequence == ackSequence {
			return index, nil
		}
	}
	return -1, fmt.Errorf("%w: after_sequence %d", ErrReconnectCursorNotFound, ackSequence)
}

func eventBelongsToSession(event factoryapi.FactoryEvent, sessionID string) bool {
	if event.Context.SessionId == nil {
		return false
	}
	return strings.TrimSpace(*event.Context.SessionId) == strings.TrimSpace(sessionID)
}

func synthesizeDispatchReconciliationEvents(
	events []factoryapi.FactoryEvent,
	ackIndex int,
	missed []factoryapi.FactoryEvent,
	scope interfaces.FactoryEventReconnectScope,
) ([]factoryapi.FactoryEvent, error) {
	if ackIndex < 0 || len(events) == 0 {
		return nil, nil
	}
	ackEvents := events[:ackIndex+1]
	ackTick := maxEventTick(ackEvents)
	fullTick := maxEventTick(events)

	ackState, err := projections.ReconstructFactoryWorldState(ackEvents, ackTick)
	if err != nil {
		return nil, err
	}
	fullState, err := projections.ReconstructFactoryWorldState(events, fullTick)
	if err != nil {
		return nil, err
	}
	if fullState.JavaScriptRuntime == nil {
		return nil, nil
	}

	ackDispatches := dispatchStatesByID(ackState.JavaScriptRuntime.Dispatches)
	missedDispatchCoverage := dispatchLifecycleCoverage(missed)
	now := time.Now().UTC()
	synthetic := make([]factoryapi.FactoryEvent, 0)

	for _, dispatch := range fullState.JavaScriptRuntime.Dispatches {
		if scope.SessionID != "" && !dispatchBelongsToReconnectScope(dispatch, scope.SessionID, events) {
			continue
		}
		previous := ackDispatches[dispatch.ID]
		if !dispatchStatusAdvanced(previous, dispatch) {
			continue
		}
		if missedDispatchCoverage[dispatch.ID] {
			continue
		}
		synthetic = append(synthetic, syntheticDispatchReconciledEvent(dispatch, events, now))
	}
	return synthetic, nil
}

func dispatchBelongsToReconnectScope(
	dispatch interfaces.FactorySessionDispatchState,
	sessionID string,
	events []factoryapi.FactoryEvent,
) bool {
	for _, event := range events {
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatch.ID {
			continue
		}
		if eventBelongsToSession(event, sessionID) {
			return true
		}
	}
	return false
}

func dispatchStatesByID(dispatches []interfaces.FactorySessionDispatchState) map[string]interfaces.FactorySessionDispatchState {
	states := make(map[string]interfaces.FactorySessionDispatchState, len(dispatches))
	for _, dispatch := range dispatches {
		states[dispatch.ID] = dispatch
	}
	return states
}

func dispatchLifecycleCoverage(events []factoryapi.FactoryEvent) map[string]bool {
	covered := make(map[string]bool)
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchReconciled, factoryapi.FactoryEventTypeDispatchInterrupted:
			if event.Context.DispatchId != nil {
				covered[*event.Context.DispatchId] = true
			}
		}
	}
	return covered
}

func dispatchStatusAdvanced(previous interfaces.FactorySessionDispatchState, current interfaces.FactorySessionDispatchState) bool {
	if previous.ID == "" {
		return isTerminalDispatchStatus(current.Status)
	}
	if previous.Status == current.Status {
		return false
	}
	return isTerminalDispatchStatus(current.Status) || dispatchRank(current.Status) > dispatchRank(previous.Status)
}

func isTerminalDispatchStatus(status string) bool {
	switch factoryapi.FactoryDispatchStatus(strings.TrimSpace(status)) {
	case factoryapi.FactoryDispatchStatusCOMPLETED, factoryapi.FactoryDispatchStatusFAILED:
		return true
	default:
		return false
	}
}

func dispatchRank(status string) int {
	switch factoryapi.FactoryDispatchStatus(strings.TrimSpace(status)) {
	case factoryapi.FactoryDispatchStatusQUEUED:
		return 1
	case factoryapi.FactoryDispatchStatusRUNNING:
		return 2
	case factoryapi.FactoryDispatchStatusCOMPLETED, factoryapi.FactoryDispatchStatusFAILED:
		return 3
	default:
		return 0
	}
}

func syntheticDispatchReconciledEvent(
	dispatch interfaces.FactorySessionDispatchState,
	events []factoryapi.FactoryEvent,
	eventTime time.Time,
) factoryapi.FactoryEvent {
	sessionID, orchestratorKind, phaseID, phaseName, tick := dispatchReconnectContext(dispatch, events)
	reconciledStatus := factoryapi.FactoryDispatchStatus(strings.TrimSpace(dispatch.Status))
	if reconciledStatus == "" {
		reconciledStatus = factoryapi.FactoryDispatchStatusCOMPLETED
	}
	payload := factoryapi.DispatchReconciledEventPayload{
		ReconciledStatus:     reconciledStatus,
		ReconciliationSource: factoryapi.STREAMREPLAY,
		Replayed:             true,
	}
	if len(dispatch.ArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), dispatch.ArtifactIDs...)
		payload.ArtifactIds = &artifactIDs
	}
	if dispatch.Usage != nil {
		retryCount := int32(dispatch.Usage.RetryCount)
		usage := factoryapi.FactoryDispatchUsage{
			InputTokens:    int64Ptr(dispatch.Usage.InputTokens),
			OutputTokens:   int64Ptr(dispatch.Usage.OutputTokens),
			TotalTokens:    int64Ptr(dispatch.Usage.TotalTokens),
			CostUsd:        float64Ptr(dispatch.Usage.CostUSD),
			DurationMillis: int64Ptr(dispatch.Usage.DurationMillis),
			RetryCount:     &retryCount,
		}
		payload.Usage = &usage
	}
	if dispatch.FailureDetail != nil {
		payload.FailureDetail = &factoryapi.FactoryDispatchFailureDetail{
			Reason:     stringPtrIfNotEmpty(dispatch.FailureDetail.Reason),
			Message:    stringPtrIfNotEmpty(dispatch.FailureDetail.Message),
			ErrorClass: stringPtrIfNotEmpty(dispatch.FailureDetail.ErrorClass),
		}
	}
	source := "stream-reconnect"
	context := factoryapi.FactoryEventContext{
		Tick:             tick,
		EventTime:        eventTime,
		Sequence:         len(events),
		SessionId:        stringPtrIfNotEmpty(sessionID),
		OrchestratorKind: orchestratorKind,
		PhaseId:          stringPtrIfNotEmpty(phaseID),
		PhaseName:        stringPtrIfNotEmpty(phaseName),
		DispatchId:       stringPtrIfNotEmpty(dispatch.ID),
		Source:           &source,
	}
	return factoryEvent(
		factoryapi.FactoryEventTypeDispatchReconciled,
		fmt.Sprintf("%s/%s/reconnect", eventIDDispatchReconciledPrefix, dispatch.ID),
		context,
		payload,
	)
}

func dispatchReconnectContext(
	dispatch interfaces.FactorySessionDispatchState,
	events []factoryapi.FactoryEvent,
) (sessionID string, orchestratorKind *factoryapi.FactoryOrchestratorKind, phaseID, phaseName string, tick int) {
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatch.ID {
			continue
		}
		if event.Context.SessionId != nil {
			sessionID = *event.Context.SessionId
		}
		orchestratorKind = event.Context.OrchestratorKind
		if event.Context.PhaseId != nil {
			phaseID = *event.Context.PhaseId
		}
		if event.Context.PhaseName != nil {
			phaseName = *event.Context.PhaseName
		}
		tick = event.Context.Tick
		return sessionID, orchestratorKind, phaseID, phaseName, tick
	}
	return "", nil, "", "", 0
}

func float64Ptr(value float64) *float64 {
	return &value
}

func maxEventTick(events []factoryapi.FactoryEvent) int {
	tick := -1
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}
