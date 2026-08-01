// Package scopedlisting owns Factory Session inventory source selection,
// merging, filtering, and deterministic ordering.
package scopedlisting

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

var ErrDurableReaderRequired = errors.New("durable Factory Session list reader is required")

type LiveReader interface {
	ListScopedLiveSessions(context.Context) ([]factorysessions.ScopedLiveSessionSummary, error)
}

type DurableReader interface {
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

// List owns source selection, scoping, filtering, deduplication, and ordering
// across live workspace and durable Factory Session inventories.
func List(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
	live LiveReader,
	durable DurableReader,
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
	scoped := execution.ApplySessionListScope(durableResult, request)
	liveRows := append(workspace, projectDurableLiveSessions(scoped.LiveSessions)...)
	sort.SliceStable(liveRows, func(left, right int) bool {
		return strings.Compare(liveRows[left].ID, liveRows[right].ID) < 0
	})
	return factorysessions.ScopedSessionListResult{
		Scope: scope, LiveSessions: liveRows,
		DurableSessions: append([]factorysessions.DurableSessionListSummary(nil), scoped.DurableSessions...),
	}, nil
}

// ProjectLiveSessions converts canonical live reads into detached list rows.
func ProjectLiveSessions(reads []factorysessions.ReadProjection) []factorysessions.ScopedLiveSessionSummary {
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
