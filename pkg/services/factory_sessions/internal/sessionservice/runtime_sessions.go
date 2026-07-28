package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	identityservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

const (
	DefaultFactorySessionID         = factorysessions.DefaultSessionID
	FactorySessionTargetKindDefault = factorysessions.TargetKindDefault
	FactorySessionTargetKindNamed   = factorysessions.TargetKindNamed
)

type (
	FactorySessionTargetKind = factorysessions.TargetKind
	FactorySessionTargetRef  = factorysessions.TargetRef
	FactorySessionTarget     = factorysessions.Target
	FactorySessionOpenResult = factorysessions.OpenResult
	liveFactorySession       = livesession.LiveSession
)

func (fs *SessionRuntime) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if fs == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	legacyRuntime, ok := session.Runtime.Factory.(factory.APIFactory)
	if !ok {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("legacy Factory Runtime submission is required")
	}
	return legacyRuntime.SubmitWorkRequest(ctx, request)
}

func (fs *SessionRuntime) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	if fs == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	mover, ok := session.Runtime.Factory.(factory.WorkMover)
	if !ok {
		return work.OperatorMoveResult{}, fmt.Errorf("legacy Factory Runtime work move is required")
	}
	return mover.MoveWork(ctx, workID, stateName, work.WorkStateChangeSourceAPI, requestID)
}

func (fs *SessionRuntime) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID)
	if err != nil {
		return nil, err
	}
	legacyRuntime, ok := session.Runtime.Factory.(factory.APIFactory)
	if !ok {
		return nil, fmt.Errorf("legacy Factory Runtime event subscription is required")
	}
	stream, err := legacyRuntime.SubscribeFactoryEvents(ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID})
	if err != nil || stream == nil {
		return stream, err
	}
	stream.FactorySessionID = strings.TrimSpace(session.ID)
	identity, identityErr := fs.identity.Normalize(ctx, identityservice.NormalizeRequest{
		BackendScopeID: strings.TrimSpace(session.Runtime.BackendScopeID),
		FolderPath:     session.FolderPath, Target: session.Target,
	})
	if identityErr != nil {
		return nil, identityErr
	}
	stream.LogicalSessionKeyID = identity.LogicalSessionKeyID
	stream.BackendScopeID = strings.TrimSpace(session.Runtime.BackendScopeID)
	return stream, nil
}

func (fs *SessionRuntime) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet], error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID)
	if err != nil {
		return nil, err
	}
	legacyObservation, err := runtimebinding.LegacyObservationForService(session.Runtime.Factory)
	if err != nil {
		return nil, err
	}
	return legacyObservation.GetEngineStateSnapshot(ctx)
}

func (fs *SessionRuntime) ObserveForSession(
	ctx context.Context,
	sessionID string,
	req factory.ObserveRequest,
) (factory.ObserveResult, error) {
	if fs == nil {
		return factory.ObserveResult{}, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID)
	if err != nil {
		return factory.ObserveResult{}, err
	}
	runtime, ok := session.Runtime.Factory.(factory.Service)
	if !ok {
		return factory.ObserveResult{}, fmt.Errorf("Factory Runtime observation is required")
	}
	return runtime.Observe(ctx, req)
}

func (fs *SessionRuntime) CloseFactorySession(ctx context.Context, sessionID string) error {
	return fs.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

func (fs *SessionRuntime) openFactorySession(ctx context.Context, factoryDir string) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	if fs.sessionIDs == nil {
		return "", fmt.Errorf("Factory Session ID generator is required")
	}
	sessionID := strings.TrimSpace(fs.sessionIDs())
	if sessionID == "" {
		return "", fmt.Errorf("Factory Session ID generator returned an empty identity")
	}
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, factoryDir, factoryDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := fs.startBackgroundSession(ctx, sessionID, replacement); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *SessionRuntime) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	return fs.requireSessionGateway().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (fs *SessionRuntime) openFactorySessionForTarget(ctx context.Context, target FactorySessionTarget) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	if fs.sessionIDs == nil {
		return "", fmt.Errorf("Factory Session ID generator is required")
	}
	sessionID := strings.TrimSpace(fs.sessionIDs())
	if sessionID == "" {
		return "", fmt.Errorf("Factory Session ID generator returned an empty identity")
	}
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, target.FolderPath, target.FactoryDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := fs.StartBackgroundSessionWithMetadata(ctx, sessionID, replacement, target); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *SessionRuntime) startBackgroundSession(ctx context.Context, sessionID string, runtimeBundle factoryRuntimeBundle) error {
	return fs.StartBackgroundSessionWithMetadata(ctx, sessionID, runtimeBundle, FactorySessionTarget{
		Ref: FactorySessionTargetRef{
			Kind: FactorySessionTargetKindDefault,
		},
		FactoryDir: runtimeBundle.Directory(),
		FolderPath: runtimeBundle.Directory(),
		Project:    filepath.Base(runtimeBundle.Directory()),
	})
}

//nolint:contextcheck // The request context bounds startup waiting, while the active service runtime context owns the long-lived session runtime and sidecars.
func (fs *SessionRuntime) StartBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	if fs == nil {
		return fmt.Errorf("factory session service is required")
	}
	if runtimeBundle == nil {
		return fmt.Errorf("runtime bundle is required")
	}
	_, err := runtimebinding.Start(
		ctx,
		fs.sessionState,
		&fs.runtimeState,
		fs.factoryRootDir,
		sessionID,
		runtimeBundle,
		target,
		factory.RuntimeModeOrDefault(fs.runtimeMode) == interfaces.RuntimeModeService,
		fs.runtimeLifecycle,
		fs.StartLiveRuntimeSidecars,
		fs.StopLiveRuntime,
	)
	return err
}

func (fs *SessionRuntime) stopFactorySession(sessionID string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	return runtimebinding.StopSession(fs.sessionState, &fs.runtimeState, sessionID, fs.StopLiveRuntime)
}

func (fs *SessionRuntime) runSessionID() string {
	if fs == nil {
		return DefaultFactorySessionID
	}
	if runState := fs.runtimeState.Active(); runState != nil && strings.TrimSpace(runState.SessionID) != "" {
		return runState.SessionID
	}
	if session := fs.sessionState.Default(); session != nil {
		return session.ID
	}
	return DefaultFactorySessionID
}

func (fs *SessionRuntime) requireIdleRuntimeForSession(
	ctx context.Context,
	sessionID string,
) error {
	observationResult, err := fs.ObserveForSession(ctx, sessionID, factory.ObserveRequest{
		Scope: factory.ObservationScopeFull,
	})
	if err != nil {
		return fmt.Errorf("read session runtime status: %w", err)
	}
	return factory.RequireIdleRuntimeFromObservation(observationResult.Observation)
}

//nolint:contextcheck // The request context bounds the save/startup wait, while the long-lived service runtime context owns the replacement session runtime and sidecars after the request returns.
func (fs *SessionRuntime) ReplaceSessionRuntime(
	ctx context.Context,
	session *livesession.LiveSession,
	name string,
	replacement factoryRuntimeBundle,
) error {
	if fs == nil {
		return fmt.Errorf("factory session service is required")
	}
	if session == nil {
		return fmt.Errorf("%w: session handle is unavailable", factorysessions.ErrSessionNotFound)
	}
	previousScope, previousScopeErr := fs.sessionPersistenceScopeFromSession(ctx, session)
	serviceMode := factory.RuntimeModeOrDefault(fs.runtimeMode) == interfaces.RuntimeModeService
	updated, err := runtimebinding.Replace(
		ctx,
		fs.sessionState,
		&fs.runtimeState,
		session,
		replacement,
		serviceMode,
		fs.runtimeLifecycle,
		fs.StartLiveRuntimeSidecars,
		fs.StopLiveRuntime,
		func(err error) {
			sessionID := ""
			if session != nil {
				sessionID = session.ID
			}
			fs.logger.Warn("session runtime replacement warning", zap.Error(err), zap.String("session_id", sessionID))
		},
	)
	if err != nil {
		return err
	}
	if previousScopeErr == nil {
		if updated != nil {
			if currentScope, err := fs.sessionPersistenceScopeFromSession(ctx, updated); err == nil {
				if diagnostic, ok := factorysessioncursors.IdentityMismatchDiagnostic(
					previousScope,
					currentScope,
					session.ID,
				); ok {
					factorysessioncursors.NewZapObserver(fs.logger).Record(diagnostic)
				}
			}
		}
	}
	return nil
}
