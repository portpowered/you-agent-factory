// Package dashboard provides read models and pretty-print rendering for the
// factory dashboard.
package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// FormatSimpleDashboard renders the snapshot-only dashboard shell. Session
// accounting requires FormatSimpleDashboardWithRenderData.
func FormatSimpleDashboard(
	es interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	topology *state.Net,
	now time.Time,
) string {
	return formatSimpleDashboard(
		es,
		topology,
		now,
		dashboardActiveView{},
		nil,
		nil,
		nil,
		dashboardSessionView{},
	)
}

// FormatSimpleDashboardWithRenderData renders a dashboard using the dedicated
// simple-dashboard render DTO for session accounting.
func FormatSimpleDashboardWithRenderData(
	es interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	renderData dashboardrender.SimpleDashboardRenderData,
	now time.Time,
) string {
	return formatSimpleDashboard(
		es,
		es.Topology,
		now,
		dashboardActiveViewFromRenderData(renderData),
		dashboardQueueCountViewsFromRenderData(renderData),
		dashboardWorkstationActivityViewsFromRenderData(renderData),
		dashboardDispatchHistoryFromRenderData(renderData.Session.DispatchHistory),
		dashboardSessionViewFromRenderData(renderData),
	)
}

func formatSimpleDashboard(
	es interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	topology *state.Net,
	now time.Time,
	active dashboardActiveView,
	queueCounts []dashboardQueueCountView,
	workstationActivity []dashboardWorkstationActivityView,
	completedHistory []dashboardDispatchHistoryView,
	summary dashboardSessionView,
) string {
	if topology == nil {
		topology = es.Topology
	}
	now = now.Local()

	var b strings.Builder

	// Header: factory state and uptime.
	b.WriteString("╔══════════════════════════════════════════╗\n")
	fmt.Fprintf(&b, "║  Factory: %-10s  Runtime: %-8s  ║\n",
		es.FactoryState, es.RuntimeStatus)
	fmt.Fprintf(&b, "║  Uptime:  %-10s  Tick: %-11d  ║\n",
		FormatDuration(es.Uptime.Truncate(time.Second)), es.TickCount)
	b.WriteString("╚══════════════════════════════════════════╝\n")

	renderActiveWorkstations(&b, active, now)
	renderQueueCounts(&b, queueCounts)
	renderWorkstationActivity(&b, workstationActivity)
	renderCompletedWorkstations(&b, completedHistory)

	if summary.HasData {
		renderSessionMetrics(&b, summary, dashboardSessionStartTime(es.Uptime, now))
	}

	return b.String()
}

func renderCompletedWorkstations(b *strings.Builder, completedHistory []dashboardDispatchHistoryView) {
	if len(completedHistory) > 0 {
		b.WriteString("\n")
		b.WriteString("Completed Workstations\n")
		b.WriteString("─────────────────────────────────────────────────────────\n")
		fmt.Fprintf(b, "  %-10s %-20s %-10s %-10s %-8s %-20s %-20s %s\n", "Status", "Workstation", "Started", "Ended", "Duration", "Inputs", "Outputs", "Reason")
		b.WriteString("  ────────────────────────────────────────────────────\n")
		for _, completed := range completedHistory {
			fmt.Fprintf(b, "  %-10s %-20s %-10s %-10s %-8s %-20s %-20s %s\n",
				displayCompletedDispatchStatus(completed.Outcome),
				completed.WorkstationName,
				formatDashboardTime(completed.StartTime),
				formatDashboardTime(completed.EndTime),
				formatDurationShort(completed.Duration),
				displayDashboardLabelList(completed.InputLabels),
				displayDashboardLabelList(completed.OutputLabels),
				displayDashboardReason(completed.Reason))
		}
	}
}

func renderQueueCounts(b *strings.Builder, queueCounts []dashboardQueueCountView) {
	if len(queueCounts) == 0 {
		return
	}

	b.WriteString("\n")
	b.WriteString("Queue Counts\n")
	b.WriteString("─────────────────────────────────────────────────────────\n")
	fmt.Fprintf(b, "  %-20s %-8s %s\n", "Place", "Tokens", "Work")
	b.WriteString("  ────────────────────────────────────────────────────\n")
	for _, queue := range queueCounts {
		fmt.Fprintf(b, "  %-20s %-8d %s\n",
			displayQueuePlace(queue),
			queue.TokenCount,
			displayDashboardLabelList(queue.WorkLabels))
	}
}

func renderWorkstationActivity(b *strings.Builder, activity []dashboardWorkstationActivityView) {
	if len(activity) == 0 {
		return
	}

	b.WriteString("\n")
	b.WriteString("Workstation Activity\n")
	b.WriteString("─────────────────────────────────────────────────────────\n")
	fmt.Fprintf(b, "  %-20s %-20s %-20s %s\n", "Workstation", "Dispatches", "Active Work", "Traces")
	b.WriteString("  ────────────────────────────────────────────────────\n")
	for _, entry := range activity {
		fmt.Fprintf(b, "  %-20s %-20s %-20s %s\n",
			displayDispatchWorkstationName(entry.WorkstationName, entry.NodeID),
			displayStringList(entry.DispatchIDs),
			displayDashboardLabelList(entry.WorkLabels),
			displayStringList(entry.TraceIDs))
	}
}

func renderSessionMetrics(b *strings.Builder, summary dashboardSessionView, startedAt time.Time) {
	b.WriteString("\n")
	b.WriteString("Session Metrics\n")
	b.WriteString("─────────────────────────────────────────\n")
	fmt.Fprintf(b, "  Start Time:     %s\n", formatDashboardTime(startedAt))
	fmt.Fprintf(b, " Workstations Dispatched:  %d%s\n",
		summary.DispatchedCount,
		formatDashboardWorkTypeCounts(summary.DispatchedByWorkType))
	fmt.Fprintf(b, " Workstations Completed:   %d%s\n",
		summary.CompletedCount,
		formatDashboardWorkTypeCounts(summary.CompletedByWorkType))
	fmt.Fprintf(b, " Workstations Failed:      %d%s\n",
		summary.FailedCount,
		formatDashboardWorkTypeCounts(summary.FailedByWorkType))

	if len(summary.FailedWorkDetails) > 0 {
		fmt.Fprintf(b, "  Failed work: %d\n", len(summary.FailedWorkDetails))
		for _, detail := range summary.FailedWorkDetails {
			fmt.Fprintf(b, "    - %s\n", displayDashboardFailedWorkDetail(detail))
		}
	} else if len(summary.FailedWorkLabels) > 0 {
		fmt.Fprintf(b, "  Failed work: %d\n", len(summary.FailedWorkLabels))
		for _, name := range summary.FailedWorkLabels {
			fmt.Fprintf(b, "    - %s\n", name)
		}
	}
	if len(summary.CompletedWorkLabels) > 0 {
		fmt.Fprintf(b, "  Completed work: %d\n", len(summary.CompletedWorkLabels))
		for _, name := range summary.CompletedWorkLabels {
			fmt.Fprintf(b, "    - %s\n", name)
		}
	}
	if len(summary.ProviderSessions) > 0 {
		b.WriteString("  Provider sessions:\n")
		for _, attempt := range summary.ProviderSessions {
			fmt.Fprintf(b, "    - %s [%s] %s\n",
				displayDashboardProviderSessionView(attempt),
				attempt.DispatchID,
				formatProviderSession(attempt.ProviderSession),
			)
		}
	}
}

func renderActiveWorkstations(b *strings.Builder, active dashboardActiveView, now time.Time) {
	if active.Count == 0 {
		return
	}

	b.WriteString("\n")
	fmt.Fprintf(b, "Active Workstations (%d)\n", active.Count)
	b.WriteString("─────────────────────────────────────────────────────────\n")
	fmt.Fprintf(b, "  %-18s %-20s %-10s %-8s %s\n", "Work Types", "Workstation", "Started", "Elapsed", "Name")
	b.WriteString("  ────────────────────────────────────────────────────\n")
	for _, entry := range active.Entries {
		fmt.Fprintf(b, "  %-18s %-20s %-10s %-8s %s\n",
			displayStringList(entry.WorkTypeIDs),
			displayDispatchWorkstationName(entry.WorkstationName, entry.TransitionID),
			formatDashboardTime(entry.StartedAt),
			formatDurationShort(now.Sub(entry.StartedAt)),
			displayStringList(entry.WorkLabels))
	}
}

type dashboardActiveView struct {
	Count   int
	Entries []dashboardActiveExecutionView
}

type dashboardActiveExecutionView struct {
	DispatchID      string
	TransitionID    string
	WorkstationName string
	StartedAt       time.Time
	WorkTypeIDs     []string
	WorkLabels      []string
}

type dashboardDispatchHistoryView struct {
	DispatchID      string
	TransitionID    string
	WorkstationName string
	Outcome         string
	StartTime       time.Time
	EndTime         time.Time
	Duration        time.Duration
	InputLabels     []string
	OutputLabels    []string
	Reason          string
}

type dashboardQueueCountView struct {
	PlaceID    string
	WorkTypeID string
	StateValue string
	TokenCount int
	WorkLabels []string
}

type dashboardWorkstationActivityView struct {
	NodeID          string
	WorkstationName string
	DispatchIDs     []string
	WorkLabels      []string
	TraceIDs        []string
}

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

type dashboardSessionView struct {
	HasData              bool
	DispatchedCount      int
	CompletedCount       int
	FailedCount          int
	DispatchedByWorkType map[string]int
	CompletedByWorkType  map[string]int
	FailedByWorkType     map[string]int
	FailedWorkLabels     []string
	CompletedWorkLabels  []string
	FailedWorkDetails    []dashboardFailedWorkDetail
	ProviderSessions     []dashboardProviderSessionView
}

type dashboardFailedWorkDetail struct {
	WorkItem        interfaces.FactoryWorldWorkItemRef
	DispatchID      string
	TransitionID    string
	WorkstationName string
	FailureReason   string
	FailureMessage  string
}

type dashboardProviderSessionView struct {
	DispatchID      string
	TransitionID    string
	WorkstationName string
	WorkItems       []interfaces.FactoryWorldWorkItemRef
	ProviderSession *interfaces.ProviderSessionMetadata
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

func (collector *worldViewFallbackWorkItemCollector) addWorkItems(
	items []interfaces.FactoryWorkItem,
) {
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		collector.workItemsByID[item.ID] = workRefForDashboardItem(item)
	}
}

func (collector *worldViewFallbackWorkItemCollector) addMissingWorkItems(
	items []interfaces.FactoryWorkItem,
) {
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

// formatDurationShort formats a duration compactly: "1m2s", "2h5m", "500ms".
func formatDurationShort(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}

func formatDashboardTime(value time.Time) string {
	if value.IsZero() {
		return "n/a"
	}
	return value.Local().Format("15:04:05")
}

func dashboardSessionStartTime(uptime time.Duration, now time.Time) time.Time {
	if uptime > 0 {
		return now.Add(-uptime)
	}
	return now
}

// FormatDuration formats a duration as "Xm" or "Xh Ym".
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return "0m"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func formatDashboardWorkTypeCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}

	workTypes := make([]string, 0, len(counts))
	for workType := range counts {
		workTypes = append(workTypes, workType)
	}
	sort.Strings(workTypes)

	parts := make([]string, 0, len(workTypes))
	for _, workType := range workTypes {
		parts = append(parts, fmt.Sprintf("%s=%d", workType, counts[workType]))
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}

func displayCompletedDispatchStatus(outcome string) string {
	switch interfaces.WorkOutcome(outcome) {
	case interfaces.OutcomeAccepted:
		return "Success"
	case interfaces.OutcomeContinue:
		return "Continue"
	case interfaces.OutcomeRejected:
		return "Rejected"
	case interfaces.OutcomeFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

func displayDashboardLabelList(labels []string) string {
	if len(labels) == 0 {
		return "n/a"
	}
	return strings.Join(labels, ", ")
}

func displayStringList(values []string) string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		filtered = append(filtered, value)
	}
	if len(filtered) == 0 {
		return "n/a"
	}
	return strings.Join(filtered, ", ")
}

func displayDispatchWorkstationName(workstationName, transitionID string) string {
	if workstationName != "" {
		return workstationName
	}
	if transitionID != "" {
		return transitionID
	}
	return "n/a"
}

func displayDashboardReason(value string) string {
	reason := strings.TrimSpace(value)
	if reason == "" {
		return "-"
	}
	return reason
}

func displayQueuePlace(queue dashboardQueueCountView) string {
	if queue.WorkTypeID == "" || queue.StateValue == "" {
		return queue.PlaceID
	}
	return queue.WorkTypeID + ":" + queue.StateValue
}

func worldDispatchInputLabels(dispatch interfaces.FactoryWorldDispatchCompletion) []string {
	labels := worldWorkItemLabelsFromItems(dispatch.InputWorkItems)
	if len(labels) > 0 {
		return labels
	}
	labels = worldInputLabels(dispatch.ConsumedInputs)
	if len(labels) > 0 {
		return labels
	}
	return sortedUniqueStrings(dispatch.WorkItemIDs)
}

func worldDispatchOutputLabels(dispatch interfaces.FactoryWorldDispatchCompletion) []string {
	labels := worldWorkItemLabelsFromItems(dispatch.OutputWorkItems)
	if len(labels) > 0 {
		return labels
	}
	if dispatch.TerminalWork != nil {
		if label := worldWorkItemLabel(workRefForDashboardItem(dispatch.TerminalWork.WorkItem)); label != "" {
			return []string{label}
		}
	}
	labels = worldInputLabels(dispatch.ConsumedInputs)
	if len(labels) > 0 {
		return labels
	}
	return sortedUniqueStrings(dispatch.WorkItemIDs)
}

func worldWorkItemLabels(workItems []interfaces.FactoryWorldWorkItemRef) []string {
	labels := make([]string, 0, len(workItems))
	seen := make(map[string]struct{}, len(workItems))
	for _, workItem := range workItems {
		label := worldWorkItemLabel(workItem)
		if label == "" {
			continue
		}
		if _, exists := seen[label]; exists {
			continue
		}
		labels = append(labels, label)
		seen[label] = struct{}{}
	}
	sort.Strings(labels)
	return labels
}

func worldWorkItemLabelsFromItems(workItems []interfaces.FactoryWorkItem) []string {
	labels := make([]string, 0, len(workItems))
	for _, workItem := range workItems {
		labels = appendUniqueLabel(labels, worldWorkItemLabel(workRefForDashboardItem(workItem)))
	}
	sort.Strings(labels)
	return labels
}

func worldInputLabels(inputs []interfaces.WorkstationInput) []string {
	labels := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if input.WorkItem == nil {
			continue
		}
		labels = appendUniqueLabel(labels, worldWorkItemLabel(workRefForDashboardItem(*input.WorkItem)))
	}
	sort.Strings(labels)
	return labels
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

func worldWorkItemLabel(workItem interfaces.FactoryWorldWorkItemRef) string {
	switch {
	case workItem.DisplayName != "":
		return workItem.DisplayName
	case workItem.WorkID != "":
		return workItem.WorkID
	default:
		return ""
	}
}

func appendUniqueLabel(labels []string, label string) []string {
	if label == "" {
		return labels
	}
	for _, existing := range labels {
		if existing == label {
			return labels
		}
	}
	return append(labels, label)
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func displayDashboardProviderSessionView(attempt dashboardProviderSessionView) string {
	if len(attempt.WorkItems) > 0 {
		labels := make([]string, 0, len(attempt.WorkItems))
		for _, workItem := range attempt.WorkItems {
			if label := worldWorkItemLabel(workItem); label != "" {
				labels = append(labels, label)
			}
		}
		if len(labels) > 0 {
			return strings.Join(labels, ", ")
		}
	}
	if attempt.WorkstationName != "" {
		return attempt.WorkstationName
	}
	if attempt.TransitionID != "" {
		return attempt.TransitionID
	}
	return "n/a"
}

func displayDashboardFailedWorkDetail(detail dashboardFailedWorkDetail) string {
	parts := make([]string, 0, 4)
	if label := worldWorkItemLabel(detail.WorkItem); label != "" {
		parts = append(parts, label)
	} else {
		parts = append(parts, "n/a")
	}
	if detail.DispatchID != "" {
		parts = append(parts, "["+detail.DispatchID+"]")
	}
	if workstation := displayDispatchWorkstationName(detail.WorkstationName, detail.TransitionID); workstation != "n/a" {
		parts = append(parts, workstation)
	}
	if reason := dashboardFailureReason(detail.FailureReason, detail.FailureMessage); reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, " ")
}

func worldDispatchReason(dispatch interfaces.FactoryWorldDispatchCompletion) string {
	return dashboardFailureReason(
		firstNonEmpty(dispatch.Result.FailureReason, dispatch.Result.Feedback),
		dispatch.Result.FailureMessage,
	)
}

func dashboardFailureReason(reason, message string) string {
	reason = strings.TrimSpace(reason)
	message = strings.TrimSpace(message)
	switch {
	case reason != "" && message != "":
		return reason + " - " + message
	case reason != "":
		return reason
	case message != "":
		return message
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatProviderSession(session *interfaces.ProviderSessionMetadata) string {
	if session == nil || session.ID == "" {
		return "n/a"
	}

	parts := make([]string, 0, 2)
	if session.Provider != "" {
		parts = append(parts, session.Provider)
	}
	if session.Kind != "" {
		parts = append(parts, session.Kind)
	}
	if len(parts) == 0 {
		return session.ID
	}
	return strings.Join(parts, " / ") + " / " + session.ID
}

func workRefForDashboardItem(item interfaces.FactoryWorkItem) interfaces.FactoryWorldWorkItemRef {
	return interfaces.FactoryWorldWorkItemRef{
		WorkID:      item.ID,
		WorkTypeID:  item.WorkTypeID,
		DisplayName: item.DisplayName,
		TraceID:     item.TraceID,
	}
}

func dashboardCompatibilityTransitionID(transitionID string) string {
	if transitionID == interfaces.SystemTimeExpiryTransitionID {
		return interfaces.SystemTimeDashboardExpiryTransitionID
	}
	return transitionID
}

func dashboardCompatibilityWorkstationName(name, transitionID string) string {
	mappedTransitionID := dashboardCompatibilityTransitionID(transitionID)
	if name != "" && name != transitionID {
		return name
	}
	return mappedTransitionID
}
