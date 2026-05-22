package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
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
	failureReasonWorkerError       = "worker_error"
	failureReasonUnknown           = "workstation_failed"
	failureMessageUnavailable      = "Workstation failed without a reported error message."
	eventHistoryStreamBufferSize   = 64
)

type eventHistorySubscription struct {
	events chan factoryapi.FactoryEvent
	inbox  chan factoryapi.FactoryEvent
	done   <-chan struct{}
}

// FactoryEventHistory stores the current-process canonical event history.
// It is intentionally in-memory and unbounded for the event-stream MVP.
type FactoryEventHistory struct {
	mu             sync.RWMutex
	net            *state.Net
	runtimeConfig  interfaces.RuntimeDefinitionLookup
	factoryRunner  string
	now            func() time.Time
	events         []factoryapi.FactoryEvent
	recorders      []func(factoryapi.FactoryEvent)
	nextID         int
	streams        map[int]*eventHistorySubscription
	runRecordedAt  time.Time
	hasRunRequest  bool
	hasRunResponse bool
}

// NewFactoryEventHistory creates an in-memory factory event history for one
// process lifetime and records no events until RecordInitialStructure is called.
func NewFactoryEventHistory(net *state.Net, now func() time.Time, runtimeConfigs ...interfaces.RuntimeDefinitionLookup) *FactoryEventHistory {
	if now == nil {
		now = time.Now
	}
	return &FactoryEventHistory{
		net:           net,
		runtimeConfig: interfaces.FirstRuntimeDefinitionLookup(runtimeConfigs...),
		now:           now,
		streams:       make(map[int]*eventHistorySubscription),
	}
}

// SetFactoryRunnerOverride preserves the effective factory-level runner
// selection when service wiring overrides the authored runtime config.
func (h *FactoryEventHistory) SetFactoryRunnerOverride(runnerID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.factoryRunner = interfaces.NormalizeRunnerID(runnerID)
}

// Events returns the recorded events in append order.
func (h *FactoryEventHistory) Events() []factoryapi.FactoryEvent {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	events := make([]factoryapi.FactoryEvent, len(h.events))
	copy(events, h.events)
	return events
}

// Subscribe returns a replay snapshot followed by live canonical events.
func (h *FactoryEventHistory) Subscribe(ctx context.Context) interfaces.FactoryEventStream {
	if h == nil {
		ch := make(chan factoryapi.FactoryEvent)
		close(ch)
		return interfaces.FactoryEventStream{Events: ch}
	}

	h.mu.Lock()
	events := make([]factoryapi.FactoryEvent, len(h.events))
	copy(events, h.events)
	id := h.nextID
	h.nextID++
	subscription := &eventHistorySubscription{
		events: make(chan factoryapi.FactoryEvent, eventHistoryStreamBufferSize),
		inbox:  make(chan factoryapi.FactoryEvent, eventHistoryStreamBufferSize),
		done:   ctx.Done(),
	}
	h.streams[id] = subscription
	h.mu.Unlock()

	go func() {
		defer close(subscription.events)
		for {
			select {
			case <-subscription.done:
				h.mu.Lock()
				delete(h.streams, id)
				h.mu.Unlock()
				return
			case event := <-subscription.inbox:
				select {
				case <-subscription.done:
					h.mu.Lock()
					delete(h.streams, id)
					h.mu.Unlock()
					return
				case subscription.events <- event:
				}
			}
		}
	}()

	return interfaces.FactoryEventStream{History: events, Events: subscription.events}
}

// AddGeneratedRecorder registers a callback invoked for every future generated
// FactoryEvent append. Existing events are replayed to the callback first so
// late recorder setup still sees a complete current-process history.
func (h *FactoryEventHistory) AddGeneratedRecorder(recorder func(factoryapi.FactoryEvent)) {
	if h == nil || recorder == nil {
		return
	}

	h.mu.Lock()
	events := make([]factoryapi.FactoryEvent, len(h.events))
	copy(events, h.events)
	h.recorders = append(h.recorders, recorder)
	h.mu.Unlock()

	for _, event := range events {
		recorder(event)
	}
}

// RecordInitialStructure records the static topology before work events.
func (h *FactoryEventHistory) RecordInitialStructure() {
	if h == nil {
		return
	}
	eventTime := h.now()
	payload := projections.ProjectInitialStructure(h.net, h.runtimeConfig)
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeInitialStructureRequest,
		eventIDInitialStructure,
		factoryapi.FactoryEventContext{Tick: 0, EventTime: eventTime},
		factoryapi.InitialStructureRequestEventPayload{Factory: generatedFactory(payload)},
	))
}

// RecordFactoryChange records a canonical topology replacement event after a
// live running factory definition change becomes active.
func (h *FactoryEventHistory) RecordFactoryChange(tick int, payload factoryapi.FactoryChangeEventPayload, eventTime time.Time) {
	if h == nil {
		return
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeFactoryChange,
		fmt.Sprintf("%s/%d", eventIDFactoryChangePrefix, tick),
		factoryapi.FactoryEventContext{Tick: tick, EventTime: eventTime},
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
	recordedAt := h.now()
	h.runRecordedAt = recordedAt
	h.hasRunRequest = true
	h.mu.Unlock()

	payload := projections.ProjectInitialStructure(h.net, h.runtimeConfig)
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeRunRequest,
		eventIDRunRequest,
		factoryapi.FactoryEventContext{Tick: 0, EventTime: recordedAt},
		factoryapi.RunRequestEventPayload{
			RecordedAt: recordedAt,
			Factory:    generatedFactory(payload),
		},
	))
}

// RecordWorkInput records a submitted work token after submit-time identity
// generation has completed.
func (h *FactoryEventHistory) RecordWorkInput(tick int, req interfaces.SubmitRequest, token interfaces.Token, eventTime time.Time) {
	if h == nil || token.ID == "" {
		return
	}
}

// RecordWorkRequest records the batch-level request before its work items are
// exposed as individual work input events.
func (h *FactoryEventHistory) RecordWorkRequest(tick int, record interfaces.WorkRequestRecord, eventTime time.Time) {
	if h == nil || record.RequestID == "" {
		return
	}
	context := factoryapi.FactoryEventContext{
		Tick:      tick,
		EventTime: eventTime,
		RequestId: stringPtr(record.RequestID),
		TraceIds:  stringSlicePtr(interfaces.CanonicalChainingTraceIDs([]string{record.TraceID})),
		WorkIds:   stringSlicePtr(workItemIDs(record.WorkItems)),
		Source:    stringPtrIfNotEmpty(record.Source),
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeWorkRequest,
		fmt.Sprintf("%s/%s", eventIDWorkRequestPrefix, record.RequestID),
		context,
		factoryapi.WorkRequestEventPayload{
			Type:          factoryapi.WorkRequestType(record.Type),
			Works:         generatedWorksPtr(record.WorkItems),
			Relations:     generatedFactoryRelationsPtr(record.Relations),
			Source:        stringPtrIfNotEmpty(record.Source),
			ParentLineage: stringSlicePtr(record.ParentLineage),
		},
	))
	for i, relation := range record.Relations {
		h.RecordRelationshipChange(tick, record.RequestID, record.TraceID, i, relation, eventTime)
	}
}

// RecordRelationshipChange records one relation created by a request batch.
func (h *FactoryEventHistory) RecordRelationshipChange(tick int, requestID string, traceID string, index int, relation interfaces.FactoryRelation, eventTime time.Time) {
	if h == nil || relation.Type == "" || relation.TargetWorkID == "" {
		return
	}
	if relation.RequestID == "" {
		relation.RequestID = requestID
	}
	if relation.TraceID == "" {
		relation.TraceID = traceID
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
		fmt.Sprintf("%s/%s/%d", eventIDRelationshipPrefix, requestID, index),
		factoryapi.FactoryEventContext{
			Tick:      tick,
			EventTime: eventTime,
			RequestId: stringPtrIfNotEmpty(requestID),
			TraceIds:  stringSlicePtr(interfaces.CanonicalChainingTraceIDs([]string{traceID, relation.TraceID})),
			WorkIds:   stringSlicePtr(uniqueStrings([]string{relation.SourceWorkID, relation.TargetWorkID})),
		},
		factoryapi.RelationshipChangeRequestEventPayload{Relation: generatedFactoryRelation(relation)},
	))
}

// RecordWorkstationRequest records a dispatch at the tick it consumed inputs.
func (h *FactoryEventHistory) RecordWorkstationRequest(tick int, record interfaces.FactoryDispatchRecord, eventTime time.Time) {
	dispatchID := record.Dispatch.DispatchID
	if h == nil || dispatchID == "" {
		return
	}
	inputTokens := workers.WorkDispatchInputTokens(record.Dispatch)
	runnerSelection := h.resolvedRunnerSelectionForDispatch(record.Dispatch)
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchRequest,
		fmt.Sprintf("%s/%s", eventIDDispatchCreatedPrefix, dispatchID),
		factoryapi.FactoryEventContext{
			Tick:                     tick,
			EventTime:                eventTime,
			DispatchId:               stringPtr(dispatchID),
			RequestId:                stringPtrIfNotEmpty(record.Dispatch.Execution.RequestID),
			TraceIds:                 stringSlicePtr(traceIDsFromTokens(inputTokens)),
			WorkIds:                  stringSlicePtr(workIDsFromTokens(inputTokens)),
			CurrentChainingTraceId:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIds: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
		},
		factoryapi.DispatchRequestEventPayload{
			TransitionId:             record.Dispatch.TransitionID,
			CurrentChainingTraceId:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIds: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
			Inputs:                   generatedDispatchConsumedWorkRefsFromTokens(inputTokens),
			Resources:                h.generatedResourcesPtr(inputTokens),
			Metadata:                 generatedDispatchRequestEventMetadataPtr(record.Dispatch.Execution.ReplayKey, runnerSelection),
		},
	))
}

// RecordWorkstationResponse records a completed dispatch and its outputs.
func (h *FactoryEventHistory) RecordWorkstationResponse(tick int, result interfaces.WorkResult, completed interfaces.CompletedDispatch) {
	if h == nil || result.DispatchID == "" {
		return
	}
	eventTime := completed.EndTime
	if eventTime.IsZero() {
		eventTime = h.now()
	}
	failureReason, failureMessage := failureDetailsForResult(result)
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchResponse,
		fmt.Sprintf("%s/%s", eventIDDispatchCompletedPrefix, result.DispatchID),
		factoryapi.FactoryEventContext{
			Tick:                     tick,
			EventTime:                eventTime,
			DispatchId:               stringPtr(result.DispatchID),
			TraceIds:                 stringSlicePtr(traceIDsFromTokens(completed.ConsumedTokens)),
			WorkIds:                  stringSlicePtr(workIDsFromTokens(completed.ConsumedTokens)),
			CurrentChainingTraceId:   stringPtrIfNotEmpty(interfaces.CurrentChainingTraceIDFromTokens(completed.ConsumedTokens)),
			PreviousChainingTraceIds: stringSlicePtr(interfaces.PreviousChainingTraceIDsFromTokens(completed.ConsumedTokens)),
		},
		factoryapi.DispatchResponseEventPayload{
			TransitionId:                result.TransitionID,
			CurrentChainingTraceId:      stringPtrIfNotEmpty(interfaces.CurrentChainingTraceIDFromTokens(completed.ConsumedTokens)),
			PreviousChainingTraceIds:    stringSlicePtr(interfaces.PreviousChainingTraceIDsFromTokens(completed.ConsumedTokens)),
			Outcome:                     factoryapi.WorkOutcome(result.Outcome),
			Output:                      stringPtrIfNotEmpty(result.Output),
			Error:                       stringPtrIfNotEmpty(result.Error),
			Feedback:                    stringPtrIfNotEmpty(result.Feedback),
			SelectedClassificationLabel: stringPtrIfNotEmpty(result.SelectedClassificationLabel),
			FailureReason:               stringPtrIfNotEmpty(failureReason),
			FailureMessage:              stringPtrIfNotEmpty(failureMessage),
			DurationMillis:              int64Ptr(completed.Duration.Milliseconds()),
			OutputWork:                  generatedWorksPtr(outputWorkItems(completed.OutputMutations, completed.ConsumedTokens)),
			OutputResources:             h.generatedOutputResourcesPtr(completed.OutputMutations),
			ProviderFailure:             interfaces.GeneratedProviderFailureMetadata(result.ProviderFailure),
		},
	))
}

// RecordInferenceEvent appends a provider-boundary inference event to the same
// canonical history used for dispatch and replay events.
func (h *FactoryEventHistory) RecordInferenceEvent(event factoryapi.FactoryEvent) {
	if h == nil || !isInferenceEventType(event.Type) {
		return
	}
	h.appendGenerated(event)
}

// RecordScriptEvent appends a script-boundary event to the same canonical
// history used for dispatch and replay events.
func (h *FactoryEventHistory) RecordScriptEvent(event factoryapi.FactoryEvent) {
	if h == nil || !isScriptEventType(event.Type) {
		return
	}
	h.appendGenerated(event)
}

// AppendRecordedEvent appends one already-shaped canonical event into the
// history so callers can bridge runtime-owned events into a wider stream.
func (h *FactoryEventHistory) AppendRecordedEvent(event factoryapi.FactoryEvent) {
	if h == nil {
		return
	}
	h.appendGenerated(event)
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
	h.hasRunResponse = true
	h.mu.Unlock()

	stateValue := factoryapi.FactoryState(state)
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeRunResponse,
		eventIDRunResponse,
		factoryapi.FactoryEventContext{Tick: tick, EventTime: eventTime},
		factoryapi.RunResponseEventPayload{
			State:  &stateValue,
			Reason: stringPtrIfNotEmpty(reason),
			WallClock: &factoryapi.WallClock{
				StartedAt:  timePtrIfNotZero(recordedAt),
				FinishedAt: timePtrIfNotZero(eventTime),
			},
		},
	))
}

// RecordFactoryStateChange records a runtime lifecycle transition.
func (h *FactoryEventHistory) RecordFactoryStateChange(tick int, previous interfaces.FactoryState, next interfaces.FactoryState, reason string, eventTime time.Time) {
	if h == nil || previous == next {
		return
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeFactoryStateResponse,
		fmt.Sprintf("%s/%d/%s", eventIDStateChangePrefix, tick, next),
		factoryapi.FactoryEventContext{Tick: tick, EventTime: eventTime},
		factoryapi.FactoryStateResponseEventPayload{
			PreviousState: generatedFactoryStatePtr(previous),
			State:         factoryapi.FactoryState(next),
			Reason:        stringPtrIfNotEmpty(reason),
		},
	))
}

func (h *FactoryEventHistory) appendGenerated(event factoryapi.FactoryEvent) {
	h.mu.Lock()
	event.SchemaVersion = factoryapi.AgentFactoryEventV1
	event.Context.Sequence = len(h.events)
	h.events = append(h.events, event)
	streams := make([]*eventHistorySubscription, 0, len(h.streams))
	for _, stream := range h.streams {
		streams = append(streams, stream)
	}
	recorders := append([]func(factoryapi.FactoryEvent){}, h.recorders...)
	h.mu.Unlock()

	for _, recorder := range recorders {
		recorder(event)
	}
	for _, stream := range streams {
		select {
		case <-stream.done:
			continue
		default:
		}
		select {
		case stream.inbox <- event:
		default:
		}
	}
}

func factoryEvent(eventType factoryapi.FactoryEventType, id string, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
	return factoryapi.FactoryEvent{
		Type:    eventType,
		Id:      id,
		Context: context,
		Payload: factoryEventPayload(payload),
	}
}

func factoryEventPayload(payload any) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		err = out.FromRunRequestEventPayload(typed)
	case factoryapi.InitialStructureRequestEventPayload:
		err = out.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.FactoryChangeEventPayload:
		err = out.FromFactoryChangeEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = out.FromWorkRequestEventPayload(typed)
	case factoryapi.RelationshipChangeRequestEventPayload:
		err = out.FromRelationshipChangeRequestEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = out.FromDispatchRequestEventPayload(typed)
	case factoryapi.DispatchResponseEventPayload:
		err = out.FromDispatchResponseEventPayload(typed)
	case factoryapi.FactoryStateResponseEventPayload:
		err = out.FromFactoryStateResponseEventPayload(typed)
	case factoryapi.RunResponseEventPayload:
		err = out.FromRunResponseEventPayload(typed)
	default:
		encoded, marshalErr := json.Marshal(typed)
		if marshalErr != nil {
			err = marshalErr
		} else {
			err = out.UnmarshalJSON(encoded)
		}
	}
	if err != nil {
		panic(fmt.Sprintf("factory event payload %T: %v", payload, err))
	}
	return out
}

func (h *FactoryEventHistory) resolvedRunnerSelectionForDispatch(dispatch interfaces.WorkDispatch) interfaces.ResolvedRunnerSelection {
	if h == nil {
		return interfaces.ResolvedRunnerSelection{}
	}
	workstationRunner, workerModelProvider := h.runnerSelectionInputsForDispatch(dispatch)
	factoryRunner := h.factoryRunnerID()
	return interfaces.ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider)
}

func (h *FactoryEventHistory) runnerSelectionInputsForDispatch(dispatch interfaces.WorkDispatch) (string, string) {
	if h == nil || h.runtimeConfig == nil {
		return "", ""
	}
	workstationName := strings.TrimSpace(dispatch.WorkstationName)
	if workstationName == "" && h.net != nil {
		if transition, ok := h.net.Transitions[dispatch.TransitionID]; ok {
			workstationName = strings.TrimSpace(transition.Name)
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
	type factoryConfigProvider interface {
		FactoryConfig() *interfaces.FactoryConfig
	}
	provider, ok := h.runtimeConfig.(factoryConfigProvider)
	if !ok {
		return ""
	}
	cfg := provider.FactoryConfig()
	if cfg == nil {
		return ""
	}
	return cfg.Runner
}

func traceIDsFromTokens(tokens []interfaces.Token) []string {
	return interfaces.PreviousChainingTraceIDsFromTokens(tokens)
}

func workIDsFromTokens(tokens []interfaces.Token) []string {
	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		values = append(values, token.Color.WorkID)
	}
	return uniqueStrings(values)
}

func workItemIDs(items []interfaces.FactoryWorkItem) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.ID)
	}
	return uniqueStrings(values)
}

func generatedStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(cloneStringMap(values))
	return &converted
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

func intPtrIfPositive(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func timePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
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

func isInferenceEventType(eventType factoryapi.FactoryEventType) bool {
	switch eventType {
	case factoryapi.FactoryEventTypeInferenceRequest, factoryapi.FactoryEventTypeInferenceResponse:
		return true
	default:
		return false
	}
}

func isScriptEventType(eventType factoryapi.FactoryEventType) bool {
	switch eventType {
	case factoryapi.FactoryEventTypeScriptRequest, factoryapi.FactoryEventTypeScriptResponse:
		return true
	default:
		return false
	}
}

func workItemFromToken(token interfaces.Token) interfaces.FactoryWorkItem {
	currentChainingTraceID := token.Color.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = token.Color.TraceID
	}
	return interfaces.FactoryWorkItem{
		ID:                       token.Color.WorkID,
		WorkTypeID:               token.Color.WorkTypeID,
		DisplayName:              token.Color.Name,
		ChainingTraceDepth:       token.Color.ChainingTraceDepth,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), token.Color.PreviousChainingTraceIDs...),
		TraceID:                  token.Color.TraceID,
		Content:                  append([]interfaces.WorkContentPart(nil), token.Color.Content...),
		ParentID:                 token.Color.ParentID,
		PlaceID:                  token.PlaceID,
		Tags:                     cloneStringMap(token.Color.Tags),
	}
}

func failureDetailsForResult(result interfaces.WorkResult) (string, string) {
	if result.Outcome != interfaces.OutcomeFailed {
		return "", ""
	}

	reason := failureReasonForResult(result)
	message := strings.TrimSpace(result.Error)
	if message == "" {
		message = failureMessageUnavailable
	}
	return reason, message
}

func failureReasonForResult(result interfaces.WorkResult) string {
	if result.ProviderFailure != nil {
		if result.ProviderFailure.Type != "" {
			return string(result.ProviderFailure.Type)
		}
		if result.ProviderFailure.Family != "" {
			return string(result.ProviderFailure.Family)
		}
	}
	if strings.TrimSpace(result.Error) != "" {
		return failureReasonWorkerError
	}
	return failureReasonUnknown
}

func outputWorkItems(mutations []interfaces.TokenMutationRecord, consumedTokens []interfaces.Token) []interfaces.FactoryWorkItem {
	items := make([]interfaces.FactoryWorkItem, 0, len(mutations))
	previousChainingTraceIDs := interfaces.PreviousChainingTraceIDsFromTokens(consumedTokens)
	for _, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.Color.DataType == interfaces.DataTypeResource {
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
