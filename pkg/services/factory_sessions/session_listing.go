package factorysessions

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

var ErrDurableSessionListReaderRequired = errors.New("durable Factory Session list reader is required")

// LiveSessionProjectionReader is the exact live-session projection role used
// by the Factory Sessions adapter.
type LiveSessionProjectionReader interface {
	ListFactorySessions(context.Context) ([]ReadProjection, error)
}

// LiveSessionListReader returns already-detached live rows to the scoped
// listing operation.
type LiveSessionListReader interface {
	ListScopedLiveSessions(context.Context) ([]ScopedLiveSessionSummary, error)
}

// DurableSessionListReader is the exact durable-session read role consumed by
// the Factory Sessions scoped listing operation.
type DurableSessionListReader interface {
	ListSessions(context.Context, ListSessionsRequest) (ListSessionsResult, error)
}

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

// ReadProjectionSessionListReader adapts the canonical live projection read
// into detached list rows without transport representation involvement.
type ReadProjectionSessionListReader struct {
	Reader LiveSessionProjectionReader
}

func (reader ReadProjectionSessionListReader) ListScopedLiveSessions(ctx context.Context) ([]ScopedLiveSessionSummary, error) {
	if reader.Reader == nil {
		return nil, fmt.Errorf("list scoped Factory Sessions: live session projection reader is required")
	}
	reads, err := reader.Reader.ListFactorySessions(ctx)
	if err != nil {
		return nil, err
	}
	return projectScopedLiveSessions(reads), nil
}

// ListScopedSessions owns source selection, scoping, filtering, deduplication,
// and ordering across live workspace and durable Factory Session inventories.
func ListScopedSessions(
	ctx context.Context,
	request ListSessionsRequest,
	live LiveSessionListReader,
	durable DurableSessionListReader,
) (ScopedSessionListResult, error) {
	scope := request.Scope
	if scope == "" {
		scope = DefaultSessionListScope
	}
	request.Scope = scope

	var workspace []ScopedLiveSessionSummary
	if scope == SessionListScopeLive || scope == SessionListScopeAll {
		if live == nil {
			return ScopedSessionListResult{}, fmt.Errorf("list scoped Factory Sessions: live session reader is required")
		}
		rows, err := live.ListScopedLiveSessions(ctx)
		if err != nil {
			return ScopedSessionListResult{}, err
		}
		workspace = append([]ScopedLiveSessionSummary(nil), rows...)
	}

	var durableResult ListSessionsResult
	if scope == SessionListScopePersisted && durable == nil {
		return ScopedSessionListResult{}, ErrDurableSessionListReaderRequired
	}
	if durable != nil {
		result, err := durable.ListSessions(ctx, ListSessionsRequest{Scope: SessionListScopeAll})
		if err != nil {
			return ScopedSessionListResult{}, err
		}
		durableResult = result
	}
	scoped := execution.ApplySessionListScope(durableResult, request)
	liveRows := append(workspace, projectDurableLiveSessions(scoped.LiveSessions)...)
	sort.SliceStable(liveRows, func(left, right int) bool {
		return strings.Compare(liveRows[left].ID, liveRows[right].ID) < 0
	})
	return ScopedSessionListResult{
		Scope: scope, LiveSessions: liveRows,
		DurableSessions: append([]DurableSessionListSummary(nil), scoped.DurableSessions...),
	}, nil
}

func projectScopedLiveSessions(reads []ReadProjection) []ScopedLiveSessionSummary {
	if len(reads) == 0 {
		return nil
	}
	projected := make([]ScopedLiveSessionSummary, 0, len(reads))
	for _, read := range reads {
		session := read.Context.Session
		if session == nil {
			continue
		}
		row := ScopedLiveSessionSummary{
			ID: CanonicalFactorySessionID(session), FactoryDir: session.FactoryDir,
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

func projectDurableLiveSessions(sessions []LiveSessionSummary) []ScopedLiveSessionSummary {
	if len(sessions) == 0 {
		return nil
	}
	projected := make([]ScopedLiveSessionSummary, 0, len(sessions))
	for _, session := range sessions {
		projected = append(projected, ScopedLiveSessionSummary{
			ID: session.ID, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault,
		})
	}
	return projected
}
