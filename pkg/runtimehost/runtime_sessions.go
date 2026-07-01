// backendsizecheck:ignore-file consolidated session runtime reads remain with runtime_sessions until dedicated service read seams split.
// pkgmaintcheck:ignore-file-lines consolidated session runtime reads remain with runtime_sessions until dedicated service read seams split.
package runtimehost

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
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

const (
	DefaultFactorySessionID         = factorysessions.DefaultSessionID
	FactorySessionTargetKindDefault = factorysessions.TargetKindDefault
	FactorySessionTargetKindNamed   = factorysessions.TargetKindNamed
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
	GetFactorySessionSyncPreflight(context.Context, string, *interfaces.FactoryEventReconnectCursor) (factoryapi.FactorySessionSyncPreflightResponse, error)
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
	StartDefaultRuntime(context.Context, context.Context, bool) (*liveRuntimeHandle, error)
	StartBackgroundSessionWithMetadata(context.Context, string, *factoryRuntimeBundle, FactorySessionTarget) error
	StartLiveRuntimeSidecars(context.Context, *liveRuntimeHandle) error
	StopLiveRuntimeSidecars(*liveRuntimeHandle)
	StopLiveRuntime(*liveRuntimeHandle) error
	ShutdownOtherLiveSessions(*liveRuntimeHandle) error
	ReplaceSessionRuntime(context.Context, *factorysessions.LiveSession, string, *factoryRuntimeBundle) error
}

type runtimeCoordinator struct {
	host *Host
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

func (fs *Host) buildLiveSessionRegistration(
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

func newCoordinator(fs *Host) FactoryCoordinator {
	return &runtimeCoordinator{host: fs}
}

func (fs *Host) requireCoordinator() FactoryCoordinator {
	if fs == nil {
		return newCoordinator(nil)
	}
	if fs.coordinator == nil {
		fs.coordinator = newCoordinator(fs)
	}
	return fs.coordinator
}

func (fs *Host) registerLiveSession(
	sessionID string,
	handle *liveRuntimeHandle,
	target FactorySessionTarget,
	selectSession bool,
) {
	if fs == nil || fs.sessions == nil || sessionID == "" || handle == nil {
		return
	}
	registration := fs.buildLiveSessionRegistration(sessionID, handle, target)
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		registration.factoryDir,
		registration.folderPath,
		registration.executionBaseDir,
		registration.targetRef,
		&liveSessionState{bundle: handle.Bundle, handle: handle, spec: registration.preparedSpec},
		sessionID == DefaultFactorySessionID,
		registration.project,
	), selectSession)
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

func (fs *Host) unregisterLiveSession(sessionID string) {
	if fs == nil || fs.sessions == nil {
		return
	}
	fs.closeSessionResponseStreams(fs.sessionByID(sessionID))
	fs.sessions.Remove(sessionID)
}

func (fs *Host) currentSession() *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.Current()
}

func (fs *Host) defaultSession() *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.Get(DefaultFactorySessionID)
}

func (fs *Host) sessionByID(sessionID string) *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.Get(sessionID)
}

func (fs *Host) requireSession(sessionID string) (*factorysessions.LiveSession, error) {
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

func (fs *Host) sessionFactory(sessionID string) (factory.Factory, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).Bundle.Factory, nil
}

func (fs *Host) sessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).Bundle.RuntimeCfg, nil
}

func (fs *Host) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	return fs.requireCoordinator().SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (c *runtimeCoordinator) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	return factoryservice.SubmitWorkRequest(ctx, liveSessionHandle(session).Bundle, request)
}

func (fs *Host) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	return fs.requireCoordinator().MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (c *runtimeCoordinator) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return interfaces.OperatorMoveResult{}, err
	}
	return factoryservice.MoveWork(ctx, liveSessionHandle(session).Bundle, workID, stateName, interfaces.WorkStateChangeSourceAPI, requestID)
}

// MoveWork applies a synchronous operator relocation on the current service-owned runtime.
func (fs *Host) MoveWork(ctx context.Context, workID, stateName string, source interfaces.WorkStateChangeSource, requestID string) (interfaces.OperatorMoveResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	return factoryservice.MoveWork(ctx, fs.currentRuntimeBundle(), workID, stateName, source, requestID)
}

func (fs *Host) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	return fs.requireCoordinator().SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (c *runtimeCoordinator) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return factoryservice.SubscribeFactoryEventsForSession(ctx, liveSessionHandle(session).Bundle, sessionID, reconnect)
}

func (fs *Host) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return fs.requireCoordinator().GetEngineStateSnapshotForSession(ctx, sessionID)
}

func (c *runtimeCoordinator) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return factoryservice.GetEngineStateSnapshot(ctx, liveSessionHandle(session).Bundle)
}

func (fs *Host) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return fs.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (c *runtimeCoordinator) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return c.host.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (fs *Host) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	return fs.requireSessionGateway().OpenFactorySession(ctx, request)
}

func (c *runtimeCoordinator) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	if c.host == nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().OpenFactorySession(ctx, request)
}

func (fs *Host) CloseFactorySession(ctx context.Context, sessionID string) error {
	return fs.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

func (c *runtimeCoordinator) CloseFactorySession(ctx context.Context, sessionID string) error {
	if c.host == nil {
		return fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

func (fs *Host) openFactorySession(ctx context.Context, factoryDir string) (string, error) {
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

func (fs *Host) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	return fs.requireSessionGateway().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (c *runtimeCoordinator) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	if c.host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (fs *Host) openFactorySessionForTarget(ctx context.Context, target FactorySessionTarget) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	sessionID := factorysessions.NewSessionID()
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, target.FolderPath, target.FactoryDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := fs.StartBackgroundSessionWithMetadata(ctx, sessionID, replacement, target); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *Host) startBackgroundSession(ctx context.Context, sessionID string, runtimeBundle *factoryRuntimeBundle) error {
	return fs.StartBackgroundSessionWithMetadata(ctx, sessionID, runtimeBundle, FactorySessionTarget{
		Ref: FactorySessionTargetRef{
			Kind: FactorySessionTargetKindDefault,
		},
		FactoryDir: runtimeBundle.Dir,
		FolderPath: runtimeBundle.Dir,
		Project:    filepath.Base(runtimeBundle.Dir),
	})
}

//nolint:contextcheck // The request context bounds startup waiting, while the active service runtime context owns the long-lived session runtime and sidecars.
func (fs *Host) StartBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle *factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	return fs.requireCoordinator().StartBackgroundSessionWithMetadata(ctx, sessionID, runtimeBundle, target)
}

func (c *runtimeCoordinator) StartBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle *factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	fs := c.host
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
		_ = fs.StopLiveRuntime(handle)
		return fmt.Errorf("start runtime session: %w", err)
	}
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService {
		if err := fs.StartLiveRuntimeSidecars(serviceCtx, handle); err != nil {
			_ = fs.StopLiveRuntime(handle)
			return fmt.Errorf("start runtime session sidecars: %w", err)
		}
	}
	fs.registerLiveSession(sessionID, handle, target, false)
	return nil
}

func (fs *Host) stopFactorySession(sessionID string) error {
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
	if err := fs.StopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (fs *Host) runSessionID() string {
	if fs == nil {
		return DefaultFactorySessionID
	}
	if runState := fs.currentRunState(); runState != nil && strings.TrimSpace(runState.sessionID) != "" {
		return runState.sessionID
	}
	return DefaultFactorySessionID
}

func (fs *Host) requireIdleRuntimeForSession(
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

func sessionServiceContext(ctx context.Context, runState *hostRunState) context.Context {
	if runState != nil && runState.ctx != nil {
		return runState.ctx
	}
	return ctx
}

func (fs *Host) startReplacementSessionRuntime(
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
		AttachSidecars:              fs.StartLiveRuntimeSidecars,
		AttachSidecarsInServiceMode: serviceMode,
	})
}

//nolint:contextcheck // The request context bounds the save/startup wait, while the long-lived service runtime context owns the replacement session runtime and sidecars after the request returns.
func (fs *Host) ReplaceSessionRuntime(
	ctx context.Context,
	session *factorysessions.LiveSession,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	return fs.requireCoordinator().ReplaceSessionRuntime(ctx, session, name, replacement)
}

func (c *runtimeCoordinator) ReplaceSessionRuntime(
	ctx context.Context,
	session *factorysessions.LiveSession,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	fs := c.host
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
		RestoreSidecars: fs.StartLiveRuntimeSidecars,
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
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		session.ID,
		replacement.Dir,
		session.FolderPath,
		executionBaseDir,
		session.Target,
		&liveSessionState{handle: replacementHandle, spec: liveSessionBuildSpec(session)},
		session.IsDefault,
		session.Project,
	), isActiveSession)
	if isActiveSession {
		fs.setRunState(serviceCtx, session.ID, replacementHandle)
	}
	if err := fs.StopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		fs.logger.Warn("prior session runtime shutdown failed", zap.Error(err), zap.String("session_id", session.ID))
	}
	return nil
}

func (fs *Host) nextLiveSessionAfterStop(sessionID string) *factorysessions.LiveSession {
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

func (fs *Host) discoverFactorySessionTargets(folderPath string) ([]FactorySessionTarget, error) {
	return factorysessions.DiscoverTargets(folderPath, fs.probeFactorySessionTarget)
}

func (fs *Host) probeFactorySessionTarget(
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

func (fs *Host) logFactorySessionTargetProbeFailure(
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

func (fs *Host) waitForServiceModeStartupWorkReadability(ctx context.Context, serviceMode bool) error {
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

	timer := time.NewTimer(hostModeStartupWorkReadabilityDelay)
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

func (fs *Host) failServiceModeStartup(currentRuntime *liveRuntimeHandle, startupErr error) error {
	fs.clearRunState()
	fs.unregisterLiveSession(DefaultFactorySessionID)
	if currentRuntime == nil {
		return startupErr
	}
	if stopErr := fs.StopLiveRuntime(currentRuntime); stopErr != nil && !errors.Is(stopErr, context.Canceled) {
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

func (fs *Host) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	return fs.requireSessionGateway().ListFactorySessions(ctx)
}

func (c *runtimeCoordinator) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if c.host == nil {
		return factoryapi.ListFactorySessionsResponse{}, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().ListFactorySessions(ctx)
}

func (fs *Host) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	return fs.requireSessionGateway().GetFactorySession(ctx, sessionID)
}

func (c *runtimeCoordinator) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	if c.host == nil {
		return factoryapi.FactorySession{}, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().GetFactorySession(ctx, sessionID)
}

func (fs *Host) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	return fs.requireSessionGateway().GetFactorySessionSyncPreflight(ctx, sessionID, reconnect)
}

func (c *runtimeCoordinator) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	if c.host == nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().GetFactorySessionSyncPreflight(ctx, sessionID, reconnect)
}

type sessionSyncPreflightTarget struct {
	session  *factorysessions.LiveSession
	remapped bool
}

func (fs *Host) resolveSessionSyncPreflightTarget(sessionID string) (sessionSyncPreflightTarget, error) {
	if fs == nil {
		return sessionSyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	if session, err := fs.requireSession(sessionID); err == nil {
		return sessionSyncPreflightTarget{session: session}, nil
	} else if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		return sessionSyncPreflightTarget{}, err
	}

	if strings.TrimSpace(sessionID) != DefaultFactorySessionID {
		return sessionSyncPreflightTarget{}, nil
	}

	if session := fs.preflightDefaultSessionSuccessor(); session != nil {
		return sessionSyncPreflightTarget{session: session, remapped: true}, nil
	}
	return sessionSyncPreflightTarget{}, nil
}

func (fs *Host) preflightDefaultSessionSuccessor() *factorysessions.LiveSession {
	if fs == nil {
		return nil
	}
	if runState := fs.currentRunState(); runState != nil {
		successorID := strings.TrimSpace(runState.sessionID)
		if successorID != "" && successorID != DefaultFactorySessionID {
			if session, err := fs.requireSession(successorID); err == nil {
				return session
			}
		}
	}
	current := fs.currentSession()
	if current == nil || current.ID == DefaultFactorySessionID {
		return nil
	}
	if session, err := fs.requireSession(current.ID); err == nil {
		return session
	}
	return nil
}

func (fs *Host) buildSessionProjectionContext(
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
	return projectionCtx, nil
}

func (fs *Host) projectJavaScriptRuntimeState(
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

func (fs *Host) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	return fs.requireSessionGateway().GetFactorySessionResult(ctx, sessionID)
}

func (c *runtimeCoordinator) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	if c.host == nil {
		return factoryapi.FactorySessionLiveResult{}, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().GetFactorySessionResult(ctx, sessionID)
}

func (fs *Host) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	return fs.requireSessionGateway().GetFactorySessionPartialResult(ctx, sessionID)
}

func (c *runtimeCoordinator) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	if c.host == nil {
		return factoryapi.FactorySessionPartialResult{}, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().GetFactorySessionPartialResult(ctx, sessionID)
}

func (fs *Host) javascriptCheckpointStoreDirect(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	if state.javascriptCheckpoints == nil {
		state.javascriptCheckpoints = factorysessions.NewJavaScriptCheckpointStore()
	}
	return state.javascriptCheckpoints
}

func (fs *Host) sessionResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	state.responseStreamsOnce.Do(func() {
		state.responseStreams = fs.newSessionResponseStreamSetInstance()
	})
	return state.responseStreams
}

func (fs *Host) sessionResponseStream(
	session *factorysessions.LiveSession,
	dispatchID string,
) *factorysessions.SessionResponseStream {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return nil
	}
	return streams.Stream(dispatchID)
}

func (fs *Host) closeSessionResponseStreams(session *factorysessions.LiveSession) {
	fs.requireSessionGateway().CloseSessionResponseStreams(session)
}

func (fs *Host) closeSessionResponseStreamsDirect(session *factorysessions.LiveSession) {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return
	}
	streams.Close()
}

func (fs *Host) closeSessionResponseStreamDispatchDirect(
	session *factorysessions.LiveSession,
	dispatchID string,
) bool {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return false
	}
	return streams.CloseDispatch(dispatchID)
}

func (fs *Host) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	return fs.requireSessionGateway().SubscribeSessionResponseStream(sessionID, dispatchID, afterSequence)
}

func (fs *Host) SessionResponseStreamDispatchIDs(sessionID string) ([]string, error) {
	return fs.requireSessionGateway().SessionResponseStreamDispatchIDs(sessionID)
}

func (fs *Host) newSessionResponseStreamInstance() *factorysessions.SessionResponseStream {
	if fs != nil && fs.newSessionResponseStream != nil {
		return fs.newSessionResponseStream()
	}
	return factorysessions.NewSessionResponseStream()
}

func (fs *Host) newSessionResponseStreamSetInstance() *factorysessions.SessionResponseStreamSet {
	return factorysessions.NewSessionResponseStreamSetWithFactory(func() *factorysessions.SessionResponseStream {
		return fs.newSessionResponseStreamInstance()
	})
}

func factorySessionBackendScopeID(fs *Host, session *factorysessions.LiveSession) string {
	_ = session
	if fs != nil && fs.cfg != nil {
		if backendScopeID := strings.TrimSpace(fs.cfg.BackendScopeID); backendScopeID != "" {
			return backendScopeID
		}
	}
	if bundle := liveSessionBundle(session); bundle != nil {
		return strings.TrimSpace(bundle.BackendScopeID)
	}
	return ""
}

func factorySessionStreamGenerationID(fs *Host, session *factorysessions.LiveSession) string {
	if fs != nil && session != nil {
		if snapshot, err := fs.GetEngineStateSnapshotForSession(context.Background(), session.ID); err == nil {
			if streamGenerationID := strings.TrimSpace(snapshot.StreamGenerationID); streamGenerationID != "" {
				return streamGenerationID
			}
		}
	}
	factorySessionID := ""
	if session != nil {
		factorySessionID = strings.TrimSpace(session.ID)
	}
	_ = factorySessionID
	if bundle := liveSessionBundle(session); bundle != nil && !bundle.StartedAtUTC.IsZero() {
		return bundle.StartedAtUTC.UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func NewInferenceProgressPublisherFactory(
	sessions *factorysessions.Registry,
	logger *zap.Logger,
) inferenceProgressPublisherFactory {
	if sessions == nil {
		return nil
	}
	gateway := newSessionGatewayService(&Host{sessions: sessions})
	return gateway.InferenceProgressPublisherFactory(logger)
}

func NewSessionDispatchCompletionObserverFactory(
	sessions *factorysessions.Registry,
) dispatchCompletionObserverFactory {
	if sessions == nil {
		return nil
	}
	gateway := newSessionGatewayService(&Host{sessions: sessions})
	return gateway.DispatchCompletionObserverFactory()
}

func (fs *Host) inferenceProgressPublisher(
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

func (fs *Host) observeResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(event.DispatchID),
		Reason:     string(event.Kind),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamPublished, fields)
}

func (fs *Host) observeResponseStreamCompaction(
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

func (fs *Host) observeResponseStreamDegraded(
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

var _ apisurface.DurableSessionExecutionAPI = (*Host)(nil)
var _ apisurface.DurableSessionListingAPI = (*Host)(nil)
var _ apisurface.DurableSessionLifecycleAPI = (*Host)(nil)

func (fs *Host) StartDurableFactorySessionAsync(
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

func (fs *Host) StartDurableFactorySessionSync(
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

func (fs *Host) durableExecutionService() factorysessionexecution.Service {
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

func (fs *Host) durableProjectRoot() string {
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

func (fs *Host) ListDurableFactorySessions(
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
func (fs *Host) ListDurableExecutionSessions(
	ctx context.Context,
	req factorysessionexecution.ListSessionsRequest,
) (factorysessionexecution.ListSessionsResult, error) {
	return fs.durableExecutionService().ListSessions(ctx, req)
}

func (fs *Host) GetDurableFactorySession(
	ctx context.Context,
	sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	result, err := fs.durableExecutionService().GetSession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	return factorysession.SessionReadResponseToAPI(result), nil
}

func (fs *Host) GetDurableFactorySessionResult(
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

func (fs *Host) ReadDurableFactorySessionEvents(
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

func (fs *Host) ListDurableFactorySessionDispatches(
	ctx context.Context,
	sessionID string,
) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	result, err := fs.durableExecutionService().ListDispatches(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	return factorysession.ListDispatchesResponseToAPI(result), nil
}

func (fs *Host) GetDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID, dispatchID string,
) (factoryapi.FactoryDispatch, error) {
	result, err := fs.durableExecutionService().GetDispatch(ctx, sessionID, dispatchID)
	if err != nil {
		return factoryapi.FactoryDispatch{}, err
	}
	return factorysession.DispatchDetailResponseToAPI(result), nil
}

func (fs *Host) ListDurableFactorySessionArtifacts(
	ctx context.Context,
	sessionID string,
) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	result, err := fs.durableExecutionService().ListArtifacts(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	return factorysession.ListArtifactsResponseToAPI(result), nil
}

func (fs *Host) GetDurableFactorySessionArtifact(
	ctx context.Context,
	sessionID, artifactID string,
) (factoryapi.FactorySessionArtifactDetail, error) {
	result, err := fs.durableExecutionService().GetArtifact(ctx, sessionID, artifactID)
	if err != nil {
		return factoryapi.FactorySessionArtifactDetail{}, err
	}
	return factorysession.ArtifactDetailResponseToAPI(result), nil
}

func (fs *Host) PauseDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().PauseDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) ResumeDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().ResumeDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) CancelDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().CancelDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) TerminateDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().TerminateDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) ApproveDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().ApproveDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().RetryDurableFactorySessionDispatch(ctx, sessionID, request)
}

func (fs *Host) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().InterruptDurableFactorySessionDispatch(ctx, sessionID, request)
}
func (fs *Host) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().PauseLiveFactorySession(ctx, sessionID, request)
}

func (fs *Host) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireSessionGateway().ResumeLiveFactorySession(ctx, sessionID, request)
}

func (fs *Host) observeLiveLifecycleControl(
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

func (fs *Host) emitLiveLifecycleControlMetric(
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

// SessionGateway is the injectable session gateway collaborator seam.
type SessionGateway interface {
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

var _ SessionGateway = (*factorysessionservice.Service)(nil)

type sessionGatewayHost struct {
	*Host
}

var _ factorysessionservice.Host = sessionGatewayHost{}

func (h sessionGatewayHost) DiscoverTargets(folderPath string) ([]factorysessions.Target, error) {
	if h.Host == nil {
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
	if h.Host == nil {
		return "", fmt.Errorf("factory service is required")
	}
	return h.openFactorySessionForTarget(ctx, target)
}

func (h sessionGatewayHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if h.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.requireSession(sessionID)
}

func (h sessionGatewayHost) ListLiveSessionIDs() []string {
	if h.Host == nil || h.Host.sessions == nil {
		return nil
	}
	return h.Host.sessions.IDs()
}

func (h sessionGatewayHost) GetLiveSession(sessionID string) *factorysessions.LiveSession {
	if h.Host == nil {
		return nil
	}
	return h.Host.sessionByID(sessionID)
}

func (h sessionGatewayHost) BuildSessionProjectionContext(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.Host == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory service is required")
	}
	return h.Host.buildSessionProjectionContext(ctx, session)
}

func (h sessionGatewayHost) ResolveSyncPreflightTarget(sessionID string) (controlplane.SyncPreflightTarget, error) {
	if h.Host == nil {
		return controlplane.SyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	target, err := h.Host.resolveSessionSyncPreflightTarget(sessionID)
	return controlplane.SyncPreflightTarget{Session: target.session, Remapped: target.remapped}, err
}

func (h sessionGatewayHost) BackendScopeID() string {
	if h.Host == nil {
		return ""
	}
	return factorySessionBackendScopeID(h.Host, nil)
}

func (h sessionGatewayHost) StreamGenerationID(session *factorysessions.LiveSession) string {
	if h.Host == nil {
		return ""
	}
	return factorySessionStreamGenerationID(h.Host, session)
}

func (h sessionGatewayHost) LiveSessionEvents(session *factorysessions.LiveSession) []factoryapi.FactoryEvent {
	handle := liveSessionHandle(session)
	if handle == nil || handle.Bundle == nil || handle.Bundle.EventHistory == nil {
		return nil
	}
	return handle.Bundle.EventHistory.Events()
}

func (h sessionGatewayHost) SessionFactory(sessionID string) (factory.Factory, error) {
	if h.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.Host.sessionFactory(sessionID)
}

func (h sessionGatewayHost) StopLiveSession(sessionID string) error {
	if h.Host == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.Host.stopFactorySession(sessionID)
}

func (h sessionGatewayHost) ObserveLiveLifecycleControl(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
	err error,
) {
	if h.Host == nil {
		return
	}
	h.Host.observeLiveLifecycleControl(sessionID, operation, control, outcome, status, err)
}

func (h sessionGatewayHost) DurableExecution() factorysessionexecution.Service {
	if h.Host == nil {
		return nil
	}
	return h.Host.durableExecutionService()
}

func (h sessionGatewayHost) ResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.Host == nil {
		return nil
	}
	return h.Host.sessionResponseStreams(session)
}

func (h sessionGatewayHost) NewResponseStream() *factorysessions.SessionResponseStream {
	if h.Host == nil {
		return factorysessions.NewSessionResponseStream()
	}
	return h.Host.newSessionResponseStreamInstance()
}

func (h sessionGatewayHost) CloseResponseStreams(session *factorysessions.LiveSession) {
	if h.Host == nil {
		return
	}
	h.Host.closeSessionResponseStreamsDirect(session)
}

func (h sessionGatewayHost) CloseResponseStreamDispatch(session *factorysessions.LiveSession, dispatchID string) bool {
	if h.Host == nil {
		return false
	}
	return h.Host.closeSessionResponseStreamDispatchDirect(session, dispatchID)
}

func (h sessionGatewayHost) JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if h.Host == nil {
		return nil
	}
	return h.Host.javascriptCheckpointStoreDirect(session)
}

func (h sessionGatewayHost) ObserveResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	if h.Host == nil {
		return
	}
	h.Host.observeResponseStreamPublished(session, sessionID, event)
}

func (h sessionGatewayHost) ObserveResponseStreamCompaction(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	summary responsestream.CompactionSummary,
) {
	if h.Host == nil {
		return
	}
	h.Host.observeResponseStreamCompaction(session, sessionID, dispatchID, summary)
}

func (h sessionGatewayHost) ObserveResponseStreamDegraded(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	reason string,
	fallbackLogger *zap.Logger,
	err error,
) {
	if h.Host == nil {
		return
	}
	h.Host.observeResponseStreamDegraded(session, sessionID, dispatchID, reason, fallbackLogger, err)
}

func newSessionGatewayService(fs *Host) *factorysessionservice.Service {
	return factorysessionservice.New(sessionGatewayHost{fs})
}

func wireSessionGatewayCollaborator(fs *Host, cfg *Config) SessionGateway {
	if cfg != nil && cfg.SessionGateway != nil {
		return cfg.SessionGateway
	}
	return newSessionGatewayService(fs)
}

func (fs *Host) requireSessionGateway() SessionGateway {
	if fs == nil {
		return newSessionGatewayService(nil)
	}
	if fs.sessionGateway == nil {
		fs.sessionGateway = newSessionGatewayService(fs)
	}
	return fs.sessionGateway
}

// ProvideSessionGatewayCollaborator constructs the session gateway for a built service shell.
func ProvideSessionGatewayCollaborator(shell HostShell, cfg *Config) SessionGateway {
	return wireSessionGatewayCollaborator(shell.Host, cfg)
}

// AttachSessionGatewayCollaborator assigns the session gateway on the service shell.
func AttachSessionGatewayCollaborator(shell HostShell, gateway SessionGateway) *Host {
	if shell.Host != nil {
		shell.Host.sessionGateway = gateway
	}
	return shell.Host
}
