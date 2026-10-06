package service

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
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
	liveFactorySession       = livesession.LiveSession
)

// BindRuntime publishes the opaque activation capability for one live
// Factory Session. The session keeps the hosted service only as a migration
// fallback; all subsequent domain operations resolve the binding first.
func (a *Assembly) scopedBindRuntime(fs *SessionRuntime, sessionID string, binding factory.RuntimeBinding) error {
	if fs == nil || fs.openingSession == nil || fs.openingSession.ID != sessionID {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return a.scopeActivation.Activate(context.Background(), SessionScope{Session: fs.openingSession, Binding: binding})
}

// SessionScope is the captured registration and opaque capability published by
// the existing Runtime activation bridge. It introduces no second registry.
type SessionScope struct {
	Session *livesession.LiveSession
	Binding factory.RuntimeBinding
}

// SessionScopeActivation publishes to the independent keyed session authority.
// Runtime activation and artifact cleanup remain owned by their existing bridge.
type SessionScopeActivation interface {
	Activate(context.Context, SessionScope) error
	Retire(context.Context, SessionScope) error
}

type scopeActivation struct {
	state *sessionruntime.Service
}

func NewScopeActivation(state *sessionruntime.Service) SessionScopeActivation {
	return &scopeActivation{state: state}
}

// Retire removes only the captured generation after its effects have joined.
// A missing or replaced registration is already retired; retries are harmless.
func (a *scopeActivation) Retire(ctx context.Context, scope SessionScope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.state.UnregisterGeneration(scope.Session)
	return nil
}

func (a *scopeActivation) Activate(ctx context.Context, scope SessionScope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Binding.IsZero() {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return a.state.UpdateRuntimeGeneration(scope.Session, func(runtime *factorysessions.LiveRuntime) error {
		binding := scope.Binding
		runtime.Binding = binding
		runtime.Factory = binding.Service()
		runtime.WorkAndEventIngress = runtimebinding.DeclaredWorkAndEventIngress(runtime.Factory)
		runtime.LiveChangeApplication = runtimebinding.NewLiveChangeApplication(runtime.Factory)
		runtime.LiveChangeAdmission = runtimebinding.NewLiveChangeAdmission(runtime.Factory)
		return nil
	})
}

func (a *Assembly) scopedSubmitWorkRequestForSession(fs *SessionRuntime, ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if fs == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(a.state, sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	legacyRuntime, ok := runtimebinding.WorkAndEventIngressForLiveRuntime(session.Runtime)
	if !ok {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Factory Runtime work submission is required")
	}
	return legacyRuntime.SubmitWorkRequest(ctx, request)
}

func (a *Assembly) scopedMoveWorkForSession(fs *SessionRuntime, ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	if fs == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(a.state, sessionID)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	runtime := runtimebinding.ServiceForLiveRuntime(session.Runtime)
	if runtime == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("Factory Runtime work move is required")
	}
	result, err := runtime.ControlMoveWork(ctx, factory.MoveWorkRequest{
		WorkID: workID, StateName: stateName,
		Source: factory.WorkMoveSource(work.WorkStateChangeSourceAPI), RequestID: requestID,
	})
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return work.OperatorMoveResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, nil
}

func (a *Assembly) scopedSubscribeFactoryEventsForSession(fs *SessionRuntime, ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(a.state, sessionID)
	if err != nil {
		return nil, err
	}
	legacyRuntime, ok := runtimebinding.WorkAndEventIngressForLiveRuntime(session.Runtime)
	if !ok {
		return nil, fmt.Errorf("Factory Runtime event subscription is required until Recordings migration")
	}
	stream, err := legacyRuntime.SubscribeFactoryEvents(
		ctx,
		reconnect,
		interfaces.FactoryEventReconnectScope{SessionID: livesession.EventScopeID(session)},
	)
	if err != nil || stream == nil {
		return stream, err
	}
	stream.FactorySessionID = strings.TrimSpace(session.ID)
	placement := session.Placement()
	identity, identityErr := a.identity.Normalize(ctx, identityservice.NormalizeRequest{
		BackendScopeID: strings.TrimSpace(session.Runtime.BackendScopeID),
		FolderPath:     placement.FolderPath, Target: placement.Target,
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
	session, err := runtimebinding.RequireLiveSession(fs.owner.state, sessionID)
	if err != nil {
		return nil, err
	}
	legacyObservation, err := runtimebinding.LegacyObservationForService(runtimebinding.ServiceForLiveRuntime(session.Runtime))
	if err != nil {
		return nil, err
	}
	return legacyObservation.GetEngineStateSnapshot(ctx)
}

func (a *Assembly) scopeObserveRuntimeForSession(fs *SessionRuntime,
	ctx context.Context,
	sessionID string,
	req factory.ObserveRequest,
) (factory.ObserveResult, error) {
	if fs == nil {
		return factory.ObserveResult{}, fmt.Errorf("factory session service is required")
	}
	session, err := runtimebinding.RequireLiveSession(a.state, sessionID)
	if err != nil {
		return factory.ObserveResult{}, err
	}
	runtime, ok := runtimebinding.ServiceForLiveRuntime(session.Runtime).(factory.Service)
	if !ok {
		return factory.ObserveResult{}, fmt.Errorf("Factory Runtime observation is required")
	}
	return runtime.Observe(ctx, req)
}

func (a *Assembly) scopedCloseFactorySession(fs *SessionRuntime, ctx context.Context, sessionID string) error {
	return fs.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

//nolint:contextcheck // The request context bounds startup waiting, while the active service runtime context owns the long-lived session runtime and sidecars.
func (a *Assembly) scopedStartBackgroundSessionWithMetadata(fs *SessionRuntime,
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
		a.state,
		&fs.runtimeState,
		fs.factoryRootDir,
		sessionID,
		runtimeBundle,
		target,
		runtimeModeOrDefault(fs.runtimeMode) == interfaces.RuntimeModeService,
		fs.runtimeLifecycle,
		fs.StartLiveRuntimeSidecars,
		fs.StopLiveRuntime,
	)
	return err
}

func (a *Assembly) scopeRunSessionID(fs *SessionRuntime) string {
	if fs == nil {
		return DefaultFactorySessionID
	}
	if runState := fs.runtimeState.Active(); runState != nil && strings.TrimSpace(runState.SessionID) != "" {
		return runState.SessionID
	}
	if session := a.state.Default(); session != nil {
		return session.ID
	}
	return DefaultFactorySessionID
}

func (a *Assembly) scopeRequireIdleRuntimeForSession(fs *SessionRuntime,
	ctx context.Context,
	sessionID string,
) error {
	observationResult, err := fs.observeRuntimeForSession(ctx, sessionID, factory.ObserveRequest{
		Scope: factory.ObservationScopeFull,
	})
	if err != nil {
		return fmt.Errorf("read session runtime status: %w", err)
	}
	return factory.RequireIdleRuntimeFromObservation(observationResult.Observation)
}

//nolint:contextcheck // The request context bounds the save/startup wait, while the long-lived service runtime context owns the replacement session runtime and sidecars after the request returns.
func (a *Assembly) scopedReplaceSessionRuntime(fs *SessionRuntime,
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
	serviceMode := runtimeModeOrDefault(fs.runtimeMode) == interfaces.RuntimeModeService
	updated, err := runtimebinding.Replace(
		ctx,
		a.state,
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
		func(sessionID string, runtime *factorysessions.LiveRuntime, record factory.RuntimeRecord) {
			a.retireWorkAdmissionProjection(sessionID, runtime, record)
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

func (fs *SessionRuntime) BindRuntime(sessionID string, binding factory.RuntimeBinding) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedBindRuntime(fs, sessionID, binding)
}

func (fs *SessionRuntime) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedSubmitWorkRequestForSession(fs, ctx, sessionID, request)
}

func (fs *SessionRuntime) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedMoveWorkForSession(fs, ctx, sessionID, workID, stateName, requestID)
}

func (fs *SessionRuntime) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedSubscribeFactoryEventsForSession(fs, ctx, sessionID, reconnect)
}

func (fs *SessionRuntime) observeRuntimeForSession(
	ctx context.Context,
	sessionID string,
	req factory.ObserveRequest,
) (factory.ObserveResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeObserveRuntimeForSession(fs, ctx, sessionID, req)
}

func (fs *SessionRuntime) CloseFactorySession(ctx context.Context, sessionID string) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedCloseFactorySession(fs, ctx, sessionID)
}

func (fs *SessionRuntime) StartBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedStartBackgroundSessionWithMetadata(fs, ctx, sessionID, runtimeBundle, target)
}

func (fs *SessionRuntime) runSessionID() string {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeRunSessionID(fs)
}

func (fs *SessionRuntime) requireIdleRuntimeForSession(
	ctx context.Context,
	sessionID string,
) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeRequireIdleRuntimeForSession(fs, ctx, sessionID)
}

func (fs *SessionRuntime) ReplaceSessionRuntime(
	ctx context.Context,
	session *livesession.LiveSession,
	name string,
	replacement factoryRuntimeBundle,
) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedReplaceSessionRuntime(fs, ctx, session, name, replacement)
}
