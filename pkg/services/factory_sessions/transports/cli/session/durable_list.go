package session

import (
	"context"
	"fmt"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/scopedlisting"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// DurableSessionLister reads persisted Factory Sessions through an injected
// durable execution collaborator.
type DurableSessionLister func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error)

type durableSessionLister = DurableSessionLister

func (lister DurableSessionLister) ListSessions(
	ctx context.Context,
	req fse.ListSessionsRequest,
) (fse.ListSessionsResult, error) {
	return listDurableSessions(ctx, lister, req)
}

type detachedLiveSessionReader []fse.ScopedLiveSessionSummary

func (reader detachedLiveSessionReader) ListScopedLiveSessions(context.Context) ([]fse.ScopedLiveSessionSummary, error) {
	return append([]fse.ScopedLiveSessionSummary(nil), reader...), nil
}

func listDurableSessions(
	ctx context.Context,
	lister durableSessionLister,
	req fse.ListSessionsRequest,
) (fse.ListSessionsResult, error) {
	if lister == nil {
		return fse.ListSessionsResult{}, fmt.Errorf("durable session lister is required")
	}
	return lister(ctx, req)
}

func mergeScopedListResult(
	ctx context.Context,
	cfg ListConfig,
	normalized fse.ListSessionsRequest,
	liveSessions []fse.LiveSessionSummary,
) (fse.ScopedSessionListResult, error) {
	var liveReader scopedlisting.LiveReader
	if normalized.Scope == fse.SessionListScopeLive || normalized.Scope == fse.SessionListScopeAll {
		rows := make(detachedLiveSessionReader, 0, len(liveSessions))
		for _, session := range liveSessions {
			rows = append(rows, fse.ScopedLiveSessionSummary{
				ID: session.ID, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
				Project: session.Project, IsDefault: session.IsDefault,
			})
		}
		liveReader = rows
	}

	var durableReader scopedlisting.DurableReader
	if normalized.Scope == fse.SessionListScopePersisted || normalized.Scope == fse.SessionListScopeAll {
		durableReader = cfg.DurableLister
	}
	result, err := scopedlisting.List(ctx, normalized, liveReader, durableReader)
	if err != nil {
		return fse.ScopedSessionListResult{}, fmt.Errorf("list durable factory sessions failed: %w", err)
	}
	return result, nil
}

func listResponseFromScopedResult(result fse.ScopedSessionListResult) factoryapi.ListFactorySessionsResponse {
	return factorysession.ScopedSessionListResponseToAPI(result)
}
