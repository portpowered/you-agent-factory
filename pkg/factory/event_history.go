package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/workers"
)

// TODO: we should move these constants to the interfaces package, actually we should move the events generally to the openapi.yaml to allow generation of the various types of events payloads.
// we should declare all events as schemas, and derive the structures internally from said events.
// record/replay should record events in the format of the schemas defined in the api package.
// the API should respond with the serialized json payloads of those openapi.yaml based schemas.
const (
	eventIDRunRequest              = "factory-event/run-started"
	eventIDRunResponse             = "factory-event/run-finished"
	eventIDInitialStructure        = "factory-event/initial-structure/0"
	eventIDWorkRequestPrefix       = "factory-event/work-request"
	eventIDRelationshipPrefix      = "factory-event/relationship-change"
	eventIDDispatchCreatedPrefix   = "factory-event/dispatch-created"
	eventIDDispatchCompletedPrefix = "factory-event/dispatch-completed"
	eventIDStateChangePrefix       = "factory-event/factory-state-change"
	failureReasonWorkerError       = "worker_error"
	failureReasonUnknown           = "workstation_failed"
	failureMessageUnavailable      = "Workstation failed without a reported error message."
)

// FactoryEventHistory stores the current-process canonical event history.
// It is intentionally in-memory and unbounded for the event-stream MVP.
type FactoryEventHistory struct {
	mu             sync.RWMutex
	net            *state.Net
	runtimeConfig  interfaces.RuntimeDefinitionLookup
	now            func() time.Time
	events         []factoryapi.FactoryEvent
	recorders      []func(factoryapi.FactoryEvent)
	nextID         int
	streams        map[int]chan factoryapi.FactoryEvent
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
		streams:       make(map[int]chan factoryapi.FactoryEvent),
	}
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
	ch := make(chan factoryapi.FactoryEvent, 64)
	h.streams[id] = ch
	h.mu.Unlock()

	go func() {
		<-ctx.Done()
		h.mu.Lock()
		stream, ok := h.streams[id]
		if ok {
			delete(h.streams, id)
			close(stream)
		}
		h.mu.Unlock()
	}()

	return interfaces.FactoryEventStream{History: events, Events: ch}
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
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchRequest,
		fmt.Sprintf("%s/%s", eventIDDispatchCreatedPrefix, dispatchID),
		factoryapi.FactoryEventContext{
			Tick:       tick,
			EventTime:  eventTime,
			DispatchId: stringPtr(dispatchID),
			RequestId:  stringPtrIfNotEmpty(record.Dispatch.Execution.RequestID),
			TraceIds:   stringSlicePtr(traceIDsFromTokens(inputTokens)),
			WorkIds:    stringSlicePtr(workIDsFromTokens(inputTokens)),
		},
		factoryapi.DispatchRequestEventPayload{
			TransitionId:             record.Dispatch.TransitionID,
			CurrentChainingTraceId:   stringPtrIfNotEmpty(record.Dispatch.CurrentChainingTraceID),
			PreviousChainingTraceIds: stringSlicePtr(record.Dispatch.PreviousChainingTraceIDs),
			Inputs:                   generatedDispatchConsumedWorkRefsFromTokens(inputTokens),
			Resources:                h.generatedResourcesPtr(inputTokens),
			Metadata:                 generatedDispatchRequestEventMetadataPtr(record.Dispatch.Execution.ReplayKey),
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
			Tick:       tick,
			EventTime:  eventTime,
			DispatchId: stringPtr(result.DispatchID),
			TraceIds:   stringSlicePtr(traceIDsFromTokens(completed.ConsumedTokens)),
			WorkIds:    stringSlicePtr(workIDsFromTokens(completed.ConsumedTokens)),
		},
		factoryapi.DispatchResponseEventPayload{
			TransitionId:             result.TransitionID,
			CurrentChainingTraceId:   stringPtrIfNotEmpty(interfaces.CurrentChainingTraceIDFromTokens(completed.ConsumedTokens)),
			PreviousChainingTraceIds: stringSlicePtr(interfaces.PreviousChainingTraceIDsFromTokens(completed.ConsumedTokens)),
			Outcome:                  factoryapi.WorkOutcome(result.Outcome),
			Output:                   stringPtrIfNotEmpty(result.Output),
			Error:                    stringPtrIfNotEmpty(result.Error),
			Feedback:                 stringPtrIfNotEmpty(result.Feedback),
			FailureReason:            stringPtrIfNotEmpty(failureReason),
			FailureMessage:           stringPtrIfNotEmpty(failureMessage),
			DurationMillis:           int64Ptr(completed.Duration.Milliseconds()),
			OutputWork:               generatedWorksPtr(outputWorkItems(completed.OutputMutations, completed.ConsumedTokens)),
			OutputResources:          h.generatedOutputResourcesPtr(completed.OutputMutations),
			ProviderFailure:          interfaces.GeneratedProviderFailureMetadata(result.ProviderFailure),
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
	streams := make([]chan factoryapi.FactoryEvent, 0, len(h.streams))
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
		case stream <- event:
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

func generatedFactory(payload interfaces.InitialStructurePayload) factoryapi.Factory {
	resources := generatedResources(payload.Resources)
	workTypes := generatedWorkTypes(payload.WorkTypes)
	workers := generatedWorkers(payload.Workers)
	workstations := generatedWorkstations(payload.Workstations, payload.Places)

	return factoryapi.Factory{
		Resources:    slicePtr(resources),
		WorkTypes:    slicePtr(workTypes),
		Workers:      slicePtr(workers),
		Workstations: slicePtr(workstations),
	}
}

func generatedResources(resources []interfaces.FactoryResource) []factoryapi.Resource {
	out := make([]factoryapi.Resource, 0, len(resources))
	for _, resource := range resources {
		name := resource.Name
		if name == "" {
			name = resource.ID
		}
		out = append(out, factoryapi.Resource{Name: name, Capacity: resource.Capacity})
	}
	return out
}

func generatedWorkTypes(workTypes []interfaces.FactoryWorkType) []factoryapi.WorkType {
	out := make([]factoryapi.WorkType, 0, len(workTypes))
	for _, workType := range workTypes {
		name := workType.Name
		if name == "" {
			name = workType.ID
		}
		states := make([]factoryapi.WorkState, 0, len(workType.States))
		for _, stateDef := range workType.States {
			states = append(states, factoryapi.WorkState{
				Name: stateDef.Value,
				Type: generatedWorkStateType(stateDef.Category),
			})
		}
		out = append(out, factoryapi.WorkType{Name: name, States: states})
	}
	return out
}

func generatedWorkStateType(category string) factoryapi.WorkStateType {
	switch state.StateCategory(category) {
	case state.StateCategoryInitial:
		return factoryapi.WorkStateTypeINITIAL
	case state.StateCategoryTerminal:
		return factoryapi.WorkStateTypeTERMINAL
	case state.StateCategoryFailed:
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

func generatedWorkers(workers []interfaces.FactoryWorker) []factoryapi.Worker {
	out := make([]factoryapi.Worker, 0, len(workers))
	for _, worker := range workers {
		name := worker.Name
		if name == "" {
			name = worker.ID
		}
		out = append(out, factoryapi.Worker{
			Name:             name,
			ExecutorProvider: interfaces.GeneratedPublicFactoryWorkerProviderPtr(worker.Provider),
			ModelProvider:    interfaces.GeneratedPublicFactoryWorkerModelProviderPtr(worker.ModelProvider),
			Model:            stringPtrIfNotEmpty(worker.Model),
			Type:             interfaces.GeneratedPublicFactoryWorkerTypePtr(worker.Config["type"]),
		})
	}
	return out
}

func generatedWorkstations(workstations []interfaces.FactoryWorkstation, places []interfaces.FactoryPlace) []factoryapi.Workstation {
	placesByID := make(map[string]interfaces.FactoryPlace, len(places))
	for _, place := range places {
		placesByID[place.ID] = place
	}
	out := make([]factoryapi.Workstation, 0, len(workstations))
	for _, workstation := range workstations {
		name := workstation.Name
		if name == "" {
			name = workstation.ID
		}
		converted := factoryapi.Workstation{
			Id:          stringPtrIfNotEmpty(workstation.ID),
			Name:        name,
			Worker:      workstation.WorkerID,
			Type:        interfaces.GeneratedPublicFactoryWorkstationTypePtr(workstation.Config["type"]),
			Inputs:      generatedWorkstationIOs(workstation.InputPlaceIDs, placesByID),
			Outputs:     generatedWorkstationIOs(workstation.OutputPlaceIDs, placesByID),
			OnContinue:  generatedWorkstationIOPtr(workstation.ContinuePlaceIDs, placesByID),
			OnRejection: generatedWorkstationIOPtr(workstation.RejectionPlaceIDs, placesByID),
			OnFailure:   generatedWorkstationIOPtr(workstation.FailurePlaceIDs, placesByID),
		}
		if workstation.Kind != "" {
			converted.Behavior = interfaces.GeneratedPublicWorkstationKindPtr(interfaces.WorkstationKind(workstation.Kind))
		}
		out = append(out, converted)
	}
	return out
}

func generatedWorkstationIOs(placeIDs []string, places map[string]interfaces.FactoryPlace) []factoryapi.WorkstationIO {
	out := make([]factoryapi.WorkstationIO, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		place, ok := places[placeID]
		if !ok {
			workType, stateValue := splitPlaceID(placeID)
			place = interfaces.FactoryPlace{TypeID: workType, State: stateValue}
		}
		out = append(out, factoryapi.WorkstationIO{WorkType: place.TypeID, State: place.State})
	}
	return out
}

func generatedWorkstationIOPtr(placeIDs []string, places map[string]interfaces.FactoryPlace) *factoryapi.WorkstationIO {
	ios := generatedWorkstationIOs(placeIDs, places)
	if len(ios) == 0 {
		return nil
	}
	return &ios[0]
}

func splitPlaceID(placeID string) (string, string) {
	before, after, ok := strings.Cut(placeID, ":")
	if !ok {
		return placeID, ""
	}
	return before, after
}

func generatedWorksPtr(items []interfaces.FactoryWorkItem) *[]factoryapi.Work {
	works := generatedWorks(items)
	return slicePtr(works)
}

func generatedWorks(items []interfaces.FactoryWorkItem) []factoryapi.Work {
	out := make([]factoryapi.Work, 0, len(items))
	for _, item := range items {
		out = append(out, generatedWork(item))
	}
	return out
}

func generatedWork(item interfaces.FactoryWorkItem) factoryapi.Work {
	name := item.DisplayName
	if name == "" {
		name = item.ID
	}
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	return factoryapi.Work{
		Name:                     name,
		WorkId:                   stringPtrIfNotEmpty(item.ID),
		WorkTypeName:             stringPtrIfNotEmpty(item.WorkTypeID),
		State:                    stringPtrIfNotEmpty(item.State),
		CurrentChainingTraceId:   stringPtrIfNotEmpty(currentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtr(item.PreviousChainingTraceIDs),
		TraceId:                  stringPtrIfNotEmpty(item.TraceID),
		Tags:                     generatedStringMapPtr(item.Tags),
	}
}

func generatedDispatchConsumedWorkRefsFromTokens(tokens []interfaces.Token) []factoryapi.DispatchConsumedWorkRef {
	out := make([]factoryapi.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		workID := token.Color.WorkID
		if workID == "" {
			workID = token.ID
		}
		if workID == "" {
			continue
		}
		out = append(out, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
	}
	return out
}

func generatedDispatchRequestEventMetadataPtr(replayKey string) *factoryapi.DispatchRequestEventMetadata {
	if replayKey == "" {
		return nil
	}
	return &factoryapi.DispatchRequestEventMetadata{ReplayKey: stringPtrIfNotEmpty(replayKey)}
}

func generatedFactoryRelationsPtr(relations []interfaces.FactoryRelation) *[]factoryapi.Relation {
	out := make([]factoryapi.Relation, 0, len(relations))
	for _, relation := range relations {
		out = append(out, generatedFactoryRelation(relation))
	}
	return slicePtr(out)
}

func generatedFactoryRelation(relation interfaces.FactoryRelation) factoryapi.Relation {
	targetName := relation.TargetWorkName
	if targetName == "" {
		targetName = relation.TargetWorkID
	}
	return factoryapi.Relation{
		Type:           factoryapi.RelationType(relation.Type),
		SourceWorkName: relation.SourceWorkName,
		TargetWorkName: targetName,
		TargetWorkId:   stringPtrIfNotEmpty(relation.TargetWorkID),
		RequiredState:  stringPtrIfNotEmpty(relation.RequiredState),
	}
}

func (h *FactoryEventHistory) generatedResourcesPtr(tokens []interfaces.Token) *[]factoryapi.Resource {
	resources := make([]factoryapi.Resource, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		resources = append(resources, h.generatedResource(token.Color.WorkTypeID))
	}
	return slicePtr(resources)
}

func (h *FactoryEventHistory) generatedOutputResourcesPtr(mutations []interfaces.TokenMutationRecord) *[]factoryapi.Resource {
	resources := make([]factoryapi.Resource, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		resources = append(resources, h.generatedResource(mutation.Token.Color.WorkTypeID))
	}
	return slicePtr(resources)
}

func (h *FactoryEventHistory) generatedResource(resourceID string) factoryapi.Resource {
	resource := factoryapi.Resource{Name: resourceID}
	if h.net != nil && h.net.Resources != nil {
		if def := h.net.Resources[resourceID]; def != nil {
			resource.Name = def.Name
			if resource.Name == "" {
				resource.Name = def.ID
			}
			resource.Capacity = def.Capacity
		}
	}
	return resource
}

func generatedFactoryStatePtr(stateValue interfaces.FactoryState) *factoryapi.FactoryState {
	if stateValue == "" {
		return nil
	}
	converted := factoryapi.FactoryState(stateValue)
	return &converted
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
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), token.Color.PreviousChainingTraceIDs...),
		TraceID:                  token.Color.TraceID,
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
