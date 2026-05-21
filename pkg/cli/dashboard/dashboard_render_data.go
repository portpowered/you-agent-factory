package dashboard

import (
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func dashboardQueueCountViewsFromRenderData(renderData dashboardrender.SimpleDashboardRenderData) []dashboardQueueCountView {
	if len(renderData.PlaceTokenCounts) == 0 {
		return nil
	}
	placeIDs := make([]string, 0, len(renderData.PlaceTokenCounts))
	for placeID, count := range renderData.PlaceTokenCounts {
		if count > 0 {
			placeIDs = append(placeIDs, placeID)
		}
	}
	sort.Strings(placeIDs)

	views := make([]dashboardQueueCountView, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		workTypeID, stateValue := state.SplitPlaceID(placeID)
		views = append(views, dashboardQueueCountView{
			PlaceID:    placeID,
			WorkTypeID: workTypeID,
			StateValue: stateValue,
			TokenCount: renderData.PlaceTokenCounts[placeID],
			WorkLabels: worldWorkItemLabels(workItemsForQueuePlace(renderData, placeID)),
		})
	}
	return views
}

func workItemsForQueuePlace(
	renderData dashboardrender.SimpleDashboardRenderData,
	placeID string,
) []interfaces.FactoryWorldWorkItemRef {
	if refs := renderData.CurrentWorkItemsByPlaceID[placeID]; len(refs) > 0 {
		return refs
	}
	return renderData.PlaceOccupancyWorkItemsByPlaceID[placeID]
}

func dashboardWorkstationActivityViewsFromRenderData(
	renderData dashboardrender.SimpleDashboardRenderData,
) []dashboardWorkstationActivityView {
	if len(renderData.WorkstationActivityByNodeID) == 0 {
		return nil
	}
	nodeIDs := make([]string, 0, len(renderData.WorkstationActivityByNodeID))
	for nodeID := range renderData.WorkstationActivityByNodeID {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)

	views := make([]dashboardWorkstationActivityView, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		activity := renderData.WorkstationActivityByNodeID[nodeID]
		views = append(views, dashboardWorkstationActivityView{
			NodeID:          nodeID,
			WorkstationName: activity.WorkstationName,
			DispatchIDs:     sortedUniqueStrings(activity.ActiveDispatchIDs),
			WorkLabels:      worldWorkItemLabels(activity.ActiveWorkItems),
			TraceIDs:        sortedUniqueStrings(activity.TraceIDs),
		})
	}
	return views
}

func dashboardDispatchHistoryFromRenderData(completed []interfaces.FactoryWorldDispatchCompletion) []dashboardDispatchHistoryView {
	views := make([]dashboardDispatchHistoryView, 0, len(completed))
	for _, dispatch := range completed {
		views = append(views, dashboardDispatchHistoryView{
			DispatchID:      dispatch.DispatchID,
			TransitionID:    dashboardCompatibilityTransitionID(dispatch.TransitionID),
			WorkstationName: displayDispatchWorkstationName(dashboardCompatibilityWorkstationName(dispatch.Workstation.Name, dispatch.TransitionID), dashboardCompatibilityTransitionID(dispatch.TransitionID)),
			Outcome:         dispatch.Result.Outcome,
			StartTime:       dispatch.StartedAt,
			EndTime:         dispatch.CompletedAt,
			Duration:        time.Duration(dispatch.DurationMillis) * time.Millisecond,
			InputLabels:     worldDispatchInputLabels(dispatch),
			OutputLabels:    worldDispatchOutputLabels(dispatch),
			Reason:          worldDispatchReason(dispatch),
		})
	}
	return views
}

func dashboardActiveViewFromRenderData(renderData dashboardrender.SimpleDashboardRenderData) dashboardActiveView {
	entries := make([]dashboardActiveExecutionView, 0, len(renderData.ActiveExecutionsByDispatchID))
	for dispatchID, execution := range renderData.ActiveExecutionsByDispatchID {
		entries = append(entries, dashboardActiveExecutionView{
			DispatchID:      dispatchID,
			TransitionID:    execution.TransitionID,
			WorkstationName: execution.WorkstationName,
			StartedAt:       execution.StartedAt,
			WorkTypeIDs:     activeWorkTypesFromWorldExecution(execution),
			WorkLabels:      activeWorkLabelsFromWorldItems(execution.WorkItems),
		})
	}
	sortActiveExecutionViews(entries)

	activeCount := renderData.InFlightDispatchCount
	if activeCount < len(entries) {
		activeCount = len(entries)
	}
	return dashboardActiveView{Count: activeCount, Entries: entries}
}

func sortActiveExecutionViews(entries []dashboardActiveExecutionView) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].TransitionID != entries[j].TransitionID {
			return entries[i].TransitionID < entries[j].TransitionID
		}
		return entries[i].DispatchID < entries[j].DispatchID
	})
}

func dashboardSessionViewFromRenderData(renderData dashboardrender.SimpleDashboardRenderData) dashboardSessionView {
	session := renderData.Session
	attempts := make([]dashboardProviderSessionView, 0, len(session.ProviderSessions))
	for _, attempt := range session.ProviderSessions {
		attempts = append(attempts, dashboardProviderSessionView{
			DispatchID:      attempt.DispatchID,
			TransitionID:    dashboardCompatibilityTransitionID(attempt.TransitionID),
			WorkstationName: dashboardCompatibilityWorkstationName(attempt.WorkstationName, attempt.TransitionID),
			WorkItems:       worldProviderSessionWorkItems(attempt),
			ProviderSession: cloneProviderSessionMetadata(&attempt.ProviderSession),
		})
	}
	completedWorkItems := worldViewWorkItemsForPlaceCategory(
		renderData.PlaceOccupancyWorkItemsByPlaceID,
		renderData.PlaceCategoriesByID,
		"TERMINAL",
	)
	if len(completedWorkItems) == 0 {
		completedWorkItems = worldViewFallbackWorkItems(
			session.DispatchHistory,
			worldViewFallbackCompletedWorkItemLane,
		)
	}
	failedWorkItems := worldViewWorkItemsForPlaceCategory(
		renderData.PlaceOccupancyWorkItemsByPlaceID,
		renderData.PlaceCategoriesByID,
		"FAILED",
	)
	if len(failedWorkItems) == 0 {
		failedWorkItems = worldViewFallbackWorkItems(
			session.DispatchHistory,
			worldViewFallbackFailedWorkItemLane,
		)
	}
	return dashboardSessionView{
		HasData:              session.HasData,
		DispatchedCount:      session.DispatchedCount,
		CompletedCount:       session.CompletedCount,
		FailedCount:          session.FailedCount,
		DispatchedByWorkType: session.DispatchedByWorkType,
		CompletedByWorkType:  session.CompletedByWorkType,
		FailedByWorkType:     session.FailedByWorkType,
		FailedWorkLabels:     worldWorkItemLabels(failedWorkItems),
		CompletedWorkLabels:  worldWorkItemLabels(completedWorkItems),
		FailedWorkDetails:    dashboardFailedWorkDetailsFromRenderData(session.DispatchHistory, failedWorkItems),
		ProviderSessions:     attempts,
	}
}

func worldViewWorkItemsForPlaceCategory(
	workItemsByPlaceID map[string][]interfaces.FactoryWorldWorkItemRef,
	placeCategories map[string]string,
	category string,
) []interfaces.FactoryWorldWorkItemRef {
	if len(placeCategories) == 0 || len(workItemsByPlaceID) == 0 {
		return nil
	}
	placeIDs := make([]string, 0, len(workItemsByPlaceID))
	for placeID := range workItemsByPlaceID {
		if placeCategories[placeID] == category {
			placeIDs = append(placeIDs, placeID)
		}
	}
	sort.Strings(placeIDs)
	workItemsByID := make(map[string]interfaces.FactoryWorldWorkItemRef)
	for _, placeID := range placeIDs {
		for _, workItem := range workItemsByPlaceID[placeID] {
			if workItem.WorkID == "" {
				continue
			}
			workItemsByID[workItem.WorkID] = workItem
		}
	}
	if len(workItemsByID) == 0 {
		return nil
	}
	workIDs := make([]string, 0, len(workItemsByID))
	for workID := range workItemsByID {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	workItems := make([]interfaces.FactoryWorldWorkItemRef, 0, len(workIDs))
	for _, workID := range workIDs {
		workItems = append(workItems, workItemsByID[workID])
	}
	return workItems
}

type worldViewFallbackWorkItemLane string

const (
	worldViewFallbackCompletedWorkItemLane worldViewFallbackWorkItemLane = "completed"
	worldViewFallbackFailedWorkItemLane    worldViewFallbackWorkItemLane = "failed"
)

func worldViewFallbackWorkItems(
	completions []interfaces.FactoryWorldDispatchCompletion,
	lane worldViewFallbackWorkItemLane,
) []interfaces.FactoryWorldWorkItemRef {
	switch lane {
	case worldViewFallbackCompletedWorkItemLane:
		return collectWorldViewFallbackWorkItems(
			completions,
			interfaces.OutcomeAccepted,
			func(collector *worldViewFallbackWorkItemCollector, completion interfaces.FactoryWorldDispatchCompletion) {
				if collector.addTerminalWork(completion.TerminalWork, func(status string) bool {
					return status != "FAILED"
				}) {
					return
				}
				collector.addWorkItems(completion.OutputWorkItems)
			},
		)
	case worldViewFallbackFailedWorkItemLane:
		return collectWorldViewFallbackWorkItems(
			completions,
			interfaces.OutcomeFailed,
			func(collector *worldViewFallbackWorkItemCollector, completion interfaces.FactoryWorldDispatchCompletion) {
				if collector.addTerminalWork(completion.TerminalWork, func(string) bool {
					return true
				}) {
					return
				}
				collector.addWorkItems(completion.OutputWorkItems)
				collector.addMissingWorkItems(completion.InputWorkItems)
			},
		)
	default:
		return nil
	}
}

type worldViewFallbackWorkItemCollector struct {
	workItemsByID map[string]interfaces.FactoryWorldWorkItemRef
}

func collectWorldViewFallbackWorkItems(
	completions []interfaces.FactoryWorldDispatchCompletion,
	outcome interfaces.WorkOutcome,
	collect func(*worldViewFallbackWorkItemCollector, interfaces.FactoryWorldDispatchCompletion),
) []interfaces.FactoryWorldWorkItemRef {
	collector := worldViewFallbackWorkItemCollector{
		workItemsByID: make(map[string]interfaces.FactoryWorldWorkItemRef),
	}
	for _, completion := range completions {
		if interfaces.WorkOutcome(completion.Result.Outcome) != outcome {
			continue
		}
		collect(&collector, completion)
	}
	return collector.sorted()
}

func (collector *worldViewFallbackWorkItemCollector) addTerminalWork(
	terminalWork *interfaces.FactoryTerminalWork,
	include func(status string) bool,
) bool {
	if terminalWork == nil || terminalWork.WorkItem.ID == "" || !include(terminalWork.Status) {
		return false
	}
	collector.workItemsByID[terminalWork.WorkItem.ID] = workRefForDashboardItem(terminalWork.WorkItem)
	return true
}

func (collector *worldViewFallbackWorkItemCollector) addWorkItems(items []interfaces.FactoryWorkItem) {
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		collector.workItemsByID[item.ID] = workRefForDashboardItem(item)
	}
}

func (collector *worldViewFallbackWorkItemCollector) addMissingWorkItems(items []interfaces.FactoryWorkItem) {
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, ok := collector.workItemsByID[item.ID]; ok {
			continue
		}
		collector.workItemsByID[item.ID] = workRefForDashboardItem(item)
	}
}

func (collector *worldViewFallbackWorkItemCollector) sorted() []interfaces.FactoryWorldWorkItemRef {
	return sortedWorldWorkItemRefs(collector.workItemsByID)
}

func dashboardFailedWorkDetailsFromRenderData(
	completions []interfaces.FactoryWorldDispatchCompletion,
	failedWorkItems []interfaces.FactoryWorldWorkItemRef,
) []dashboardFailedWorkDetail {
	if len(failedWorkItems) == 0 {
		return nil
	}
	completionByWorkID := make(map[string]interfaces.FactoryWorldDispatchCompletion)
	for _, completion := range completions {
		if interfaces.WorkOutcome(completion.Result.Outcome) != interfaces.OutcomeFailed {
			continue
		}
		for _, workID := range worldFailedWorkIDsForDispatch(completion) {
			completionByWorkID[workID] = completion
		}
	}
	out := make([]dashboardFailedWorkDetail, 0, len(failedWorkItems))
	for _, workItem := range failedWorkItems {
		completion, ok := completionByWorkID[workItem.WorkID]
		if !ok {
			out = append(out, dashboardFailedWorkDetail{WorkItem: workItem})
			continue
		}
		out = append(out, dashboardFailedWorkDetail{
			WorkItem:        workItem,
			DispatchID:      completion.DispatchID,
			TransitionID:    dashboardCompatibilityTransitionID(completion.TransitionID),
			WorkstationName: dashboardCompatibilityWorkstationName(completion.Workstation.Name, completion.TransitionID),
			FailureReason:   completion.Result.FailureReason,
			FailureMessage:  completion.Result.FailureMessage,
		})
	}
	return out
}

func worldFailedWorkIDsForDispatch(dispatch interfaces.FactoryWorldDispatchCompletion) []string {
	workIDs := make([]string, 0, len(dispatch.InputWorkItems)+len(dispatch.OutputWorkItems)+len(dispatch.WorkItemIDs)+1)
	if dispatch.TerminalWork != nil && dispatch.TerminalWork.WorkItem.ID != "" {
		workIDs = append(workIDs, dispatch.TerminalWork.WorkItem.ID)
	}
	for _, item := range dispatch.OutputWorkItems {
		workIDs = append(workIDs, item.ID)
	}
	for _, item := range dispatch.InputWorkItems {
		workIDs = append(workIDs, item.ID)
	}
	workIDs = append(workIDs, dispatch.WorkItemIDs...)
	return sortedUniqueStrings(workIDs)
}

func sortedWorldWorkItemRefs(
	workItemsByID map[string]interfaces.FactoryWorldWorkItemRef,
) []interfaces.FactoryWorldWorkItemRef {
	if len(workItemsByID) == 0 {
		return nil
	}
	workIDs := make([]string, 0, len(workItemsByID))
	for workID := range workItemsByID {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	workItems := make([]interfaces.FactoryWorldWorkItemRef, 0, len(workIDs))
	for _, workID := range workIDs {
		workItems = append(workItems, workItemsByID[workID])
	}
	return workItems
}

func cloneProviderSessionMetadata(session *interfaces.ProviderSessionMetadata) *interfaces.ProviderSessionMetadata {
	if session == nil || session.ID == "" {
		return nil
	}
	clone := *session
	return &clone
}

func worldProviderSessionWorkItems(session interfaces.FactoryWorldProviderSessionRecord) []interfaces.FactoryWorldWorkItemRef {
	workItems := make([]interfaces.FactoryWorldWorkItemRef, 0, len(session.ConsumedInputs))
	for _, input := range session.ConsumedInputs {
		if input.WorkItem == nil {
			continue
		}
		workItems = append(workItems, workRefForDashboardItem(*input.WorkItem))
	}
	if len(workItems) > 0 {
		return workItems
	}
	workItems = make([]interfaces.FactoryWorldWorkItemRef, 0, len(session.WorkItemIDs))
	for _, workID := range session.WorkItemIDs {
		if strings.TrimSpace(workID) == "" {
			continue
		}
		workItems = append(workItems, interfaces.FactoryWorldWorkItemRef{WorkID: workID})
	}
	if len(workItems) == 0 {
		return nil
	}
	return workItems
}

func activeWorkTypesFromWorldExecution(execution dashboardrender.SimpleDashboardActiveExecution) []string {
	workTypes := append([]string(nil), execution.WorkTypeIDs...)
	seen := make(map[string]struct{}, len(workTypes))
	for _, workType := range workTypes {
		seen[workType] = struct{}{}
	}
	for _, workItem := range execution.WorkItems {
		if workItem.WorkTypeID == "" {
			continue
		}
		if _, exists := seen[workItem.WorkTypeID]; exists {
			continue
		}
		workTypes = append(workTypes, workItem.WorkTypeID)
		seen[workItem.WorkTypeID] = struct{}{}
	}
	sort.Strings(workTypes)
	return workTypes
}

func activeWorkLabelsFromWorldItems(workItems []interfaces.FactoryWorldWorkItemRef) []string {
	labels := make([]string, 0, len(workItems))
	for _, workItem := range workItems {
		if label := worldWorkItemLabel(workItem); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}
