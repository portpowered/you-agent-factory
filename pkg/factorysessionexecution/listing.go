package factorysessionexecution

import (
	"sort"
	"strings"
	"time"
)

// IsPersistedListCandidate reports whether one durable session belongs in the
// persisted scope. Persisted listing covers terminal, interrupted, and recoverable
// durable sessions indexed outside the live workspace.
func IsPersistedListCandidate(summary DurableSessionListSummary) bool {
	if IsTerminalLifecycleStatus(summary.Status) {
		return true
	}
	if summary.Status == LifecycleStatusInterrupted {
		return true
	}
	return summary.Recoverable
}

// IsRecoverableSession reports whether one durable session is interrupted or has
// a stale lease while still appearing active.
func IsRecoverableSession(status LifecycleStatus, staleLease bool) bool {
	if status == LifecycleStatusInterrupted {
		return true
	}
	return staleLease && !IsTerminalLifecycleStatus(status)
}

// DeriveSessionActionAvailability classifies which lifecycle controls are valid
// for one durable session status.
func DeriveSessionActionAvailability(status LifecycleStatus) SessionActionAvailability {
	return SessionActionAvailability{
		CanPause:         EvaluateLifecycleControl(LifecycleControlPause, status) == LifecycleControlOutcomeAccepted,
		CanResume:        EvaluateLifecycleControl(LifecycleControlResume, status) == LifecycleControlOutcomeAccepted,
		CanCancel:        EvaluateLifecycleControl(LifecycleControlCancel, status) == LifecycleControlOutcomeAccepted,
		CanTerminate:     EvaluateLifecycleControl(LifecycleControlTerminate, status) == LifecycleControlOutcomeAccepted,
		CanApprove:       status == LifecycleStatusAwaitingApproval,
		CanRetryDispatch: AllowsRetryDispatchOnTerminal(status),
	}
}

// DurableListSummaryFromSessionRead projects one durable session read model into
// a list row with action availability and recoverability metadata.
func DurableListSummaryFromSessionRead(read SessionReadResult) DurableSessionListSummary {
	artifactCount := read.ArtifactCount
	if artifactCount == 0 {
		artifactCount = len(read.ArtifactRefs)
	}
	return DurableSessionListSummary{
		SessionID:        read.SessionID,
		Status:           read.Status,
		OrchestratorKind: read.OrchestratorKind,
		Dialect:          read.Dialect,
		ResolvedSource:   read.ResolvedSource,
		SourceHash:       read.SourceHash,
		Policy:           read.Policy,
		Phase:            read.Phase,
		Progress:         read.Progress,
		ResultSummary:    read.ResultSummary,
		ArtifactCount:    artifactCount,
		StaleLease:       read.StaleLease,
		Recoverable:      IsRecoverableSession(read.Status, read.StaleLease),
		Lifecycle:        read.Lifecycle,
		Links:            read.Links,
		Actions:          DeriveSessionActionAvailability(read.Status),
	}
}

// MatchesDurableSessionListFilters reports whether one durable list row satisfies
// the normalized listing filters.
func MatchesDurableSessionListFilters(summary DurableSessionListSummary, filters SessionListFilters) bool {
	if len(filters.Statuses) > 0 && !containsLifecycleStatus(filters.Statuses, summary.Status) {
		return false
	}
	if len(filters.OrchestratorKinds) > 0 && !containsString(filters.OrchestratorKinds, summary.OrchestratorKind) {
		return false
	}
	if filters.SourceKind != "" && summary.ResolvedSource.Kind != filters.SourceKind {
		return false
	}
	if filters.SourceRef != "" && !strings.Contains(summary.ResolvedSource.SourceRef, filters.SourceRef) {
		return false
	}
	if filters.ProjectBoundary != "" {
		project := strings.TrimSpace(summary.ResolvedSource.Metadata["project"])
		if project == "" || !strings.Contains(project, filters.ProjectBoundary) {
			return false
		}
	}
	if filters.Recoverable != nil && summary.Recoverable != *filters.Recoverable {
		return false
	}
	if filters.StaleLease != nil && summary.StaleLease != *filters.StaleLease {
		return false
	}
	if !matchesLifecycleTimeFilters(summary.Lifecycle, filters) {
		return false
	}
	return true
}

// FilterDurableSessionSummaries returns durable rows that satisfy the listing filters.
func FilterDurableSessionSummaries(
	summaries []DurableSessionListSummary,
	filters SessionListFilters,
) []DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	filtered := make([]DurableSessionListSummary, 0, len(summaries))
	for _, summary := range summaries {
		if MatchesDurableSessionListFilters(summary, filters) {
			filtered = append(filtered, summary)
		}
	}
	return filtered
}

// FilterPersistedSessionSummaries returns durable rows eligible for persisted scope.
func FilterPersistedSessionSummaries(summaries []DurableSessionListSummary) []DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	filtered := make([]DurableSessionListSummary, 0, len(summaries))
	for _, summary := range summaries {
		if IsPersistedListCandidate(summary) {
			filtered = append(filtered, summary)
		}
	}
	return filtered
}

// DeduplicateLiveSessionsForAllScope removes live rows whose ids already appear in
// the durable list so scope=all merges deterministically by session id.
func DeduplicateLiveSessionsForAllScope(
	live []LiveSessionSummary,
	durable []DurableSessionListSummary,
) []LiveSessionSummary {
	if len(live) == 0 || len(durable) == 0 {
		return append([]LiveSessionSummary(nil), live...)
	}
	durableIDs := make(map[string]struct{}, len(durable))
	for _, summary := range durable {
		if id := strings.TrimSpace(summary.SessionID); id != "" {
			durableIDs[id] = struct{}{}
		}
	}
	filtered := make([]LiveSessionSummary, 0, len(live))
	for _, session := range live {
		if _, exists := durableIDs[strings.TrimSpace(session.ID)]; exists {
			continue
		}
		filtered = append(filtered, session)
	}
	return filtered
}

// SortDurableSessionSummaries returns a stable session-id sort for deterministic output.
func SortDurableSessionSummaries(summaries []DurableSessionListSummary) []DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	sorted := append([]DurableSessionListSummary(nil), summaries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].SessionID, sorted[j].SessionID) < 0
	})
	return sorted
}

// SortLiveSessionSummaries returns a stable id sort for deterministic output.
func SortLiveSessionSummaries(sessions []LiveSessionSummary) []LiveSessionSummary {
	if len(sessions) == 0 {
		return nil
	}
	sorted := append([]LiveSessionSummary(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].ID, sorted[j].ID) < 0
	})
	return sorted
}

// ApplySessionListScope shapes one listing result for the requested scope.
func ApplySessionListScope(result ListSessionsResult, request ListSessionsRequest) ListSessionsResult {
	scope := request.Scope
	if scope == "" {
		scope = DefaultSessionListScope
	}

	durable := FilterDurableSessionSummaries(result.DurableSessions, request.Filters)
	live := append([]LiveSessionSummary(nil), result.LiveSessions...)

	switch scope {
	case SessionListScopeLive:
		return ListSessionsResult{
			Scope:        scope,
			LiveSessions: SortLiveSessionSummaries(live),
		}
	case SessionListScopePersisted:
		return ListSessionsResult{
			Scope:           scope,
			DurableSessions: SortDurableSessionSummaries(FilterPersistedSessionSummaries(durable)),
		}
	default:
		return ListSessionsResult{
			Scope:           scope,
			LiveSessions:    SortLiveSessionSummaries(DeduplicateLiveSessionsForAllScope(live, durable)),
			DurableSessions: SortDurableSessionSummaries(durable),
		}
	}
}

func containsLifecycleStatus(values []LifecycleStatus, target LifecycleStatus) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func matchesLifecycleTimeFilters(lifecycle *LifecycleTimestamps, filters SessionListFilters) bool {
	if lifecycle == nil {
		return filters.CreatedAfter == nil &&
			filters.CreatedBefore == nil &&
			filters.UpdatedAfter == nil &&
			filters.UpdatedBefore == nil
	}
	if filters.CreatedAfter != nil {
		createdAt := firstLifecycleTimestamp(lifecycle.QueuedAt, lifecycle.StartedAt)
		if createdAt == nil || createdAt.Before(*filters.CreatedAfter) {
			return false
		}
	}
	if filters.CreatedBefore != nil {
		createdAt := firstLifecycleTimestamp(lifecycle.QueuedAt, lifecycle.StartedAt)
		if createdAt == nil || createdAt.After(*filters.CreatedBefore) {
			return false
		}
	}
	if filters.UpdatedAfter != nil {
		updatedAt := latestLifecycleTimestamp(lifecycle)
		if updatedAt == nil || updatedAt.Before(*filters.UpdatedAfter) {
			return false
		}
	}
	if filters.UpdatedBefore != nil {
		updatedAt := latestLifecycleTimestamp(lifecycle)
		if updatedAt == nil || updatedAt.After(*filters.UpdatedBefore) {
			return false
		}
	}
	return true
}

func firstLifecycleTimestamp(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func latestLifecycleTimestamp(lifecycle *LifecycleTimestamps) *time.Time {
	if lifecycle == nil {
		return nil
	}
	var latest *time.Time
	for _, candidate := range []*time.Time{
		lifecycle.UpdatedAt,
		lifecycle.FinishedAt,
		lifecycle.InterruptedAt,
		lifecycle.TerminatedAt,
		lifecycle.ResumedAt,
		lifecycle.PausedAt,
		lifecycle.StartedAt,
		lifecycle.AwaitingApprovalAt,
		lifecycle.QueuedAt,
	} {
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			latest = candidate
		}
	}
	return latest
}
