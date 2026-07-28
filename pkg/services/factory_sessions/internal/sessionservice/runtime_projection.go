package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
)

type sessionSyncPreflightTarget struct {
	session    *livesession.LiveSession
	remapped   bool
	unresolved bool
}

func (fs *SessionRuntime) resolveSessionSyncPreflightTarget(
	sessionID string,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (sessionSyncPreflightTarget, error) {
	if fs == nil {
		return sessionSyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	if session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID); err == nil {
		return sessionSyncPreflightTarget{session: session}, nil
	} else if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		return sessionSyncPreflightTarget{}, err
	}
	if strings.TrimSpace(sessionID) == DefaultFactorySessionID {
		if session := runtimebinding.DefaultSessionSuccessor(fs.sessionState, &fs.runtimeState); session != nil {
			return sessionSyncPreflightTarget{session: session, remapped: true}, nil
		}
	}
	if hasLogicalResolveHint(logicalResolve) {
		return fs.resolveSessionSyncPreflightByLogicalKey(sessionID, logicalResolve)
	}
	return sessionSyncPreflightTarget{}, nil
}

func hasLogicalResolveHint(hint *interfaces.FactorySessionLogicalResolveHint) bool {
	return hint != nil &&
		strings.TrimSpace(hint.BackendScopeID) != "" &&
		strings.TrimSpace(hint.LogicalSessionKeyID) != ""
}

func (fs *SessionRuntime) resolveSessionSyncPreflightByLogicalKey(
	requestedSessionID string,
	hint *interfaces.FactorySessionLogicalResolveHint,
) (sessionSyncPreflightTarget, error) {
	configuredScope := fs.backendScopeID
	serviceScope := runtimebinding.BackendScopeID(configuredScope, nil)
	if serviceScope == "" || strings.TrimSpace(hint.BackendScopeID) != serviceScope {
		return sessionSyncPreflightTarget{unresolved: true}, nil
	}
	session := fs.identity.ResolveLogical(fs.sessionState.Registry(), serviceScope, hint.LogicalSessionKeyID)
	if session == nil {
		return sessionSyncPreflightTarget{unresolved: true}, nil
	}
	remapped := strings.TrimSpace(requestedSessionID) != "" &&
		session.ID != strings.TrimSpace(requestedSessionID)
	return sessionSyncPreflightTarget{session: session, remapped: remapped}, nil
}

func (fs *SessionRuntime) buildSessionProjectionContext(
	ctx context.Context,
	session *livesession.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if session == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("%w", factorysessions.ErrSessionNotFound)
	}
	runtimeCfg, err := runtimebinding.RuntimeConfigForSession(fs.sessionState, session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	observationResult, err := fs.ObserveForSession(ctx, session.ID, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	bundle := runtimebinding.BundleFromSession(session)
	var checkpointStore factoryruntime.JavaScriptCheckpointStore
	if interfaces.IsJavaScriptOrchestratorFactory(runtimeCfg.FactoryConfig()) {
		checkpointStore = fs.requireSessionGateway().JavaScriptCheckpointStore(session)
	}
	startedAt := time.Time{}
	backendScopeID := ""
	if bundle != nil {
		startedAt = bundle.StartTime()
	}
	backendScopeID = runtimebinding.BackendScopeID(fs.backendScopeID, session)
	resolvedIdentity, err := fs.identity.Normalize(ctx, identity.NormalizeRequest{
		BackendScopeID: backendScopeID, FolderPath: session.FolderPath, Target: session.Target,
	})
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	return sessionprojection.BuildProjectionContext(sessionprojection.ProjectionBuildInput{
		Session: session, RuntimeConfig: runtimeCfg,
		Observation: observationResult.Observation,
		BackendScopeID: backendScopeID, LogicalSessionKey: resolvedIdentity.LogicalSessionKeyID,
		NormalizedTarget: &resolvedIdentity.RuntimeTarget, RuntimeStartedAt: startedAt,
		CheckpointStore: checkpointStore, Events: runtimebinding.CanonicalEventsFromSession(session),
		WorldStateProjector: fs.worldStateProjector, Now: fs.clock.Now().UTC(),
	})
}

func (fs *SessionRuntime) sessionPersistenceScopeFromSession(
	ctx context.Context,
	session *livesession.LiveSession,
) (factorysessioncursors.IdentityScope, error) {
	if fs == nil || session == nil {
		return factorysessioncursors.IdentityScope{}, fmt.Errorf("factory service is required")
	}
	projectionCtx, err := fs.buildSessionProjectionContext(ctx, session)
	if err != nil {
		return factorysessioncursors.IdentityScope{}, err
	}
	runtime := sessionprojection.ProjectRuntimeContract(projectionCtx)
	scope := factorysessioncursors.IdentityScope{
		BackendScopeID:      runtimebinding.BackendScopeID(fs.backendScopeID, session),
		LogicalSessionKeyID: projectionCtx.LogicalSessionKeyID,
		FactorySessionID:    strings.TrimSpace(session.ID),
	}
	if runtime.StreamIdentity != nil {
		scope.BackendScopeID = strings.TrimSpace(runtime.StreamIdentity.BackendScopeID)
		scope.FactorySessionID = strings.TrimSpace(runtime.StreamIdentity.FactorySessionID)
		scope.StreamGenerationID = strings.TrimSpace(runtime.StreamIdentity.StreamGenerationID)
	}
	return factorysessioncursors.NormalizeScope(scope), nil
}
