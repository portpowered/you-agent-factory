package events

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	eventsnapshot "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events/snapshot"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TODO: we should move these constants to the interfaces package, actually we should move the events generally to the openapi.yaml to allow generation of the various types of events payloads.
// we should declare all events as schemas, and derive the structures internally from said events.
// record/replay should record events in the format of the schemas defined in the api package.
// the API should respond with the serialized json payloads of those openapi.yaml based schemas.
const (
	eventIDRunRequest                       = "factory-event/run-started"
	eventIDRunResponse                      = RunFinishedFactoryEventID
	eventIDInitialStructure                 = "factory-event/initial-structure/0"
	eventIDFactoryChangePrefix              = "factory-event/factory-change"
	eventIDWorkRequestPrefix                = "factory-event/work-request"
	eventIDRelationshipPrefix               = "factory-event/relationship-change"
	eventIDDispatchCreatedPrefix            = "factory-event/dispatch-created"
	eventIDDispatchCompletedPrefix          = "factory-event/dispatch-completed"
	eventIDHumanApprovalRequestedPrefix     = "factory-event/human-approval-requested"
	eventIDDispatchWorkerSessionAssocPrefix = "factory-event/dispatch-worker-session-association"
	eventIDStateChangePrefix                = "factory-event/factory-state-change"
	eventIDWorkStateChangePrefix            = "factory-event/work-state-change"
	failureReasonWorkerError                = "worker_error"
	failureReasonUnknown                    = "workstation_failed"
	failureMessageUnavailable               = "Workstation failed without a reported error message."
	eventHistoryStreamBufferSize            = 64
	eventHistoryCloseDrainTimeout           = time.Second
)

type eventHistorySubscription struct {
	events         chan interfaces.FactoryEvent
	inbox          chan interfaces.FactoryEvent
	done           <-chan struct{}
	overflow       chan struct{}
	overflowOnce   sync.Once
	terminal       chan struct{}
	terminalOnce   sync.Once
	drained        chan struct{}
	dispatchID     string
	limit          int
	pendingMu      sync.Mutex
	pending        int
	terminalClosed bool
}

func (subscription *eventHistorySubscription) signalOverflow() {
	subscription.overflowOnce.Do(func() {
		close(subscription.overflow)
	})
}

func (subscription *eventHistorySubscription) signalTerminal() {
	subscription.pendingMu.Lock()
	subscription.terminalClosed = true
	subscription.pendingMu.Unlock()
	subscription.terminalOnce.Do(func() {
		close(subscription.terminal)
	})
}

// CloseLiveSubscriptions ends active live subscriptions without appending new
// canonical events. Callers invoke this after terminal lifecycle events are
// recorded so SSE clients observe the final timeline and then a closed stream.
// It waits until each subscription has handed off all events accepted before
// the terminal signal, while canceled subscribers are allowed to exit early.
// A non-cooperative subscriber is released through overflow after one shared
// bounded drain deadline so teardown cannot block forever.
func (h *FactoryEventHistory) CloseLiveSubscriptions() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.liveClosed = true
	streams := make([]*eventHistorySubscription, 0, len(h.streams))
	for _, subscription := range h.streams {
		streams = append(streams, subscription)
		subscription.signalTerminal()
	}
	h.mu.Unlock()
	if len(streams) == 0 {
		return
	}
	drainDeadline := time.NewTimer(eventHistoryCloseDrainTimeout)
	defer drainDeadline.Stop()
	for index, subscription := range streams {
		select {
		case <-subscription.drained:
		case <-drainDeadline.C:
			for _, pending := range streams[index:] {
				pending.signalOverflow()
			}
			return
		}
	}
}

// FactoryEventHistory stores the current-process canonical event history.
// It is intentionally in-memory and unbounded for the event-stream MVP.
type FactoryEventHistory struct {
	mu                                 sync.RWMutex
	initialStructure                   interfaces.InitialStructurePayload
	runtimeConfig                      interfaces.RuntimeDefinitionLookup
	factoryRunner                      string
	initialFactory                     *interfaces.FactorySnapshot
	initialSecretProvenance            []recordings.RecordingSecret
	now                                func() time.Time
	streamGenerationID                 string
	events                             []interfaces.FactoryEvent
	secretProvenance                   map[string][]recordings.RecordingSecret
	sessionProjection                  *projections.IncrementalSessionProjection
	sessionProjectionErr               error
	recorders                          []func(interfaces.FactoryEvent)
	eventTypeRecorders                 []func(interfaces.FactoryEventType)
	deferredSessionCompletionRecorders []func()
	deferredSessionCompletionPending   bool
	deferredSessionCompletionPublished bool
	nextID                             int
	streams                            map[int]*eventHistorySubscription
	runRecordedAt                      time.Time
	hasRunRequest                      bool
	hasRunResponse                     bool
	hasInitialStructure                bool
	sessionStartedAt                   time.Time
	hasSessionStarted                  bool
	hasSessionCompleted                bool
	liveClosed                         bool
	sessionID                          string
	nextSessionSequence                int
	canonicalEventsCalls               atomic.Uint64
	canonicalEventsCopied              atomic.Uint64
	fullHistoryReductions              atomic.Uint64
	runtimeReadRecorder                recordings.RuntimeReadMetricsRecorder
	durabilityReader                   recordings.CompletedFlushWatermarkReader
}

// NewFactoryEventHistory creates an in-memory factory event history for one
// process lifetime and records no events until RecordInitialStructure is called.
func NewFactoryEventHistory(topology recordings.InitialStructureSource, now func() time.Time, streamGenerationID string, runtimeConfigs ...interfaces.RuntimeDefinitionLookup) *FactoryEventHistory {
	streamGenerationID = strings.TrimSpace(streamGenerationID)
	if now == nil || streamGenerationID == "" {
		return nil
	}
	runtimeConfig := interfaces.FirstRuntimeDefinitionLookup(runtimeConfigs...)
	var initialStructure interfaces.InitialStructurePayload
	if topology != nil {
		initialStructure = topology.RecordingInitialStructure(runtimeConfig)
	}
	return &FactoryEventHistory{
		initialStructure:   initialStructure,
		runtimeConfig:      runtimeConfig,
		now:                now,
		streamGenerationID: streamGenerationID,
		sessionProjection:  projections.NewIncrementalSessionProjection(),
		secretProvenance:   make(map[string][]recordings.RecordingSecret),
		streams:            make(map[int]*eventHistorySubscription),
	}
}

// StreamGenerationID returns the stable opaque identifier for this live event
// history instance.
func (h *FactoryEventHistory) StreamGenerationID() string {
	if h == nil {
		return ""
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.streamGenerationID
}

// SetFactoryRunnerOverride preserves the effective factory-level runner
// selection when service wiring overrides the authored runtime config.
func (h *FactoryEventHistory) SetFactoryRunnerOverride(runnerID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.factoryRunner = workers.NormalizeRunnerID(runnerID)
}

// CanonicalEvents returns detached Factory-owned events in append order.
func (h *FactoryEventHistory) CanonicalEvents() []interfaces.FactoryEvent {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	h.canonicalEventsCalls.Add(1)
	h.canonicalEventsCopied.Add(uint64(len(h.events)))
	return cloneFactoryEvents(h.events)
}

// Subscribe returns a replay snapshot followed by live canonical events.
func (h *FactoryEventHistory) Subscribe(
	ctx context.Context,
	reconnect *interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) (interfaces.FactoryEventStream, error) {
	if h == nil {
		ch := make(chan interfaces.FactoryEvent)
		close(ch)
		return interfaces.FactoryEventStream{Events: ch}, nil
	}

	h.mu.Lock()
	events := cloneFactoryEventsForStream(h.events, scope)
	streamGenerationID := h.streamGenerationID
	if reconnect != nil {
		replayed, err := buildDomainReconnectReplay(cloneFactoryEvents(h.events), *reconnect, scope)
		if err != nil {
			h.mu.Unlock()
			return interfaces.FactoryEventStream{}, err
		}
		events = cloneFactoryEventsForStream(replayed, scope)
	}
	if h.liveClosed {
		closed := make(chan interfaces.FactoryEvent)
		close(closed)
		h.mu.Unlock()
		return interfaces.FactoryEventStream{
			StreamGenerationID: streamGenerationID,
			History:            events,
			Events:             closed,
		}, nil
	}
	bufferSize := scope.Limit
	if bufferSize <= 0 {
		bufferSize = eventHistoryStreamBufferSize
	}
	id := h.nextID
	h.nextID++
	subscription := &eventHistorySubscription{
		events:     make(chan interfaces.FactoryEvent),
		inbox:      make(chan interfaces.FactoryEvent, bufferSize),
		done:       ctx.Done(),
		overflow:   make(chan struct{}),
		terminal:   make(chan struct{}),
		drained:    make(chan struct{}),
		dispatchID: strings.TrimSpace(scope.DispatchID),
		limit:      bufferSize,
	}
	h.streams[id] = subscription
	h.mu.Unlock()

	go h.relayLiveSubscription(id, subscription)

	return interfaces.FactoryEventStream{
		StreamGenerationID: streamGenerationID,
		History:            events,
		Events:             subscription.events,
	}, nil
}

// AddEventRecorder registers a callback invoked for every future canonical
// Factory event append. Existing events are replayed to the callback first so
// late recorder setup still sees a complete current-process history.
func (h *FactoryEventHistory) AddEventRecorder(recorder func(interfaces.FactoryEvent)) {
	if h == nil || recorder == nil {
		return
	}

	h.mu.Lock()
	events := cloneFactoryEvents(h.events)
	h.recorders = append(h.recorders, recorder)
	// Replay under the history lock so a newly registered recorder observes
	// the existing prefix before any later append can be delivered to it.
	for _, event := range events {
		recorder(event)
	}
	h.mu.Unlock()
}

// AddEventTypeRecorder registers a transport-independent callback for the type
// of every future canonical Factory event. Existing event types are replayed
// first so late Factory Session lifecycle bindings observe terminal history.
func (h *FactoryEventHistory) AddEventTypeRecorder(recorder func(interfaces.FactoryEventType)) {
	if h == nil || recorder == nil {
		return
	}

	h.mu.Lock()
	eventTypes := make([]interfaces.FactoryEventType, len(h.events))
	for index, event := range h.events {
		eventTypes[index] = event.Type
	}
	h.eventTypeRecorders = append(h.eventTypeRecorders, recorder)
	// Keep the replayed prefix and future type notifications in append order.
	for _, eventType := range eventTypes {
		recorder(eventType)
	}
	h.mu.Unlock()
}

// RecordInitialStructure records the static topology before work events.
func (h *FactoryEventHistory) RecordInitialStructure() {
	if h == nil {
		return
	}
	eventTime := interfaces.CanonicalEventTime(h.now())
	payload := h.initialStructure
	factory := eventsnapshot.FromInitialStructure(payload)
	h.mu.Lock()
	if h.hasInitialStructure {
		h.mu.Unlock()
		return
	}
	h.hasInitialStructure = true
	if h.initialFactory != nil {
		factory = h.initialFactory.Clone()
	}
	provenance := append([]recordings.RecordingSecret(nil), h.initialSecretProvenance...)
	h.mu.Unlock()
	h.appendEventWithProvenance(domainFactoryEvent(
		interfaces.FactoryEventTypeInitialStructureRequest,
		eventIDInitialStructure,
		interfaces.FactoryEventContext{Tick: 0, EventTime: eventTime},
		interfaces.InitialStructureRequestEventPayload{Factory: factory},
	), provenance)
}

// RecordFactoryChange records a canonical topology replacement event after a
// live running factory definition change becomes active.
func (h *FactoryEventHistory) RecordFactoryChange(tick int, payload interfaces.FactoryChangeEventPayload, eventTime time.Time) {
	if h == nil {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeFactoryChange,
		fmt.Sprintf("%s/%d", eventIDFactoryChangePrefix, tick),
		interfaces.FactoryEventContext{Tick: tick, EventTime: eventTime},
		payload,
	))
}

// RecordRunRequest records the canonical run request event before the runtime
// begins streaming structure or work lifecycle events.
func (h *FactoryEventHistory) RecordRunRequest() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.hasRunRequest {
		h.mu.Unlock()
		return
	}
	recordedAt := interfaces.CanonicalEventTime(h.now())
	h.runRecordedAt = recordedAt
	h.hasRunRequest = true
	payload := h.initialStructure
	factory := eventsnapshot.FromInitialStructure(payload)
	if h.initialFactory != nil {
		factory = h.initialFactory.Clone()
	}
	provenance := append([]recordings.RecordingSecret(nil), h.initialSecretProvenance...)
	h.mu.Unlock()
	h.appendEventWithProvenance(domainFactoryEvent(
		interfaces.FactoryEventTypeRunRequest,
		eventIDRunRequest,
		interfaces.FactoryEventContext{Tick: 0, EventTime: recordedAt},
		interfaces.RunRequestEventPayload{
			RecordedAt: recordedAt,
			Factory:    factory,
		},
	), provenance)
}

// RecordWorkInput records a submitted work token after submit-time identity
// generation has completed.
func (h *FactoryEventHistory) RecordWorkInput(tick int, req work.SubmitRequest, token workers.Token, eventTime time.Time) {
	if h == nil || token.ID == "" {
		return
	}
}

// RecordWorkRequest records the batch-level request before its work items are
// exposed as individual work input events.
func (h *FactoryEventHistory) RecordWorkRequest(tick int, record work.WorkRequestRecord, eventTime time.Time) {
	if h == nil || record.RequestID == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	context := interfaces.FactoryEventContext{
		Tick:      tick,
		EventTime: eventTime,
		RequestID: stringPtr(record.RequestID),
		TraceIDs:  stringSlicePtr(work.CanonicalChainingTraceIDs([]string{record.TraceID})),
		WorkIDs:   stringSlicePtr(workItemIDs(record.WorkItems)),
		Source:    stringPtrIfNotEmpty(record.Source),
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeWorkRequest,
		fmt.Sprintf("%s/%s", eventIDWorkRequestPrefix, record.RequestID),
		h.sessionScopedContext(context),
		work.WorkRequestEventPayload{
			Type:          record.Type,
			Works:         requestEventWorks(record.WorkItems),
			Relations:     eventRelations(record.Relations),
			Source:        record.Source,
			ParentLineage: append([]string(nil), record.ParentLineage...),
		},
	))
	for i, relation := range record.Relations {
		h.RecordRelationshipChange(tick, record.RequestID, record.TraceID, i, relation, eventTime)
	}
}

// RecordRelationshipChange records one relation created by a request batch.
func (h *FactoryEventHistory) RecordRelationshipChange(tick int, requestID string, traceID string, index int, relation work.FactoryRelation, eventTime time.Time) {
	if h == nil || relation.Type == "" || relation.TargetWorkID == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	if relation.RequestID == "" {
		relation.RequestID = requestID
	}
	if relation.TraceID == "" {
		relation.TraceID = traceID
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeRelationshipChangeRequest,
		fmt.Sprintf("%s/%s/%d", eventIDRelationshipPrefix, requestID, index),
		h.sessionScopedContext(interfaces.FactoryEventContext{
			Tick:      tick,
			EventTime: eventTime,
			RequestID: stringPtrIfNotEmpty(requestID),
			TraceIDs:  stringSlicePtr(work.CanonicalChainingTraceIDs([]string{traceID, relation.TraceID})),
			WorkIDs:   stringSlicePtr(uniqueStrings([]string{relation.SourceWorkID, relation.TargetWorkID})),
		}),
		work.RelationshipChangeRequestEventPayload{Relation: eventRelation(relation)},
	))
}

// RecordWorkstationRequest records a dispatch at the tick it consumed inputs.
func (h *FactoryEventHistory) RecordWorkstationRequest(tick int, record interfaces.FactoryDispatchRecord, eventTime time.Time) {
	dispatchID := record.Dispatch.DispatchID
	if h == nil || dispatchID == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	inputTokens := workers.WorkDispatchInputTokens(record.Dispatch)
	runnerSelection := h.resolvedRunnerSelectionForDispatch(record.Dispatch)
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeDispatchRequest,
		fmt.Sprintf("%s/%s", eventIDDispatchCreatedPrefix, dispatchID),
		h.sessionScopedContext(interfaces.FactoryEventContext{
			Tick:                     tick,
			EventTime:                eventTime,
			DispatchID:               stringPtr(dispatchID),
			RequestID:                stringPtrIfNotEmpty(record.Dispatch.Execution.RequestID),
			TraceIDs:                 stringSlicePtr(traceIDsFromTokens(inputTokens)),
			WorkIDs:                  stringSlicePtr(workIDsFromTokens(inputTokens)),
			CurrentChainingTraceID:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIDs: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
		}),
		interfaces.DispatchRequestEventPayload{
			TransitionID:             record.Dispatch.TransitionID,
			ExpectedArtifactContext:  cloneExpectedArtifactTemplateContext(record.Dispatch.ExpectedArtifactContext),
			CurrentChainingTraceID:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIDs: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
			Inputs:                   dispatchConsumedWorkRefsFromTokens(inputTokens),
			Resources:                h.dispatchResourcesPtr(inputTokens),
			Metadata:                 dispatchRequestEventMetadataPtr(record.Dispatch.Execution.ReplayKey, runnerSelection),
		},
	))
}

// RecordHumanApprovalRequested records the durable operator-input boundary
// immediately after its matching DISPATCH_REQUEST. It carries no mutable Work
// content or display copy; replay resolves those facts from the topology and
// the event context.
func (h *FactoryEventHistory) RecordHumanApprovalRequested(tick int, record interfaces.FactoryDispatchRecord, eventTime time.Time) {
	if h == nil || !record.HumanApproval || record.Dispatch.DispatchID == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	inputTokens := workers.WorkDispatchInputTokens(record.Dispatch)
	approvalID := "approval-" + record.Dispatch.DispatchID
	workstationID := humanApprovalWorkstationID(h.runtimeConfig, record.Dispatch)
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeHumanApprovalRequested,
		fmt.Sprintf("%s/%s", eventIDHumanApprovalRequestedPrefix, approvalID),
		h.sessionScopedContext(interfaces.FactoryEventContext{
			Tick:                     tick,
			EventTime:                eventTime,
			DispatchID:               stringPtr(record.Dispatch.DispatchID),
			RequestID:                stringPtrIfNotEmpty(record.Dispatch.Execution.RequestID),
			TraceIDs:                 stringSlicePtr(traceIDsFromTokens(inputTokens)),
			WorkIDs:                  stringSlicePtr(workIDsFromTokens(inputTokens)),
			CurrentChainingTraceID:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIDs: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
		}),
		interfaces.HumanApprovalRequestedEventPayload{
			ApprovalID:    approvalID,
			WorkstationID: workstationID,
			Decisions: []interfaces.HumanApprovalDecision{
				interfaces.HumanApprovalDecisionApprove,
				interfaces.HumanApprovalDecisionReject,
			},
			Status: interfaces.HumanApprovalStatusPending,
		},
	))
}

func humanApprovalWorkstationID(
	runtimeConfig interfaces.RuntimeDefinitionLookup,
	dispatch work.WorkDispatch,
) string {
	workstationID := strings.TrimSpace(dispatch.TransitionID)
	if workstationID == "" {
		workstationID = strings.TrimSpace(dispatch.WorkstationName)
	}
	if runtimeConfig == nil {
		return workstationID
	}
	for _, name := range []string{dispatch.WorkstationName, dispatch.TransitionID} {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		workstation, ok := runtimeConfig.Workstation(name)
		if !ok || workstation == nil {
			continue
		}
		return interfaces.CanonicalFactoryGraphWorkstationID(*workstation)
	}
	return workstationID
}

// RecordDispatchWorkerSessionAssociation records the canonical, stable
// dispatch-to-Worker-Session identity association. Callers must commit this
// record before any event that depends on it (the Worker Session's own
// opening or output records) can be observed.
func (h *FactoryEventHistory) RecordDispatchWorkerSessionAssociation(tick int, dispatchID string, workerSessionID string, requestID string, eventTime time.Time) {
	h.RecordDispatchWorkerSessionAssociationWithExecution(
		tick,
		dispatchID,
		workerSessionID,
		requestID,
		recordings.DispatchWorkerSessionExecutionFacts{},
		eventTime,
	)
}

// RecordDispatchWorkerSessionAssociationWithExecution retains the resolved
// execution facts alongside the internal association. The public Factory
// Event mapper decodes only workerSessionId, while Runtime replay reads the
// additional fields directly from the canonical payload.
func (h *FactoryEventHistory) RecordDispatchWorkerSessionAssociationWithExecution(
	tick int,
	dispatchID string,
	workerSessionID string,
	requestID string,
	facts recordings.DispatchWorkerSessionExecutionFacts,
	eventTime time.Time,
) {
	if h == nil || dispatchID == "" || workerSessionID == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		fmt.Sprintf("%s/%s", eventIDDispatchWorkerSessionAssocPrefix, dispatchID),
		h.sessionScopedContext(interfaces.FactoryEventContext{
			Tick:       tick,
			EventTime:  eventTime,
			DispatchID: stringPtr(dispatchID),
			RequestID:  stringPtrIfNotEmpty(requestID),
		}),
		dispatchWorkerSessionAssociationEventPayload{
			WorkerSessionID: workerSessionID,
			Model:           strings.TrimSpace(facts.Model),
			ReasoningEffort: strings.TrimSpace(facts.ReasoningEffort),
		},
	))
}

// dispatchWorkerSessionAssociationEventPayload is intentionally private: the
// resolved execution facts are replay metadata for the Worker Session read
// projection, not additions to the public Factory Event response contract.
type dispatchWorkerSessionAssociationEventPayload struct {
	WorkerSessionID string `json:"workerSessionId"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

// RecordWorkstationResponse records a completed dispatch and its outputs.
func (h *FactoryEventHistory) RecordWorkstationResponse(tick int, result workers.WorkResult, completed interfaces.CompletedDispatch) {
	if h == nil || result.DispatchID == "" {
		return
	}
	eventTime := completed.EndTime
	if eventTime.IsZero() {
		eventTime = h.now()
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	failureReason, failureMessage := failureDetailsForResult(result)
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeDispatchResponse,
		fmt.Sprintf("%s/%s", eventIDDispatchCompletedPrefix, result.DispatchID),
		h.sessionScopedContext(interfaces.FactoryEventContext{
			Tick:                     tick,
			EventTime:                eventTime,
			DispatchID:               stringPtr(result.DispatchID),
			TraceIDs:                 stringSlicePtr(traceIDsFromTokens(completed.ConsumedTokens)),
			WorkIDs:                  stringSlicePtr(workIDsFromTokens(completed.ConsumedTokens)),
			CurrentChainingTraceID:   stringPtrIfNotEmpty(workers.CurrentChainingTraceID(completed.ConsumedTokens, interfaces.SystemTimeWorkTypeID)),
			PreviousChainingTraceIDs: stringSlicePtr(workers.PreviousChainingTraceIDs(completed.ConsumedTokens)),
		}),
		workers.DispatchResponseEventPayload{
			TransitionID:                result.TransitionID,
			Cancellation:                result.Cancellation.Clone(),
			CurrentChainingTraceID:      stringPtrIfNotEmpty(workers.CurrentChainingTraceID(completed.ConsumedTokens, interfaces.SystemTimeWorkTypeID)),
			PreviousChainingTraceIDs:    stringSlicePtr(workers.PreviousChainingTraceIDs(completed.ConsumedTokens)),
			Outcome:                     result.Outcome,
			Output:                      stringPtrIfNotEmpty(result.Output),
			StructuredResult:            jsonvalue.Clone(result.StructuredResult),
			StructuredResultPresent:     jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent),
			Error:                       stringPtrIfNotEmpty(result.Error),
			Feedback:                    stringPtrIfNotEmpty(result.Feedback),
			SelectedClassificationLabel: stringPtrIfNotEmpty(result.SelectedClassificationLabel),
			ArtifactVerification:        result.ArtifactVerification.Clone(),
			FailureDetail:               failureDetail(failureReason, failureMessage),
			DurationMillis:              int64Ptr(completed.Duration.Milliseconds()),
			OutputWork:                  eventWorksPtr(outputWorkItems(completed.OutputMutations, completed.ConsumedTokens)),
			OutputResources:             h.dispatchOutputResourcesPtr(completed.OutputMutations),
			ProviderFailure:             workers.CloneWorkFailureMetadata(result.FailureMetadata),
			Usage:                       dispatchUsageEventPayload(result, completed),
		},
	))
}

// dispatchUsageEventPayload derives the usage facts that belong on the
// existing Petri DISPATCH_RESPONSE. Duration comes from the completed
// dispatch; token facts come only from normalized provider response metadata.
func dispatchUsageEventPayload(
	result workers.WorkResult,
	completed interfaces.CompletedDispatch,
) *workers.DispatchUsageEventPayload {
	durationMillis := completed.Duration.Milliseconds()
	usage := &workers.DispatchUsageEventPayload{DurationMillis: &durationMillis}
	inputTokens, hasInput := providerResponseToken(result.Diagnostics, workers.ProviderResponseMetadataInputTokens)
	outputTokens, hasOutput := providerResponseToken(result.Diagnostics, workers.ProviderResponseMetadataOutputTokens)
	if hasInput {
		usage.InputTokens = &inputTokens
	}
	if hasOutput {
		usage.OutputTokens = &outputTokens
	}
	if hasInput && hasOutput && inputTokens <= math.MaxInt64-outputTokens {
		totalTokens := inputTokens + outputTokens
		usage.TotalTokens = &totalTokens
	}
	return usage
}

func providerResponseToken(diagnostics *workers.WorkDiagnostics, key string) (int64, bool) {
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata == nil {
		return 0, false
	}
	value := strings.TrimSpace(diagnostics.Provider.ResponseMetadata[key])
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

// RecordRunResponse records the canonical run completion event after the
// runtime has reached a terminal state.
func (h *FactoryEventHistory) RecordRunResponse(tick int, state interfaces.FactoryState, reason string, eventTime time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.hasRunResponse {
		h.mu.Unlock()
		return
	}
	recordedAt := h.runRecordedAt
	if recordedAt.IsZero() {
		recordedAt = eventTime
		h.runRecordedAt = recordedAt
	}
	recordedAt = interfaces.CanonicalEventTime(recordedAt)
	eventTime = interfaces.CanonicalEventTime(eventTime)
	h.hasRunResponse = true
	h.mu.Unlock()

	stateValue := state
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeRunResponse,
		eventIDRunResponse,
		interfaces.FactoryEventContext{Tick: tick, EventTime: eventTime},
		interfaces.RunResponseEventPayload{
			State:  &stateValue,
			Reason: stringPtrIfNotEmpty(reason),
			WallClock: &interfaces.RunEventWallClock{
				StartedAt:  timePtrIfNotZero(recordedAt),
				FinishedAt: timePtrIfNotZero(eventTime),
			},
		},
	))
}

func domainFactoryEvent(eventType interfaces.FactoryEventType, id string, context interfaces.FactoryEventContext, payload any) interfaces.FactoryEvent {
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("encode factory event payload %T: %v", payload, err))
	}
	return interfaces.FactoryEvent{
		Type:    eventType,
		Id:      id,
		Context: context,
		Payload: encoded,
	}
}

func (h *FactoryEventHistory) resolvedRunnerSelectionForDispatch(dispatch work.WorkDispatch) workers.ResolvedRunnerSelection {
	if h == nil {
		return workers.ResolvedRunnerSelection{}
	}
	workstationRunner, workerModelProvider := h.runnerSelectionInputsForDispatch(dispatch)
	factoryRunner := h.factoryRunnerID()
	return workers.ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider)
}

func (h *FactoryEventHistory) runnerSelectionInputsForDispatch(dispatch work.WorkDispatch) (string, string) {
	if h == nil || h.runtimeConfig == nil {
		return "", ""
	}
	workstationName := strings.TrimSpace(dispatch.WorkstationName)
	if workstationName == "" {
		for _, workstation := range h.initialStructure.Workstations {
			if workstation.ID == dispatch.TransitionID {
				workstationName = strings.TrimSpace(workstation.Name)
				break
			}
		}
	}
	var workstationRunner string
	if workstationName != "" {
		if workstation, ok := h.runtimeConfig.Workstation(workstationName); ok && workstation != nil {
			workstationRunner = workstation.Runner
		}
	}
	worker, ok := h.runtimeConfig.Worker(dispatch.WorkerType)
	if !ok || worker == nil {
		return workstationRunner, ""
	}
	return workstationRunner, worker.ModelProvider
}

func (h *FactoryEventHistory) factoryRunnerID() string {
	if h == nil || h.runtimeConfig == nil {
		if h == nil {
			return ""
		}
		h.mu.RLock()
		defer h.mu.RUnlock()
		return h.factoryRunner
	}
	h.mu.RLock()
	override := h.factoryRunner
	h.mu.RUnlock()
	if override != "" {
		return override
	}
	provider, ok := h.runtimeConfig.(interfaces.RuntimeFactoryConfigLookup)
	if !ok {
		return ""
	}
	cfg := provider.FactoryConfig()
	if cfg == nil {
		return ""
	}
	return cfg.Runner
}

func traceIDsFromTokens(tokens []workers.Token) []string {
	return workers.PreviousChainingTraceIDs(tokens)
}

func workIDsFromTokens(tokens []workers.Token) []string {
	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == workers.DataTypeResource {
			continue
		}
		values = append(values, token.Color.WorkID)
	}
	return uniqueStrings(values)
}

func workItemIDs(items []work.FactoryWorkItem) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.ID)
	}
	return uniqueStrings(values)
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func timePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = interfaces.CanonicalEventTime(value)
	return &value
}

func stringSlicePtr(values []string) *[]string {
	return slicePtr(values)
}

func slicePtr[T any](values []T) *[]T {
	if len(values) == 0 {
		return nil
	}
	out := make([]T, len(values))
	copy(out, values)
	return &out
}

func workItemFromToken(token workers.Token) work.FactoryWorkItem {
	currentChainingTraceID := token.Color.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = token.Color.TraceID
	}
	return work.FactoryWorkItem{
		ID:                       token.Color.WorkID,
		WorkTypeID:               token.Color.WorkTypeID,
		DisplayName:              token.Color.Name,
		ChainingTraceDepth:       token.Color.ChainingTraceDepth,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), token.Color.PreviousChainingTraceIDs...),
		TraceID:                  token.Color.TraceID,
		Content:                  append([]work.WorkContentPart(nil), token.Color.Content...),
		ParentID:                 token.Color.ParentID,
		State:                    token.State,
		StructuredResult:         jsonvalue.Clone(token.Color.StructuredResult),
		Tags:                     cloneStringMap(token.Color.Tags),
		StructuredResultPresent:  jsonvalue.Present(token.Color.StructuredResult, token.Color.StructuredResultPresent),
	}
}

func failureDetailsForResult(result workers.WorkResult) (string, string) {
	if result.Outcome != workers.OutcomeFailed {
		return "", ""
	}

	reason := failureReasonForResult(result)
	message := strings.TrimSpace(result.Error)
	if result.FailureDetail != nil {
		if typedReason := strings.TrimSpace(string(result.FailureDetail.Reason)); typedReason != "" {
			reason = typedReason
		}
		if typedMessage := strings.TrimSpace(result.FailureDetail.Message); typedMessage != "" {
			message = typedMessage
		}
	}
	if message == "" {
		message = failureMessageUnavailable
	}
	return reason, message
}

func failureReasonForResult(result workers.WorkResult) string {
	failureMetadata := result.FailureMetadata
	if failureMetadata != nil {
		if failureMetadata.Type != "" {
			return string(failureMetadata.Type)
		}
		if failureMetadata.Family != "" {
			return string(failureMetadata.Family)
		}
	}
	if strings.TrimSpace(result.Error) != "" {
		return failureReasonWorkerError
	}
	return failureReasonUnknown
}

func outputWorkItems(mutations []interfaces.TokenMutationRecord, consumedTokens []workers.Token) []work.FactoryWorkItem {
	items := make([]work.FactoryWorkItem, 0, len(mutations))
	previousChainingTraceIDs := workers.PreviousChainingTraceIDs(consumedTokens)
	for _, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.Color.DataType == workers.DataTypeResource {
			continue
		}
		item := workItemFromToken(*mutation.Token)
		item.PreviousChainingTraceIDs = previousChainingTraceIDs
		items = append(items, item)
	}
	return items
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	clone := make(map[string]string, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

func failureDetail(reason, message string) *workers.FailureDetail {
	return failureDetailValue(reason, message)
}

func failureDetailValue(reason, message string) *workers.FailureDetail {
	reason = strings.TrimSpace(reason)
	message = strings.TrimSpace(message)
	if reason == "" || message == "" {
		return nil
	}
	return &workers.FailureDetail{
		Reason:  normalizedFailureReason(reason),
		Message: message,
	}
}

func normalizedFailureReason(reason string) workers.WorkFailureType {
	candidate := workers.WorkFailureType(strings.TrimSpace(reason))
	switch candidate {
	case workers.WorkFailureTypeAuthFailure,
		workers.WorkFailureTypePermanentBadRequest,
		workers.WorkFailureTypeThrottled,
		workers.WorkFailureTypeInternalServerError,
		workers.WorkFailureTypeTimeout,
		workers.WorkFailureTypeMisconfigured,
		workers.WorkFailureTypeMissingExecutable,
		workers.WorkFailureTypeCommandLineTooLong,
		workers.WorkFailureTypeStructuredOutputSchemaViolation,
		workers.WorkFailureTypeExpectedArtifactsUnsatisfied:
		return candidate
	default:
		return workers.WorkFailureTypeUnknown
	}
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(input))
	out := make([]string, 0, len(input))
	for _, value := range input {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
