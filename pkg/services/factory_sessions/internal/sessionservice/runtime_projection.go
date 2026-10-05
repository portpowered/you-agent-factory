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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// sessionIdentityReader reads routing facts without retaining the runtime host.
// Active selection remains the runtimebinding.State compatibility bridge (T15).
type sessionIdentityReader struct {
	state        *sessionruntime.Service
	active       *runtimebinding.State
	backendScope string
	identity     identity.Service
}

func (r sessionIdentityReader) ResolveSyncPreflightTarget(
	sessionID string,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (controlplane.SyncPreflightTarget, error) {
	if session, err := runtimebinding.RequireLiveSession(r.state, sessionID); err == nil {
		return controlplane.SyncPreflightTarget{Session: session}, nil
	} else if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		return controlplane.SyncPreflightTarget{}, err
	}
	if strings.TrimSpace(sessionID) == DefaultFactorySessionID {
		if session := runtimebinding.DefaultSessionSuccessor(r.state, r.active); session != nil {
			return controlplane.SyncPreflightTarget{Session: session, Remapped: true}, nil
		}
	}
	if hasLogicalResolveHint(logicalResolve) {
		return r.resolveByLogicalKey(sessionID, logicalResolve)
	}
	return controlplane.SyncPreflightTarget{}, nil
}

func hasLogicalResolveHint(hint *interfaces.FactorySessionLogicalResolveHint) bool {
	return hint != nil &&
		strings.TrimSpace(hint.BackendScopeID) != "" &&
		strings.TrimSpace(hint.LogicalSessionKeyID) != ""
}

func (r sessionIdentityReader) resolveByLogicalKey(
	requestedSessionID string,
	hint *interfaces.FactorySessionLogicalResolveHint,
) (controlplane.SyncPreflightTarget, error) {
	serviceScope := r.BackendScopeID()
	if serviceScope == "" || strings.TrimSpace(hint.BackendScopeID) != serviceScope {
		return controlplane.SyncPreflightTarget{Unresolved: true}, nil
	}
	session := r.identity.ResolveLogical(r.state.Registry(), serviceScope, hint.LogicalSessionKeyID)
	if session == nil {
		return controlplane.SyncPreflightTarget{Unresolved: true}, nil
	}
	remapped := strings.TrimSpace(requestedSessionID) != "" &&
		session.ID != strings.TrimSpace(requestedSessionID)
	return controlplane.SyncPreflightTarget{Session: session, Remapped: remapped}, nil
}

func (r sessionIdentityReader) BackendScopeID() string {
	var session *livesession.LiveSession
	if r.state != nil {
		session = r.state.Current()
	}
	return r.backendScopeForSession(session)
}

func (r sessionIdentityReader) backendScopeForSession(session *livesession.LiveSession) string {
	scope := r.backendScope
	if bound := runtimebinding.SessionStateFrom(session); bound != nil && strings.TrimSpace(scope) == "" {
		scope = bound.ProjectionBackendScope
	}
	return runtimebinding.BackendScopeID(scope, session)
}

func (r sessionIdentityReader) LogicalSessionKeyID(session *livesession.LiveSession) string {
	if session == nil {
		return ""
	}
	placement := session.Placement()
	resolved, err := r.identity.Normalize(context.Background(), identity.NormalizeRequest{
		BackendScopeID: r.backendScopeForSession(session), FolderPath: placement.FolderPath, Target: placement.Target,
	})
	if err != nil {
		return ""
	}
	return resolved.LogicalSessionKeyID
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

func (h keyedSessionHost) BuildSessionProjectionContext(ctx context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	bound := runtimebinding.SessionStateFrom(session)
	reader := h.sessionProjectionReader
	if bound != nil && reader.backendScope == "" {
		reader.backendScope = bound.ProjectionBackendScope
	}
	if bound != nil && bound.Clock != nil {
		reader.clock = bound.Clock
	}
	return reader.BuildSessionProjectionContext(ctx, session)
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
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory runtime observation is required")
	}
	observationResult, err := runtime.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	bundle := runtimebinding.BundleFromSession(session)
	snapshot, sessionProjectionFacts, err := r.readProjectionFacts(ctx, session)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	var checkpointStore factoryruntime.JavaScriptCheckpointStore
	if interfaces.IsJavaScriptOrchestratorFactory(runtimeCfg.FactoryConfig()) {
		checkpointStore = sessionCheckpointStore(session, r.checkpoints)
	}
	startedAt := time.Time{}
	if bundle != nil {
		startedAt = bundle.StartTime()
	}
	backendScopeID := runtimebinding.BackendScopeID(r.backendScope, session)
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

func (r sessionProjectionReader) readProjectionFacts(ctx context.Context, session *livesession.LiveSession) (*legacysnapshot.Snapshot, *recordings.SessionProjectionFacts, error) {
	var snapshot *legacysnapshot.Snapshot
	var err error
	if runtime := runtimebinding.ServiceForLiveRuntime(session.Runtime); runtime != nil {
		if provider, ok := runtime.(legacysnapshot.WorkProvider); ok && provider != nil {
			snapshot, err = provider.GetWorkStateSnapshot(ctx)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	var sessionProjectionFacts *recordings.SessionProjectionFacts
	bundle := runtimebinding.BundleFromSession(session)
	if bundle != nil {
		if reader, ok := bundle.RecordingLedger().(recordings.SessionProjectionReader); ok && reader != nil {
			facts, factsErr := reader.CurrentSessionProjectionFacts()
			if factsErr != nil {
				return nil, nil, factsErr
			}
			sessionProjectionFacts = &facts
		}
	}
	return snapshot, sessionProjectionFacts, nil
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
