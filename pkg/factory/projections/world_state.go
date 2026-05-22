package projections

import (
	"fmt"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	tokenKindResource = "resource"
	tokenKindWork     = "work"
)

// ReconstructFactoryWorldState applies canonical factory events in tick order
// and returns the reconstructed world state at selectedTick. Events after the
// selected tick are ignored.
func ReconstructFactoryWorldState(events []factoryapi.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
	reducer := newFactoryWorldReducer(selectedTick)
	ordered := append([]factoryapi.FactoryEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]
		if left.Context.Tick != right.Context.Tick {
			return left.Context.Tick < right.Context.Tick
		}
		if left.Context.Sequence != right.Context.Sequence {
			return left.Context.Sequence < right.Context.Sequence
		}
		if !left.Context.EventTime.Equal(right.Context.EventTime) {
			return left.Context.EventTime.Before(right.Context.EventTime)
		}
		return left.Id < right.Id
	})

	for _, event := range ordered {
		if event.Context.Tick > selectedTick {
			continue
		}
		if err := reducer.apply(event); err != nil {
			return interfaces.FactoryWorldState{}, err
		}
	}

	return reducer.state(), nil
}

type factoryWorldReducer struct {
	stateValue   interfaces.FactoryWorldState
	placeTokens  map[string]map[string]struct{}
	tokenPlaces  map[string]string
	tokenWorkIDs map[string]string
	tokenKinds   map[string]string
	placeCats    map[string]string
	workPlaces   map[string]string
}

func newFactoryWorldReducer(selectedTick int) *factoryWorldReducer {
	return &factoryWorldReducer{
		stateValue: interfaces.FactoryWorldState{
			Tick:                          selectedTick,
			WorkRequestsByID:              make(map[string]interfaces.WorkRequestPayload),
			RelationsByWorkID:             make(map[string][]interfaces.FactoryRelation),
			WorkItemsByID:                 make(map[string]interfaces.FactoryWorkItem),
			ActiveWorkItemsByID:           make(map[string]interfaces.FactoryWorkItem),
			TerminalWorkByID:              make(map[string]interfaces.FactoryTerminalWork),
			FailedWorkItemsByID:           make(map[string]interfaces.FactoryWorkItem),
			FailureDetailsByWorkID:        make(map[string]interfaces.FactoryWorldFailureDetail),
			InferenceAttemptsByDispatchID: make(map[string]map[string]interfaces.FactoryWorldInferenceAttempt),
			ScriptRequestsByDispatchID:    make(map[string]map[string]interfaces.FactoryWorldScriptRequest),
			ScriptResponsesByDispatchID:   make(map[string]map[string]interfaces.FactoryWorldScriptResponse),
			PlaceOccupancyByID:            make(map[string]interfaces.FactoryPlaceOccupancy),
			ActiveDispatches:              make(map[string]interfaces.FactoryWorldDispatch),
			TracesByID:                    make(map[string]interfaces.FactoryWorldTrace),
		},
		placeTokens:  make(map[string]map[string]struct{}),
		tokenPlaces:  make(map[string]string),
		tokenWorkIDs: make(map[string]string),
		tokenKinds:   make(map[string]string),
		placeCats:    make(map[string]string),
		workPlaces:   make(map[string]string),
	}
}

func (r *factoryWorldReducer) apply(event factoryapi.FactoryEvent) error {
	r.stateValue.EventTime = event.Context.EventTime
	switch event.Type {
	case factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeFactoryChange:
		return r.applyStructureEvent(event)
	case factoryapi.FactoryEventTypeWorkRequest:
		return r.applyWorkRequestEvent(event)
	case factoryapi.FactoryEventTypeRelationshipChangeRequest:
		return r.applyRelationshipChangeEvent(event)
	case factoryapi.FactoryEventTypeDispatchRequest:
		return r.applyDispatchRequestEvent(event)
	case factoryapi.FactoryEventTypeInferenceRequest:
		return r.applyInferenceRequestEvent(event)
	case factoryapi.FactoryEventTypeInferenceResponse:
		return r.applyInferenceResponseEvent(event)
	case factoryapi.FactoryEventTypeScriptRequest:
		return r.applyScriptRequestEvent(event)
	case factoryapi.FactoryEventTypeScriptResponse:
		return r.applyScriptResponseEvent(event)
	case factoryapi.FactoryEventTypeDispatchResponse:
		return r.applyDispatchResponseEvent(event)
	case factoryapi.FactoryEventTypeFactoryStateResponse:
		return r.applyFactoryStateResponseEvent(event)
	case factoryapi.FactoryEventTypeRunResponse:
		return nil
	}
	return nil
}

func (r *factoryWorldReducer) applyStructureEvent(event factoryapi.FactoryEvent) error {
	switch event.Type {
	case factoryapi.FactoryEventTypeRunRequest:
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			return err
		}
		if !r.hasTopology() {
			r.applyInitialStructure(initialStructureFromGenerated(factoryapi.InitialStructureRequestEventPayload{
				Factory: payload.Factory,
			}))
		}
	case factoryapi.FactoryEventTypeInitialStructureRequest:
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			return err
		}
		r.applyInitialStructure(initialStructureFromGenerated(payload))
	case factoryapi.FactoryEventTypeFactoryChange:
		payload, err := event.Payload.AsFactoryChangeEventPayload()
		if err != nil {
			return err
		}
		r.applyInitialStructure(initialStructureFromGenerated(factoryapi.InitialStructureRequestEventPayload(payload)))
	}
	return nil
}

func (r *factoryWorldReducer) applyWorkRequestEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		return err
	}
	r.applyWorkRequest(event.Context, payload)
	return nil
}

func (r *factoryWorldReducer) applyRelationshipChangeEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		return err
	}
	r.applyRelationshipChange(event.Context, payload)
	return nil
}

func (r *factoryWorldReducer) applyDispatchRequestEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		return err
	}
	r.applyDispatchCreated(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyInferenceRequestEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsInferenceRequestEventPayload()
	if err != nil {
		return err
	}
	r.applyInferenceRequest(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyInferenceResponseEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsInferenceResponseEventPayload()
	if err != nil {
		return err
	}
	r.applyInferenceResponse(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyScriptRequestEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsScriptRequestEventPayload()
	if err != nil {
		return err
	}
	r.applyScriptRequest(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyScriptResponseEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsScriptResponseEventPayload()
	if err != nil {
		return err
	}
	r.applyScriptResponse(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyDispatchResponseEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		return err
	}
	r.applyDispatchCompleted(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyFactoryStateResponseEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsFactoryStateResponseEventPayload()
	if err != nil {
		return err
	}
	r.applyFactoryStateChange(payload)
	return nil
}

func (r *factoryWorldReducer) hasTopology() bool {
	return len(r.stateValue.Topology.Places) > 0 ||
		len(r.stateValue.Topology.Resources) > 0 ||
		len(r.stateValue.Topology.WorkTypes) > 0 ||
		len(r.stateValue.Topology.Workstations) > 0
}

func (r *factoryWorldReducer) applyInitialStructure(payload interfaces.InitialStructurePayload) {
	r.stateValue.Topology = payload
	for _, place := range payload.Places {
		r.placeCats[place.ID] = place.Category
	}
	for _, resource := range payload.Resources {
		r.seedResourceTokens(resource)
	}
}

func (r *factoryWorldReducer) applyWorkRequest(context factoryapi.FactoryEventContext, payload factoryapi.WorkRequestEventPayload) {
	requestID := stringValue(context.RequestId)
	if requestID == "" {
		requestID = firstRequestID(payload.Works)
	}
	if requestID == "" {
		return
	}
	traceID := firstString(context.TraceIds)
	workItems := factoryWorkItemsFromGenerated(payload.Works)
	for i := range workItems {
		if workItems[i].TraceID == "" {
			workItems[i].TraceID = traceID
		}
		if workItems[i].PlaceID == "" {
			workItems[i].PlaceID = r.initialPlaceForWorkType(workItems[i].WorkTypeID)
		}
	}
	r.stateValue.WorkRequestsByID[requestID] = interfaces.WorkRequestPayload{
		RequestID:     requestID,
		Type:          interfaces.WorkRequestType(payload.Type),
		TraceID:       traceID,
		Source:        stringValue(payload.Source),
		ParentLineage: cloneStringSlice(sliceValue(payload.ParentLineage)),
		WorkItems:     cloneWorkItems(workItems),
	}
	for _, item := range workItems {
		r.stateValue.WorkItemsByID[item.ID] = item
		r.stateValue.ActiveWorkItemsByID[item.ID] = item
		r.addWorkToken(item.ID, item.PlaceID, item)
		r.addTraceWork(item.TraceID, item.ID)
	}
	for _, relation := range r.factoryRelationsFromGenerated(payload.Relations, context) {
		r.addRelation(relation)
	}
}

func (r *factoryWorldReducer) applyRelationshipChange(context factoryapi.FactoryEventContext, payload factoryapi.RelationshipChangeRequestEventPayload) {
	r.addRelation(r.factoryRelationFromGenerated(payload.Relation, context))
}

func (r *factoryWorldReducer) applyFactoryStateChange(payload factoryapi.FactoryStateResponseEventPayload) {
	r.stateValue.FactoryStatePrevious = factoryStateString(payload.PreviousState)
	r.stateValue.FactoryState = string(payload.State)
	r.stateValue.FactoryStateReason = stringValue(payload.Reason)
}

func (r *factoryWorldReducer) state() interfaces.FactoryWorldState {
	r.rebuildOccupancy()
	r.sortTraceSlices()
	return r.stateValue
}

func (r *factoryWorldReducer) factoryRelationFromGenerated(relation factoryapi.Relation, context factoryapi.FactoryEventContext) interfaces.FactoryRelation {
	requestItems := r.requestWorkItems(stringValue(context.RequestId))
	targetWorkID := stringValue(relation.TargetWorkId)
	if targetWorkID == "" {
		targetWorkID = workIDForRequestName(requestItems, relation.TargetWorkName)
	}
	sourceWorkID := workIDForRequestName(requestItems, relation.SourceWorkName)
	if sourceWorkID == "" {
		sourceWorkID = sourceWorkIDFromContext(context, targetWorkID)
	}
	return factoryRelationFromGenerated(
		relation,
		stringValue(context.RequestId),
		firstString(context.TraceIds),
		sourceWorkID,
		targetWorkID,
	)
}

func (r *factoryWorldReducer) requestWorkItems(requestID string) []interfaces.FactoryWorkItem {
	if requestID == "" {
		return nil
	}
	return r.stateValue.WorkRequestsByID[requestID].WorkItems
}

func workIDForRequestName(items []interfaces.FactoryWorkItem, workName string) string {
	if workName == "" {
		return ""
	}
	for _, item := range items {
		if item.DisplayName == workName {
			return item.ID
		}
	}
	return ""
}

func sourceWorkIDFromContext(context factoryapi.FactoryEventContext, targetWorkID string) string {
	for _, workID := range sliceValue(context.WorkIds) {
		if workID != "" && workID != targetWorkID {
			return workID
		}
	}
	return ""
}

func (r *factoryWorldReducer) addWorkToken(tokenID string, placeID string, item interfaces.FactoryWorkItem) {
	if tokenID == "" || placeID == "" {
		return
	}
	r.addToken(tokenID, placeID, tokenKindWork)
	r.tokenWorkIDs[tokenID] = item.ID
	r.workPlaces[item.ID] = placeID
	if r.isTerminalPlace(placeID) {
		r.stateValue.TerminalWorkByID[item.ID] = interfaces.FactoryTerminalWork{WorkItem: item, Status: r.placeCats[placeID]}
		delete(r.stateValue.ActiveWorkItemsByID, item.ID)
		r.addTraceTerminal(item.TraceID, item.ID)
	} else if r.isFailedPlace(placeID) {
		r.stateValue.FailedWorkItemsByID[item.ID] = item
		delete(r.stateValue.ActiveWorkItemsByID, item.ID)
		r.addTraceFailed(item.TraceID, item.ID)
	} else {
		r.stateValue.ActiveWorkItemsByID[item.ID] = item
	}
}

func (r *factoryWorldReducer) addToken(tokenID string, placeID string, kind string) {
	r.removeToken(tokenID)
	if r.placeTokens[placeID] == nil {
		r.placeTokens[placeID] = make(map[string]struct{})
	}
	r.placeTokens[placeID][tokenID] = struct{}{}
	r.tokenPlaces[tokenID] = placeID
	r.tokenKinds[tokenID] = kind
}

func (r *factoryWorldReducer) removeToken(tokenID string) {
	if tokenID == "" {
		return
	}
	placeID := r.tokenPlaces[tokenID]
	r.removeTokenFromPlaceIndex(placeID, tokenID)
	delete(r.tokenPlaces, tokenID)
	delete(r.tokenKinds, tokenID)
	delete(r.tokenWorkIDs, tokenID)
}

func (r *factoryWorldReducer) removeTokenFromPlaceIndex(placeID string, tokenID string) {
	if placeID == "" {
		return
	}
	delete(r.placeTokens[placeID], tokenID)
	if len(r.placeTokens[placeID]) == 0 {
		delete(r.placeTokens, placeID)
	}
}

func (r *factoryWorldReducer) seedResourceTokens(resource interfaces.FactoryResource) {
	if resource.ID == "" || resource.Capacity <= 0 {
		return
	}
	placeID := resourceAvailablePlaceID(resource.ID)
	for i := range resource.Capacity {
		r.addToken(resourceTokenID(resource.ID, i), placeID, tokenKindResource)
	}
}

func (r *factoryWorldReducer) consumeResourceUnits(resources *[]factoryapi.Resource) []interfaces.FactoryResourceUnit {
	generated := resourceUnitsFromGenerated(resources)
	if len(generated) == 0 {
		return nil
	}
	consumed := make([]interfaces.FactoryResourceUnit, 0, len(generated))
	for _, resource := range generated {
		tokenID := r.firstAvailableResourceTokenID(resource.ResourceID)
		unit := interfaces.FactoryResourceUnit{
			ResourceID: resource.ResourceID,
			TokenID:    tokenID,
			PlaceID:    resourceAvailablePlaceID(resource.ResourceID),
		}
		if tokenID != "" {
			r.removeToken(tokenID)
		}
		consumed = append(consumed, unit)
	}
	return consumed
}

func (r *factoryWorldReducer) releaseResourceUnits(consumed []interfaces.FactoryResourceUnit, resources *[]factoryapi.Resource) {
	released := make([]bool, len(consumed))
	for _, resource := range resourceUnitsFromGenerated(resources) {
		index := firstConsumedResourceIndex(consumed, released, resource.ResourceID)
		if index < 0 {
			continue
		}
		released[index] = true
		unit := consumed[index]
		if unit.TokenID == "" {
			continue
		}
		placeID := unit.PlaceID
		if placeID == "" {
			placeID = resourceAvailablePlaceID(unit.ResourceID)
		}
		r.addToken(unit.TokenID, placeID, tokenKindResource)
	}
}

func (r *factoryWorldReducer) firstAvailableResourceTokenID(resourceID string) string {
	if resourceID == "" {
		return ""
	}
	tokenIDs := make([]string, 0, len(r.placeTokens[resourceAvailablePlaceID(resourceID)]))
	for tokenID := range r.placeTokens[resourceAvailablePlaceID(resourceID)] {
		if r.tokenKinds[tokenID] == tokenKindResource {
			tokenIDs = append(tokenIDs, tokenID)
		}
	}
	tokenIDs = sortedStrings(tokenIDs)
	if len(tokenIDs) == 0 {
		return ""
	}
	return tokenIDs[0]
}

func firstConsumedResourceIndex(resources []interfaces.FactoryResourceUnit, released []bool, resourceID string) int {
	for i, resource := range resources {
		if released[i] || resource.ResourceID != resourceID {
			continue
		}
		return i
	}
	return -1
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type worldStateWorkerMetadata struct {
	Provider string
	Model    string
}

func resourceAvailablePlaceID(resourceID string) string {
	return generatedPlaceID(resourceID, interfaces.ResourceStateAvailable)
}

func resourceTokenID(resourceID string, index int) string {
	return fmt.Sprintf("%s:resource:%d", resourceID, index)
}

func (r *factoryWorldReducer) initialPlaceForWorkType(workTypeID string) string {
	fallback := ""
	for _, place := range r.stateValue.Topology.Places {
		if !placeMatchesWorkType(place, workTypeID) {
			continue
		}
		if fallback == "" {
			fallback = place.ID
		}
		if place.Category == "INITIAL" {
			return place.ID
		}
	}
	return fallback
}

func (r *factoryWorldReducer) outputPlaceForWork(workstationID string, outcome factoryapi.WorkOutcome, workTypeID string) string {
	workstation, ok := r.topologyWorkstation(workstationID)
	if !ok {
		return ""
	}
	return r.outputPlaceForOutcome(workstation, outcome, workTypeID)
}

func (r *factoryWorldReducer) outputPlaceForOutcome(
	workstation interfaces.FactoryWorkstation,
	outcome factoryapi.WorkOutcome,
	workTypeID string,
) string {
	routes, ok := routedOutputPlaces(workstation, outcome)
	if !ok {
		return ""
	}
	if route := r.matchOutputRoute(routes, workTypeID); route != "" {
		return route
	}
	if outcome == factoryapi.WorkOutcomeFailed {
		return r.failedPlaceForWorkType(workTypeID)
	}
	return ""
}

func routedOutputPlaces(workstation interfaces.FactoryWorkstation, outcome factoryapi.WorkOutcome) ([]string, bool) {
	switch outcome {
	case factoryapi.WorkOutcomeContinue:
		if len(workstation.ContinuePlaceIDs) == 0 {
			return nil, false
		}
		return workstation.ContinuePlaceIDs, true
	case factoryapi.WorkOutcomeRejected:
		if len(workstation.RejectionPlaceIDs) == 0 {
			return nil, false
		}
		return workstation.RejectionPlaceIDs, true
	case factoryapi.WorkOutcomeFailed:
		if len(workstation.FailurePlaceIDs) > 0 {
			return workstation.FailurePlaceIDs, true
		}
	}
	return workstation.OutputPlaceIDs, true
}

func (r *factoryWorldReducer) matchOutputRoute(routes []string, workTypeID string) string {
	for _, placeID := range routes {
		if place, ok := r.topologyPlace(placeID); ok && placeMatchesWorkType(place, workTypeID) {
			return place.ID
		}
		if placeIDMatchesWorkType(placeID, workTypeID) {
			return placeID
		}
	}
	return ""
}

func (r *factoryWorldReducer) failedPlaceForWorkType(workTypeID string) string {
	for _, place := range r.stateValue.Topology.Places {
		if placeMatchesWorkType(place, workTypeID) && place.Category == "FAILED" {
			return place.ID
		}
	}
	return ""
}

func placeMatchesWorkType(place interfaces.FactoryPlace, workTypeID string) bool {
	if place.TypeID == workTypeID {
		return true
	}
	return placeIDMatchesWorkType(place.ID, workTypeID)
}

func placeIDMatchesWorkType(placeID string, workTypeID string) bool {
	if placeID == "" || workTypeID == "" {
		return false
	}
	prefix, _, ok := strings.Cut(placeID, ":")
	if !ok {
		return placeID == workTypeID
	}
	return prefix == workTypeID
}

func (r *factoryWorldReducer) terminalWorkForCompletion(outcome factoryapi.WorkOutcome, workIDs []string) *interfaces.FactoryTerminalWork {
	for _, workID := range sortedStrings(workIDs) {
		item, ok := r.stateValue.WorkItemsByID[workID]
		if !ok || item.PlaceID == "" {
			continue
		}
		category := r.placeCats[item.PlaceID]
		if category == "TERMINAL" || category == "FAILED" || outcome == factoryapi.WorkOutcomeFailed {
			return &interfaces.FactoryTerminalWork{WorkItem: item, Status: category}
		}
	}
	return nil
}

func (r *factoryWorldReducer) rebuildOccupancy() {
	occupancy := make(map[string]interfaces.FactoryPlaceOccupancy, len(r.placeTokens))
	for placeID, tokens := range r.placeTokens {
		entry := interfaces.FactoryPlaceOccupancy{PlaceID: placeID}
		for tokenID := range tokens {
			switch r.tokenKinds[tokenID] {
			case tokenKindResource:
				entry.ResourceTokenIDs = append(entry.ResourceTokenIDs, tokenID)
			default:
				if workID := r.tokenWorkIDs[tokenID]; workID != "" {
					entry.WorkItemIDs = append(entry.WorkItemIDs, workID)
				}
			}
		}
		entry.WorkItemIDs = sortedStrings(entry.WorkItemIDs)
		entry.ResourceTokenIDs = sortedStrings(entry.ResourceTokenIDs)
		entry.TokenCount = len(entry.WorkItemIDs) + len(entry.ResourceTokenIDs)
		occupancy[placeID] = entry
	}
	r.stateValue.PlaceOccupancyByID = occupancy
}

func (r *factoryWorldReducer) isTerminalPlace(placeID string) bool {
	return r.placeCats[placeID] == "TERMINAL"
}

func (r *factoryWorldReducer) isFailedPlace(placeID string) bool {
	return r.placeCats[placeID] == "FAILED"
}

func (r *factoryWorldReducer) addTraceWork(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.WorkItemIDs = appendUnique(trace.WorkItemIDs, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addTraceDispatch(traceID string, dispatchID string) {
	if traceID == "" || dispatchID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.DispatchIDs = appendUnique(trace.DispatchIDs, dispatchID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addTraceTerminal(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.TerminalWork = appendUnique(trace.TerminalWork, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addTraceFailed(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.FailedWorkIDs = appendUnique(trace.FailedWorkIDs, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addRelation(relation interfaces.FactoryRelation) {
	if relation.SourceWorkID == "" || relation.TargetWorkID == "" {
		return
	}
	existing := r.stateValue.RelationsByWorkID[relation.SourceWorkID]
	for _, current := range existing {
		if current.Type == relation.Type &&
			current.TargetWorkID == relation.TargetWorkID &&
			current.RequiredState == relation.RequiredState &&
			current.RequestID == relation.RequestID {
			return
		}
	}
	r.stateValue.RelationsByWorkID[relation.SourceWorkID] = append(existing, relation)
}

func (r *factoryWorldReducer) sortTraceSlices() {
	for traceID, trace := range r.stateValue.TracesByID {
		trace.WorkItemIDs = sortedStrings(trace.WorkItemIDs)
		trace.DispatchIDs = sortedStrings(trace.DispatchIDs)
		trace.TerminalWork = sortedStrings(trace.TerminalWork)
		trace.FailedWorkIDs = sortedStrings(trace.FailedWorkIDs)
		r.stateValue.TracesByID[traceID] = trace
	}
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

func cloneWorkItems(input []interfaces.FactoryWorkItem) []interfaces.FactoryWorkItem {
	if len(input) == 0 {
		return nil
	}
	out := make([]interfaces.FactoryWorkItem, len(input))
	for i, item := range input {
		out[i] = item
		out[i].Tags = cloneStringMap(item.Tags)
		out[i].Content = append([]interfaces.WorkContentPart(nil), item.Content...)
	}
	return out
}

func sortedWorkItems(input []interfaces.FactoryWorkItem) []interfaces.FactoryWorkItem {
	if len(input) == 0 {
		return nil
	}
	out := cloneWorkItems(input)
	sort.Slice(out, func(i, j int) bool {
		if out[i].WorkTypeID != out[j].WorkTypeID {
			return out[i].WorkTypeID < out[j].WorkTypeID
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	deduped := out[:0]
	var previous string
	for i, value := range out {
		if value == "" || (i > 0 && value == previous) {
			previous = value
			continue
		}
		deduped = append(deduped, value)
		previous = value
	}
	return deduped
}

func mergeFactoryWorkItem(existing interfaces.FactoryWorkItem, incoming interfaces.FactoryWorkItem) interfaces.FactoryWorkItem {
	if incoming.ID == "" {
		incoming.ID = existing.ID
	}
	if incoming.WorkTypeID == "" {
		incoming.WorkTypeID = existing.WorkTypeID
	}
	if incoming.State == "" {
		incoming.State = existing.State
	}
	if incoming.DisplayName == "" {
		incoming.DisplayName = existing.DisplayName
	}
	if incoming.ChainingTraceDepth == 0 {
		incoming.ChainingTraceDepth = existing.ChainingTraceDepth
	}
	if incoming.TraceID == "" {
		incoming.TraceID = existing.TraceID
	}
	if incoming.CurrentChainingTraceID == "" {
		incoming.CurrentChainingTraceID = existing.CurrentChainingTraceID
	}
	if incoming.PreviousChainingTraceIDs == nil {
		incoming.PreviousChainingTraceIDs = append([]string(nil), existing.PreviousChainingTraceIDs...)
	}
	if incoming.Content == nil {
		incoming.Content = append([]interfaces.WorkContentPart(nil), existing.Content...)
	}
	if incoming.ParentID == "" {
		incoming.ParentID = existing.ParentID
	}
	if incoming.PlaceID == "" {
		incoming.PlaceID = existing.PlaceID
	}
	if incoming.Tags == nil {
		incoming.Tags = cloneStringMap(existing.Tags)
	}
	return incoming
}

func (r *factoryWorldReducer) transitionIDForDispatch(dispatchID string) string {
	if dispatchID == "" {
		return ""
	}
	if dispatch, ok := r.stateValue.ActiveDispatches[dispatchID]; ok {
		return dispatch.TransitionID
	}
	for _, completion := range r.stateValue.CompletedDispatches {
		if completion.DispatchID == dispatchID {
			return completion.TransitionID
		}
	}
	for _, completion := range r.stateValue.FailedDispatches {
		if completion.DispatchID == dispatchID {
			return completion.TransitionID
		}
	}
	return ""
}

func (r *factoryWorldReducer) workstationRefForTransition(transitionID string) interfaces.FactoryWorkstationRef {
	workstation, ok := r.topologyWorkstation(transitionID)
	if !ok {
		return interfaces.FactoryWorkstationRef{ID: transitionID, Name: transitionID}
	}
	name := workstation.Name
	if name == "" {
		name = workstation.ID
	}
	return interfaces.FactoryWorkstationRef{ID: workstation.ID, Name: name}
}

func (r *factoryWorldReducer) workerForTransition(transitionID string) worldStateWorkerMetadata {
	workstation, ok := r.topologyWorkstation(transitionID)
	if !ok || workstation.WorkerID == "" {
		return worldStateWorkerMetadata{}
	}
	for _, worker := range r.stateValue.Topology.Workers {
		if worker.ID != workstation.WorkerID {
			continue
		}
		return worldStateWorkerMetadata{
			Provider: firstNonEmpty(worker.Provider, worker.ModelProvider),
			Model:    worker.Model,
		}
	}
	return worldStateWorkerMetadata{}
}

func (r *factoryWorldReducer) topologyWorkstation(transitionID string) (interfaces.FactoryWorkstation, bool) {
	for _, workstation := range r.stateValue.Topology.Workstations {
		if workstation.ID == transitionID || workstation.Name == transitionID {
			if workstation.ID == "" {
				workstation.ID = transitionID
			}
			return workstation, true
		}
	}
	return interfaces.FactoryWorkstation{}, false
}

func (r *factoryWorldReducer) topologyPlace(placeID string) (interfaces.FactoryPlace, bool) {
	for _, place := range r.stateValue.Topology.Places {
		if place.ID == placeID {
			return place, true
		}
	}
	return interfaces.FactoryPlace{}, false
}
