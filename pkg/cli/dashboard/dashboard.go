// Package dashboard provides read models and pretty-print rendering for the
// factory dashboard.
package dashboard

import (
	"fmt"
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

	_ = topology
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
