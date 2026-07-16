package events

import (
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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
	OrchestratorKind    string
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	DispatchKind        interfaces.FactoryDispatchKind
	Label               string
	CoordinationRef     string
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
	OrchestratorKind    string
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	Reason              string
	ObservedStatus      interfaces.FactoryDispatchStatus
	RetryPlanned        bool
	ProviderSessionRef  *workerexecution.ProviderSessionMetadata
	CheckpointRef       *interfaces.FactorySessionJavaScriptCheckpointEventRef
}

// DispatchReconciledInput carries replay-safe facts for DISPATCH_RECONCILED.
type DispatchReconciledInput struct {
	SessionID            string
	OrchestratorKind     string
	OrchestratorDialect  string
	PhaseID              string
	PhaseName            string
	DispatchID           string
	Source               string
	Tick                 int
	ReconciledStatus     interfaces.FactoryDispatchStatus
	ReconciliationSource interfaces.DispatchReconciliationSource
	Replayed             bool
	Usage                *interfaces.FactoryDispatchUsage
	ResultArtifactRef    *interfaces.FactoryArtifactRef
	ArtifactIDs          []string
	FailureDetail        *workerexecution.FailureDetail
}

// ArtifactCreatedInput carries replay-safe facts for ARTIFACT_CREATED.
type ArtifactCreatedInput struct {
	SessionID           string
	OrchestratorKind    string
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	Artifact            interfaces.FactoryArtifact
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
	payload := interfaces.DispatchQueuedEventPayload{
		DispatchKind: input.DispatchKind,
	}
	if label := strings.TrimSpace(input.Label); label != "" {
		payload.Label = &label
	}
	if coordinationRef := strings.TrimSpace(input.CoordinationRef); coordinationRef != "" {
		payload.CoordinationRef = &coordinationRef
	}
	if runnerID := strings.TrimSpace(input.RunnerID); runnerID != "" {
		payload.RunnerID = &runnerID
	}
	if model := strings.TrimSpace(input.Model); model != "" {
		payload.Model = &model
	}
	if provider := strings.TrimSpace(input.Provider); provider != "" {
		payload.Provider = &provider
	}
	if parentDispatchID := strings.TrimSpace(input.ParentDispatchID); parentDispatchID != "" {
		payload.ParentDispatchID = &parentDispatchID
	}
	if retryOfDispatchID := strings.TrimSpace(input.RetryOfDispatchID); retryOfDispatchID != "" {
		payload.RetryOfDispatchID = &retryOfDispatchID
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
		payload.InputArtifactIDs = &artifactIDs
	}
	if len(input.InputWorkIDs) > 0 {
		workIDs := append([]string(nil), input.InputWorkIDs...)
		payload.InputWorkIDs = &workIDs
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeDispatchQueued,
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
	payload := interfaces.DispatchInterruptedEventPayload{
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
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeDispatchInterrupted,
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
	payload := interfaces.DispatchReconciledEventPayload{
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
		payload.ArtifactIDs = &artifactIDs
	}
	if input.FailureDetail != nil {
		failureDetail := *input.FailureDetail
		payload.FailureDetail = &failureDetail
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeDispatchReconciled,
		fmt.Sprintf("%s/%s", eventIDDispatchReconciledPrefix, input.DispatchID),
		context,
		payload,
	))
}

// RecordArtifactCreated records a canonical artifact creation marker.
func (h *FactoryEventHistory) RecordArtifactCreated(input ArtifactCreatedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Artifact.ID) == "" {
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
	payload := interfaces.ArtifactCreatedEventPayload{
		Artifact: input.Artifact,
	}
	if input.CapturedAt != nil {
		capturedAt := canonicalDispatchLifecycleEventTime(*input.CapturedAt)
		payload.CapturedAt = &capturedAt
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeArtifactCreated,
		fmt.Sprintf("%s/%s", eventIDArtifactCreatedPrefix, input.Artifact.ID),
		context,
		payload,
	))
}

func (h *FactoryEventHistory) dispatchLifecycleContext(
	sessionID string,
	orchestratorKind string,
	orchestratorDialect string,
	phaseID string,
	phaseName string,
	dispatchID string,
	source string,
	tick int,
	eventTime time.Time,
	sessionSequence int,
) interfaces.FactoryEventContext {
	context := h.domainSessionLifecycleContext(sessionID, orchestratorKind, orchestratorDialect, source, tick, eventTime, sessionSequence)
	if phaseID := strings.TrimSpace(phaseID); phaseID != "" {
		context.PhaseID = &phaseID
	}
	if phaseName := strings.TrimSpace(phaseName); phaseName != "" {
		context.PhaseName = &phaseName
	}
	if dispatchID := strings.TrimSpace(dispatchID); dispatchID != "" {
		context.DispatchID = &dispatchID
	}
	return context
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

// BuildCanonicalReconnectReplay returns the historical canonical events a
// domain consumer should apply after reconnecting from an acknowledged cursor.
func BuildCanonicalReconnectReplay(
	events []interfaces.FactoryEvent,
	cursor interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) ([]interfaces.FactoryEvent, error) {
	return buildDomainReconnectReplay(cloneFactoryEvents(events), cursor, scope)
}

func buildDomainReconnectReplay(
	events []interfaces.FactoryEvent,
	cursor interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) ([]interfaces.FactoryEvent, error) {
	if cursor.AfterEventID == "" && cursor.AfterSequence == nil {
		replay := make([]interfaces.FactoryEvent, len(events))
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
		replay := make([]interfaces.FactoryEvent, len(missed))
		copy(replay, missed)
		return replay, nil
	}

	replay := make([]interfaces.FactoryEvent, 0, len(missed)+len(reconciled))
	replay = append(replay, missed...)
	replay = append(replay, reconciled...)
	return replay, nil
}

func findAcknowledgedEventIndex(
	events []interfaces.FactoryEvent,
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
			sequence := event.Context.Sequence
			if event.Context.SessionSequence != nil {
				sequence = *event.Context.SessionSequence
			}
			if sequence == ackSequence {
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

func eventBelongsToSession(event interfaces.FactoryEvent, sessionID string) bool {
	if event.Context.SessionID == nil {
		return false
	}
	return strings.TrimSpace(*event.Context.SessionID) == strings.TrimSpace(sessionID)
}

func synthesizeDispatchReconciliationEvents(
	events []interfaces.FactoryEvent,
	ackIndex int,
	missed []interfaces.FactoryEvent,
	scope interfaces.FactoryEventReconnectScope,
) ([]interfaces.FactoryEvent, error) {
	if ackIndex < 0 || len(events) == 0 {
		return nil, nil
	}
	ackEvents := events[:ackIndex+1]
	ackTick := maxEventTick(ackEvents)
	fullTick := maxEventTick(events)

	ackState, err := projections.ReconstructCanonicalFactoryWorldState(ackEvents, ackTick)
	if err != nil {
		return nil, err
	}
	fullState, err := projections.ReconstructCanonicalFactoryWorldState(events, fullTick)
	if err != nil {
		return nil, err
	}
	if fullState.JavaScriptRuntime == nil {
		return nil, nil
	}

	ackDispatches := dispatchStatesByID(ackState.JavaScriptRuntime.Dispatches)
	missedDispatchCoverage := dispatchLifecycleCoverage(missed)
	now := time.Now().UTC()
	synthetic := make([]interfaces.FactoryEvent, 0)

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
	events []interfaces.FactoryEvent,
) bool {
	for _, event := range events {
		if event.Context.DispatchID == nil || *event.Context.DispatchID != dispatch.ID {
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

func dispatchLifecycleCoverage(events []interfaces.FactoryEvent) map[string]bool {
	covered := make(map[string]bool)
	for _, event := range events {
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchReconciled, interfaces.FactoryEventTypeDispatchInterrupted:
			if event.Context.DispatchID != nil {
				covered[*event.Context.DispatchID] = true
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
	switch interfaces.FactoryDispatchStatus(strings.TrimSpace(status)) {
	case interfaces.FactoryDispatchStatusCompleted, interfaces.FactoryDispatchStatusFailed:
		return true
	default:
		return false
	}
}

func dispatchRank(status string) int {
	switch interfaces.FactoryDispatchStatus(strings.TrimSpace(status)) {
	case interfaces.FactoryDispatchStatusQueued:
		return 1
	case interfaces.FactoryDispatchStatusRunning:
		return 2
	case interfaces.FactoryDispatchStatusCompleted, interfaces.FactoryDispatchStatusFailed:
		return 3
	default:
		return 0
	}
}

func syntheticDispatchReconciledEvent(
	dispatch interfaces.FactorySessionDispatchState,
	events []interfaces.FactoryEvent,
	eventTime time.Time,
) interfaces.FactoryEvent {
	sessionID, orchestratorKind, phaseID, phaseName, tick := dispatchReconnectContext(dispatch, events)
	reconciledStatus := interfaces.FactoryDispatchStatus(strings.TrimSpace(dispatch.Status))
	if reconciledStatus == "" {
		reconciledStatus = interfaces.FactoryDispatchStatusCompleted
	}
	payload := interfaces.DispatchReconciledEventPayload{
		ReconciledStatus:     reconciledStatus,
		ReconciliationSource: interfaces.DispatchReconciliationSourceStreamReplay,
		Replayed:             true,
	}
	if len(dispatch.ArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), dispatch.ArtifactIDs...)
		payload.ArtifactIDs = &artifactIDs
	}
	if dispatch.Usage != nil {
		retryCount := int32(dispatch.Usage.RetryCount)
		usage := interfaces.FactoryDispatchUsage{
			InputTokens:    int64Ptr(dispatch.Usage.InputTokens),
			OutputTokens:   int64Ptr(dispatch.Usage.OutputTokens),
			TotalTokens:    int64Ptr(dispatch.Usage.TotalTokens),
			CostUSD:        float64Ptr(dispatch.Usage.CostUSD),
			DurationMillis: int64Ptr(dispatch.Usage.DurationMillis),
			RetryCount:     &retryCount,
		}
		payload.Usage = &usage
	}
	if dispatch.FailureDetail != nil {
		payload.FailureDetail = &workerexecution.FailureDetail{
			Reason:  workerexecution.WorkFailureType(dispatch.FailureDetail.Reason),
			Message: dispatch.FailureDetail.Message,
		}
	}
	source := "stream-reconnect"
	context := interfaces.FactoryEventContext{
		Tick:             tick,
		EventTime:        eventTime,
		Sequence:         len(events),
		SessionID:        stringPtrIfNotEmpty(sessionID),
		OrchestratorKind: orchestratorKind,
		PhaseID:          stringPtrIfNotEmpty(phaseID),
		PhaseName:        stringPtrIfNotEmpty(phaseName),
		DispatchID:       stringPtrIfNotEmpty(dispatch.ID),
		Source:           &source,
	}
	return domainFactoryEvent(
		interfaces.FactoryEventTypeDispatchReconciled,
		fmt.Sprintf("%s/%s/reconnect", eventIDDispatchReconciledPrefix, dispatch.ID),
		context,
		payload,
	)
}

func dispatchReconnectContext(
	dispatch interfaces.FactorySessionDispatchState,
	events []interfaces.FactoryEvent,
) (sessionID string, orchestratorKind *string, phaseID, phaseName string, tick int) {
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Context.DispatchID == nil || *event.Context.DispatchID != dispatch.ID {
			continue
		}
		if event.Context.SessionID != nil {
			sessionID = *event.Context.SessionID
		}
		orchestratorKind = event.Context.OrchestratorKind
		if event.Context.PhaseID != nil {
			phaseID = *event.Context.PhaseID
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

func maxEventTick(events []interfaces.FactoryEvent) int {
	tick := -1
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}
