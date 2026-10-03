package runtime

import (
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// Failed cron recovery is deliberately separate from accepted completion. The
// input must have one matched consumption, with no subsequent live ownership.
func restoredConclusiveFailedCronDispatch(restored *interfaces.FactoryWorldState, net *state.Net, workID string, item work.FactoryWorkItem) (string, bool) {
	if workID == "" || item.ID != workID || !isCronAutomationWork(item) ||
		!restoredCronWorkShapeIsCompatible(net, item) || restoredWorkHasRecordedOccupancy(restored, workID) ||
		restoredWorkInActiveDispatches(restored.ActiveDispatches, workID) {
		return "", false
	}
	var candidate interfaces.FactoryWorldDispatchCompletion
	count := 0
	for _, completion := range restored.CompletedDispatches {
		if restoredDispatchCompletionsContainWork([]interfaces.FactoryWorldDispatchCompletion{completion}, workID) {
			candidate = completion
			count++
		}
	}
	if count != 1 || !restoredFailedCronCompletionMatches(candidate, item) {
		return "", false
	}
	transition := net.Transitions[candidate.TransitionID]
	if transition == nil || transition.Name != item.Tags[interfaces.TimeWorkTagKeyCronWorkstation] {
		return "", false
	}
	consumesPending := false
	for _, arc := range transition.InputArcs {
		if arc.PlaceID == interfaces.SystemTimePendingPlaceID && arc.Mode == interfaces.ArcModeConsume {
			consumesPending = true
		}
	}
	if !consumesPending {
		return "", false
	}
	failedCount := 0
	for _, failure := range restored.FailedDispatches {
		if !restoredDispatchContainsWork(failure.WorkItemIDs, workID) {
			continue
		}
		if failure.DispatchID != candidate.DispatchID || failure.TransitionID != candidate.TransitionID ||
			!restoredFailedCronCompletionMatches(failure, item) {
			return "", false
		}
		failedCount++
	}
	if failedCount != 1 {
		return "", false
	}
	for _, move := range restored.WorkStateChangesByWorkID[workID] {
		if !completionFollowsRestoredMove(candidate, move) {
			return "", false
		}
	}
	// The consumed payload ref must still be the latest canonical admission.
	// A later admission (even within the same tick) creates a different ref.
	consumed := restored.PayloadLineage.ResolveConsumedInputSnapshot(candidate.DispatchID, workID)
	if consumed.Status != work.WorkPayloadResolutionResolved || consumed.Snapshot == nil ||
		consumed.Snapshot.WorkID != workID || !isCronAutomationWork(consumed.Snapshot.WorkItem) ||
		consumed.Snapshot.WorkItem.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != item.Tags[interfaces.TimeWorkTagKeyCronWorkstation] ||
		restored.PayloadLineage.LatestSnapshotIDByWorkID[workID] != consumed.Snapshot.SnapshotID {
		return "", false
	}
	return candidate.DispatchID, true
}

func restoredFailedCronCompletionMatches(completion interfaces.FactoryWorldDispatchCompletion, item work.FactoryWorkItem) bool {
	if !restoredDispatchContainsWork(completion.WorkItemIDs, item.ID) || strings.TrimSpace(completion.DispatchID) == "" || strings.TrimSpace(completion.TransitionID) == "" ||
		completion.Result.Outcome != string(workerexecution.OutcomeFailed) ||
		len(completion.OutputWorkItems) == 0 || restoredOutputContainsWork(completion.OutputWorkItems, item.ID) ||
		(completion.TerminalWork != nil && completion.TerminalWork.WorkItem.ID == item.ID) {
		return false
	}
	matched := 0
	for _, input := range completion.ConsumedInputs {
		if input.WorkItem == nil || input.WorkItem.ID != item.ID {
			continue
		}
		if input.TokenID == "" || input.PlaceID != interfaces.SystemTimePendingPlaceID ||
			input.WorkItem.WorkTypeID != item.WorkTypeID || input.WorkItem.State != item.State ||
			!isCronAutomationWork(*input.WorkItem) ||
			input.WorkItem.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != item.Tags[interfaces.TimeWorkTagKeyCronWorkstation] {
			return false
		}
		matched++
	}
	return matched == 1
}

func logRestoredWorkRecovery(cfg *runtimeConfig, recovery restoredWorkRecovery) {
	if cfg == nil {
		return
	}
	logger := logging.EnsureLogger(cfg.logger)
	for _, workID := range sortedRestoredKeys(recovery.failedCronDispatchIDs) {
		logger.Warn("restore Work board: retained consumed automation Work in history",
			"event", "run.restore.disposition",
			"session_id", sessionIDFromFactoryConfig(cfg), "recording_id", strings.TrimSpace(cfg.recordingID),
			"work_id", workID, "dispatch_id", recovery.failedCronDispatchIDs[workID],
			"outcome", string(workerexecution.OutcomeFailed),
			"disposition", "consumed_automation_without_current_placement")
	}
	workIDs := make([]string, 0, len(recovery.legacyWorkIDs))
	for workID := range recovery.legacyWorkIDs {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
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
