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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
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
	return fs.projectionReader().BuildSessionProjectionContext(ctx, session)
}

// sessionProjectionReader reads keyed runtime facts without retaining a
// SessionRuntime or re-entering its gateway. RuntimeRecord/Run access remains
// the compatibility bridge owned by T15.
type sessionProjectionReader struct {
	state        *sessionruntime.Service
	backendScope string
	identity     identity.Service
	clock        factoryruntime.Clock
	projector    factoryruntime.WorldStateProjector
	checkpoints  factoryruntime.JavaScriptCheckpointStoreFactory
}

func (fs *SessionRuntime) projectionReader() sessionProjectionReader {
	return sessionProjectionReader{
		state: fs.sessionState, backendScope: fs.backendScopeID, identity: fs.identity,
		clock: fs.clock, projector: fs.worldStateProjector, checkpoints: fs.newJavaScriptCheckpointStore,
	}
}

func (r sessionProjectionReader) BuildSessionProjectionContext(
	ctx context.Context,
	session *livesession.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if session == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("%w", factorysessions.ErrSessionNotFound)
	}
	runtimeCfg, err := runtimebinding.RuntimeConfigForSession(r.state, session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	selected, err := runtimebinding.RequireLiveSession(r.state, session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	runtime := runtimebinding.ServiceForLiveRuntime(selected.Runtime)
	if runtime == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("Factory Runtime observation is required")
	}
	observationResult, err := runtime.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	bundle := runtimebinding.BundleFromSession(session)
	var snapshot *legacysnapshot.Snapshot
	if runtime := runtimebinding.ServiceForLiveRuntime(session.Runtime); runtime != nil {
		if provider, ok := runtime.(legacysnapshot.WorkProvider); ok && provider != nil {
			snapshot, err = provider.GetWorkStateSnapshot(ctx)
			if err != nil {
				return factorysessions.ProjectionContext{}, err
			}
		}
	}
	var sessionProjectionFacts *recordings.SessionProjectionFacts
	if bundle != nil {
		if reader, ok := bundle.RecordingLedger().(recordings.SessionProjectionReader); ok && reader != nil {
			facts, factsErr := reader.CurrentSessionProjectionFacts()
			if factsErr != nil {
				return factorysessions.ProjectionContext{}, factsErr
			}
			sessionProjectionFacts = &facts
		}
	}
	var checkpointStore factoryruntime.JavaScriptCheckpointStore
	if interfaces.IsJavaScriptOrchestratorFactory(runtimeCfg.FactoryConfig()) {
		checkpointStore = sessionCheckpointStore(session, r.checkpoints)
	}
	startedAt := time.Time{}
	backendScopeID := ""
	if bundle != nil {
		startedAt = bundle.StartTime()
	}
	backendScopeID = runtimebinding.BackendScopeID(r.backendScope, session)
	placement := session.Placement()
	resolvedIdentity, err := r.identity.Normalize(ctx, identity.NormalizeRequest{
		BackendScopeID: backendScopeID, FolderPath: placement.FolderPath, Target: placement.Target,
	})
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	return sessionprojection.BuildProjectionContext(sessionprojection.ProjectionBuildInput{
		Session: session, RuntimeConfig: runtimeCfg,
		Observation: observationResult.Observation, Snapshot: snapshot,
		BackendScopeID: backendScopeID, LogicalSessionKey: resolvedIdentity.LogicalSessionKeyID,
		NormalizedTarget: &resolvedIdentity.RuntimeTarget, RuntimeStartedAt: startedAt,
		CheckpointStore: checkpointStore, SessionProjection: sessionProjectionFacts,
		WorldStateProjector: r.projector, Now: r.clock.Now().UTC(),
	})
}

// BuildSessionProjectionContext exposes the existing projection on the
// session-owned runtime stored with its canonical registry entry.
func (fs *SessionRuntime) BuildSessionProjectionContext(
	ctx context.Context,
	session *livesession.LiveSession,
) (factorysessions.ProjectionContext, error) {
	return fs.buildSessionProjectionContext(ctx, session)
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
