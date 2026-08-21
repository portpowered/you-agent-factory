package runtime

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var _ factory.Service = (*factoryImpl)(nil)

// GetEngineStateSnapshot returns the aggregate observability snapshot for
// service-facing callers.
func (f *factoryImpl) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	snapshotStartedAt := f.clock.Now()
	runtimeSnap := f.engine.GetRuntimeStateSnapshot()
	engineSnapshotDuration := f.clock.Now().Sub(snapshotStartedAt)
	runtimeSnap.StreamGenerationID = f.eventHistory.StreamGenerationID()

	f.mu.RLock()
	currentState := f.state
	startedAt := f.startedAt
	now := f.clock.Now()
	f.mu.RUnlock()

	worldStateStartedAt := f.clock.Now()
	worldState := f.currentWorldState(runtimeSnap.TickCount)
	worldStateDuration := f.clock.Now().Sub(worldStateStartedAt)
	runtimeSnap.RuntimeStatus = f.deriveRuntimeStatus(currentState, runtimeSnap, worldState)
	uptime := time.Duration(0)
	if !startedAt.IsZero() {
		uptime = now.Sub(startedAt)
	}

	snap := state.NewEngineStateSnapshot(runtimeSnap, string(currentState), uptime, f.topology)
	snap.LifecycleControlStatus = lifecycleControlStatusFromWorldState(worldState, string(currentState))
	enablementStartedAt := f.clock.Now()
	snap.EnabledTransitions = scheduler.NewEnablementEvaluator(
		f.logger,
		f.clock.Now,
		f.cfg.runtimeConfig,
	).FindEnabledTransitionsWithSnapshot(ctx, f.topology, &snap)
	f.logger.Debug(
		"factory runtime state snapshot phases",
		"engine_snapshot_duration_ms", engineSnapshotDuration.Milliseconds(),
		"world_state_duration_ms", worldStateDuration.Milliseconds(),
		"enablement_duration_ms", f.clock.Now().Sub(enablementStartedAt).Milliseconds(),
		"total_duration_ms", f.clock.Now().Sub(snapshotStartedAt).Milliseconds(),
		"token_count", len(runtimeSnap.Marking.Tokens),
		"dispatch_count", len(runtimeSnap.Dispatches),
		"in_flight_count", runtimeSnap.InFlightCount,
		"result_count", len(runtimeSnap.Results),
		"dispatch_history_count", len(runtimeSnap.DispatchHistory),
		"active_throttle_pause_count", len(runtimeSnap.ActiveThrottlePauses),
		"enabled_transition_count", len(snap.EnabledTransitions),
	)
	return &snap, nil
}

// CleanInvocationSnapshot projects the engine-owned invocation facts into the
// narrow transport contract. The raw snapshot remains entirely inside the
// Factory Runtime implementation package.
func (f *factoryImpl) CleanInvocationSnapshot(ctx context.Context) (factory.CleanInvocationSnapshot, error) {
	return cleanInvocationSnapshot(ctx, f.GetEngineStateSnapshot)
}

type cleanInvocationSnapshotReader func(
	context.Context,
) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)

func cleanInvocationSnapshot(
	ctx context.Context,
	read cleanInvocationSnapshotReader,
) (factory.CleanInvocationSnapshot, error) {
	snapshot, err := read(ctx)
	if err != nil {
		return factory.CleanInvocationSnapshot{}, err
	}
	return projectCleanInvocationSnapshot(snapshot), nil
}

func projectCleanInvocationSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) factory.CleanInvocationSnapshot {
	if snapshot == nil {
		return factory.CleanInvocationSnapshot{}
	}
	result := factory.CleanInvocationSnapshot{
		Work:            make([]factory.CleanInvocationWork, 0, len(snapshot.Marking.Tokens)),
		DispatchHistory: make([]factory.CleanInvocationDispatch, 0, len(snapshot.DispatchHistory)),
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		result.Work = append(result.Work, cleanInvocationWorkFromToken(snapshot.Topology, token))
	}
	for _, completion := range snapshot.DispatchHistory {
		projected := factory.CleanInvocationDispatch{
			Outcome: completionOutcome(completion.Outcome),
			Reason:  completion.Reason,
		}
		if completion.FailureMetadata != nil {
			projected.FailureType = string(completion.FailureMetadata.Type)
		}
		projected.Consumed = make([]factory.CleanInvocationWork, 0, len(completion.ConsumedTokens))
		for index := range completion.ConsumedTokens {
			token := factorytoken.FromWorker(completion.ConsumedTokens[index])
			projected.Consumed = append(projected.Consumed, cleanInvocationWorkFromToken(snapshot.Topology, &token))
		}
		projected.Outputs = make([]factory.CleanInvocationWork, 0, len(completion.OutputMutations))
		for _, mutation := range completion.OutputMutations {
			if mutation.Token == nil {
				continue
			}
			token := factorytoken.FromWorker(*mutation.Token)
			projected.Outputs = append(projected.Outputs, cleanInvocationWorkFromToken(snapshot.Topology, &token))
		}
		result.DispatchHistory = append(result.DispatchHistory, projected)
	}
	return result
}

func cleanInvocationWorkFromToken(topology *state.Net, token *factorytoken.Token) factory.CleanInvocationWork {
	if token == nil {
		return factory.CleanInvocationWork{}
	}
	category := factory.StateCategoryProcessing
	if topology != nil {
		category = topology.StateCategoryForPlace(token.PlaceID)
	}
	return factory.CleanInvocationWork{
		WorkID:        token.Color.WorkID,
		WorkTypeID:    token.Color.WorkTypeID,
		StateCategory: string(category),
		Output:        string(token.Color.Payload),
		TraceID:       token.Color.TraceID,
		DataType:      string(token.Color.DataType),
	}
}

func completionOutcome(outcome workerexecution.WorkOutcome) string {
	return string(outcome)
}

// seedRestoredWork copies only recorded Work into a fresh marking. Resource
// tokens have already been generated from the current topology and clock by
// buildRuntimeMarking, so recorded resource occupancy is intentionally not an
// input to this conversion.
func seedRestoredWork(
	marking *petri.Marking,
	net *state.Net,
	restored *interfaces.FactoryWorldState,
	now time.Time,
	resourcePlaceIDs map[string]struct{},
) {
	if marking == nil || net == nil || restored == nil {
		return
	}
	items := restoredWorkItems(restored)
	placements := restoredWorkPlacements(restored, items)
	requestIDs := restoredWorkRequestIDs(restored)
	parentIDs := make(map[string]struct{})

	workIDs := make([]string, 0, len(items))
	for workID := range items {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	for _, workID := range workIDs {
		token, ok := restoredWorkTokenForPlacement(
			marking,
			net,
			restored,
			items[workID],
			placements[workID],
			requestIDs[workID],
			restored.RelationsByWorkID[workID],
			now,
			resourcePlaceIDs,
		)
		if !ok {
			continue
		}
		marking.AddToken(token)
		registerRestoredWorkParent(marking, token, parentIDs)
	}

	for _, parentID := range sortedStringKeys(parentIDs) {
		marking.CompleteParentChildRegistration(parentID)
	}
}

func restoredWorkItems(restored *interfaces.FactoryWorldState) map[string]work.FactoryWorkItem {
	items := make(map[string]work.FactoryWorkItem)
	if restored == nil {
		return items
	}
	mergeRestoredWorkItems(items, restored.WorkItemsByID, false)
	mergeRestoredWorkItems(items, restored.ActiveWorkItemsByID, true)
	mergeRestoredTerminalWork(items, restored.TerminalWorkByID)
	mergeRestoredWorkItems(items, restored.FailedWorkItemsByID, true)
	return items
}

func mergeRestoredWorkItems(
	items map[string]work.FactoryWorkItem,
	source map[string]work.FactoryWorkItem,
	keepExisting bool,
) {
	for workID, item := range source {
		workID, item, ok := normalizedRestoredWorkItem(workID, item)
		if !ok || (keepExisting && hasRestoredWorkItem(items, workID)) {
			continue
		}
		items[workID] = item
	}
}

func mergeRestoredTerminalWork(
	items map[string]work.FactoryWorkItem,
	source map[string]interfaces.FactoryTerminalWork,
) {
	for workID, terminal := range source {
		workID, item, ok := normalizedRestoredWorkItem(workID, terminal.WorkItem)
		if !ok || hasRestoredWorkItem(items, workID) {
			continue
		}
		items[workID] = item
	}
}

func normalizedRestoredWorkItem(
	workID string,
	item work.FactoryWorkItem,
) (string, work.FactoryWorkItem, bool) {
	if workID == "" {
		workID = item.ID
	}
	if workID == "" {
		return "", item, false
	}
	if item.ID == "" {
		item.ID = workID
	}
	return workID, item, true
}

func hasRestoredWorkItem(items map[string]work.FactoryWorkItem, workID string) bool {
	_, exists := items[workID]
	return exists
}

func restoredWorkCount(restored *interfaces.FactoryWorldState) int {
	return len(restoredWorkItems(restored))
}

func restoredWorkPlacements(
	restored *interfaces.FactoryWorldState,
	items map[string]work.FactoryWorkItem,
) map[string]string {
	placements := make(map[string]string)
	if restored == nil {
		return placements
	}
	placeIDs := make([]string, 0, len(restored.PlaceOccupancyByID))
	for placeID := range restored.PlaceOccupancyByID {
		placeIDs = append(placeIDs, placeID)
	}
	sort.Strings(placeIDs)
	for _, placeID := range placeIDs {
		workIDs := append([]string(nil), restored.PlaceOccupancyByID[placeID].WorkItemIDs...)
		sort.Strings(workIDs)
		for _, workID := range workIDs {
			if workID == "" {
				continue
			}
			if _, exists := placements[workID]; !exists {
				placements[workID] = placeID
			}
		}
	}
	if restored.PlaceOccupancyByID == nil {
		for workID, item := range items {
			if item.WorkTypeID != "" && item.State != "" {
				placements[workID] = state.PlaceID(item.WorkTypeID, item.State)
			}
		}
	}
	return placements
}

func restoredWorkTokenForPlacement(
	marking *petri.Marking,
	net *state.Net,
	restored *interfaces.FactoryWorldState,
	item work.FactoryWorkItem,
	placeID string,
	requestID string,
	relations []work.FactoryRelation,
	now time.Time,
	resourcePlaceIDs map[string]struct{},
) (*factorytoken.Token, bool) {
	placeID = restoredWorkPlacementID(restored, item, placeID)
	if !restoredWorkPlacementIsValid(net, placeID, resourcePlaceIDs) {
		return nil, false
	}
	token := restoredWorkToken(item, placeID, requestID, relations, now)
	if token.ID == "" {
		return nil, false
	}
	if _, exists := marking.Tokens[token.ID]; exists {
		// A Work identity must never replace an authoritative current resource
		// token if a historical stream happens to reuse its token ID. Keep the
		// Work ID in Color while allocating an opaque runtime token key.
		token.ID = uniqueRestoredWorkTokenID(marking, token.ID)
	}
	return token, true
}

func restoredWorkPlacementID(
	restored *interfaces.FactoryWorldState,
	item work.FactoryWorkItem,
	placeID string,
) string {
	if placeID != "" || restored.PlaceOccupancyByID != nil {
		return placeID
	}
	if item.WorkTypeID != "" && item.State != "" {
		return state.PlaceID(item.WorkTypeID, item.State)
	}
	return ""
}

func restoredWorkRequestIDs(restored *interfaces.FactoryWorldState) map[string]string {
	requestIDs := make(map[string]string)
	if restored == nil {
		return requestIDs
	}
	requestKeys := make([]string, 0, len(restored.WorkRequestsByID))
	for requestID := range restored.WorkRequestsByID {
		requestKeys = append(requestKeys, requestID)
	}
	sort.Strings(requestKeys)
	for _, requestKey := range requestKeys {
		request := restored.WorkRequestsByID[requestKey]
		requestID := request.RequestID
		if requestID == "" {
			requestID = requestKey
		}
		for _, item := range request.WorkItems {
			if item.ID == "" {
				continue
			}
			if _, exists := requestIDs[item.ID]; !exists {
				requestIDs[item.ID] = requestID
			}
		}
	}
	return requestIDs
}

func restoredWorkPlacementIsValid(
	net *state.Net,
	placeID string,
	resourcePlaceIDs map[string]struct{},
) bool {
	if placeID == "" {
		return false
	}
	if _, isResourcePlace := resourcePlaceIDs[placeID]; isResourcePlace {
		return false
	}
	_, exists := net.Places[placeID]
	return exists
}

func restoredWorkToken(
	item work.FactoryWorkItem,
	placeID string,
	requestID string,
	relations []work.FactoryRelation,
	now time.Time,
) *factorytoken.Token {
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	chainingTraceDepth := item.ChainingTraceDepth
	if chainingTraceDepth == 0 && currentChainingTraceID != "" {
		chainingTraceDepth = 1
	}
	return &factorytoken.Token{
		ID:      item.ID,
		PlaceID: placeID,
		Color: factorytoken.Color{
			Name:                     item.DisplayName,
			RequestID:                requestID,
			WorkID:                   item.ID,
			WorkTypeID:               item.WorkTypeID,
			DataType:                 factorytoken.DataTypeWork,
			ChainingTraceDepth:       chainingTraceDepth,
			CurrentChainingTraceID:   currentChainingTraceID,
			PreviousChainingTraceIDs: work.CanonicalChainingTraceIDs(item.PreviousChainingTraceIDs),
			TraceID:                  item.TraceID,
			ParentID:                 item.ParentID,
			Tags:                     work.CloneTags(item.Tags),
			Relations:                restoredWorkRelations(relations),
			Content:                  work.CloneWorkContentParts(item.Content),
			StructuredResult:         jsonvalue.Clone(item.StructuredResult),
			StructuredResultPresent:  item.StructuredResultPresent,
		},
		CreatedAt: now,
		EnteredAt: now,
		History: factorytoken.History{
			TotalVisits:         make(map[string]int),
			ConsecutiveFailures: make(map[string]int),
			PlaceVisits:         make(map[string]int),
		},
	}
}

func restoredWorkRelations(relations []work.FactoryRelation) []work.Relation {
	if len(relations) == 0 {
		return nil
	}
	converted := make([]work.Relation, len(relations))
	for index, relation := range relations {
		converted[index] = work.Relation{
			Type:          work.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkID,
			RequiredState: relation.RequiredState,
		}
	}
	return converted
}

func registerRestoredWorkParent(
	marking *petri.Marking,
	token *factorytoken.Token,
	parentIDs map[string]struct{},
) {
	if token.Color.ParentID == "" {
		return
	}
	marking.RecordParentChildRegistration(token)
	parentIDs[token.Color.ParentID] = struct{}{}
}

func sortedStringKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func uniqueRestoredWorkTokenID(marking *petri.Marking, workID string) string {
	base := "restored-work:" + workID
	candidate := base
	for suffix := 2; ; suffix++ {
		if _, exists := marking.Tokens[candidate]; !exists {
			return candidate
		}
		candidate = base + ":" + fmt.Sprintf("%d", suffix)
	}
}
