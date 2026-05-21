package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
