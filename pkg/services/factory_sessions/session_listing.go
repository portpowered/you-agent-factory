package factorysessions

import (
	"sort"
	"strings"
	"time"
)

// ScopedLiveSessionSummary is the detached, representation-neutral live row
// returned after Factory Sessions has merged workspace and durable live rows.
type ScopedLiveSessionSummary struct {
	ID               string
	FactoryDir       string
	FolderPath       string
	Project          string
	IsDefault        bool
	Target           TargetRef
	Runtime          *RuntimeProjection
	NormalizedTarget *RuntimeLogicalTarget
}

// ScopedSessionListResult is the complete owner-projected result consumed by
// customer transports. Transports map it but do not reapply scope or merge
// policy.
type ScopedSessionListResult struct {
	Scope           SessionListScope
	LiveSessions    []ScopedLiveSessionSummary
	DurableSessions []DurableSessionListSummary
}

// Listing policy helpers are published as values so the Sessions root keeps its
// existing contract-only package surface while owning the canonical policy.
var (
	ApplySessionListScope              = applySessionListScope
	IsPersistedListCandidate           = isPersistedListCandidate
	LiveListSummaryFromSessionRead     = liveListSummaryFromSessionRead
	DurableListSummaryFromSessionRead  = durableListSummaryFromSessionRead
	MatchesDurableSessionListFilters   = matchesDurableSessionListFilters
	FilterDurableSessionSummaries      = filterDurableSessionSummaries
	FilterPersistedSessionSummaries    = filterPersistedSessionSummaries
	DeduplicateLiveSessionsForAllScope = deduplicateLiveSessionsForAllScope
	SortDurableSessionSummaries        = sortDurableSessionSummaries
	SortLiveSessionSummaries           = sortLiveSessionSummaries
	DeriveSessionActionAvailability    = deriveSessionActionAvailability
	IsRecoverableSession               = isRecoverableSession
)

func applySessionListScope(result ListSessionsResult, request ListSessionsRequest) ListSessionsResult {
	scope := request.Scope
	if scope == "" {
		scope = DefaultSessionListScope
	}

	durable := filterDurableSessionSummaries(result.DurableSessions, request.Filters)
	live := append([]LiveSessionSummary(nil), result.LiveSessions...)

	switch scope {
	case SessionListScopeLive:
		return ListSessionsResult{
			Scope:        scope,
			LiveSessions: sortLiveSessionSummaries(live),
		}
	case SessionListScopePersisted:
		return ListSessionsResult{
			Scope:           scope,
			DurableSessions: sortDurableSessionSummaries(filterPersistedSessionSummaries(durable)),
		}
	default:
		return ListSessionsResult{
			Scope:           scope,
			LiveSessions:    sortLiveSessionSummaries(deduplicateLiveSessionsForAllScope(live, durable)),
			DurableSessions: sortDurableSessionSummaries(durable),
		}
	}
}

// IsPersistedListCandidate reports whether one durable session belongs in the
// persisted scope. Persisted listing covers terminal, interrupted, and recoverable
// durable sessions indexed outside the live workspace.
func isPersistedListCandidate(summary DurableSessionListSummary) bool {
	if IsTerminalLifecycleStatus(summary.Status) {
		return true
	}
	if summary.Status == LifecycleStatusInterrupted {
		return true
	}
	return summary.Recoverable
}

// LiveListSummaryFromSessionRead projects one in-process durable session into a
// live-scope row owned by the current service instance.
func liveListSummaryFromSessionRead(read SessionReadResult) LiveSessionSummary {
	project := strings.TrimSpace(read.ResolvedSource.Metadata["project"])
	return LiveSessionSummary{
		ID:      read.SessionID,
		Project: project,
	}
}

// DurableListSummaryFromSessionRead projects one durable session read model into
// a list row with action availability and recoverability metadata.
func durableListSummaryFromSessionRead(read SessionReadResult) DurableSessionListSummary {
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
		Recoverable:      isRecoverableSession(read.Status, read.StaleLease),
		Lifecycle:        read.Lifecycle,
		Links:            read.Links,
		Actions:          deriveSessionActionAvailability(read.Status),
	}
}

func filterDurableSessionSummaries(
	summaries []DurableSessionListSummary,
	filters SessionListFilters,
) []DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	filtered := make([]DurableSessionListSummary, 0, len(summaries))
	for _, summary := range summaries {
		if matchesDurableSessionListFilters(summary, filters) {
			filtered = append(filtered, summary)
		}
	}
	return filtered
}

// pkgmaintcheck:ignore-cyclomatic-complexity this filter matcher keeps durable listing scope predicates together on one reviewer-readable seam.
func matchesDurableSessionListFilters(summary DurableSessionListSummary, filters SessionListFilters) bool {
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
	return matchesLifecycleTimeFilters(summary.Lifecycle, filters)
}

func filterPersistedSessionSummaries(summaries []DurableSessionListSummary) []DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	filtered := make([]DurableSessionListSummary, 0, len(summaries))
	for _, summary := range summaries {
		if isPersistedListCandidate(summary) {
			filtered = append(filtered, summary)
		}
	}
	return filtered
}

func deduplicateLiveSessionsForAllScope(
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

func sortDurableSessionSummaries(summaries []DurableSessionListSummary) []DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	sorted := append([]DurableSessionListSummary(nil), summaries...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return strings.Compare(sorted[left].SessionID, sorted[right].SessionID) < 0
	})
	return sorted
}

func sortLiveSessionSummaries(sessions []LiveSessionSummary) []LiveSessionSummary {
	if len(sessions) == 0 {
		return nil
	}
	sorted := append([]LiveSessionSummary(nil), sessions...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return strings.Compare(sorted[left].ID, sorted[right].ID) < 0
	})
	return sorted
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

// pkgmaintcheck:ignore-cyclomatic-complexity this helper keeps created/updated time-range filtering together for durable session listing.
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

// DeriveSessionActionAvailability classifies which lifecycle controls are valid
// for one durable session status.
func deriveSessionActionAvailability(status LifecycleStatus) SessionActionAvailability {
	return SessionActionAvailability{
		CanPause:             EvaluateLifecycleControl(LifecycleControlPause, status) == LifecycleControlOutcomeAccepted,
		CanResume:            EvaluateLifecycleControl(LifecycleControlResume, status) == LifecycleControlOutcomeAccepted,
		CanCancel:            EvaluateLifecycleControl(LifecycleControlCancel, status) == LifecycleControlOutcomeAccepted,
		CanTerminate:         EvaluateLifecycleControl(LifecycleControlTerminate, status) == LifecycleControlOutcomeAccepted,
		CanApprove:           status == LifecycleStatusAwaitingApproval,
		CanRetryDispatch:     AllowsRetryDispatchOnTerminal(status),
		CanInterruptDispatch: AllowsInterruptDispatchOnSession(status),
	}
}

// IsRecoverableSession reports whether one durable session is interrupted or has
// a stale lease while still appearing active.
func isRecoverableSession(status LifecycleStatus, staleLease bool) bool {
	if status == LifecycleStatusInterrupted {
		return true
	}
	return staleLease && !IsTerminalLifecycleStatus(status)
}
