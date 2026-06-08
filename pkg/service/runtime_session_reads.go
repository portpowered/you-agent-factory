package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func (fs *FactoryService) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	return fs.requireCoordinator().ListFactorySessions(ctx)
}

func (c *runtimeFactoryCoordinator) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	fs := c.service
	if fs == nil || fs.sessions == nil {
		return factoryapi.ListFactorySessionsResponse{}, nil
	}
	sessionIDs := fs.sessions.IDs()
	summaries := make([]factoryapi.FactorySessionSummary, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session := fs.sessions.Get(sessionID)
		if session == nil {
			continue
		}
		projectionCtx, err := fs.buildSessionProjectionContext(ctx, session)
		if err != nil {
			summaries = append(summaries, factorysessions.SummaryResponse(session))
			continue
		}
		summaries = append(summaries, factorysessions.SummaryWithRuntime(projectionCtx))
	}
	sortFactorySessionSummaries(summaries)
	return factoryapi.ListFactorySessionsResponse{Sessions: summaries}, nil
}

func (fs *FactoryService) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	return fs.requireCoordinator().GetFactorySession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	projectionCtx, err := fs.buildSessionProjectionContext(ctx, session)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	return factorysessions.SessionResponse(projectionCtx), nil
}

func (fs *FactoryService) buildSessionProjectionContext(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if session == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("%w", apisurface.ErrFactorySessionNotFound)
	}
	runtimeCfg, err := fs.sessionRuntimeConfig(session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	projectionCtx := factorysessions.ProjectionContext{
		Session:    session,
		FactoryCfg: factoryCfg,
		Now:        time.Now().UTC(),
	}
	if interfaces.IsJavaScriptOrchestratorFactory(factoryCfg) {
		checkpointStore := fs.javascriptCheckpointStore(session)
		projectionCtx.JavaScript = factorysessions.JavaScriptRuntimeStateFromCheckpoints(
			checkpointStore,
			projectionCtx.JavaScript,
		)
		projectionCtx.JavaScriptCheckpoints = checkpointStore.List()
		return projectionCtx, nil
	}
	snapshot, err := fs.GetEngineStateSnapshotForSession(ctx, session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	projectionCtx.Snapshot = snapshot
	projectionCtx.Enabled = factorysessions.EnabledTransitionsForSnapshot(ctx, snapshot, runtimeCfg)
	return projectionCtx, nil
}

func (fs *FactoryService) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionResult, error) {
	return fs.requireCoordinator().GetFactorySessionResult(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionResult, error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	projectionCtx, err := fs.buildSessionProjectionContext(ctx, session)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(projectionCtx.FactoryCfg) {
		return factoryapi.FactorySessionResult{}, fmt.Errorf("%w", apisurface.ErrFactorySessionResultUnavailable)
	}
	return factorysessions.ProjectSessionResult(sessionID, projectionCtx, fs.javascriptCheckpointStore(session)), nil
}

func (fs *FactoryService) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	return fs.requireCoordinator().GetFactorySessionPartialResult(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.FactorySessionPartialResult{}, err
	}
	projectionCtx, err := fs.buildSessionProjectionContext(ctx, session)
	if err != nil {
		return factoryapi.FactorySessionPartialResult{}, err
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(projectionCtx.FactoryCfg) {
		return factoryapi.FactorySessionPartialResult{}, fmt.Errorf("%w", apisurface.ErrFactorySessionResultUnavailable)
	}
	return factorysessions.ProjectSessionPartialResult(sessionID, projectionCtx, fs.javascriptCheckpointStore(session)), nil
}

func (fs *FactoryService) javascriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	if state.javascriptCheckpoints == nil {
		state.javascriptCheckpoints = factorysessions.NewJavaScriptCheckpointStore()
	}
	return state.javascriptCheckpoints
}

func sortFactorySessionSummaries(summaries []factoryapi.FactorySessionSummary) {
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].IsDefault != summaries[j].IsDefault {
			return summaries[i].IsDefault
		}
		return summaries[i].Id < summaries[j].Id
	})
}
