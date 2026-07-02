// backendsizecheck:ignore-file consolidated session runtime reads remain with runtime_sessions until dedicated service read seams split.
// pkgmaintcheck:ignore-file-lines consolidated session runtime reads remain with runtime_sessions until dedicated service read seams split.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/events"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/sessionpersistence"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

const (
	defaultFactorySessionID                     = factorysessions.DefaultSessionID
	FactorySessionTargetKindDefault             = factorysessions.TargetKindDefault
	FactorySessionTargetKindNamed               = factorysessions.TargetKindNamed
	runtimeMetricSessionResponseStreamPublished = "session_response_stream.published"
	runtimeMetricSessionResponseStreamCompacted = "session_response_stream.compacted"
	runtimeMetricSessionResponseStreamDegraded  = "session_response_stream.degraded"
)

type (
	FactorySessionTargetKind          = factorysessions.TargetKind
	FactorySessionTargetRef           = factorysessions.TargetRef
	FactorySessionTarget              = factorysessions.Target
	FactorySessionOpenResult          = factorysessions.OpenResult
	liveFactorySession                = factorysessions.LiveSession
	inferenceProgressPublisherFactory func(sessionID string) workerprovider.InferenceProgressPublisher
	dispatchCompletionObserverFactory func(sessionID string) func(dispatchID string)
)

// FactoryCoordinator owns session tracking and runtime lifecycle orchestration.
type FactoryCoordinator interface {
	ActivateNamedFactory(context.Context, string) error
	ListFactorySessions(context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(context.Context, string) (factoryapi.FactorySession, error)
	GetFactorySessionSyncPreflight(context.Context, string, interfaces.FactorySessionSyncPreflightOptions) (factoryapi.FactorySessionSyncPreflightResponse, error)
	GetFactorySessionResult(context.Context, string) (factoryapi.FactorySessionLiveResult, error)
	GetFactorySessionPartialResult(context.Context, string) (factoryapi.FactorySessionPartialResult, error)
	OpenFactorySession(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	CloseFactorySession(context.Context, string) error
	OpenFactorySessionFromFolder(context.Context, string, *FactorySessionTargetRef, bool, bool) (*FactorySessionOpenResult, error)
	SubmitWorkRequestForSession(context.Context, string, interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error)
	MoveWorkForSession(context.Context, string, string, string, string) (interfaces.OperatorMoveResult, error)
	SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	GetEngineStateSnapshotForSession(context.Context, string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
	startDefaultRuntime(context.Context, context.Context, bool) (*liveRuntimeHandle, error)
	startBackgroundSessionWithMetadata(context.Context, string, *factoryRuntimeBundle, FactorySessionTarget) error
	startLiveRuntimeSidecars(context.Context, *liveRuntimeHandle) error
	stopLiveRuntimeSidecars(*liveRuntimeHandle)
	stopLiveRuntime(*liveRuntimeHandle) error
	shutdownOtherLiveSessions(*liveRuntimeHandle) error
	replaceSessionRuntime(context.Context, *factorysessions.LiveSession, string, *factoryRuntimeBundle) error
}

type runtimeFactoryCoordinator struct {
	service *FactoryService
}

func liveSessionHandle(session *factorysessions.LiveSession) *liveRuntimeHandle {
	if session == nil {
		return nil
	}
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	return state.handle
}

func liveSessionRuntimeState(session *factorysessions.LiveSession) *liveSessionState {
	if session == nil {
		return nil
	}
	state, _ := session.Handle.(*liveSessionState)
	return state
}

func liveSessionBundle(session *factorysessions.LiveSession) *factoryRuntimeBundle {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	if state.handle != nil {
		return state.handle.Bundle
	}
	return state.bundle
}

func liveSessionBuildSpec(session *factorysessions.LiveSession) *runtimebuild.SessionBuildSpec {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	return state.spec
}

type liveSessionRegistration struct {
	factoryDir       string
	folderPath       string
	executionBaseDir string
	targetRef        FactorySessionTargetRef
	project          string
	preparedSpec     *runtimebuild.SessionBuildSpec
}

func (fs *FactoryService) buildLiveSessionRegistration(
	sessionID string,
	handle *liveRuntimeHandle,
	target FactorySessionTarget,
) liveSessionRegistration {
	registration := liveSessionRegistration{
		factoryDir: strings.TrimSpace(target.FactoryDir),
		folderPath: strings.TrimSpace(target.FolderPath),
		targetRef:  target.Ref,
		project:    strings.TrimSpace(target.Project),
	}
	if registration.factoryDir == "" && handle.Bundle != nil {
		registration.factoryDir = handle.Bundle.Dir
	}
	if registration.folderPath == "" {
		registration.folderPath = fs.factoryRootDir
	}
	if registration.folderPath == "" {
		registration.folderPath = registration.factoryDir
	}
	if registration.targetRef.Kind == "" {
		registration.targetRef = FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault}
	}
	if registration.project == "" {
		registration.project = filepath.Base(registration.folderPath)
	}
	registration.executionBaseDir = liveSessionExecutionBaseDir(handle, registration.folderPath, registration.factoryDir)
	if existing := liveSessionRuntimeState(fs.sessionByID(sessionID)); existing != nil {
		registration.preparedSpec = existing.spec
	}
	return registration
}

func liveSessionExecutionBaseDir(handle *liveRuntimeHandle, folderPath string, factoryDir string) string {
	executionBaseDir := ""
	if handle != nil && handle.Bundle != nil && handle.Bundle.RuntimeCfg != nil {
		executionBaseDir = strings.TrimSpace(handle.Bundle.RuntimeCfg.RuntimeBaseDir())
	}
	if executionBaseDir == "" {
		executionBaseDir = folderPath
	}
	if executionBaseDir == "" {
		executionBaseDir = factoryDir
	}
	return executionBaseDir
}

func newFactoryCoordinator(fs *FactoryService) FactoryCoordinator {
	return &runtimeFactoryCoordinator{service: fs}
}

func (fs *FactoryService) requireCoordinator() FactoryCoordinator {
	if fs == nil {
		return newFactoryCoordinator(nil)
	}
	if fs.coordinator == nil {
		fs.coordinator = newFactoryCoordinator(fs)
	}
	return fs.coordinator
}

func (fs *FactoryService) registerLiveSession(
	sessionID string,
	handle *liveRuntimeHandle,
	target FactorySessionTarget,
	selectSession bool,
) {
	if fs == nil || fs.sessions == nil || sessionID == "" || handle == nil {
		return
	}
	registration := fs.buildLiveSessionRegistration(sessionID, handle, target)
	session := factorysessions.NewLiveSession(
		sessionID,
		registration.factoryDir,
		registration.folderPath,
		registration.executionBaseDir,
		registration.targetRef,
		&liveSessionState{bundle: handle.Bundle, handle: handle, spec: registration.preparedSpec},
		sessionID == defaultFactorySessionID,
		registration.project,
	)
	factorysessions.EnsureRuntimeFactorySessionID(session)
	fs.sessions.Upsert(session, selectSession)
}

func defaultSessionTargetFromRuntimeBundle(
	runtimeBundle *factoryRuntimeBundle,
	factoryRootDir string,
) FactorySessionTarget {
	target := FactorySessionTarget{
		Ref: FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
	}
	if runtimeBundle != nil {
		target.FactoryDir = runtimeBundle.Dir
		target.FolderPath = runtimeBundle.FolderPath
	}
	if strings.TrimSpace(target.FolderPath) == "" {
		target.FolderPath = factoryRootDir
	}
	if strings.TrimSpace(target.Project) == "" && target.FolderPath != "" {
		target.Project = filepath.Base(target.FolderPath)
	}
	return target
}

func (fs *FactoryService) unregisterLiveSession(sessionID string) {
	if fs == nil || fs.sessions == nil {
		return
	}
	fs.closeSessionResponseStreams(fs.sessionByID(sessionID))
	fs.sessions.Remove(sessionID)
}

func (fs *FactoryService) currentSession() *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.Current()
}

func (fs *FactoryService) defaultSession() *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.Get(defaultFactorySessionID)
}

func (fs *FactoryService) sessionByID(sessionID string) *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return nil
	}
	if session := fs.sessions.Get(trimmed); session != nil {
		return session
	}
	if logicaltarget.IsDefaultSessionSelector(trimmed) {
		return fs.defaultSession()
	}
	for _, id := range fs.sessions.IDs() {
		session := fs.sessions.Get(id)
		if session == nil {
			continue
		}
		if factorysessions.CanonicalFactorySessionID(session) == trimmed {
			return session
		}
	}
	return nil
}

func (fs *FactoryService) requireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	session := fs.sessionByID(sessionID)
	handle := liveSessionHandle(session)
	if session == nil || handle == nil || handle.Bundle == nil {
		return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	return session, nil
}

func (fs *FactoryService) sessionFactory(sessionID string) (factory.Factory, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).Bundle.Factory, nil
}

func (fs *FactoryService) sessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).Bundle.RuntimeCfg, nil
}

func (fs *FactoryService) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	return fs.requireCoordinator().SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (c *runtimeFactoryCoordinator) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	return factoryservice.SubmitWorkRequest(ctx, liveSessionHandle(session).Bundle, request)
}

func (fs *FactoryService) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	return fs.requireCoordinator().MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (c *runtimeFactoryCoordinator) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return interfaces.OperatorMoveResult{}, err
	}
	return factoryservice.MoveWork(ctx, liveSessionHandle(session).Bundle, workID, stateName, interfaces.WorkStateChangeSourceAPI, requestID)
}

// MoveWork applies a synchronous operator relocation on the current service-owned runtime.
func (fs *FactoryService) MoveWork(ctx context.Context, workID, stateName string, source interfaces.WorkStateChangeSource, requestID string) (interfaces.OperatorMoveResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	return factoryservice.MoveWork(ctx, fs.currentRuntimeBundle(), workID, stateName, source, requestID)
}

func (fs *FactoryService) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	return fs.requireCoordinator().SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (c *runtimeFactoryCoordinator) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	stream, err := factoryservice.SubscribeFactoryEventsForSession(ctx, liveSessionHandle(session).Bundle, sessionID, reconnect)
	if err != nil || stream == nil || session == nil {
		return stream, err
	}
	stream.FactorySessionID = factorysessions.CanonicalFactorySessionID(session)
	stream.LogicalSessionKeyID = factorySessionLogicalSessionKeyID(fs, session)
	return stream, err
}

func (fs *FactoryService) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return fs.requireCoordinator().GetEngineStateSnapshotForSession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return factoryservice.GetEngineStateSnapshot(ctx, liveSessionHandle(session).Bundle)
}

func (fs *FactoryService) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return fs.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return c.service.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (fs *FactoryService) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	return fs.requireSessionGateway().OpenFactorySession(ctx, request)
}

func (c *runtimeFactoryCoordinator) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	if c.service == nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("factory service is required")
	}
	return c.service.requireSessionGateway().OpenFactorySession(ctx, request)
}

func (fs *FactoryService) CloseFactorySession(ctx context.Context, sessionID string) error {
	return fs.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) CloseFactorySession(ctx context.Context, sessionID string) error {
	if c.service == nil {
		return fmt.Errorf("factory service is required")
	}
	return c.service.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

func (fs *FactoryService) openFactorySession(ctx context.Context, factoryDir string) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	sessionID := factorysessions.NewSessionID()
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, factoryDir, factoryDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := fs.startBackgroundSession(ctx, sessionID, replacement); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *FactoryService) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	return fs.requireSessionGateway().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (c *runtimeFactoryCoordinator) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	if c.service == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return c.service.requireSessionGateway().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (fs *FactoryService) openFactorySessionForTarget(ctx context.Context, target FactorySessionTarget) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	sessionID := factorysessions.NewSessionID()
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, target.FolderPath, target.FactoryDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := fs.startBackgroundSessionWithMetadata(ctx, sessionID, replacement, target); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *FactoryService) startBackgroundSession(ctx context.Context, sessionID string, runtimeBundle *factoryRuntimeBundle) error {
	return fs.startBackgroundSessionWithMetadata(ctx, sessionID, runtimeBundle, FactorySessionTarget{
		Ref: FactorySessionTargetRef{
			Kind: FactorySessionTargetKindDefault,
		},
		FactoryDir: runtimeBundle.Dir,
		FolderPath: runtimeBundle.Dir,
		Project:    filepath.Base(runtimeBundle.Dir),
	})
}

//nolint:contextcheck // The request context bounds startup waiting, while the active service runtime context owns the long-lived session runtime and sidecars.
func (fs *FactoryService) startBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle *factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	return fs.requireCoordinator().startBackgroundSessionWithMetadata(ctx, sessionID, runtimeBundle, target)
}

func (c *runtimeFactoryCoordinator) startBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle *factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	fs := c.service
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	if runtimeBundle == nil {
		return fmt.Errorf("runtime bundle is required")
	}
	serviceCtx := ctx
	if runState := fs.currentRunState(); runState != nil && runState.ctx != nil {
		serviceCtx = runState.ctx
	}
	handle := fs.startLiveRuntime(serviceCtx, runtimeBundle)
	if err := fs.waitForLiveRuntimeStart(ctx, handle); err != nil {
		_ = fs.stopLiveRuntime(handle)
		return fmt.Errorf("start runtime session: %w", err)
	}
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService {
		if err := fs.startLiveRuntimeSidecars(serviceCtx, handle); err != nil {
			_ = fs.stopLiveRuntime(handle)
			return fmt.Errorf("start runtime session sidecars: %w", err)
		}
	}
	fs.registerLiveSession(sessionID, handle, target, false)
	return nil
}

func (fs *FactoryService) stopFactorySession(sessionID string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	session := fs.sessionByID(sessionID)
	handle := liveSessionHandle(session)
	if session == nil || handle == nil {
		return fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}

	runState := fs.currentRunState()
	if runState != nil && runState.sessionID == sessionID {
		successor := fs.nextLiveSessionAfterStop(sessionID)
		if successor != nil {
			fs.setRunState(runState.ctx, successor.ID, liveSessionHandle(successor))
		} else {
			fs.clearRunState()
		}
	}

	fs.unregisterLiveSession(sessionID)
	if err := fs.stopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (fs *FactoryService) runSessionID() string {
	if fs == nil {
		return defaultFactorySessionID
	}
	if runState := fs.currentRunState(); runState != nil && strings.TrimSpace(runState.sessionID) != "" {
		return runState.sessionID
	}
	return defaultFactorySessionID
}

func (fs *FactoryService) requireIdleRuntimeForSession(
	ctx context.Context,
	sessionID string,
) error {
	snapshot, err := fs.GetEngineStateSnapshotForSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("read session runtime status: %w", err)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusIdle {
		return fmt.Errorf("%w: current runtime status is %s", ErrFactoryActivationRequiresIdle, snapshot.RuntimeStatus)
	}
	if snapshotHasActiveWork(snapshot) {
		return fmt.Errorf("%w: current runtime has active work", ErrFactoryActivationRequiresIdle)
	}
	return nil
}

func sessionServiceContext(ctx context.Context, runState *serviceRunState) context.Context {
	if runState != nil && runState.ctx != nil {
		return runState.ctx
	}
	return ctx
}

func (fs *FactoryService) startReplacementSessionRuntime(
	ctx context.Context,
	serviceCtx context.Context,
	replacement *factoryRuntimeBundle,
) (*liveRuntimeHandle, error) {
	serviceMode := runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService
	return factoryservice.StartReplacement(factoryservice.StartReplacementInput{
		ReadinessCtx:                ctx,
		ServiceCtx:                  serviceCtx,
		Bundle:                      replacement,
		Clock:                       fs.clock,
		AttachSidecars:              fs.startLiveRuntimeSidecars,
		AttachSidecarsInServiceMode: serviceMode,
	})
}

//nolint:contextcheck // The request context bounds the save/startup wait, while the long-lived service runtime context owns the replacement session runtime and sidecars after the request returns.
func (fs *FactoryService) replaceSessionRuntime(
	ctx context.Context,
	session *factorysessions.LiveSession,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	return fs.requireCoordinator().replaceSessionRuntime(ctx, session, name, replacement)
}

func (c *runtimeFactoryCoordinator) replaceSessionRuntime(
	ctx context.Context,
	session *factorysessions.LiveSession,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	fs := c.service
	if session == nil {
		return fmt.Errorf("%w: session handle is unavailable", apisurface.ErrFactorySessionNotFound)
	}
	handle := liveSessionHandle(session)
	if handle == nil {
		return fmt.Errorf("%w: session handle is unavailable", apisurface.ErrFactorySessionNotFound)
	}
	runState := fs.currentRunState()
	serviceCtx := sessionServiceContext(ctx, runState)
	isActiveSession := runState != nil && runState.sessionID == session.ID

	serviceMode := runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService
	attempt := &factoryservice.ReplacementAttempt{
		Current:         handle,
		ServiceCtx:      serviceCtx,
		ServiceMode:     serviceMode,
		RestoreSidecars: fs.startLiveRuntimeSidecars,
	}
	attempt.Begin()
	defer attempt.End()

	replacementHandle, err := fs.startReplacementSessionRuntime(ctx, serviceCtx, replacement)
	if err != nil {
		return err
	}

	fs.publishFactoryChangeEvent(ctx, handle, replacement)
	attempt.Commit()
	executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir)
	if replacement.RuntimeCfg != nil {
		if runtimeBaseDir := strings.TrimSpace(replacement.RuntimeCfg.RuntimeBaseDir()); runtimeBaseDir != "" {
			executionBaseDir = runtimeBaseDir
		}
	}
	fs.closeSessionResponseStreams(session)
	replacementSession := factorysessions.NewLiveSession(
		session.ID,
		replacement.Dir,
		session.FolderPath,
		executionBaseDir,
		session.Target,
		&liveSessionState{handle: replacementHandle, spec: liveSessionBuildSpec(session)},
		session.IsDefault,
		session.Project,
	)
	replacementSession.RuntimeFactorySessionID = session.RuntimeFactorySessionID
	factorysessions.EnsureRuntimeFactorySessionID(replacementSession)
	fs.sessions.Upsert(replacementSession, isActiveSession)
	if isActiveSession {
		fs.setRunState(serviceCtx, session.ID, replacementHandle)
	}
	if err := fs.stopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		fs.logger.Warn("prior session runtime shutdown failed", zap.Error(err), zap.String("session_id", session.ID))
	}
	return nil
}

func (fs *FactoryService) nextLiveSessionAfterStop(sessionID string) *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	for _, id := range fs.sessions.IDs() {
		if id == sessionID {
			continue
		}
		next := fs.sessionByID(id)
		if next != nil && liveSessionHandle(next) != nil {
			return next
		}
	}
	return nil
}

func (fs *FactoryService) discoverFactorySessionTargets(folderPath string) ([]FactorySessionTarget, error) {
	return factorysessions.DiscoverTargets(folderPath, fs.probeFactorySessionTarget)
}

func (fs *FactoryService) probeFactorySessionTarget(
	folderPath string,
	factoryDir string,
	ref factorysessions.TargetRef,
) (factorysessions.Target, bool, *factorysessions.DiscoveryFailure) {
	if fs == nil {
		return factorysessions.Target{}, false, nil
	}
	loaded, err := configload.LoadRuntimeConfigFromFactoryDir(factoryDir, fs.coordinatorPolicy().workstationLoader)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && !configload.IsFactoryLayoutNotFound(err) && !configload.IsNamedFactoryNotFound(err) {
			fs.logFactorySessionTargetProbeFailure(folderPath, factoryDir, ref, err)
			return factorysessions.Target{}, false, &factorysessions.DiscoveryFailure{
				FactoryDir: factoryDir,
				Ref:        ref,
				Summary:    err.Error(),
			}
		}
		return factorysessions.Target{}, false, nil
	}

	project := ""
	if cfg := loaded.FactoryConfig(); cfg != nil {
		project = strings.TrimSpace(cfg.Project)
		if project == "" {
			project = strings.TrimSpace(cfg.Name)
		}
	}
	return factorysessions.BuildTargetFromConfig(folderPath, factoryDir, ref, project), true, nil
}

func (fs *FactoryService) logFactorySessionTargetProbeFailure(
	folderPath string,
	factoryDir string,
	ref factorysessions.TargetRef,
	err error,
) {
	if fs == nil || err == nil {
		return
	}
	logger := fs.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{
		zap.String("submitted_folder_path", folderPath),
		zap.String("target_factory_dir", factoryDir),
		zap.String("target_kind", string(ref.Kind)),
		zap.String("target_display_name", factorysessions.TargetDisplayName(ref)),
		zap.String("failure_summary", err.Error()),
		zap.Error(err),
	}
	if ref.Kind == factorysessions.TargetKindNamed && strings.TrimSpace(ref.Name) != "" {
		fields = append(fields, zap.String("target_name", strings.TrimSpace(ref.Name)))
	}
	logger.Error("factory session discovery target runtime config load failed", fields...)
}

func (fs *FactoryService) waitForServiceModeStartupWorkReadability(ctx context.Context, serviceMode bool) error {
	policy := fs.coordinatorPolicy()
	if !serviceMode || policy.workFile == "" || policy.apiServerReady == nil || policy.port <= 0 || policy.apiServerStarter == nil {
		return nil
	}
	apiServerExit := fs.apiServerExit
	select {
	case <-policy.apiServerReady:
	case err := <-apiServerExit:
		return startupReadinessError(err)
	case <-ctx.Done():
		return ctx.Err()
	}

	timer := time.NewTimer(serviceModeStartupWorkReadabilityDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case err := <-apiServerExit:
		return startupReadinessError(err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (fs *FactoryService) failServiceModeStartup(currentRuntime *liveRuntimeHandle, startupErr error) error {
	fs.clearRunState()
	fs.unregisterLiveSession(defaultFactorySessionID)
	if currentRuntime == nil {
		return startupErr
	}
	if stopErr := fs.stopLiveRuntime(currentRuntime); stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		return errors.Join(startupErr, stopErr)
	}
	return startupErr
}

func startupReadinessError(err error) error {
	if err == nil {
		return fmt.Errorf("wait for service-mode startup work readiness: API server stopped before signaling readiness")
	}
	return fmt.Errorf("wait for service-mode startup work readiness: %w", err)
}

func (fs *FactoryService) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	return fs.requireSessionGateway().ListFactorySessions(ctx)
}

func (c *runtimeFactoryCoordinator) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if c.service == nil {
		return factoryapi.ListFactorySessionsResponse{}, fmt.Errorf("factory service is required")
	}
	return c.service.requireSessionGateway().ListFactorySessions(ctx)
}

func (fs *FactoryService) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	return fs.requireSessionGateway().GetFactorySession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	if c.service == nil {
		return factoryapi.FactorySession{}, fmt.Errorf("factory service is required")
	}
	return c.service.requireSessionGateway().GetFactorySession(ctx, sessionID)
}

func (fs *FactoryService) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	options interfaces.FactorySessionSyncPreflightOptions,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	response, err := fs.requireCoordinator().GetFactorySessionSyncPreflight(ctx, sessionID, options)
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	fs.recordSessionPersistenceInvalidationFromPreflight(response)
	return response, nil
}

func (c *runtimeFactoryCoordinator) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	options interfaces.FactorySessionSyncPreflightOptions,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	fs := c.service
	response := newFactorySessionSyncPreflightResponse(sessionID, options.Reconnect)
	if strings.HasPrefix(sessionID, "dur-sess-") {
		response.ReasonCode = factoryapi.SessionNotFound
		return response, nil
	}

	resolved, err := fs.resolveSessionSyncPreflightTarget(sessionID, options)
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	if resolved.invalidTarget {
		response.ReasonCode = factoryapi.InvalidTargetReference
		return response, nil
	}
	if resolved.session == nil {
		response.ReasonCode = factoryapi.SessionNotFound
		return response, nil
	}
	session := resolved.session

	response.BackendScopeId = stringPointer(factorySessionBackendScopeID(fs, session))
	response.LogicalSessionKeyId = stringPointer(factorySessionLogicalSessionKeyID(fs, session))
	response.FactorySessionId = stringPointer(factorysessions.CanonicalFactorySessionID(session))
	response.StreamGenerationId = stringPointer(factorySessionStreamGenerationID(fs, session))
	response.NormalizedTarget = factorySessionNormalizedLogicalTarget(fs, session)
	if resolved.remapped {
		response.ReasonCode = factoryapi.LogicalSessionRemap
		return response, nil
	}

	if !response.ReconnectCursor.Provided {
		response.ReasonCode = factoryapi.Ok
		response.CheckpointReusable = true
		return response, nil
	}

	handle := liveSessionHandle(session)
	eventsSnapshot := []factoryapi.FactoryEvent(nil)
	if handle != nil && handle.Bundle != nil && handle.Bundle.EventHistory != nil {
		eventsSnapshot = handle.Bundle.EventHistory.Events()
	}
	_, err = events.BuildReconnectReplay(
		eventsSnapshot,
		*options.Reconnect,
		interfaces.FactoryEventReconnectScope{SessionID: session.ID},
	)
	if err != nil {
		if errors.Is(err, events.ErrReconnectCursorNotFound) {
			response.ReasonCode = factoryapi.CursorStale
			return response, nil
		}
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}

	response.ReasonCode = factoryapi.Ok
	response.CheckpointReusable = true
	response.ReconnectCursor.ValidForStreamGeneration = true
	return response, nil
}

type sessionSyncPreflightTarget struct {
	session       *factorysessions.LiveSession
	remapped      bool
	invalidTarget bool
}

func (fs *FactoryService) resolveSessionSyncPreflightTarget(
	sessionID string,
	options interfaces.FactorySessionSyncPreflightOptions,
) (sessionSyncPreflightTarget, error) {
	if fs == nil {
		return sessionSyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	requestedID := strings.TrimSpace(sessionID)
	requestedBackendScopeID := stringPointerValue(options.BackendScopeID)
	requestedLogicalSessionKeyID := strings.TrimSpace(stringPointerValue(options.LogicalSessionKeyID))

	if requestedBackendScopeID != "" && !factorySessionBackendScopeMatches(fs, requestedBackendScopeID) {
		return sessionSyncPreflightTarget{}, nil
	}
	if requestedLogicalSessionKeyID != "" && !logicaltarget.IsLogicalSessionKeyID(requestedLogicalSessionKeyID) {
		return sessionSyncPreflightTarget{invalidTarget: true}, nil
	}

	directSession, err := fs.lookupDirectSessionForPreflight(requestedID)
	if err != nil {
		return sessionSyncPreflightTarget{}, err
	}
	logicalSession := fs.lookupLogicalSessionForPreflight(requestedLogicalSessionKeyID)
	if target, ok := mergeDirectAndLogicalPreflightSessions(requestedID, directSession, logicalSession); ok {
		return target, nil
	}
	if requestedID == defaultFactorySessionID {
		if session := fs.preflightDefaultSessionSuccessor(); session != nil {
			return sessionSyncPreflightTarget{session: session, remapped: true}, nil
		}
	}
	return sessionSyncPreflightTarget{}, nil
}

func (fs *FactoryService) lookupDirectSessionForPreflight(requestedID string) (*factorysessions.LiveSession, error) {
	if requestedID == "" {
		return nil, nil
	}
	session, err := fs.requireSession(requestedID)
	if err == nil {
		return session, nil
	}
	if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		return nil, nil
	}
	return nil, err
}

func (fs *FactoryService) lookupLogicalSessionForPreflight(requestedLogicalSessionKeyID string) *factorysessions.LiveSession {
	if requestedLogicalSessionKeyID == "" {
		return nil
	}
	return fs.findLiveSessionByLogicalSessionKeyID(requestedLogicalSessionKeyID)
}

func mergeDirectAndLogicalPreflightSessions(
	requestedID string,
	directSession *factorysessions.LiveSession,
	logicalSession *factorysessions.LiveSession,
) (sessionSyncPreflightTarget, bool) {
	switch {
	case directSession != nil && logicalSession != nil:
		if directSession.ID == logicalSession.ID {
			return sessionSyncPreflightTarget{session: directSession}, true
		}
		return sessionSyncPreflightTarget{session: logicalSession, remapped: true}, true
	case logicalSession != nil:
		remapped := requestedID == "" || requestedID != logicalSession.ID
		return sessionSyncPreflightTarget{session: logicalSession, remapped: remapped}, true
	case directSession != nil:
		return sessionSyncPreflightTarget{session: directSession}, true
	default:
		return sessionSyncPreflightTarget{}, false
	}
}

func (fs *FactoryService) findLiveSessionByLogicalSessionKeyID(logicalSessionKeyID string) *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	trimmed := strings.TrimSpace(logicalSessionKeyID)
	if !logicaltarget.IsLogicalSessionKeyID(trimmed) {
		return nil
	}
	for _, sessionID := range fs.sessions.IDs() {
		session := fs.sessions.Get(sessionID)
		if session == nil {
			continue
		}
		if factorySessionLogicalSessionKeyID(fs, session) == trimmed {
			return session
		}
	}
	return nil
}

func factorySessionBackendScopeMatches(fs *FactoryService, requestedBackendScopeID string) bool {
	if fs == nil {
		return false
	}
	activeScopeID := strings.TrimSpace(factorySessionBackendScopeID(fs, nil))
	requestedScopeID := strings.TrimSpace(requestedBackendScopeID)
	return activeScopeID != "" && requestedScopeID == activeScopeID
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (fs *FactoryService) preflightDefaultSessionSuccessor() *factorysessions.LiveSession {
	if fs == nil {
		return nil
	}
	if runState := fs.currentRunState(); runState != nil {
		successorID := strings.TrimSpace(runState.sessionID)
		if successorID != "" && successorID != defaultFactorySessionID {
			if session, err := fs.requireSession(successorID); err == nil {
				return session
			}
		}
	}
	current := fs.currentSession()
	if current == nil || current.ID == defaultFactorySessionID {
		return nil
	}
	if session, err := fs.requireSession(current.ID); err == nil {
		return session
	}
	return nil
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
		Session:          session,
		FactoryCfg:       factoryCfg,
		BackendScopeID:   factorySessionBackendScopeID(fs, session),
		RuntimeStartedAt: liveSessionBundle(session).StartedAtUTC,
		Now:              time.Now().UTC(),
	}
	snapshot, err := fs.GetEngineStateSnapshotForSession(ctx, session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	projectionCtx.Snapshot = snapshot
	projectionCtx.LifecycleControlStatus = snapshot.LifecycleControlStatus
	checkpointStore := (*factorysessions.JavaScriptCheckpointStore)(nil)
	if interfaces.IsJavaScriptOrchestratorFactory(factoryCfg) {
		checkpointStore = fs.requireSessionGateway().JavaScriptCheckpointStore(session)
		projectionCtx.JavaScriptCheckpoints = checkpointStore.List()
	}
	projectionCtx.JavaScript, err = fs.projectJavaScriptRuntimeState(session, checkpointStore, snapshot.TickCount)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	projectionCtx.Enabled = factorysessions.EnabledTransitionsForSnapshot(ctx, snapshot, runtimeCfg)
	projectionCtx.LogicalSessionKeyID = factorySessionLogicalSessionKeyID(fs, session)
	projectionCtx.NormalizedTarget = factorySessionNormalizedLogicalTarget(fs, session)
	return projectionCtx, nil
}

func (fs *FactoryService) projectJavaScriptRuntimeState(
	session *factorysessions.LiveSession,
	checkpointStore *factorysessions.JavaScriptCheckpointStore,
	selectedTick int,
) (*interfaces.FactorySessionJavaScriptRuntimeState, error) {
	state := (*interfaces.FactorySessionJavaScriptRuntimeState)(nil)
	handle := liveSessionHandle(session)
	if handle != nil && handle.Bundle != nil && handle.Bundle.EventHistory != nil {
		worldState, err := projections.ReconstructFactoryWorldState(handle.Bundle.EventHistory.Events(), selectedTick)
		if err != nil {
			return nil, err
		}
		state = worldState.JavaScriptRuntime
	}
	return factorysessions.JavaScriptRuntimeStateFromCheckpoints(checkpointStore, state), nil
}

func (fs *FactoryService) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	return fs.requireSessionGateway().GetFactorySessionResult(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	if c.service == nil {
		return factoryapi.FactorySessionLiveResult{}, fmt.Errorf("factory service is required")
	}
	return c.service.requireSessionGateway().GetFactorySessionResult(ctx, sessionID)
}

func (fs *FactoryService) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	return fs.requireSessionGateway().GetFactorySessionPartialResult(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	if c.service == nil {
		return factoryapi.FactorySessionPartialResult{}, fmt.Errorf("factory service is required")
	}
	return c.service.requireSessionGateway().GetFactorySessionPartialResult(ctx, sessionID)
}

func (fs *FactoryService) javascriptCheckpointStoreDirect(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	if state.javascriptCheckpoints == nil {
		state.javascriptCheckpoints = factorysessions.NewJavaScriptCheckpointStore()
	}
	return state.javascriptCheckpoints
}

func (fs *FactoryService) sessionResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	state.responseStreamsOnce.Do(func() {
		state.responseStreams = fs.newSessionResponseStreamSetInstance()
	})
	return state.responseStreams
}

func (fs *FactoryService) sessionResponseStream(
	session *factorysessions.LiveSession,
	dispatchID string,
) *factorysessions.SessionResponseStream {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return nil
	}
	return streams.Stream(dispatchID)
}

func (fs *FactoryService) closeSessionResponseStreams(session *factorysessions.LiveSession) {
	fs.requireSessionGateway().CloseSessionResponseStreams(session)
}

func (fs *FactoryService) closeSessionResponseStreamsDirect(session *factorysessions.LiveSession) {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return
	}
	streams.Close()
}

func (fs *FactoryService) closeSessionResponseStreamDispatchDirect(
	session *factorysessions.LiveSession,
	dispatchID string,
) bool {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return false
	}
	return streams.CloseDispatch(dispatchID)
}

func (fs *FactoryService) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	return fs.requireSessionGateway().SubscribeSessionResponseStream(sessionID, dispatchID, afterSequence)
}

func (fs *FactoryService) SessionResponseStreamDispatchIDs(sessionID string) ([]string, error) {
	return fs.requireSessionGateway().SessionResponseStreamDispatchIDs(sessionID)
}

func (fs *FactoryService) newSessionResponseStreamInstance() *factorysessions.SessionResponseStream {
	if fs != nil && fs.newSessionResponseStream != nil {
		return fs.newSessionResponseStream()
	}
	return factorysessions.NewSessionResponseStream()
}

func (fs *FactoryService) newSessionResponseStreamSetInstance() *factorysessions.SessionResponseStreamSet {
	return factorysessions.NewSessionResponseStreamSetWithFactory(func() *factorysessions.SessionResponseStream {
		return fs.newSessionResponseStreamInstance()
	})
}

func newFactorySessionSyncPreflightResponse(
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) factoryapi.FactorySessionSyncPreflightResponse {
	response := factoryapi.FactorySessionSyncPreflightResponse{
		RequestedSessionId: strings.TrimSpace(sessionID),
		ReasonCode:         factoryapi.SessionNotFound,
		ReconnectCursor: factoryapi.FactorySessionSyncPreflightReconnectCursor{
			Provided: reconnect != nil && (strings.TrimSpace(reconnect.AfterEventID) != "" || reconnect.AfterSequence != nil),
		},
	}
	if reconnect == nil {
		return response
	}
	if afterEventID := strings.TrimSpace(reconnect.AfterEventID); afterEventID != "" {
		response.ReconnectCursor.AfterEventId = &afterEventID
	}
	if reconnect.AfterSequence != nil {
		value := int64(*reconnect.AfterSequence)
		response.ReconnectCursor.AfterSequence = &value
	}
	return response
}

func factorySessionBackendScopeID(fs *FactoryService, session *factorysessions.LiveSession) string {
	_ = session
	if fs != nil && fs.cfg != nil {
		if backendScopeID := strings.TrimSpace(fs.cfg.BackendScopeID); backendScopeID != "" {
			return backendScopeID
		}
	}
	if bundle := liveSessionBundle(session); bundle != nil {
		if backendScopeID := strings.TrimSpace(bundle.BackendScopeID); backendScopeID != "" {
			return backendScopeID
		}
	}
	return ""
}

func factorySessionLogicalSessionKeyID(fs *FactoryService, session *factorysessions.LiveSession) string {
	if session == nil {
		return ""
	}
	backendScopeID := factorySessionBackendScopeID(fs, session)
	if backendScopeID == "" {
		return ""
	}
	ref, err := logicaltarget.NormalizeTargetRef(
		backendScopeID,
		session.FolderPath,
		session.Target,
	)
	if err != nil {
		return ""
	}
	return logicaltarget.DeriveLogicalSessionKeyID(ref)
}

func factorySessionNormalizedLogicalTarget(
	fs *FactoryService,
	session *factorysessions.LiveSession,
) *factoryapi.FactorySessionLogicalTarget {
	if session == nil {
		return nil
	}
	backendScopeID := factorySessionBackendScopeID(fs, session)
	if backendScopeID == "" {
		return nil
	}
	target, err := logicaltarget.APILogicalTargetFromSession(backendScopeID, session)
	if err != nil || target == nil {
		return nil
	}
	return target
}

func factorySessionStreamGenerationID(fs *FactoryService, session *factorysessions.LiveSession) string {
	if fs != nil && session != nil {
		if snapshot, err := fs.GetEngineStateSnapshotForSession(context.Background(), session.ID); err == nil {
			if streamGenerationID := strings.TrimSpace(snapshot.StreamGenerationID); streamGenerationID != "" {
				return streamGenerationID
			}
		}
	}
	if bundle := liveSessionBundle(session); bundle != nil && !bundle.StartedAtUTC.IsZero() {
		return bundle.StartedAtUTC.UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func stringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func mapInferenceProgressFragment(fragment workerprovider.InferenceProgressFragment) responsestream.Event {
	kind := responsestream.EventKindProgressFragment
	switch fragment.Kind {
	case workerprovider.ResponseFragmentKind:
		kind = responsestream.EventKindResponseFragment
	case workerprovider.CompletedFragmentKind:
		kind = responsestream.EventKindStreamCompleted
	case workerprovider.FailedFragmentKind:
		kind = responsestream.EventKindStreamFailed
	}
	return responsestream.Event{
		Kind:               kind,
		Type:               responsestream.EventType(strings.TrimSpace(fragment.Type)),
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(fragment.ProviderSessionRef),
		Payload:            fragment.Payload,
		ExternalEventType:  strings.TrimSpace(fragment.ExternalEventType),
		Metadata:           cloneStringMap(fragment.Metadata),
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func newInferenceProgressPublisherFactory(
	sessions *factorysessions.Registry,
	logger *zap.Logger,
) inferenceProgressPublisherFactory {
	if sessions == nil {
		return nil
	}
	gateway := newSessionGatewayService(&FactoryService{sessions: sessions})
	return gateway.InferenceProgressPublisherFactory(logger)
}

func newSessionDispatchCompletionObserverFactory(
	sessions *factorysessions.Registry,
) dispatchCompletionObserverFactory {
	if sessions == nil {
		return nil
	}
	gateway := newSessionGatewayService(&FactoryService{sessions: sessions})
	return gateway.DispatchCompletionObserverFactory()
}

func (fs *FactoryService) inferenceProgressPublisher(
	sessionID string,
	logger *zap.Logger,
) workerprovider.InferenceProgressPublisher {
	if fs == nil {
		return nil
	}
	factory := fs.requireSessionGateway().InferenceProgressPublisherFactory(logger)
	if factory == nil {
		return nil
	}
	return factory(sessionID)
}

func (fs *FactoryService) observeResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(event.DispatchID),
		Reason:     string(event.Kind),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamPublished, fields)
}

func (fs *FactoryService) observeResponseStreamCompaction(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	summary responsestream.CompactionSummary,
) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(dispatchID),
		Reason:     string(summary.Reason),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamCompacted, fields)
	if handle := liveSessionHandle(session); handle != nil && handle.Bundle != nil && handle.Bundle.Logger != nil {
		handle.Bundle.Logger.Warn("session response stream compacted internal provider progress",
			zap.String("session_id", sessionID),
			zap.String("dispatch_id", dispatchID),
			zap.String("compaction_reason", string(summary.Reason)),
			zap.Int("dropped_sequence_count", summary.DroppedSequenceCount),
			zap.Int64("first_retained_sequence", summary.FirstRetainedSequence),
			zap.Int64("last_dropped_sequence", summary.LastDroppedSequence),
		)
	}
}

func (fs *FactoryService) observeResponseStreamDegraded(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	reason string,
	fallbackLogger *zap.Logger,
	err error,
) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(dispatchID),
		Reason:     strings.TrimSpace(reason),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamDegraded, fields)

	log := fallbackLogger
	if handle := liveSessionHandle(session); handle != nil && handle.Bundle != nil && handle.Bundle.Logger != nil {
		log = handle.Bundle.Logger
	}
	if log == nil {
		return
	}
	logFields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("dispatch_id", strings.TrimSpace(dispatchID)),
		zap.String("reason", strings.TrimSpace(reason)),
	}
	if err != nil {
		logFields = append(logFields, zap.Error(err))
	}
	log.Warn("internal provider progress publication degraded", logFields...)
}

func emitSessionResponseStreamMetric(
	session *factorysessions.LiveSession,
	sessionID string,
	name string,
	fields metrics.Fields,
) {
	handle := liveSessionHandle(session)
	if handle == nil || handle.Bundle == nil {
		return
	}
	if fields.DispatchID == "" {
		fields.DispatchID = sessionID
	}
	if err := handle.Bundle.MetricsEmitter().Counter(context.Background(), name, 1, fields); err != nil {
		handle.Bundle.RuntimeLogger().Warn("session response stream metric emission failed",
			zap.String("metric_name", name),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
}

var _ apisurface.DurableSessionExecutionAPI = (*FactoryService)(nil)
var _ apisurface.DurableSessionListingAPI = (*FactoryService)(nil)
var _ apisurface.DurableSessionLifecycleAPI = (*FactoryService)(nil)

func (fs *FactoryService) StartDurableFactorySessionAsync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	startReq, err := factorysession.StartRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	result, err := fs.durableExecutionService().StartAsync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	return factorysession.AsyncStartResponseToAPI(result), nil
}

func (fs *FactoryService) StartDurableFactorySessionSync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	startReq, err := factorysession.StartRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	result, err := fs.durableExecutionService().StartSync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	return factorysession.SyncStartResponseToAPI(result), nil
}

func (fs *FactoryService) durableExecutionService() factorysessionexecution.Service {
	if fs == nil {
		return factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{})
	}
	fs.durableExecutionMu.Lock()
	defer fs.durableExecutionMu.Unlock()
	if fs.durableExecution == nil {
		fs.durableExecution = factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
			ProjectRoot: fs.durableProjectRoot(),
		})
	}
	return fs.durableExecution
}

func (fs *FactoryService) durableProjectRoot() string {
	if fs == nil {
		return ""
	}
	if fs.cfg != nil {
		if root := strings.TrimSpace(fs.cfg.ExecutionBaseDir); root != "" {
			return root
		}
		if root := strings.TrimSpace(fs.cfg.Dir); root != "" {
			return root
		}
	}
	return strings.TrimSpace(fs.factoryRootDir)
}

func (fs *FactoryService) ListDurableFactorySessions(
	ctx context.Context,
	params factoryapi.ListFactorySessionsParams,
) (factoryapi.ListFactorySessionsResponse, error) {
	req, err := factorysession.ListSessionsRequestFromAPI(params)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	result, err := fs.ListDurableExecutionSessions(ctx, req)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return factorysession.ListSessionsResponseToAPI(result), nil
}

// ListDurableExecutionSessions returns the shared durable session listing projection
// used by API merge logic before workspace rows are combined.
func (fs *FactoryService) ListDurableExecutionSessions(
	ctx context.Context,
	req factorysessionexecution.ListSessionsRequest,
) (factorysessionexecution.ListSessionsResult, error) {
	return fs.durableExecutionService().ListSessions(ctx, req)
}

func (fs *FactoryService) GetDurableFactorySession(
	ctx context.Context,
	sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	result, err := fs.durableExecutionService().GetSession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	return factorysession.SessionReadResponseToAPI(result), nil
}

func (fs *FactoryService) GetDurableFactorySessionResult(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetFactorySessionResultsParams,
) (factoryapi.FactorySessionResult, error) {
	req, err := factorysession.ResultRequestFromAPI(params)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	result, err := fs.durableExecutionService().GetResult(ctx, sessionID, req)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	return factorysession.ResultResponseToAPI(result), nil
}

func (fs *FactoryService) ReadDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) (*interfaces.FactoryEventStream, error) {
	reconnect, err := factorysession.EventReconnectRequestFromAPI(params)
	if err != nil {
		return nil, err
	}
	result, err := fs.durableExecutionService().ReadEvents(ctx, sessionID, reconnect)
	if err != nil {
		if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
			return nil, apisurface.ErrFactorySessionNotFound
		}
		if errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
			return nil, fmt.Errorf("%w: %v", apisurface.ErrInvalidEventReconnectCursor, err)
		}
		return nil, err
	}
	return factorysession.FactoryEventStreamFromReadResult(result), nil
}

func (fs *FactoryService) ListDurableFactorySessionDispatches(
	ctx context.Context,
	sessionID string,
) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	result, err := fs.durableExecutionService().ListDispatches(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	return factorysession.ListDispatchesResponseToAPI(result), nil
}

func (fs *FactoryService) GetDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID, dispatchID string,
) (factoryapi.FactoryDispatch, error) {
	result, err := fs.durableExecutionService().GetDispatch(ctx, sessionID, dispatchID)
	if err != nil {
		return factoryapi.FactoryDispatch{}, err
	}
	return factorysession.DispatchDetailResponseToAPI(result), nil
}

func (fs *FactoryService) ListDurableFactorySessionArtifacts(
	ctx context.Context,
	sessionID string,
) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	result, err := fs.durableExecutionService().ListArtifacts(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	return factorysession.ListArtifactsResponseToAPI(result), nil
}

func (fs *FactoryService) GetDurableFactorySessionArtifact(
	ctx context.Context,
	sessionID, artifactID string,
) (factoryapi.FactorySessionArtifactDetail, error) {
	result, err := fs.durableExecutionService().GetArtifact(ctx, sessionID, artifactID)
	if err != nil {
		return factoryapi.FactorySessionArtifactDetail{}, err
	}
	return factorysession.ArtifactDetailResponseToAPI(result), nil
}

func (fs *FactoryService) PauseDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().PauseDurableFactorySession(ctx, sessionID, request)
}

func (fs *FactoryService) ResumeDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().ResumeDurableFactorySession(ctx, sessionID, request)
}

func (fs *FactoryService) CancelDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().CancelDurableFactorySession(ctx, sessionID, request)
}

func (fs *FactoryService) TerminateDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().TerminateDurableFactorySession(ctx, sessionID, request)
}

func (fs *FactoryService) ApproveDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().ApproveDurableFactorySession(ctx, sessionID, request)
}

func (fs *FactoryService) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().RetryDurableFactorySessionDispatch(ctx, sessionID, request)
}

func (fs *FactoryService) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().InterruptDurableFactorySessionDispatch(ctx, sessionID, request)
}
func (fs *FactoryService) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().PauseLiveFactorySession(ctx, sessionID, request)
}

func (fs *FactoryService) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().ResumeLiveFactorySession(ctx, sessionID, request)
}

const (
	runtimeMetricLifecycleControl = "runtime.lifecycle_control"
)

func (fs *FactoryService) observeLiveLifecycleControl(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
	err error,
) {
	if fs == nil {
		return
	}

	outcomeClass := lifecycleControlOutcomeClass(outcome, err)
	fields := liveLifecycleControlLogFields(sessionID, operation, outcomeClass, status, control)
	switch outcomeClass {
	case lifecycleControlOutcomeClassNotFound,
		string(factorysessionexecution.LifecycleControlOutcomeInvalidState),
		string(factorysessionexecution.LifecycleControlOutcomeTerminalSession):
		fs.logger.Warn("factory session lifecycle control rejected", fields...)
	default:
		fs.logger.Info("factory session lifecycle control", fields...)
	}

	fs.emitLiveLifecycleControlMetric(sessionID, operation, outcomeClass)
}

func lifecycleControlOutcomeClass(
	outcome factorysessionexecution.LifecycleControlOutcome,
	err error,
) string {
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return lifecycleControlOutcomeClassNotFound
		}
		var controlErr *factorysessionexecution.ControlError
		if errors.As(err, &controlErr) {
			return string(controlErr.Outcome)
		}
		return "ERROR"
	}
	if outcome == "" {
		return "ERROR"
	}
	return string(outcome)
}

const (
	lifecycleControlOutcomeClassNotFound = "NOT_FOUND"
)

func liveLifecycleControlLogFields(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcomeClass string,
	status factorysessionexecution.LifecycleStatus,
	control factorysessionexecution.ControlRequest,
) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("operation", string(operation)),
		zap.String("outcome", outcomeClass),
	}
	if status != "" {
		fields = append(fields, zap.String("lifecycle_control_status", string(status)))
	}
	if requestID := control.RequestID; requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}
	return fields
}

func (fs *FactoryService) emitLiveLifecycleControlMetric(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcomeClass string,
) {
	if fs == nil {
		return
	}
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return
	}
	bundle := liveSessionHandle(session).Bundle
	if bundle == nil {
		return
	}
	bundle.EmitMetricCounter(runtimeMetricLifecycleControl, 1, metrics.Fields{
		Outcome: outcomeClass,
		Reason:  string(operation),
	})
}

// sessionGateway is the injectable session gateway collaborator seam.
type sessionGateway interface {
	OpenFactorySession(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	OpenFactorySessionFromFolder(context.Context, string, *FactorySessionTargetRef, bool, bool) (*FactorySessionOpenResult, error)
	ListFactorySessions(context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(context.Context, string) (factoryapi.FactorySession, error)
	GetFactorySessionSyncPreflight(context.Context, string, *interfaces.FactoryEventReconnectCursor) (factoryapi.FactorySessionSyncPreflightResponse, error)
	GetFactorySessionResult(context.Context, string) (factoryapi.FactorySessionLiveResult, error)
	GetFactorySessionPartialResult(context.Context, string) (factoryapi.FactorySessionPartialResult, error)
	PauseLiveFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeLiveFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CloseFactorySession(context.Context, string) error
	PauseDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CancelDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	TerminateDurableFactorySession(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ApproveDurableFactorySession(context.Context, string, factoryapi.FactorySessionApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	RetryDurableFactorySessionDispatch(context.Context, string, factoryapi.FactorySessionRetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	InterruptDurableFactorySessionDispatch(context.Context, string, factoryapi.FactorySessionInterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	SubscribeSessionResponseStream(sessionID string, dispatchID string, afterSequence int64) (*factorysessions.SessionResponseStreamSubscription, error)
	SessionResponseStreamDispatchIDs(sessionID string) ([]string, error)
	CloseSessionResponseStreams(session *factorysessions.LiveSession)
	JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore
	InferenceProgressPublisherFactory(logger *zap.Logger) func(sessionID string) workerprovider.InferenceProgressPublisher
	DispatchCompletionObserverFactory() func(sessionID string) func(string)
}

var _ sessionGateway = (*factorysessionservice.Service)(nil)

type sessionGatewayHost struct {
	*FactoryService
}

var _ factorysessionservice.Host = sessionGatewayHost{}

func (h sessionGatewayHost) DiscoverTargets(folderPath string) ([]factorysessions.Target, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.discoverFactorySessionTargets(folderPath)
}

func (h sessionGatewayHost) InitializeFactoryScaffold(factoryDir string) error {
	if err := initcmd.Init(initcmd.InitConfig{
		Dir:         factoryDir,
		Diagnostics: io.Discard,
	}); err != nil {
		return factorysessions.NewValidationError(
			factorysessions.ValidationReasonUnreadable,
			"folderPath",
			fmt.Errorf("initialize factory scaffold: %w", err),
		)
	}
	return nil
}

func (h sessionGatewayHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if h.FactoryService == nil {
		return "", fmt.Errorf("factory service is required")
	}
	return h.openFactorySessionForTarget(ctx, target)
}

func (h sessionGatewayHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.requireSession(sessionID)
}

func (h sessionGatewayHost) ListLiveSessionIDs() []string {
	if h.FactoryService == nil || h.FactoryService.sessions == nil {
		return nil
	}
	return h.FactoryService.sessions.IDs()
}

func (h sessionGatewayHost) GetLiveSession(sessionID string) *factorysessions.LiveSession {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.sessionByID(sessionID)
}

func (h sessionGatewayHost) BuildSessionProjectionContext(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.FactoryService == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.buildSessionProjectionContext(ctx, session)
}

func (h sessionGatewayHost) ResolveSyncPreflightTarget(sessionID string) (controlplane.SyncPreflightTarget, error) {
	if h.FactoryService == nil {
		return controlplane.SyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	target, err := h.FactoryService.resolveSessionSyncPreflightTarget(sessionID, interfaces.FactorySessionSyncPreflightOptions{})
	return controlplane.SyncPreflightTarget{Session: target.session, Remapped: target.remapped}, err
}

func (h sessionGatewayHost) BackendScopeID() string {
	if h.FactoryService == nil {
		return ""
	}
	return factorySessionBackendScopeID(h.FactoryService, nil)
}

func (h sessionGatewayHost) StreamGenerationID(session *factorysessions.LiveSession) string {
	if h.FactoryService == nil {
		return ""
	}
	return factorySessionStreamGenerationID(h.FactoryService, session)
}

func (h sessionGatewayHost) LiveSessionEvents(session *factorysessions.LiveSession) []factoryapi.FactoryEvent {
	handle := liveSessionHandle(session)
	if handle == nil || handle.Bundle == nil || handle.Bundle.EventHistory == nil {
		return nil
	}
	return handle.Bundle.EventHistory.Events()
}

func (h sessionGatewayHost) SessionFactory(sessionID string) (factory.Factory, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.sessionFactory(sessionID)
}

func (h sessionGatewayHost) StopLiveSession(sessionID string) error {
	if h.FactoryService == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.FactoryService.stopFactorySession(sessionID)
}

func (h sessionGatewayHost) ObserveLiveLifecycleControl(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
	err error,
) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeLiveLifecycleControl(sessionID, operation, control, outcome, status, err)
}

func (h sessionGatewayHost) DurableExecution() factorysessionexecution.Service {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.durableExecutionService()
}

func (h sessionGatewayHost) ResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.sessionResponseStreams(session)
}

func (h sessionGatewayHost) NewResponseStream() *factorysessions.SessionResponseStream {
	if h.FactoryService == nil {
		return factorysessions.NewSessionResponseStream()
	}
	return h.FactoryService.newSessionResponseStreamInstance()
}

func (h sessionGatewayHost) CloseResponseStreams(session *factorysessions.LiveSession) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.closeSessionResponseStreamsDirect(session)
}

func (h sessionGatewayHost) CloseResponseStreamDispatch(session *factorysessions.LiveSession, dispatchID string) bool {
	if h.FactoryService == nil {
		return false
	}
	return h.FactoryService.closeSessionResponseStreamDispatchDirect(session, dispatchID)
}

func (h sessionGatewayHost) JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.javascriptCheckpointStoreDirect(session)
}

func (h sessionGatewayHost) ObserveResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeResponseStreamPublished(session, sessionID, event)
}

func (h sessionGatewayHost) ObserveResponseStreamCompaction(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	summary responsestream.CompactionSummary,
) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeResponseStreamCompaction(session, sessionID, dispatchID, summary)
}

func (h sessionGatewayHost) ObserveResponseStreamDegraded(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	reason string,
	fallbackLogger *zap.Logger,
	err error,
) {
	if h.FactoryService == nil {
		return
	}
	h.FactoryService.observeResponseStreamDegraded(session, sessionID, dispatchID, reason, fallbackLogger, err)
}

func newSessionGatewayService(fs *FactoryService) *factorysessionservice.Service {
	return factorysessionservice.New(sessionGatewayHost{fs})
}

func wireSessionGatewayCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) sessionGateway {
	if cfg != nil && cfg.SessionGateway != nil {
		return cfg.SessionGateway
	}
	return newSessionGatewayService(fs)
}

func (fs *FactoryService) requireSessionGateway() sessionGateway {
	if fs == nil {
		return newSessionGatewayService(nil)
	}
	if fs.sessionGateway == nil {
		fs.sessionGateway = newSessionGatewayService(fs)
	}
	return fs.sessionGateway
}

// ProvideSessionGatewayCollaborator constructs the session gateway for a built service shell.
func ProvideSessionGatewayCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) sessionGateway {
	return wireSessionGatewayCollaborator(shell.Service, cfg)
}

// AttachSessionGatewayCollaborator assigns the session gateway on the service shell.
func AttachSessionGatewayCollaborator(shell FactoryServiceShell, gateway sessionGateway) *FactoryService {
	if shell.Service != nil {
		shell.Service.sessionGateway = gateway
	}
	return shell.Service
}

func (fs *FactoryService) recordSessionPersistenceInvalidationFromPreflight(
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	if diagnostic, ok := sessionpersistence.InvalidationFromSyncPreflight(response); ok {
		fs.recordSessionPersistenceInvalidation(diagnostic)
	}
}

func (fs *FactoryService) recordSessionPersistenceInvalidation(
	diagnostic sessionpersistence.InvalidationDiagnostic,
) {
	if fs == nil {
		return
	}
	fs.sessionPersistenceObserver().Record(diagnostic)
}

func (fs *FactoryService) sessionPersistenceObserver() sessionpersistence.Observer {
	return sessionpersistence.Observer{
		Logger: sessionPersistenceZapLogger{logger: fs.logger},
	}
}

type sessionPersistenceZapLogger struct {
	logger *zap.Logger
}

func (l sessionPersistenceZapLogger) Info(msg string, fields map[string]string) {
	if l.logger == nil {
		return
	}
	zapFields := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		zapFields = append(zapFields, zap.String(key, value))
	}
	l.logger.Info(msg, zapFields...)
}
