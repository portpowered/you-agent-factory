package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/rootobservation"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var _ factory.Service = (*factoryImpl)(nil)

func (f *factoryImpl) Observe(ctx context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	if !validObservationScope(req.Scope) {
		return factory.ObserveResult{}, factory.ErrInvalidObservationScope
	}
	if f == nil {
		return factory.ObserveResult{}, factory.ErrNotRunning
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return factory.ObserveResult{}, err
		}
	}
	f.mu.RLock()
	state := f.state
	startedAt := f.startedAt
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
	default:
		return factory.ObserveResult{}, factory.ErrNotRunning
	}
	if f.engine == nil {
		return factory.ObserveResult{}, factory.ErrNotRunning
	}
	// Runtime observation deliberately reads only the engine-owned detached
	// boundary. GetEngineStateSnapshot also reconstructs canonical world state
	// and evaluates enablement for migration-only callers; neither operation is
	// part of a live status read.
	snapshot := f.engine.GetRuntimeStateSnapshot()
	snapshot.FactoryState = string(state)
	snapshot.Topology = f.topology
	snapshot.RuntimeStatus = f.deriveRuntimeStatus(state, snapshot)
	snapshot.LifecycleControlStatus = string(durableLifecycleStatus(state))
	snapshot.StreamGenerationID = ""
	if f.eventHistory != nil {
		snapshot.StreamGenerationID = f.eventHistory.StreamGenerationID()
	}
	if !startedAt.IsZero() && f.clock != nil {
		snapshot.Uptime = f.clock.Now().Sub(startedAt)
	}
	result := factory.ObserveResult{Observation: rootobservation.Project(&snapshot, req.Scope)}
	f.recordRuntimeObservationMetric(req.Scope)
	return result, nil
}

const runtimeReadObservationMetricName = "factory_runtime.read.observation"

func (f *factoryImpl) recordRuntimeObservationMetric(scope factory.ObservationScope) {
	recorder, ok := f.eventHistory.(interface {
		RecordRuntimeReadMetric(recordings.RuntimeReadMetric)
	})
	if !ok || recorder == nil {
		return
	}
	recorder.RecordRuntimeReadMetric(recordings.RuntimeReadMetric{
		Name: runtimeReadObservationMetricName,
		Labels: map[string]string{
			"scope":                    string(scope),
			"runtime_snapshot_reads":   "1",
			"operation_count":          "1",
			"canonical_history_visits": "0",
			"canonical_events_copied":  "0",
			"full_history_reductions":  "0",
		},
	})
}

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
	runtimeSnap.RuntimeStatus = f.deriveRuntimeStatus(currentState, runtimeSnap)
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

// GetWorkStateSnapshot returns the published runtime boundary needed by the
// Work read adapter. Work list reads do not need enablement, uptime, or the
// unrelated Factory world-state projection; keeping those calculations out of
// this path avoids replaying the complete canonical history for every page.
func (f *factoryImpl) GetWorkStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	if f == nil || f.engine == nil {
		return nil, factory.ErrNotRunning
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	runtimeSnap := f.engine.GetRuntimeStateSnapshot()
	runtimeSnap.Topology = f.topology
	if f.eventHistory != nil {
		runtimeSnap.StreamGenerationID = f.eventHistory.StreamGenerationID()
	}
	f.mu.RLock()
	currentState := f.state
	f.mu.RUnlock()
	runtimeSnap.FactoryState = string(currentState)
	runtimeSnap.RuntimeStatus = f.deriveRuntimeStatus(currentState, runtimeSnap)
	if currentState == interfaces.FactoryStatePaused {
		runtimeSnap.LifecycleControlStatus = string(interfaces.FactorySessionLifecycleStatusPaused)
	}
	return &runtimeSnap, nil
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
	workTypeID, stateValue := state.SplitPlaceID(token.PlaceID)
	if strings.TrimSpace(token.Color.WorkTypeID) != "" {
		workTypeID = token.Color.WorkTypeID
	}
	return factory.CleanInvocationWork{
		WorkID:        token.Color.WorkID,
		Name:          token.Color.Name,
		WorkTypeID:    workTypeID,
		State:         stateValue,
		StateCategory: string(category),
		FailureReason: token.History.LastError,
		Output:        string(token.Color.Payload),
		TraceID:       token.Color.TraceID,
		DataType:      string(token.Color.DataType),
	}
}

func completionOutcome(outcome workerexecution.WorkOutcome) string {
	return string(outcome)
}

func validateRestoredWorkState(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	placements map[string]string,
	resourcePlaceIDs map[string]struct{},
	toleratedWorkIDs map[string]struct{},
) error {
	if restored == nil || net == nil {
		return nil
	}
	if err := validateRestoredWorkSourceIdentities(restored, items); err != nil {
		return err
	}
	if err := validateRestoredDispatchWorkReferences(restored, items); err != nil {
		return err
	}
	occupiedWorkIDs, err := validateRestoredOccupancy(restored, net, items, resourcePlaceIDs)
	if err != nil {
		return err
	}
	if err := validateRestoredPlacementMap(placements, occupiedWorkIDs, net, items, resourcePlaceIDs); err != nil {
		return err
	}
	if err := validateRestoredDispatchInputs(restored, placements, occupiedWorkIDs, items); err != nil {
		return err
	}
	return validateRestoredLiveWork(restored, placements, toleratedWorkIDs)
}

func validateRestoredOccupancy(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	resourcePlaceIDs map[string]struct{},
) (map[string]string, error) {
	occupiedWorkIDs := make(map[string]string, len(restored.PlaceOccupancyByID))
	for placeKey, entry := range restored.PlaceOccupancyByID {
		placeID := strings.TrimSpace(placeKey)
		if placeID == "" {
			return nil, fmt.Errorf("restore Work board: occupancy contains an empty place ID")
		}
		if entry.PlaceID == "" || entry.PlaceID != placeKey {
			return nil, fmt.Errorf(
				"restore Work board: occupancy entry %q has inconsistent place ID %q",
				placeKey,
				entry.PlaceID,
			)
		}
		if _, isResourcePlace := resourcePlaceIDs[placeID]; isResourcePlace {
			if len(entry.WorkItemIDs) > 0 {
				return nil, fmt.Errorf(
					"restore Work board: resource place %q contains Work IDs %#v",
					placeID,
					entry.WorkItemIDs,
				)
			}
			continue
		}
		place, exists := net.Places[placeID]
		if !exists || place == nil {
			return nil, fmt.Errorf(
				"restore Work board: occupied place %q is not present in the current Factory topology",
				placeID,
			)
		}
		for _, workID := range entry.WorkItemIDs {
			if strings.TrimSpace(workID) == "" {
				return nil, fmt.Errorf("restore Work board: place %q contains an empty Work ID", placeID)
			}
			item, exists := items[workID]
			if !exists {
				return nil, fmt.Errorf(
					"restore Work board: place %q references unknown Work %q",
					placeID,
					workID,
				)
			}
			if previousPlace, exists := occupiedWorkIDs[workID]; exists {
				return nil, fmt.Errorf(
					"restore Work board: Work %q is occupied at both %q and %q",
					workID,
					previousPlace,
					placeID,
				)
			}
			occupiedWorkIDs[workID] = placeID
			if err := validateRestoredWorkItemPlacement(workID, item, place); err != nil {
				return nil, err
			}
		}
	}
	return occupiedWorkIDs, nil
}

func validateRestoredPlacementMap(
	placements map[string]string,
	occupiedWorkIDs map[string]string,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	resourcePlaceIDs map[string]struct{},
) error {
	for workID, placeID := range placements {
		if _, exists := occupiedWorkIDs[workID]; exists {
			continue
		}
		place, exists := net.Places[placeID]
		if !exists || place == nil {
			return fmt.Errorf(
				"restore Work board: Work %q references place %q, which is not present in the current Factory topology",
				workID,
				placeID,
			)
		}
		if _, isResourcePlace := resourcePlaceIDs[placeID]; isResourcePlace {
			return fmt.Errorf("restore Work board: Work %q references resource place %q", workID, placeID)
		}
		item, exists := items[workID]
		if !exists {
			return fmt.Errorf("restore Work board: placement references unknown Work %q", workID)
		}
		if err := validateRestoredWorkItemPlacement(workID, item, place); err != nil {
			return err
		}
	}
	return nil
}

func validateRestoredLiveWork(
	restored *interfaces.FactoryWorldState,
	placements map[string]string,
	toleratedWorkIDs map[string]struct{},
) error {
	if err := requireRestoredWorkPlacements("active", restored.ActiveWorkItemsByID, placements, toleratedWorkIDs); err != nil {
		return err
	}
	terminalItems := make(map[string]work.FactoryWorkItem, len(restored.TerminalWorkByID))
	for workID, terminal := range restored.TerminalWorkByID {
		terminalItems[workID] = terminal.WorkItem
	}
	if err := requireRestoredWorkPlacements("terminal", terminalItems, placements, toleratedWorkIDs); err != nil {
		return err
	}
	return requireRestoredWorkPlacements("failed", restored.FailedWorkItemsByID, placements, toleratedWorkIDs)
}

func validateRestoredDispatchInputs(
	restored *interfaces.FactoryWorldState,
	placements map[string]string,
	occupiedWorkIDs map[string]string,
	items map[string]work.FactoryWorkItem,
) error {
	seenDispatchWork := make(map[string]string)
	for dispatchID, dispatch := range restored.ActiveDispatches {
		for _, input := range dispatch.Inputs {
			workID, _ := restoredDispatchWorkID(input)
			if err := validateRestoredDispatchInput(
				dispatchID,
				input,
				placements,
				occupiedWorkIDs,
				items,
				seenDispatchWork,
				restoredWorkIsTerminalOrFailed(restored, workID),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRestoredDispatchInput(
	dispatchID string,
	input interfaces.WorkstationInput,
	placements map[string]string,
	occupiedWorkIDs map[string]string,
	items map[string]work.FactoryWorkItem,
	seenDispatchWork map[string]string,
	superseded bool,
) error {
	workID, ok := restoredDispatchWorkID(input)
	if !ok {
		return nil
	}
	if workID == "" {
		return fmt.Errorf("restore Work board: active dispatch %q contains a Work input without an identity", dispatchID)
	}
	if input.WorkItem != nil && input.WorkItem.ID != "" && input.TokenID != "" && input.TokenID != input.WorkItem.ID {
		return fmt.Errorf(
			"restore Work board: active dispatch %q input token %q does not match Work %q",
			dispatchID,
			input.TokenID,
			input.WorkItem.ID,
		)
	}
	if superseded {
		return nil
	}
	if input.PlaceID == "" {
		return fmt.Errorf("restore Work board: active dispatch %q input for Work %q has no logical place", dispatchID, workID)
	}
	if previousDispatch, exists := seenDispatchWork[workID]; exists {
		return fmt.Errorf(
			"restore Work board: Work %q is referenced by active dispatches %q and %q",
			workID,
			previousDispatch,
			dispatchID,
		)
	}
	seenDispatchWork[workID] = dispatchID
	if occupiedPlace, exists := occupiedWorkIDs[workID]; exists {
		return fmt.Errorf(
			"restore Work board: Work %q is both occupied at %q and consumed by active dispatch %q",
			workID,
			occupiedPlace,
			dispatchID,
		)
	}
	item, exists := items[workID]
	if !exists {
		return fmt.Errorf("restore Work board: active dispatch %q references unknown Work %q", dispatchID, workID)
	}
	placeID, exists := placements[workID]
	if !exists || placeID != input.PlaceID {
		return fmt.Errorf(
			"restore Work board: active dispatch %q places Work %q at %q, want restored place %q",
			dispatchID,
			workID,
			input.PlaceID,
			placeID,
		)
	}
	if item.ID != workID {
		return fmt.Errorf("restore Work board: active dispatch %q references mismatched Work %q", dispatchID, workID)
	}
	return nil
}

func validateRestoredWorkSourceIdentities(
	restored *interfaces.FactoryWorldState,
	items map[string]work.FactoryWorkItem,
) error {
	for workID, item := range items {
		if err := validateRestoredWorkIdentity("Work", workID, item.ID, true); err != nil {
			return err
		}
	}
	for name, source := range map[string]map[string]work.FactoryWorkItem{
		"work":   restored.WorkItemsByID,
		"active": restored.ActiveWorkItemsByID,
		"failed": restored.FailedWorkItemsByID,
	} {
		for workID, item := range source {
			if err := validateRestoredWorkIdentity(name, workID, item.ID, false); err != nil {
				return err
			}
		}
	}
	for workID, terminal := range restored.TerminalWorkByID {
		if err := validateRestoredWorkIdentity("terminal", workID, terminal.WorkItem.ID, false); err != nil {
			return err
		}
	}
	return nil
}

func validateRestoredWorkIdentity(category, workID, itemID string, strict bool) error {
	if strict && (strings.TrimSpace(workID) == "" || strings.TrimSpace(itemID) == "") {
		return fmt.Errorf("restore Work board: Work index key %q does not match Work identity %q", workID, itemID)
	}
	if !strict && strings.TrimSpace(workID) == "" && strings.TrimSpace(itemID) == "" {
		return fmt.Errorf("restore Work board: %s Work entry has no Work identity", category)
	}
	if workID != "" && itemID != "" && workID != itemID {
		return fmt.Errorf(
			"restore Work board: %s Work index key %q does not match Work identity %q",
			category,
			workID,
			itemID,
		)
	}
	return nil
}

func validateRestoredWorkItemPlacement(workID string, item work.FactoryWorkItem, place *petri.Place) error {
	if item.ID != workID {
		return fmt.Errorf(
			"restore Work board: Work index key %q does not match Work identity %q",
			workID,
			item.ID,
		)
	}
	if item.WorkTypeID != "" && place.TypeID != "" && item.WorkTypeID != place.TypeID {
		return fmt.Errorf(
			"restore Work board: Work %q type %q is incompatible with place %q type %q",
			workID,
			item.WorkTypeID,
			place.ID,
			place.TypeID,
		)
	}
	if item.State != "" && place.State != "" && item.State != place.State {
		return fmt.Errorf(
			"restore Work board: Work %q state %q is incompatible with place %q state %q",
			workID,
			item.State,
			place.ID,
			place.State,
		)
	}
	return nil
}

func requireRestoredWorkPlacements(
	category string,
	items map[string]work.FactoryWorkItem,
	placements map[string]string,
	toleratedWorkIDs map[string]struct{},
) error {
	for workID, item := range items {
		if workID == "" {
			workID = item.ID
		}
		if workID == "" {
			return fmt.Errorf("restore Work board: %s Work entry has no Work identity", category)
		}
		if _, tolerated := toleratedWorkIDs[workID]; tolerated {
			continue
		}
		if _, exists := placements[workID]; !exists {
			return fmt.Errorf(
				"restore Work board: %s Work %q has no current place occupancy",
				category,
				workID,
			)
		}
	}
	return nil
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

func restoredWorkPlacements(
	restored *interfaces.FactoryWorldState,
	items map[string]work.FactoryWorkItem,
) (map[string]string, error) {
	placements := make(map[string]string)
	if restored == nil {
		return placements, nil
	}
	if err := addRestoredOccupancyPlacements(placements, restored.PlaceOccupancyByID); err != nil {
		return nil, err
	}
	if err := addRestoredDispatchPlacements(placements, restored.ActiveDispatches, restored); err != nil {
		return nil, err
	}
	if err := addRestoredWorkStateChangePlacements(placements, restored.WorkStateChangesByWorkID); err != nil {
		return nil, err
	}
	completedDispatchPlacements, err := restoredCompletedDispatchPlacements(restored.CompletedDispatches, items)
	if err != nil {
		return nil, err
	}
	for _, workID := range sortedRestoredKeys(completedDispatchPlacements) {
		placeID := completedDispatchPlacements[workID]
		// Completed dispatch output is a historical snapshot. A later canonical
		// Work state change or current occupancy is the authoritative placement
		// when the output describes an intermediate state.
		if _, exists := placements[workID]; !exists {
			placements[workID] = placeID
		}
	}
	if restored.PlaceOccupancyByID == nil {
		if err := addRestoredItemPlacements(placements, items); err != nil {
			return nil, err
		}
	}
	return placements, nil
}

func restoredDispatchWorkID(input interfaces.WorkstationInput) (string, bool) {
	if input.Resource != nil && input.WorkItem == nil {
		return "", false
	}
	if input.WorkItem != nil && input.WorkItem.ID != "" {
		return input.WorkItem.ID, true
	}
	return input.TokenID, true
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
	placeID = restoredWorkPlacementID(restored, net, item, placeID)
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
	net *state.Net,
	item work.FactoryWorkItem,
	placeID string,
) string {
	if placeID != "" {
		return placeID
	}
	if restored != nil && item.ID != "" {
		for _, dispatch := range restored.ActiveDispatches {
			if restoredDispatchIsHumanApproval(restored, net, dispatch) {
				continue
			}
			for _, workID := range dispatch.WorkItemIDs {
				if workID == item.ID && item.WorkTypeID != "" && item.State != "" {
					// An active dispatch has consumed its Work token from
					// occupancy. Keep the recorded Work placement so the
					// interrupted dispatch can be resumed after a restart.
					return state.PlaceID(item.WorkTypeID, item.State)
				}
			}
		}
	}
	if restored != nil && restored.PlaceOccupancyByID != nil {
		return ""
	}
	if item.WorkTypeID != "" && item.State != "" {
		return state.PlaceID(item.WorkTypeID, item.State)
	}
	return ""
}

func restoredDispatchIsHumanApproval(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	dispatch interfaces.FactoryWorldDispatch,
) bool {
	if restored != nil && dispatch.DispatchID != "" {
		for _, approval := range restored.PendingHumanApprovalsByID {
			if approval.DispatchID == dispatch.DispatchID {
				return true
			}
		}
	}
	if net == nil || dispatch.TransitionID == "" {
		return false
	}
	transition := net.Transitions[dispatch.TransitionID]
	return transition != nil && transition.Type == petri.TransitionHumanApproval
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

// orderedRuntimeWorkDispatchTokens preserves the authored workstation input
// order for detached execution. The scheduler intentionally records observed
// child tokens before the consumed parent token; prompt templates and model
// operation bindings, however, address inputs by the workstation's declared
// slots. The legacy workstation executor made this ordering adjustment before
// it built its request, so the shared Workers execution path must do the same.
func orderedRuntimeWorkDispatchTokens(
	cfg *runtimeConfig,
	request workerexecution.WorkstationDispatchRequest,
	invocation *work.InvocationArguments,
) ([]workerexecution.Token, error) {
	tokens := workerexecution.WorkDispatchInputTokens(request.Execution.Dispatch)
	if len(tokens) < 2 {
		return tokens, nil
	}
	lookup, ok := runtimeDefinitionLookup(cfg)
	if !ok {
		return tokens, nil
	}
	workstation, found, err := resolveRuntimeWorkstationDefinition(
		cfg,
		lookup,
		request,
		invocation,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve workstation input order: %w", err)
	}
	if !found || workstation == nil {
		return tokens, nil
	}

	byPlace := make(map[string][]int)
	for index, token := range tokens {
		byPlace[workerTokenPlaceKey(token)] = append(byPlace[workerTokenPlaceKey(token)], index)
	}
	ordered := make([]workerexecution.Token, 0, len(tokens))
	used := make([]bool, len(tokens))
	appendPlaceTokens := func(placeID string) {
		for _, index := range byPlace[placeID] {
			used[index] = true
			ordered = append(ordered, tokens[index])
		}
	}
	for _, input := range workstation.Inputs {
		appendPlaceTokens(fmt.Sprintf("%s:%s", input.WorkTypeName, input.StateName))
	}
	for _, resource := range workstation.Resources {
		appendPlaceTokens(fmt.Sprintf("%s:%s", resource.Name, interfaces.ResourceStateAvailable))
	}
	for index, token := range tokens {
		if used[index] {
			continue
		}
		ordered = append(ordered, token)
	}
	return ordered, nil
}
