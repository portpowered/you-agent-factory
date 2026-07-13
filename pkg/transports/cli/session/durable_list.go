package session

import (
	"context"
	"fmt"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// DurableSessionLister reads persisted Factory Sessions through an injected
// durable execution collaborator.
type DurableSessionLister func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error)

type durableSessionLister = DurableSessionLister

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
) (fse.ListSessionsResult, error) {
	needsDurable := normalized.Scope == fse.SessionListScopePersisted || normalized.Scope == fse.SessionListScopeAll
	if !needsDurable {
		return fse.ApplySessionListScope(fse.ListSessionsResult{
			Scope:        normalized.Scope,
			LiveSessions: liveSessions,
		}, normalized), nil
	}

	durableResult, err := listDurableSessions(ctx, cfg.DurableLister, fse.ListSessionsRequest{
		Scope: fse.SessionListScopeAll,
	})
	if err != nil {
		return fse.ListSessionsResult{}, fmt.Errorf("list durable factory sessions failed: %w", err)
	}

	return fse.ApplySessionListScope(fse.ListSessionsResult{
		Scope:           normalized.Scope,
		LiveSessions:    liveSessions,
		DurableSessions: durableResult.DurableSessions,
	}, normalized), nil
}

func listResponseFromScopedResult(result fse.ListSessionsResult) factoryapi.ListFactorySessionsResponse {
	return factorysession.ListSessionsResponseToAPI(result)
}
