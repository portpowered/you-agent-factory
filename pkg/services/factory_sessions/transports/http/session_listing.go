package http

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// ErrDurableReaderRequired reports that persisted-scope listing requires a durable
// Sessions root reader.
var ErrDurableReaderRequired = errors.New("durable Factory Session list reader is required")

// LiveSessionListReader is the exact detached live-list role consumed by the
// Factory Sessions HTTP adapter.
type LiveSessionListReader interface {
	ListScopedLiveSessions(context.Context) ([]factorysessions.ScopedLiveSessionSummary, error)
}

// LiveSessionProjectionReader is the canonical root-service read role adapted
// to the detached HTTP listing input.
type LiveSessionProjectionReader interface {
	ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error)
}

// ReadProjectionSessionListReader adapts canonical live projections without
// applying generated HTTP representation policy.
type ReadProjectionSessionListReader struct {
	Reader LiveSessionProjectionReader
}

func (reader ReadProjectionSessionListReader) ListScopedLiveSessions(ctx context.Context) ([]factorysessions.ScopedLiveSessionSummary, error) {
	if reader.Reader == nil {
		return nil, fmt.Errorf("list scoped Factory Sessions: live session projection reader is required")
	}
	reads, err := reader.Reader.ListFactorySessions(ctx)
	if err != nil {
		return nil, err
	}
	return projectLiveSessions(reads), nil
}

type scopedLiveReader interface {
	ListScopedLiveSessions(context.Context) ([]factorysessions.ScopedLiveSessionSummary, error)
}

type scopedDurableReader interface {
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

func mergeScopedSessionList(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
	live scopedLiveReader,
	durable scopedDurableReader,
) (factorysessions.ScopedSessionListResult, error) {
	scope := request.Scope
	if scope == "" {
		scope = factorysessions.DefaultSessionListScope
	}
	request.Scope = scope

	var workspace []factorysessions.ScopedLiveSessionSummary
	if scope == factorysessions.SessionListScopeLive || scope == factorysessions.SessionListScopeAll {
		if live == nil {
			return factorysessions.ScopedSessionListResult{}, fmt.Errorf("list scoped Factory Sessions: live session reader is required")
		}
		rows, err := live.ListScopedLiveSessions(ctx)
		if err != nil {
			return factorysessions.ScopedSessionListResult{}, err
		}
		workspace = append([]factorysessions.ScopedLiveSessionSummary(nil), rows...)
	}

	var durableResult factorysessions.ListSessionsResult
	if scope == factorysessions.SessionListScopePersisted && durable == nil {
		return factorysessions.ScopedSessionListResult{}, ErrDurableReaderRequired
	}
	if durable != nil {
		result, err := durable.ListSessions(ctx, factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeAll})
		if err != nil {
			return factorysessions.ScopedSessionListResult{}, err
		}
		durableResult = result
	}
	scoped := applySessionListScope(durableResult, request)
	liveRows := append(workspace, projectDurableLiveSessions(scoped.LiveSessions)...)
	sort.SliceStable(liveRows, func(left, right int) bool {
		return strings.Compare(liveRows[left].ID, liveRows[right].ID) < 0
	})
	return factorysessions.ScopedSessionListResult{
		Scope:           scope,
		LiveSessions:    liveRows,
		DurableSessions: append([]factorysessions.DurableSessionListSummary(nil), scoped.DurableSessions...),
	}, nil
}

func projectLiveSessions(reads []factorysessions.ReadProjection) []factorysessions.ScopedLiveSessionSummary {
	if len(reads) == 0 {
		return nil
	}
	projected := make([]factorysessions.ScopedLiveSessionSummary, 0, len(reads))
	for _, read := range reads {
		session := read.Context.Session
		if session == nil {
			continue
		}
		row := factorysessions.ScopedLiveSessionSummary{
			ID: read.Context.FactorySessionID, FactoryDir: session.FactoryDir,
			FolderPath: session.FolderPath, Project: session.Project,
			IsDefault: session.IsDefault, Target: session.Target,
		}
		if read.RuntimeAvailable {
			runtime := read.Runtime
			row.Runtime = &runtime
		}
		if read.Context.NormalizedTarget != nil {
			target := *read.Context.NormalizedTarget
			row.NormalizedTarget = &target
		}
		projected = append(projected, row)
	}
	return projected
}

func projectDurableLiveSessions(sessions []factorysessions.LiveSessionSummary) []factorysessions.ScopedLiveSessionSummary {
	if len(sessions) == 0 {
		return nil
	}
	projected := make([]factorysessions.ScopedLiveSessionSummary, 0, len(sessions))
	for _, session := range sessions {
		projected = append(projected, factorysessions.ScopedLiveSessionSummary{
			ID: session.ID, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault,
		})
	}
	return projected
}

func applySessionListScope(result factorysessions.ListSessionsResult, request factorysessions.ListSessionsRequest) factorysessions.ListSessionsResult {
	scope := request.Scope
	if scope == "" {
		scope = factorysessions.DefaultSessionListScope
	}

	durable := append([]factorysessions.DurableSessionListSummary(nil), result.DurableSessions...)
	live := append([]factorysessions.LiveSessionSummary(nil), result.LiveSessions...)

	switch scope {
	case factorysessions.SessionListScopeLive:
		return factorysessions.ListSessionsResult{
			Scope:        scope,
			LiveSessions: sortLiveSessionSummaries(live),
		}
	case factorysessions.SessionListScopePersisted:
		return factorysessions.ListSessionsResult{
			Scope:           scope,
			DurableSessions: sortDurableSessionSummaries(filterPersistedSessionSummaries(durable)),
		}
	default:
		return factorysessions.ListSessionsResult{
			Scope:           scope,
			LiveSessions:    sortLiveSessionSummaries(deduplicateLiveSessionsForAllScope(live, durable)),
			DurableSessions: sortDurableSessionSummaries(durable),
		}
	}
}

func filterPersistedSessionSummaries(summaries []factorysessions.DurableSessionListSummary) []factorysessions.DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	filtered := make([]factorysessions.DurableSessionListSummary, 0, len(summaries))
	for _, summary := range summaries {
		if isPersistedListCandidate(summary) {
			filtered = append(filtered, summary)
		}
	}
	return filtered
}

func isPersistedListCandidate(summary factorysessions.DurableSessionListSummary) bool {
	if factorysessions.IsTerminalLifecycleStatus(summary.Status) {
		return true
	}
	if summary.Status == factorysessions.LifecycleStatusInterrupted {
		return true
	}
	return summary.Recoverable
}

func deduplicateLiveSessionsForAllScope(
	live []factorysessions.LiveSessionSummary,
	durable []factorysessions.DurableSessionListSummary,
) []factorysessions.LiveSessionSummary {
	if len(live) == 0 || len(durable) == 0 {
		return append([]factorysessions.LiveSessionSummary(nil), live...)
	}
	durableIDs := make(map[string]struct{}, len(durable))
	for _, summary := range durable {
		if id := strings.TrimSpace(summary.SessionID); id != "" {
			durableIDs[id] = struct{}{}
		}
	}
	filtered := make([]factorysessions.LiveSessionSummary, 0, len(live))
	for _, session := range live {
		if _, exists := durableIDs[strings.TrimSpace(session.ID)]; exists {
			continue
		}
		filtered = append(filtered, session)
	}
	return filtered
}

func sortDurableSessionSummaries(summaries []factorysessions.DurableSessionListSummary) []factorysessions.DurableSessionListSummary {
	if len(summaries) == 0 {
		return nil
	}
	sorted := append([]factorysessions.DurableSessionListSummary(nil), summaries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].SessionID, sorted[j].SessionID) < 0
	})
	return sorted
}

func sortLiveSessionSummaries(sessions []factorysessions.LiveSessionSummary) []factorysessions.LiveSessionSummary {
	if len(sessions) == 0 {
		return nil
	}
	sorted := append([]factorysessions.LiveSessionSummary(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].ID, sorted[j].ID) < 0
	})
	return sorted
}
