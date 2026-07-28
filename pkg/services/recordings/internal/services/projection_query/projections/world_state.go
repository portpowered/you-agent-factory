// backendsizecheck:ignore-file JavaScript world replay remains consolidated with world_state until dedicated projection seams split.
// pkgmaintcheck:ignore-file-lines JavaScript world replay remains consolidated with world_state until dedicated projection seams split.
package projections

import (
	"fmt"
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	tokenKindResource = "resource"
	tokenKindWork     = "work"
)

// ReconstructCanonicalFactoryWorldState applies the Factory-owned event
// envelope directly. Generated event conversion belongs at compatibility and
// transport boundaries, not in the canonical reducer.
func ReconstructCanonicalFactoryWorldState(events []interfaces.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
	reducer := newFactoryWorldReducer(selectedTick)
	ordered := make([]interfaces.FactoryEvent, len(events))
	for index, event := range events {
		ordered[index] = event.Clone()
	}
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
	stateValue             interfaces.FactoryWorldState
	placeTokens            map[string]map[string]struct{}
	tokenPlaces            map[string]string
	tokenWorkIDs           map[string]string
	tokenKinds             map[string]string
	placeCats              map[string]string
	workPlaces             map[string]string
	interruptedDispatchIDs map[string]struct{}
}

func newFactoryWorldReducer(selectedTick int) *factoryWorldReducer {
	return &factoryWorldReducer{
		stateValue: interfaces.FactoryWorldState{
			Tick:                          selectedTick,
			PayloadLineage:                work.WorkPayloadLineageProjection{},
			WorkRequestsByID:              make(map[string]interfaces.WorkRequestPayload),
			RelationsByWorkID:             make(map[string][]work.FactoryRelation),
			WorkItemsByID:                 make(map[string]work.FactoryWorkItem),
			ActiveWorkItemsByID:           make(map[string]work.FactoryWorkItem),
			TerminalWorkByID:              make(map[string]interfaces.FactoryTerminalWork),
			FailedWorkItemsByID:           make(map[string]work.FactoryWorkItem),
			FailureDetailsByWorkID:        make(map[string]interfaces.FactoryWorldFailureDetail),
			InferenceAttemptsByDispatchID: make(map[string]map[string]interfaces.FactoryWorldInferenceAttempt),
			ScriptRequestsByDispatchID:    make(map[string]map[string]interfaces.FactoryWorldScriptRequest),
			ScriptResponsesByDispatchID:   make(map[string]map[string]interfaces.FactoryWorldScriptResponse),
			AgentRunResponsesByDispatchID: make(map[string]map[string]interfaces.FactoryWorldAgentRunResponse),
			PlaceOccupancyByID:            make(map[string]interfaces.FactoryPlaceOccupancy),
			ActiveDispatches:              make(map[string]interfaces.FactoryWorldDispatch),
			TracesByID:                    make(map[string]interfaces.FactoryWorldTrace),
			WorkStateChangesByWorkID:      make(map[string][]interfaces.FactoryWorldWorkStateChangeRecord),
		},
		placeTokens:            make(map[string]map[string]struct{}),
		tokenPlaces:            make(map[string]string),
		tokenWorkIDs:           make(map[string]string),
		tokenKinds:             make(map[string]string),
		placeCats:              make(map[string]string),
		workPlaces:             make(map[string]string),
		interruptedDispatchIDs: make(map[string]struct{}),
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity world-state replay keeps canonical event routing on one reducer switch.
func (r *factoryWorldReducer) apply(event interfaces.FactoryEvent) error {
	r.stateValue.EventTime = event.Context.EventTime
	switch event.Type {
	case interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventTypeFactoryChange:
		return r.applyStructureEvent(event)
	case interfaces.FactoryEventTypeWorkRequest:
		return r.applyWorkRequestEvent(event)
	case interfaces.FactoryEventTypeRelationshipChangeRequest:
		return r.applyRelationshipChangeEvent(event)
	case interfaces.FactoryEventTypeDispatchRequest:
		return r.applyDispatchRequestEvent(event)
	case interfaces.FactoryEventTypeDispatchResponse:
		return r.applyDispatchResponseEvent(event)
	case interfaces.FactoryEventTypeFactoryStateResponse:
		return r.applyFactoryStateResponseEvent(event)
	case interfaces.FactoryEventTypeWorkStateChange:
		return r.applyWorkStateChangeEvent(event)
	case interfaces.FactoryEventTypeRunResponse:
		return nil
	case interfaces.FactoryEventTypeInferenceRequest,
		interfaces.FactoryEventTypeInferenceResponse,
		interfaces.FactoryEventTypeScriptRequest,
		interfaces.FactoryEventTypeScriptResponse,
		interfaces.FactoryEventTypeAgentRunResponse:
		return r.applyWorkerExecutionEvent(event)
	case interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeSessionPaused,
		interfaces.FactoryEventTypeSessionResumed,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionCompleted:
		_, err := r.applySessionLifecycleEvent(event)
		return err
	case interfaces.FactoryEventTypeDispatchQueued,
		interfaces.FactoryEventTypeDispatchInterrupted,
		interfaces.FactoryEventTypeDispatchReconciled:
		_, err := r.applyDispatchLifecycleEvent(event)
		return err
	case interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
		interfaces.FactoryEventTypeJavaScriptCheckpointRef,
		interfaces.FactoryEventTypeJavaScriptPhaseChange,
		interfaces.FactoryEventTypeArtifactCreated:
		_, err := r.applyOrchestratorLifecycleEvent(event)
		return err
	}
	return nil
}

func (r *factoryWorldReducer) applyWorkStateChangeEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.WorkStateChangeEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	r.recordWorkStateChange(event, payload)
	r.applyWorkStateChange(payload)
	return nil
}

func (r *factoryWorldReducer) applyStructureEvent(event interfaces.FactoryEvent) error {
	switch event.Type {
	case interfaces.FactoryEventTypeRunRequest:
		var payload interfaces.RunRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		if !r.hasTopology() {
			if err := r.applyCanonicalFactory(payload.Factory); err != nil {
				return err
			}
		}
	case interfaces.FactoryEventTypeInitialStructureRequest:
		var payload interfaces.InitialStructureRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		if err := r.applyCanonicalFactory(payload.Factory); err != nil {
			return err
		}
	case interfaces.FactoryEventTypeFactoryChange:
		var payload interfaces.FactoryChangeEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		if err := r.applyCanonicalFactory(payload.Factory); err != nil {
			return err
		}
	}
	return nil
}

func (r *factoryWorldReducer) applyWorkRequestEvent(event interfaces.FactoryEvent) error {
	var payload work.WorkRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	if payload.Type == "" {
		return fmt.Errorf("decode %s factory event payload: missing work request type", event.Type)
	}
	r.applyWorkRequest(event.Context, payload)
	return nil
}

func (r *factoryWorldReducer) applyRelationshipChangeEvent(event interfaces.FactoryEvent) error {
	var payload work.RelationshipChangeRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	if payload.Relation.Type == "" {
		return fmt.Errorf("decode %s factory event payload: missing relationship type", event.Type)
	}
	r.applyRelationshipChange(event.Context, payload)
	return nil
}

func (r *factoryWorldReducer) applyDispatchRequestEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.DispatchRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	r.applyDispatchCreated(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyDispatchResponseEvent(event interfaces.FactoryEvent) error {
	var payload workerexecution.DispatchResponseEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	r.applyDispatchCompleted(event, payload)
	return nil
}

func (r *factoryWorldReducer) applyFactoryStateResponseEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.FactoryStateResponseEventPayload
	if err := event.DecodePayload(&payload); err != nil {
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

func (r *factoryWorldReducer) applyCanonicalFactory(snapshot *interfaces.FactorySnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("decode Factory snapshot for world projection: snapshot is required")
	}
	topology, err := initialStructureFromSnapshot(snapshot)
	if err != nil {
		return err
	}
	r.stateValue.Factory = snapshot.Clone()
	r.applyInitialStructure(topology)
	return nil
}

func (r *factoryWorldReducer) applyWorkRequest(context interfaces.FactoryEventContext, payload work.WorkRequestEventPayload) {
	requestID := stringValue(context.RequestID)
	if requestID == "" {
		requestID = firstWorkRequestID(payload.Works)
	}
	if requestID == "" {
		return
	}
	traceID := firstString(context.TraceIDs)
	workItems := factoryWorkItemsFromRequest(payload.Works)
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
		Type:          work.WorkRequestType(payload.Type),
		TraceID:       traceID,
		Source:        payload.Source,
		ParentLineage: cloneStringSlice(payload.ParentLineage),
		WorkItems:     cloneWorkItems(workItems),
	}
	for _, item := range workItems {
		r.stateValue.PayloadLineage.RecordWorkRequestSnapshot(context.Tick, requestID, item)
		r.stateValue.WorkItemsByID[item.ID] = item
		r.stateValue.ActiveWorkItemsByID[item.ID] = item
		r.addWorkToken(item.ID, item.PlaceID, item)
		r.addTraceWork(item.TraceID, item.ID)
	}
	for _, relation := range r.factoryRelationsFromRequest(payload.Relations, context) {
		r.addRelation(relation)
	}
}

func (r *factoryWorldReducer) applyRelationshipChange(context interfaces.FactoryEventContext, payload work.RelationshipChangeRequestEventPayload) {
	r.addRelation(r.factoryRelationFromRequest(payload.Relation, context))
}

func (r *factoryWorldReducer) applyFactoryStateChange(payload interfaces.FactoryStateResponseEventPayload) {
	r.stateValue.FactoryStatePrevious = ""
	if payload.PreviousState != nil {
		r.stateValue.FactoryStatePrevious = string(*payload.PreviousState)
	}
	r.stateValue.FactoryState = string(payload.State)
	r.stateValue.FactoryStateReason = stringValue(payload.Reason)
}

func (r *factoryWorldReducer) state() interfaces.FactoryWorldState {
	r.rebuildOccupancy()
	r.sortTraceSlices()
	return r.stateValue
}

func (r *factoryWorldReducer) factoryRelationFromRequest(relation work.WorkRequestEventRelation, context interfaces.FactoryEventContext) work.FactoryRelation {
	requestItems := r.requestWorkItems(stringValue(context.RequestID))
	targetWorkID := relation.TargetWorkID
	if targetWorkID == "" {
		targetWorkID = workIDForRequestName(requestItems, relation.TargetWorkName)
	}
	sourceWorkID := workIDForRequestName(requestItems, relation.SourceWorkName)
	if sourceWorkID == "" {
		sourceWorkID = sourceWorkIDFromCanonicalContext(context, targetWorkID)
	}
	return factoryRelationFromRequest(
		relation,
		stringValue(context.RequestID),
		firstString(context.TraceIDs),
		sourceWorkID,
		targetWorkID,
	)
}

func (r *factoryWorldReducer) requestWorkItems(requestID string) []work.FactoryWorkItem {
	if requestID == "" {
		return nil
	}
	return r.stateValue.WorkRequestsByID[requestID].WorkItems
}

func workIDForRequestName(items []work.FactoryWorkItem, workName string) string {
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

func sourceWorkIDFromCanonicalContext(context interfaces.FactoryEventContext, targetWorkID string) string {
	for _, workID := range sliceValue(context.WorkIDs) {
		if workID != "" && workID != targetWorkID {
			return workID
		}
	}
	return ""
}

func (r *factoryWorldReducer) addWorkToken(tokenID string, placeID string, item work.FactoryWorkItem) {
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

func (r *factoryWorldReducer) consumeResourceUnits(resources *[]interfaces.DispatchResourceRef) []interfaces.FactoryResourceUnit {
	if resources == nil || len(*resources) == 0 {
		return nil
	}
	consumed := make([]interfaces.FactoryResourceUnit, 0, len(*resources))
	for _, resource := range *resources {
		if resource.Name == "" {
			continue
		}
		tokenID := r.firstAvailableResourceTokenID(resource.Name)
		unit := interfaces.FactoryResourceUnit{
			ResourceID: resource.Name,
			TokenID:    tokenID,
			PlaceID:    resourceAvailablePlaceID(resource.Name),
		}
		if tokenID != "" {
			r.removeToken(tokenID)
		}
		consumed = append(consumed, unit)
	}
	return consumed
}

func (r *factoryWorldReducer) releaseResourceUnits(consumed []interfaces.FactoryResourceUnit, resources *[]workerexecution.DispatchResourceEventRef) {
	released := make([]bool, len(consumed))
	for _, resource := range sliceValue(resources) {
		index := firstConsumedResourceIndex(consumed, released, resource.Name)
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
	return topologyPlaceID(resourceID, interfaces.ResourceStateAvailable)
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

func (r *factoryWorldReducer) outputPlaceForWork(workstationID string, outcome workerexecution.WorkOutcome, workTypeID string) string {
	workstation, ok := r.topologyWorkstation(workstationID)
	if !ok {
		return ""
	}
	return r.outputPlaceForOutcome(workstation, outcome, workTypeID)
}

func (r *factoryWorldReducer) outputPlaceForOutcome(
	workstation interfaces.FactoryWorkstation,
	outcome workerexecution.WorkOutcome,
	workTypeID string,
) string {
	routes, ok := routedOutputPlaces(workstation, outcome)
	if !ok {
		return ""
	}
	if route := r.matchOutputRoute(routes, workTypeID); route != "" {
		return route
	}
	if outcome == workerexecution.OutcomeFailed {
		return r.failedPlaceForWorkType(workTypeID)
	}
	return ""
}

func routedOutputPlaces(workstation interfaces.FactoryWorkstation, outcome workerexecution.WorkOutcome) ([]string, bool) {
	switch outcome {
	case workerexecution.OutcomeContinue:
		if len(workstation.ContinuePlaceIDs) == 0 {
			return nil, false
		}
		return workstation.ContinuePlaceIDs, true
	case workerexecution.OutcomeRejected:
		if len(workstation.RejectionPlaceIDs) == 0 {
			return nil, false
		}
		return workstation.RejectionPlaceIDs, true
	case workerexecution.OutcomeFailed:
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

func (r *factoryWorldReducer) placeForWorkTypeState(workTypeID string, stateValue string) string {
	for _, place := range r.stateValue.Topology.Places {
		if !placeMatchesWorkType(place, workTypeID) {
			continue
		}
		if place.State == stateValue {
			return place.ID
		}
	}
	if workTypeID != "" && stateValue != "" {
		return workTypeID + ":" + stateValue
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

func (r *factoryWorldReducer) terminalWorkForCompletion(outcome workerexecution.WorkOutcome, workIDs []string) *interfaces.FactoryTerminalWork {
	for _, workID := range sortedStrings(workIDs) {
		item, ok := r.stateValue.WorkItemsByID[workID]
		if !ok || item.PlaceID == "" {
			continue
		}
		category := r.placeCats[item.PlaceID]
		if category == "TERMINAL" || category == "FAILED" || outcome == workerexecution.OutcomeFailed {
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

func (r *factoryWorldReducer) removeTraceFailed(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.FailedWorkIDs = removeString(trace.FailedWorkIDs, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) removeTraceTerminal(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TerminalWork = removeString(trace.TerminalWork, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func removeString(values []string, target string) []string {
	if target == "" || len(values) == 0 {
		return values
	}
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *factoryWorldReducer) addRelation(relation work.FactoryRelation) {
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

func cloneWorkItems(input []work.FactoryWorkItem) []work.FactoryWorkItem {
	if len(input) == 0 {
		return nil
	}
	out := make([]work.FactoryWorkItem, len(input))
	for i, item := range input {
		out[i] = item
		out[i].Tags = cloneStringMap(item.Tags)
		out[i].Content = append([]work.WorkContentPart(nil), item.Content...)
	}
	return out
}

func sortedWorkItems(input []work.FactoryWorkItem) []work.FactoryWorkItem {
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

func mergeFactoryWorkItem(existing work.FactoryWorkItem, incoming work.FactoryWorkItem) work.FactoryWorkItem {
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
		incoming.Content = append([]work.WorkContentPart(nil), existing.Content...)
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
func (r *factoryWorldReducer) applyOrchestratorLifecycleEvent(event interfaces.FactoryEvent) (bool, error) {
	switch event.Type {
	case interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorCheckpointWritten:
		_, err := r.applyOrchestratorProgressEvent(event)
		return true, err
	case interfaces.FactoryEventTypeJavaScriptCheckpointRef:
		return true, r.applyJavaScriptCheckpointRefEvent(event)
	case interfaces.FactoryEventTypeJavaScriptPhaseChange:
		return true, r.applyJavaScriptPhaseChangeEvent(event)
	case interfaces.FactoryEventTypeArtifactCreated:
		return true, r.applyArtifactCreatedEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applyJavaScriptCheckpointRefEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.JavaScriptCheckpointRefEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	checkpoint := interfaces.FactorySessionJavaScriptCheckpointRef{
		ID: payload.CheckpointID,
		ArtifactRef: &interfaces.JavaScriptCheckpointArtifactRef{
			ID:         payload.ArtifactRef.ID,
			Kind:       payload.ArtifactRef.Kind,
			Visibility: payload.ArtifactRef.Visibility,
		},
	}
	if payload.ArtifactRef.ContentHash != nil {
		checkpoint.ArtifactRef.ContentHash = *payload.ArtifactRef.ContentHash
	}
	if payload.ArtifactRef.SizeBytes != nil {
		checkpoint.ArtifactRef.SizeBytes = *payload.ArtifactRef.SizeBytes
	}
	if payload.Label != nil {
		checkpoint.Label = *payload.Label
	}
	if payload.Summary != nil {
		checkpoint.Summary = *payload.Summary
	}
	if payload.Timestamp != nil {
		checkpoint.Timestamp = payload.Timestamp.UTC()
	}
	r.stateValue.JavaScriptCheckpoints = append(r.stateValue.JavaScriptCheckpoints, checkpoint)
	if runtime := r.ensureJavaScriptRuntime(); runtime != nil {
		runtime.Checkpoints = append(runtime.Checkpoints, checkpoint)
	}
	return nil
}

func (r *factoryWorldReducer) applyJavaScriptPhaseChangeEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.JavaScriptPhaseChangeEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	runtime := r.ensureJavaScriptRuntime()
	runtime.Phase = payload.Phase
	runtime.Phases = append([]string(nil), payload.Phases...)
	if payload.ArgsDigest != nil {
		runtime.ArgsDigest = *payload.ArgsDigest
	}
	runtime.ScriptStatus = string(payload.ScriptStatus)
	runtime.QueuedDispatches = payload.ChildDispatchCounts.Queued
	runtime.RunningDispatches = payload.ChildDispatchCounts.Running
	runtime.CompletedDispatches = payload.ChildDispatchCounts.Completed
	return nil
}

func (r *factoryWorldReducer) applyArtifactCreatedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.ArtifactCreatedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	artifact := projectArtifactCreatedPayload(payload)
	r.stateValue.Artifacts = append(r.stateValue.Artifacts, artifact)
	if runtime := r.ensureJavaScriptRuntime(); runtime != nil {
		runtime.Artifacts = append(runtime.Artifacts, artifact)
	}
	return nil
}

func (r *factoryWorldReducer) ensureJavaScriptRuntime() *interfaces.FactorySessionJavaScriptRuntimeState {
	if r.stateValue.JavaScriptRuntime == nil {
		r.stateValue.JavaScriptRuntime = &interfaces.FactorySessionJavaScriptRuntimeState{}
	}
	return r.stateValue.JavaScriptRuntime
}

func projectArtifactCreatedPayload(payload interfaces.ArtifactCreatedEventPayload) interfaces.FactorySessionArtifactState {
	artifact := payload.Artifact
	state := interfaces.FactorySessionArtifactState{
		ID:         artifact.ID,
		Kind:       artifact.Kind,
		Visibility: artifact.Visibility,
	}
	if artifact.Label != nil {
		state.Label = *artifact.Label
	}
	if artifact.Summary != nil {
		state.Summary = *artifact.Summary
	}
	if artifact.AuditMode != nil {
		state.AuditMode = string(*artifact.AuditMode)
	}
	if artifact.ContentHash != nil {
		state.ContentHash = *artifact.ContentHash
	}
	if artifact.SizeBytes != nil {
		state.SizeBytes = *artifact.SizeBytes
	}
	if counts := artifactRedactionCountsFromDomain(artifact.RedactionCounts); len(counts) > 0 {
		state.RedactionCounts = counts
	}
	if metadata := artifactCaptureMetadataFromDomain(artifact.CaptureMetadata); len(metadata) > 0 {
		state.CaptureMetadata = metadata
	}
	if payload.CapturedAt != nil {
		state.CapturedAt = payload.CapturedAt.UTC()
	}
	return state
}

func artifactRedactionCountsFromDomain(counts *interfaces.FactoryArtifactRedactionCounts) map[string]int {
	if counts == nil {
		return nil
	}
	redactions := make(map[string]int)
	if counts.Secrets != nil {
		redactions["secrets"] = int(*counts.Secrets)
	}
	if counts.Paths != nil {
		redactions["paths"] = int(*counts.Paths)
	}
	if counts.Tokens != nil {
		redactions["tokens"] = int(*counts.Tokens)
	}
	return redactions
}

func artifactCaptureMetadataFromDomain(metadata *interfaces.FactoryArtifactCaptureMetadata) map[string]string {
	if metadata == nil {
		return nil
	}
	capture := make(map[string]string)
	if metadata.CapturedAt != nil {
		capture["capturedAt"] = metadata.CapturedAt.UTC().Format(time.RFC3339)
	}
	if metadata.SourceDispatchID != nil {
		capture["sourceDispatchId"] = *metadata.SourceDispatchID
	}
	if metadata.MIMEType != nil {
		capture["mimeType"] = *metadata.MIMEType
	}
	return capture
}
