package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	restoredCompletedAutomationStatus   = "COMPLETED"
	restoredLegacyAutomationDisposition = "completed_automation_without_recoverable_place"
)

type restoredWorkRecovery struct {
	toleratedWorkIDs map[string]struct{}
	legacyWorkIDs    map[string]struct{}
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
		if terminal.Status != restoredCompletedAutomationStatus || workID == "" {
			continue
		}
		if restoredWorkHasRecordedOccupancy(restored, workID) {
			continue
		}
		item, exists := items[workID]
		if !exists || !isCronAutomationWork(item) || !restoredWorkHasConclusiveCompletion(restored, workID) {
			continue
		}
		recovery.toleratedWorkIDs = addRestoredRecoveryWorkID(recovery.toleratedWorkIDs, workID)
	}

	for workID, active := range restored.ActiveWorkItemsByID {
		if workID == "" {
			continue
		}
		if restoredWorkHasRecordedOccupancy(restored, workID) {
			continue
		}
		if _, terminal := restored.TerminalWorkByID[workID]; terminal {
			continue
		}
		if _, failed := restored.FailedWorkItemsByID[workID]; failed {
			continue
		}
		item := active
		if indexed, exists := items[workID]; exists {
			item = indexed
		}
		if !isCronAutomationWork(item) || !restoredCronWorkShapeIsCompatible(net, item) {
			continue
		}
		if !restoredWorkHasConclusiveCompletion(restored, workID) {
			continue
		}
		recovery.toleratedWorkIDs = addRestoredRecoveryWorkID(recovery.toleratedWorkIDs, workID)
		recovery.legacyWorkIDs = addRestoredRecoveryWorkID(recovery.legacyWorkIDs, workID)
	}
	return recovery
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
			"work_id", workID,
			"disposition", restoredLegacyAutomationDisposition,
		)
	}
}

func validateRestoredWorkConsistency(
	restored *interfaces.FactoryWorldState,
) error {
	if restored == nil {
		return nil
	}
	workIDs := make([]string, 0, len(restored.ActiveWorkItemsByID))
	for workID := range restored.ActiveWorkItemsByID {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	for _, workID := range workIDs {
		if _, terminal := restored.TerminalWorkByID[workID]; terminal {
			return fmt.Errorf("restore Work board: Work %q is both active and terminal", workID)
		}
		if _, failed := restored.FailedWorkItemsByID[workID]; failed {
			return fmt.Errorf("restore Work board: Work %q is both active and failed", workID)
		}
	}
	terminalIDs := make([]string, 0, len(restored.TerminalWorkByID))
	for workID := range restored.TerminalWorkByID {
		terminalIDs = append(terminalIDs, workID)
	}
	sort.Strings(terminalIDs)
	for _, workID := range terminalIDs {
		if _, failed := restored.FailedWorkItemsByID[workID]; failed {
			return fmt.Errorf("restore Work board: Work %q is both terminal and failed", workID)
		}
	}
	return nil
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
	}
	for index, completion := range restored.CompletedDispatches {
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
	}
	for index, completion := range restored.FailedDispatches {
		if err := validateRestoredWorkReferences("failed dispatch", completion.DispatchID, completion.WorkItemIDs, items); err != nil {
			return err
		}
		if completion.DispatchID == "" {
			return fmt.Errorf("restore Work board: failed dispatch at index %d has no dispatch identity", index)
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
