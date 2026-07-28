package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	eventsnapshot "github.com/portpowered/infinite-you/pkg/services/recordings/events/snapshot"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TODO: we should move these constants to the interfaces package, actually we should move the events generally to the openapi.yaml to allow generation of the various types of events payloads.
// we should declare all events as schemas, and derive the structures internally from said events.
// record/replay should record events in the format of the schemas defined in the api package.
// the API should respond with the serialized json payloads of those openapi.yaml based schemas.
const (
	eventIDRunRequest              = "factory-event/run-started"
	eventIDRunResponse             = "factory-event/run-finished"
	eventIDInitialStructure        = "factory-event/initial-structure/0"
	eventIDFactoryChangePrefix     = "factory-event/factory-change"
	eventIDWorkRequestPrefix       = "factory-event/work-request"
	eventIDRelationshipPrefix      = "factory-event/relationship-change"
	eventIDDispatchCreatedPrefix   = "factory-event/dispatch-created"
	eventIDDispatchCompletedPrefix = "factory-event/dispatch-completed"
	eventIDStateChangePrefix       = "factory-event/factory-state-change"
	eventIDWorkStateChangePrefix   = "factory-event/work-state-change"
	failureReasonWorkerError       = "worker_error"
	failureReasonUnknown           = "workstation_failed"
	failureMessageUnavailable      = "Workstation failed without a reported error message."
	eventHistoryStreamBufferSize   = 64
)

type eventHistorySubscription struct {
	events       chan interfaces.FactoryEvent
	inbox        chan interfaces.FactoryEvent
	done         <-chan struct{}
	overflow     chan struct{}
	overflowOnce sync.Once
}

func (subscription *eventHistorySubscription) signalOverflow() {
	subscription.overflowOnce.Do(func() {
		close(subscription.overflow)
	})
}

// CloseLiveSubscriptions ends active live subscriptions without appending new
// canonical events. Callers invoke this after terminal lifecycle events are
// recorded so SSE clients observe the final timeline and then a closed stream.
func (h *FactoryEventHistory) CloseLiveSubscriptions() {
	if h == nil {
		return
	}
	h.mu.Lock()
	streams := make([]*eventHistorySubscription, 0, len(h.streams))
	for _, subscription := range h.streams {
		streams = append(streams, subscription)
	}
	h.mu.Unlock()
	for _, subscription := range streams {
		subscription.signalOverflow()
	}
}

// FactoryEventHistory stores the current-process canonical event history.
// It is intentionally in-memory and unbounded for the event-stream MVP.
type FactoryEventHistory struct {
	mu                  sync.RWMutex
	initialStructure    interfaces.InitialStructurePayload
	runtimeConfig       interfaces.RuntimeDefinitionLookup
	factoryRunner       string
	initialFactory      *interfaces.FactorySnapshot
	now                 func() time.Time
	streamGenerationID  string
	events              []interfaces.FactoryEvent
	recorders           []func(interfaces.FactoryEvent)
	eventTypeRecorders  []func(interfaces.FactoryEventType)
	nextID              int
	streams             map[int]*eventHistorySubscription
	runRecordedAt       time.Time
	hasRunRequest       bool
	hasRunResponse      bool
	sessionStartedAt    time.Time
	hasSessionStarted   bool
	hasSessionCompleted bool
	nextSessionSequence int
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
	h.factoryRunner = workerrunner.NormalizeRunnerID(runnerID)
}

// SetInitialStructureFactory overrides the canonical Factory snapshot emitted
// by INITIAL_STRUCTURE. Runtime callers can keep execution configs thin while
// service callers expose an editable event-sourced document.
func (h *FactoryEventHistory) SetInitialStructureFactory(factory *interfaces.FactorySnapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.initialFactory = factory.Clone()
}

// CanonicalEvents returns detached Factory-owned events in append order.
func (h *FactoryEventHistory) CanonicalEvents() []interfaces.FactoryEvent {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

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
	events := cloneFactoryEvents(h.events)
	streamGenerationID := h.streamGenerationID
	if reconnect != nil {
		replayed, err := buildDomainReconnectReplay(events, *reconnect, scope)
		if err != nil {
			h.mu.Unlock()
			return interfaces.FactoryEventStream{}, err
		}
		events = replayed
	}
	id := h.nextID
	h.nextID++
	subscription := &eventHistorySubscription{
		events:   make(chan interfaces.FactoryEvent, eventHistoryStreamBufferSize),
		inbox:    make(chan interfaces.FactoryEvent, eventHistoryStreamBufferSize),
		done:     ctx.Done(),
		overflow: make(chan struct{}),
	}
	h.streams[id] = subscription
	h.mu.Unlock()

	go func() {
		defer close(subscription.events)
		defer func() {
			h.mu.Lock()
			delete(h.streams, id)
			h.mu.Unlock()
		}()
		for {
			select {
			case <-subscription.done:
				return
			case <-subscription.overflow:
				return
			case event := <-subscription.inbox:
				select {
				case <-subscription.done:
					return
				case <-subscription.overflow:
					return
				case subscription.events <- event.Clone():
				}
			}
		}
	}()

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
	h.mu.Unlock()

	for _, event := range events {
		recorder(event)
	}
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
	h.mu.Unlock()

	for _, eventType := range eventTypes {
		recorder(eventType)
	}
}

// AppendRecordedEvent appends one already-shaped canonical domain event so
// runtime owners can bridge their events into this history without depending
// on a transport representation.
func (h *FactoryEventHistory) AppendRecordedEvent(event interfaces.FactoryEvent) {
	if h == nil {
		return
	}
	event.Context.EventTime = interfaces.CanonicalEventTime(event.Context.EventTime)
	h.appendEvent(event)
}

// RecordInitialStructure records the static topology before work events.
func (h *FactoryEventHistory) RecordInitialStructure() {
	if h == nil {
		return
	}
	eventTime := interfaces.CanonicalEventTime(h.now())
	payload := h.initialStructure
	factory := eventsnapshot.FromInitialStructure(payload)
	h.mu.RLock()
	if h.initialFactory != nil {
		factory = h.initialFactory.Clone()
	}
	h.mu.RUnlock()
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeInitialStructureRequest,
		eventIDInitialStructure,
		interfaces.FactoryEventContext{Tick: 0, EventTime: eventTime},
		interfaces.InitialStructureRequestEventPayload{Factory: factory},
	))
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
	h.mu.Unlock()

	payload := h.initialStructure
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeRunRequest,
		eventIDRunRequest,
		interfaces.FactoryEventContext{Tick: 0, EventTime: recordedAt},
		interfaces.RunRequestEventPayload{
			RecordedAt: recordedAt,
			Factory:    eventsnapshot.FromInitialStructure(payload),
		},
	))
}

// RecordWorkInput records a submitted work token after submit-time identity
// generation has completed.
func (h *FactoryEventHistory) RecordWorkInput(tick int, req work.SubmitRequest, token workerexecution.Token, eventTime time.Time) {
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
		context,
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
		interfaces.FactoryEventContext{
			Tick:      tick,
			EventTime: eventTime,
			RequestID: stringPtrIfNotEmpty(requestID),
			TraceIDs:  stringSlicePtr(work.CanonicalChainingTraceIDs([]string{traceID, relation.TraceID})),
			WorkIDs:   stringSlicePtr(uniqueStrings([]string{relation.SourceWorkID, relation.TargetWorkID})),
		},
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
		interfaces.FactoryEventContext{
			Tick:                     tick,
			EventTime:                eventTime,
			DispatchID:               stringPtr(dispatchID),
			RequestID:                stringPtrIfNotEmpty(record.Dispatch.Execution.RequestID),
			TraceIDs:                 stringSlicePtr(traceIDsFromTokens(inputTokens)),
			WorkIDs:                  stringSlicePtr(workIDsFromTokens(inputTokens)),
			CurrentChainingTraceID:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIDs: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
		},
		interfaces.DispatchRequestEventPayload{
			TransitionID:             record.Dispatch.TransitionID,
			CurrentChainingTraceID:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIDs: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
			Inputs:                   dispatchConsumedWorkRefsFromTokens(inputTokens),
			Resources:                h.dispatchResourcesPtr(inputTokens),
			Metadata:                 dispatchRequestEventMetadataPtr(record.Dispatch.Execution.ReplayKey, runnerSelection),
		},
	))
}

// RecordWorkstationResponse records a completed dispatch and its outputs.
func (h *FactoryEventHistory) RecordWorkstationResponse(tick int, result workerexecution.WorkResult, completed interfaces.CompletedDispatch) {
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
		interfaces.FactoryEventContext{
			Tick:                     tick,
			EventTime:                eventTime,
			DispatchID:               stringPtr(result.DispatchID),
			TraceIDs:                 stringSlicePtr(traceIDsFromTokens(completed.ConsumedTokens)),
			WorkIDs:                  stringSlicePtr(workIDsFromTokens(completed.ConsumedTokens)),
			CurrentChainingTraceID:   stringPtrIfNotEmpty(workerexecution.CurrentChainingTraceID(completed.ConsumedTokens, interfaces.SystemTimeWorkTypeID)),
			PreviousChainingTraceIDs: stringSlicePtr(workerexecution.PreviousChainingTraceIDs(completed.ConsumedTokens)),
		},
		workerexecution.DispatchResponseEventPayload{
			TransitionID:                result.TransitionID,
			CurrentChainingTraceID:      stringPtrIfNotEmpty(workerexecution.CurrentChainingTraceID(completed.ConsumedTokens, interfaces.SystemTimeWorkTypeID)),
			PreviousChainingTraceIDs:    stringSlicePtr(workerexecution.PreviousChainingTraceIDs(completed.ConsumedTokens)),
			Outcome:                     result.Outcome,
			Output:                      stringPtrIfNotEmpty(result.Output),
			Error:                       stringPtrIfNotEmpty(result.Error),
			Feedback:                    stringPtrIfNotEmpty(result.Feedback),
			SelectedClassificationLabel: stringPtrIfNotEmpty(result.SelectedClassificationLabel),
			FailureDetail:               failureDetail(failureReason, failureMessage),
			DurationMillis:              int64Ptr(completed.Duration.Milliseconds()),
			OutputWork:                  eventWorksPtr(outputWorkItems(completed.OutputMutations, completed.ConsumedTokens)),
			OutputResources:             h.dispatchOutputResourcesPtr(completed.OutputMutations),
			ProviderFailure:             workerexecution.CloneWorkFailureMetadata(result.FailureMetadata),
		},
	))
}

// RecordModelEvent appends worker-owned model execution facts to canonical
// history while Factory owns the envelope, vocabulary, and ordering.
func (h *FactoryEventHistory) RecordModelEvent(event workerexecution.ModelEvent) {
	if h == nil || strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.DispatchID) == "" {
		return
	}
	eventType, payload := modelFactoryEventPayload(event)
	if eventType == "" || payload == nil {
		return
	}
	h.appendEvent(domainFactoryEvent(
		eventType,
		event.ID,
		interfaces.FactoryEventContext{
			Tick:       event.Tick,
			EventTime:  interfaces.CanonicalEventTime(event.EventTime),
			DispatchID: stringPtrIfNotEmpty(event.DispatchID),
			RequestID:  stringPtrIfNotEmpty(event.RequestID),
			TraceIDs:   stringSlicePtr(event.TraceIDs),
			WorkIDs:    stringSlicePtr(event.WorkIDs),
		},
		payload,
	))
}

func modelFactoryEventPayload(event workerexecution.ModelEvent) (interfaces.FactoryEventType, any) {
	switch event.Kind {
	case workerexecution.ModelEventKindRequest:
		if event.Request != nil && event.Response == nil {
			return interfaces.FactoryEventTypeModelRequest, *event.Request
		}
	case workerexecution.ModelEventKindResponse:
		if event.Response != nil && event.Request == nil {
			return interfaces.FactoryEventTypeModelResponse, *event.Response
		}
	}
	return "", nil
}

// RecordScriptEvent appends worker-owned script facts to the canonical history
// while Factory owns the envelope, vocabulary, and ordering.
func (h *FactoryEventHistory) RecordScriptEvent(event workerexecution.ScriptEvent) {
	if h == nil || strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.DispatchID) == "" {
		return
	}
	eventType, payload := scriptFactoryEventPayload(event)
	if eventType == "" || payload == nil {
		return
	}
	h.appendEvent(domainFactoryEvent(
		eventType,
		event.ID,
		interfaces.FactoryEventContext{
			Tick:       event.Tick,
			EventTime:  interfaces.CanonicalEventTime(event.EventTime),
			DispatchID: stringPtrIfNotEmpty(event.DispatchID),
			RequestID:  stringPtrIfNotEmpty(event.RequestID),
			TraceIDs:   stringSlicePtr(event.TraceIDs),
			WorkIDs:    stringSlicePtr(event.WorkIDs),
		},
		payload,
	))
}

func scriptFactoryEventPayload(event workerexecution.ScriptEvent) (interfaces.FactoryEventType, any) {
	switch event.Kind {
	case workerexecution.ScriptEventKindRequest:
		if event.Request != nil && event.Response == nil {
			return interfaces.FactoryEventTypeScriptRequest, *event.Request
		}
	case workerexecution.ScriptEventKindResponse:
		if event.Response != nil && event.Request == nil {
			return interfaces.FactoryEventTypeScriptResponse, *event.Response
		}
	}
	return "", nil
}

// RecordAgentRunEvent appends an agent-run boundary event to the same
// canonical history used for dispatch and replay events.
func (h *FactoryEventHistory) RecordAgentRunEvent(event workerexecution.AgentRunResponseEvent) {
	if h == nil || strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.DispatchID) == "" {
		return
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeAgentRunResponse,
		event.ID,
		interfaces.FactoryEventContext{
			EventTime:  interfaces.CanonicalEventTime(event.EventTime),
			DispatchID: stringPtr(event.DispatchID),
		},
		event.Payload,
	))
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

// RecordWorkStateChange records a canonical marking relocation for operator or
// cascade recovery paths.
func (h *FactoryEventHistory) RecordWorkStateChange(tick int, record work.WorkStateChangeRecord, eventTime time.Time) {
	if h == nil || record.WorkID == "" || record.Source == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	workTypeName := strings.TrimSpace(record.WorkTypeName)
	if workTypeName == "" {
		workTypeName = record.WorkTypeID
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeWorkStateChange,
		fmt.Sprintf("%s/%s/%d", eventIDWorkStateChangePrefix, record.WorkID, tick),
		interfaces.FactoryEventContext{
			Tick:      tick,
			EventTime: eventTime,
			RequestID: stringPtrIfNotEmpty(record.RequestID),
			WorkIDs:   stringSlicePtr([]string{record.WorkID}),
		},
		interfaces.WorkStateChangeEventPayload{
			WorkID:        record.WorkID,
			WorkTypeName:  workTypeName,
			FromState:     record.FromState,
			ToState:       record.ToState,
			FromPlaceID:   record.FromPlaceID,
			ToPlaceID:     record.ToPlaceID,
			Source:        record.Source,
			TriggerWorkID: stringPtrIfNotEmpty(record.TriggerWorkID),
			Reason:        stringPtrIfNotEmpty(record.Reason),
		},
	))
}

// RecordFactoryStateChange records a runtime lifecycle transition.
func (h *FactoryEventHistory) RecordFactoryStateChange(tick int, previous interfaces.FactoryState, next interfaces.FactoryState, reason string, eventTime time.Time) {
	if h == nil || previous == next {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	nextState := next
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeFactoryStateResponse,
		fmt.Sprintf("%s/%d/%s", eventIDStateChangePrefix, tick, next),
		interfaces.FactoryEventContext{Tick: tick, EventTime: eventTime},
		interfaces.FactoryStateResponseEventPayload{
			PreviousState: &previous,
			State:         nextState,
			Reason:        stringPtrIfNotEmpty(reason),
		},
	))
}

func (h *FactoryEventHistory) appendEvent(event interfaces.FactoryEvent) {
	h.mu.Lock()
	event.SchemaVersion = interfaces.FactoryEventSchemaVersionV1
	event.Context.Sequence = len(h.events)
	h.events = append(h.events, event)
	streams := make([]*eventHistorySubscription, 0, len(h.streams))
	for _, stream := range h.streams {
		streams = append(streams, stream)
	}
	recorders := append([]func(interfaces.FactoryEvent){}, h.recorders...)
	eventTypeRecorders := append([]func(interfaces.FactoryEventType){}, h.eventTypeRecorders...)
	h.mu.Unlock()

	for _, recorder := range recorders {
		recorder(event.Clone())
	}
	for _, recorder := range eventTypeRecorders {
		recorder(event.Type)
	}
	for _, stream := range streams {
		select {
		case <-stream.done:
			continue
		case <-stream.overflow:
			continue
		default:
		}
		select {
		case stream.inbox <- event.Clone():
		default:
			stream.signalOverflow()
		}
	}
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

func (h *FactoryEventHistory) resolvedRunnerSelectionForDispatch(dispatch work.WorkDispatch) workerexecution.ResolvedRunnerSelection {
	if h == nil {
		return workerexecution.ResolvedRunnerSelection{}
	}
	workstationRunner, workerModelProvider := h.runnerSelectionInputsForDispatch(dispatch)
	factoryRunner := h.factoryRunnerID()
	return workerrunner.ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider)
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

func traceIDsFromTokens(tokens []workerexecution.Token) []string {
	return workerexecution.PreviousChainingTraceIDs(tokens)
}

func workIDsFromTokens(tokens []workerexecution.Token) []string {
	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == workerexecution.DataTypeResource {
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

func workItemFromToken(token workerexecution.Token) work.FactoryWorkItem {
	currentChainingTraceID := token.Color.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = token.Color.TraceID
	}
	_, stateValue := splitPlaceID(token.PlaceID)
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
		State:                    stateValue,
		PlaceID:                  token.PlaceID,
		Tags:                     cloneStringMap(token.Color.Tags),
	}
}

func failureDetailsForResult(result workerexecution.WorkResult) (string, string) {
	if result.Outcome != workerexecution.OutcomeFailed {
		return "", ""
	}

	reason := failureReasonForResult(result)
	message := strings.TrimSpace(result.Error)
	if message == "" {
		message = failureMessageUnavailable
	}
	return reason, message
}

func failureReasonForResult(result workerexecution.WorkResult) string {
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

func outputWorkItems(mutations []interfaces.TokenMutationRecord, consumedTokens []workerexecution.Token) []work.FactoryWorkItem {
	items := make([]work.FactoryWorkItem, 0, len(mutations))
	previousChainingTraceIDs := workerexecution.PreviousChainingTraceIDs(consumedTokens)
	for _, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.Color.DataType == workerexecution.DataTypeResource {
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

func failureDetail(reason, message string) *workerexecution.FailureDetail {
	return failureDetailValue(reason, message)
}

func failureDetailValue(reason, message string) *workerexecution.FailureDetail {
	reason = strings.TrimSpace(reason)
	message = strings.TrimSpace(message)
	if reason == "" || message == "" {
		return nil
	}
	return &workerexecution.FailureDetail{
		Reason:  normalizedFailureReason(reason),
		Message: message,
	}
}

func normalizedFailureReason(reason string) workerexecution.WorkFailureType {
	candidate := workerexecution.WorkFailureType(strings.TrimSpace(reason))
	switch candidate {
	case workerexecution.WorkFailureTypeAuthFailure,
		workerexecution.WorkFailureTypePermanentBadRequest,
		workerexecution.WorkFailureTypeThrottled,
		workerexecution.WorkFailureTypeInternalServerError,
		workerexecution.WorkFailureTypeTimeout,
		workerexecution.WorkFailureTypeMisconfigured,
		workerexecution.WorkFailureTypeMissingExecutable,
		workerexecution.WorkFailureTypeCommandLineTooLong:
		return candidate
	default:
		return workerexecution.WorkFailureTypeUnknown
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
