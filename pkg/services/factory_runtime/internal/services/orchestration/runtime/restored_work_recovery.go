package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	restoredCompletedAutomationStatus   = "COMPLETED"
	restoredLegacyAutomationDisposition = "completed_automation_without_recoverable_place"
)

type restoredWorkRecovery struct {
	excludedWorkIDs  map[string]struct{}
	toleratedWorkIDs map[string]struct{}
	legacyWorkIDs    map[string]struct{}
}

func restoreRestoredWorkMarking(
	cfg *runtimeConfig,
	marking *petri.Marking,
	constructionNow time.Time,
	resourcePlaceIDs, recordedDispatchWorkIDs map[string]struct{},
) (map[string]struct{}, error) {
	restoredItems := restoredWorkItems(cfg.restoredWorldState)
	recovery := classifyRestoredWorkRecovery(cfg.restoredWorldState, cfg.net, restoredItems)
	excludedWorkIDs := cloneRestoredWorkIDSet(recovery.excludedWorkIDs)
	if cfg.skipRestoredDispatchReconciliation {
		if excludedWorkIDs == nil {
			excludedWorkIDs = make(map[string]struct{}, len(recordedDispatchWorkIDs))
		}
		for workID := range recordedDispatchWorkIDs {
			excludedWorkIDs[workID] = struct{}{}
		}
	}
	seededWorkIDs, err := seedRestoredWork(
		marking,
		cfg.net,
		cfg.restoredWorldState,
		constructionNow,
		resourcePlaceIDs,
		excludedWorkIDs,
		recovery.toleratedWorkIDs,
	)
	if err != nil {
		return nil, err
	}
	logRestoredWorkRecovery(cfg, recovery)
	return seededWorkIDs, nil
}

func classifyRestoredWorkRecovery(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
) restoredWorkRecovery {
	recovery := restoredWorkRecovery{}
	if restored == nil {
		return recovery
	}

	for workID, terminal := range restored.TerminalWorkByID {
		if !isRestoredCompletedAutomation(restored, net, items, workID, terminal.Status) {
			continue
		}
		// A completed cron Work may still be represented by the internal
		// pending-time place when its canonical occupancy was reconstructed
		// from completion facts. It is a historical scheduling marker, not a
		// new live trigger, so never seed it into the successor scheduler.
		recovery.excludedWorkIDs = addRestoredRecoveryWorkID(recovery.excludedWorkIDs, workID)
		if restoredWorkHasRecordedOccupancy(restored, workID) {
			continue
		}
		recovery.toleratedWorkIDs = addRestoredRecoveryWorkID(recovery.toleratedWorkIDs, workID)
	}

	for workID, active := range restored.ActiveWorkItemsByID {
		if !isRestoredLegacyAutomation(restored, net, items, workID, active) {
			continue
		}
		recovery.excludedWorkIDs = addRestoredRecoveryWorkID(recovery.excludedWorkIDs, workID)
		recovery.toleratedWorkIDs = addRestoredRecoveryWorkID(recovery.toleratedWorkIDs, workID)
		recovery.legacyWorkIDs = addRestoredRecoveryWorkID(recovery.legacyWorkIDs, workID)
	}
	return recovery
}

func isRestoredCompletedAutomation(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	workID, status string,
) bool {
	item, exists := items[workID]
	return status == restoredCompletedAutomationStatus && workID != "" && exists &&
		isCronAutomationWork(item) && restoredCronWorkShapeIsCompatible(net, item) &&
		restoredWorkHasConclusiveCompletion(restored, workID)
}

func isRestoredLegacyAutomation(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	workID string,
	active work.FactoryWorkItem,
) bool {
	if workID == "" || restoredWorkHasRecordedOccupancy(restored, workID) {
		return false
	}
	if _, terminal := restored.TerminalWorkByID[workID]; terminal {
		return false
	}
	if _, failed := restored.FailedWorkItemsByID[workID]; failed {
		return false
	}
	item := active
	if indexed, exists := items[workID]; exists {
		item = indexed
	}
	return isCronAutomationWork(item) && restoredCronWorkShapeIsCompatible(net, item) &&
		restoredWorkHasConclusiveCompletion(restored, workID)
}

func restoredWorkHasRecordedOccupancy(restored *interfaces.FactoryWorldState, workID string) bool {
	if restored == nil || workID == "" {
		return false
	}
	for _, occupancy := range restored.PlaceOccupancyByID {
		for _, occupiedWorkID := range occupancy.WorkItemIDs {
			if occupiedWorkID == workID {
				return true
			}
		}
	}
	return false
}

func addRestoredRecoveryWorkID(ids map[string]struct{}, workID string) map[string]struct{} {
	if ids == nil {
		ids = make(map[string]struct{})
	}
	ids[workID] = struct{}{}
	return ids
}

func cloneRestoredWorkIDSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]struct{}, len(source))
	for workID := range source {
		clone[workID] = struct{}{}
	}
	return clone
}

func addRestoredOccupancyPlacements(
	placements map[string]string,
	occupancy map[string]interfaces.FactoryPlaceOccupancy,
) error {
	for _, placeID := range sortedRestoredKeys(occupancy) {
		workIDs := append([]string(nil), occupancy[placeID].WorkItemIDs...)
		sort.Strings(workIDs)
		for _, workID := range workIDs {
			if err := addRestoredPlacement(placements, workID, placeID); err != nil {
				return err
			}
		}
	}
	return nil
}

func addRestoredDispatchPlacements(
	placements map[string]string,
	dispatches map[string]interfaces.FactoryWorldDispatch,
) error {
	for _, dispatchID := range sortedRestoredDispatchIDs(dispatches) {
		for _, input := range dispatches[dispatchID].Inputs {
			workID, ok := restoredDispatchWorkID(input)
			if !ok || input.PlaceID == "" {
				continue
			}
			if err := addRestoredPlacement(placements, workID, input.PlaceID); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoredCompletedDispatchPlacements(
	completions []interfaces.FactoryWorldDispatchCompletion,
	items map[string]work.FactoryWorkItem,
) (map[string]string, error) {
	latest := make(map[string]map[string]struct{})
	for _, completion := range completions {
		current := make(map[string]map[string]struct{})
		touched := make(map[string]struct{}, len(completion.WorkItemIDs))
		for _, workID := range completion.WorkItemIDs {
			if workID != "" {
				touched[workID] = struct{}{}
			}
		}
		for _, item := range completion.OutputWorkItems {
			if item.ID != "" {
				touched[item.ID] = struct{}{}
			}
			addRestoredItemPlacementCandidate(current, item, items)
		}
		if completion.TerminalWork != nil {
			if completion.TerminalWork.WorkItem.ID != "" {
				touched[completion.TerminalWork.WorkItem.ID] = struct{}{}
			}
			addRestoredItemPlacementCandidate(current, completion.TerminalWork.WorkItem, items)
		}
		for workID := range touched {
			latest[workID] = current[workID]
		}
	}
	for _, workID := range sortedRestoredKeys(latest) {
		placeIDs := sortedStringKeys(latest[workID])
		if len(placeIDs) > 1 {
			return nil, fmt.Errorf("restore Work board: Work %q has conflicting completed-dispatch places %q", workID, placeIDs)
		}
		if len(placeIDs) == 1 {
			latest[workID] = map[string]struct{}{placeIDs[0]: {}}
		}
	}
	placements := make(map[string]string, len(latest))
	for workID, placeIDs := range latest {
		for placeID := range placeIDs {
			placements[workID] = placeID
		}
	}
	return placements, nil
}

func addRestoredWorkStateChangePlacements(
	placements map[string]string,
	changes map[string][]interfaces.FactoryWorldWorkStateChangeRecord,
) error {
	for _, workID := range sortedRestoredKeys(changes) {
		records := changes[workID]
		for index := len(records) - 1; index >= 0; index-- {
			placeID := strings.TrimSpace(records[index].ToPlaceID)
			if placeID == "" {
				continue
			}
			if err := addRestoredPlacement(placements, workID, placeID); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func addRestoredItemPlacementCandidate(
	candidates map[string]map[string]struct{},
	item work.FactoryWorkItem,
	items map[string]work.FactoryWorkItem,
) {
	if item.ID == "" {
		return
	}
	if indexed, ok := items[item.ID]; ok {
		if item.WorkTypeID == "" {
			item.WorkTypeID = indexed.WorkTypeID
		}
		if item.State == "" {
			item.State = indexed.State
		}
	}
	placeID := state.PlaceID(item.WorkTypeID, item.State)
	if placeID == "" {
		return
	}
	if candidates[item.ID] == nil {
		candidates[item.ID] = make(map[string]struct{})
	}
	candidates[item.ID][placeID] = struct{}{}
}

func addRestoredItemPlacements(placements map[string]string, items map[string]work.FactoryWorkItem) error {
	for _, workID := range sortedRestoredKeys(items) {
		item := items[workID]
		if err := addRestoredPlacement(placements, workID, state.PlaceID(item.WorkTypeID, item.State)); err != nil {
			return err
		}
	}
	return nil
}

func addRestoredPlacement(placements map[string]string, workID, placeID string) error {
	if workID == "" || placeID == "" {
		return nil
	}
	if existing, exists := placements[workID]; exists {
		if existing != placeID {
			return fmt.Errorf("restore Work board: Work %q has conflicting current places %q and %q", workID, existing, placeID)
		}
		return nil
	}
	placements[workID] = placeID
	return nil
}

func sortedRestoredKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isCronAutomationWork(item work.FactoryWorkItem) bool {
	return interfaces.IsSystemTimeWorkType(item.WorkTypeID) &&
		item.Tags[interfaces.TimeWorkTagKeySource] == interfaces.TimeWorkSourceCron &&
		strings.TrimSpace(item.Tags[interfaces.TimeWorkTagKeyCronWorkstation]) != ""
}

func restoredCronWorkShapeIsCompatible(net *state.Net, item work.FactoryWorkItem) bool {
	if net == nil || item.State != interfaces.SystemTimePendingState {
		return false
	}
	place := net.Places[interfaces.SystemTimePendingPlaceID]
	return place != nil && place.TypeID == interfaces.SystemTimeWorkTypeID && place.State == interfaces.SystemTimePendingState
}

func restoredWorkHasConclusiveCompletion(restored *interfaces.FactoryWorldState, workID string) bool {
	if restored == nil || workID == "" {
		return false
	}
	completed := 0
	for _, completion := range restored.CompletedDispatches {
		if !restoredDispatchContainsWork(completion.WorkItemIDs, workID) {
			continue
		}
		if completion.DispatchID == "" || completion.Result.Outcome != string(workerexecution.OutcomeAccepted) {
			return false
		}
		completed++
	}
	if completed != 1 {
		return false
	}
	for _, completion := range restored.FailedDispatches {
		if restoredDispatchContainsWork(completion.WorkItemIDs, workID) {
			return false
		}
	}
	for _, dispatch := range restored.ActiveDispatches {
		if restoredDispatchContainsWork(dispatch.WorkItemIDs, workID) {
			return false
		}
	}
	return true
}

func restoredDispatchContainsWork(workIDs []string, workID string) bool {
	for _, candidate := range workIDs {
		if candidate == workID {
			return true
		}
	}
	return false
}

func logRestoredWorkRecovery(cfg *runtimeConfig, recovery restoredWorkRecovery) {
	if cfg == nil || len(recovery.legacyWorkIDs) == 0 {
		return
	}
	workIDs := make([]string, 0, len(recovery.legacyWorkIDs))
	for workID := range recovery.legacyWorkIDs {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	logger := logging.EnsureLogger(cfg.logger)
	for _, workID := range workIDs {
		logger.Warn(
			"restore Factory Runtime Work board: skipping completed automation Work without recoverable place",
			"session_id", sessionIDFromFactoryConfig(cfg),
			"recording_id", strings.TrimSpace(cfg.recordingID),
			"work_id", workID,
			"disposition", restoredLegacyAutomationDisposition,
		)
	}
}

func validateRestoredDispatchWorkReferences(
	restored *interfaces.FactoryWorldState,
	items map[string]work.FactoryWorkItem,
) error {
	if restored == nil {
		return nil
	}
	activeDispatchIDs := sortedRestoredDispatchIDs(restored.ActiveDispatches)
	for _, dispatchID := range activeDispatchIDs {
		dispatch := restored.ActiveDispatches[dispatchID]
		if err := validateRestoredActiveDispatch(dispatchID, dispatch, items); err != nil {
			return err
		}
	}
	for index, completion := range restored.CompletedDispatches {
		if err := validateRestoredCompletedDispatch(index, completion, items); err != nil {
			return err
		}
	}
	for index, completion := range restored.FailedDispatches {
		if err := validateRestoredFailedDispatch(index, completion, items); err != nil {
			return err
		}
	}
	approvalIDs := sortedRestoredApprovalIDs(restored.PendingHumanApprovalsByID)
	for _, approvalID := range approvalIDs {
		approval := restored.PendingHumanApprovalsByID[approvalID]
		if err := validateRestoredWorkReferences("pending approval", approvalID, approval.WorkItemIDs, items); err != nil {
			return err
		}
	}
	return nil
}

func validateRestoredActiveDispatch(
	dispatchID string,
	dispatch interfaces.FactoryWorldDispatch,
	items map[string]work.FactoryWorkItem,
) error {
	if err := validateRestoredWorkReferences("active dispatch", dispatchID, dispatch.WorkItemIDs, items); err != nil {
		return err
	}
	for _, input := range dispatch.Inputs {
		workID, ok := restoredDispatchWorkID(input)
		if !ok || workID == "" {
			continue
		}
		if err := validateRestoredWorkReferences("active dispatch", dispatchID, []string{workID}, items); err != nil {
			return err
		}
	}
	return nil
}

func validateRestoredCompletedDispatch(
	index int,
	completion interfaces.FactoryWorldDispatchCompletion,
	items map[string]work.FactoryWorkItem,
) error {
	if err := validateRestoredWorkReferences("completed dispatch", completion.DispatchID, completion.WorkItemIDs, items); err != nil {
		return err
	}
	if err := validateRestoredWorkItems("completed dispatch", completion.DispatchID, completion.InputWorkItems, items); err != nil {
		return err
	}
	if err := validateRestoredWorkItems("completed dispatch", completion.DispatchID, completion.OutputWorkItems, items); err != nil {
		return err
	}
	if completion.DispatchID == "" {
		return fmt.Errorf("restore Work board: completed dispatch at index %d has no dispatch identity", index)
	}
	return nil
}

func validateRestoredFailedDispatch(
	index int,
	completion interfaces.FactoryWorldDispatchCompletion,
	items map[string]work.FactoryWorkItem,
) error {
	if err := validateRestoredWorkReferences("failed dispatch", completion.DispatchID, completion.WorkItemIDs, items); err != nil {
		return err
	}
	if completion.DispatchID == "" {
		return fmt.Errorf("restore Work board: failed dispatch at index %d has no dispatch identity", index)
	}
	return nil
}

func validateRestoredWorkReferences(
	category string,
	identity string,
	workIDs []string,
	items map[string]work.FactoryWorkItem,
) error {
	for _, workID := range workIDs {
		if workID == "" {
			continue
		}
		if _, exists := items[workID]; !exists {
			return fmt.Errorf("restore Work board: %s %q references unknown Work %q", category, identity, workID)
		}
	}
	return nil
}

func validateRestoredWorkItems(
	category string,
	identity string,
	itemsToValidate []work.FactoryWorkItem,
	items map[string]work.FactoryWorkItem,
) error {
	for _, item := range itemsToValidate {
		if item.ID == "" {
			continue
		}
		if err := validateRestoredWorkReferences(category, identity, []string{item.ID}, items); err != nil {
			return err
		}
	}
	return nil
}

func sortedRestoredDispatchIDs(dispatches map[string]interfaces.FactoryWorldDispatch) []string {
	ids := make([]string, 0, len(dispatches))
	for dispatchID := range dispatches {
		ids = append(ids, dispatchID)
	}
	sort.Strings(ids)
	return ids
}

func sortedRestoredApprovalIDs(approvals map[string]interfaces.FactoryWorldHumanApproval) []string {
	ids := make([]string, 0, len(approvals))
	for approvalID := range approvals {
		ids = append(ids, approvalID)
	}
	sort.Strings(ids)
	return ids
}

func recordedDispatchStateMaps(
	world interfaces.FactoryWorldState,
) map[string]interfaces.FactoryWorldDispatchCompletion {
	completed := make(map[string]interfaces.FactoryWorldDispatchCompletion, len(world.CompletedDispatches))
	for _, dispatch := range world.CompletedDispatches {
		completed[dispatch.DispatchID] = dispatch
	}
	for _, dispatch := range world.FailedDispatches {
		completed[dispatch.DispatchID] = dispatch
	}
	return completed
}

func recordedDispatchEnd(
	dispatch interfaces.FactoryWorldDispatchCompletion,
	events []interfaces.FactoryEvent,
	dispatchID string,
) *time.Time {
	ended := dispatch.CompletedAt
	if ended.IsZero() {
		ended = eventTimeForDispatch(events, dispatchID)
	}
	if ended.IsZero() {
		return nil
	}
	ended = ended.UTC()
	return &ended
}

func withRecordedWorkerStreamGeneration(
	subscription workersessions.ObservationSubscription,
	workerSessionID string,
) workersessions.ObservationSubscription {
	if subscription.NextFunc == nil {
		return subscription
	}
	next := subscription.NextFunc
	generationID := recordedWorkerStreamGenerationForIdentity(workerSessionID)
	subscription.NextFunc = func(ctx context.Context) workersessions.ObservationDelivery {
		delivery := next(ctx)
		if delivery.Kind == workersessions.ObservationDeliveryRecord ||
			delivery.Kind == workersessions.ObservationDeliveryTerminal ||
			delivery.Kind == workersessions.ObservationDeliveryTerminalReplay {
			delivery.Event.Cursor.StreamGenerationID = generationID
		}
		return delivery
	}
	return subscription
}

func recordedObservationGeneration(ledger recordings.RuntimeLedger, workerSessionID string, durableHistory bool) string {
	if durableHistory {
		return recordedWorkerStreamGenerationForIdentity(workerSessionID)
	}
	generationID := ""
	if ledger != nil {
		generationID = strings.TrimSpace(ledger.StreamGenerationID())
	}
	return generationID
}

func recordedWorkerStreamGenerationForIdentity(workerSessionID string) string {
	return "worker-recording/" + strings.TrimSpace(workerSessionID)
}

func recordedObservationReplaySummary(
	fact recordedDispatchObservation,
	health workerRecordingHealth,
) *workersessions.ReplaySummary {
	status := health.status
	if status == "" {
		if fact.state.Terminal() {
			status = recordings.WorkerRecordingStatusComplete
		} else {
			status = recordings.WorkerRecordingStatusIncomplete
		}
	}
	reason := "recording-" + strings.ToLower(string(status))
	if health.reason != "" {
		reason = health.reason
	}
	return &workersessions.ReplaySummary{Complete: status == recordings.WorkerRecordingStatusComplete, Reason: reason}
}

func recordedObservationHistoryHasTerminal(events []interfaces.FactoryEvent, dispatchID string) bool {
	for _, event := range events {
		if stringPointerValue(event.Context.DispatchID) == dispatchID && recordedWorkerSessionTerminalEvent(event) {
			return true
		}
	}
	return false
}

type recordedDispatchInterruptionFact struct {
	workIDs       []string
	interruptedAt time.Time
	eventTime     time.Time
	reason        string
}

func recordedDispatchInterruption(
	events []interfaces.FactoryEvent,
	dispatchID string,
) (recordedDispatchInterruptionFact, bool) {
	var fact recordedDispatchInterruptionFact
	found := false
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchInterrupted ||
			stringPointerValue(event.Context.DispatchID) != dispatchID {
			continue
		}
		var payload interfaces.DispatchInterruptedEventPayload
		if json.Unmarshal(event.Payload, &payload) != nil {
			continue
		}
		fact = recordedDispatchInterruptionFact{
			workIDs:       append([]string(nil), pointerStringSlice(event.Context.WorkIDs)...),
			interruptedAt: payload.InterruptedAt,
			eventTime:     event.Context.EventTime,
			reason:        payload.Reason,
		}
		found = true
	}
	return fact, found
}

func durableWorkerCursor(cursor *workersessions.ObservationCursor) bool {
	return cursor != nil && strings.HasPrefix(strings.TrimSpace(cursor.StreamGenerationID), "worker-recording/")
}

func recordedObservationHistory(
	events []interfaces.FactoryEvent,
	dispatchID string,
	cursor *workersessions.ObservationCursor,
) []interfaces.FactoryEvent {
	ordered := make([]interfaces.FactoryEvent, 0, len(events))
	for _, event := range cloneAndSortFactoryEvents(events) {
		if stringPointerValue(event.Context.DispatchID) == dispatchID &&
			(cursor == nil || (event.Context.Sequence > 0 && uint64(event.Context.Sequence) > cursor.Position)) {
			ordered = append(ordered, event)
		}
	}
	return ordered
}

func observationStreamLimit(limit int) int {
	if limit <= 0 {
		return workersessions.DefaultObservationStreamLimit
	}
	return limit
}

func (s *recordedWorkerSessionObservation) readRecordedTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
	fact recordedDispatchObservation,
) (workersessions.ReadTranscriptResult, error) {
	if fact.provider == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	if s.Service != nil {
		live, err := s.Service.ReadTranscript(ctx, req)
		if err == nil {
			return historicalTranscriptResult(fact, live.Entries, req.ProviderSession)
		}
		if errors.Is(err, workersessions.ErrObservationCanceled) {
			return workersessions.ReadTranscriptResult{}, err
		}
	}
	if s.providerSessions == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	projected, err := s.providerSessions.Project(providersessions.ProjectRequest{
		Session: req.ProviderSession.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationCanceled
		}
		if recordedTranscriptSourceUnavailable(err) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
		}
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("%w: %v", workersessions.ErrObservationTranscriptProjectionUnavailable, err)
	}
	return historicalTranscriptResult(fact, recordedTranscriptEntries(projected.Detail.Transcript), req.ProviderSession)
}

func historicalTranscriptResult(
	fact recordedDispatchObservation,
	entries []workersessions.TranscriptEntry,
	ref providers.SessionRef,
) (workersessions.ReadTranscriptResult, error) {
	result := workersessions.ReadTranscriptResult{
		WorkerSessionID: fact.workerSessionID,
		ProviderSession: ref.Clone(),
		WorkIDs:         append([]string(nil), fact.workIDs...),
		TurnID:          fact.turnID,
		AttemptID:       fact.dispatchID,
		State:           fact.state,
		Entries:         entries,
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate historical Worker Session transcript: %w", err)
	}
	return result, nil
}
