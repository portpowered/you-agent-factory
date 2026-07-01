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
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
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
	startDefaultRuntime(context.Context, context.Context, bool) (*liveRuntimeHandle, error)
	startBackgroundSessionWithMetadata(context.Context, string, *factoryRuntimeBundle, FactorySessionTarget) error
	startLiveRuntimeSidecars(context.Context, *liveRuntimeHandle) error
	stopLiveRuntimeSidecars(*liveRuntimeHandle)
	restoreLiveRuntimeSidecars(*serviceRunState)
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
		return state.handle.runtime
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
	if registration.factoryDir == "" && handle.runtime != nil {
		registration.factoryDir = handle.runtime.dir
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
	if handle != nil && handle.runtime != nil && handle.runtime.runtimeCfg != nil {
		executionBaseDir = strings.TrimSpace(handle.runtime.runtimeCfg.RuntimeBaseDir())
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
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		registration.factoryDir,
		registration.folderPath,
		registration.executionBaseDir,
		registration.targetRef,
		&liveSessionState{bundle: handle.runtime, handle: handle, spec: registration.preparedSpec},
		sessionID == defaultFactorySessionID,
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
		target.FactoryDir = runtimeBundle.dir
		target.FolderPath = runtimeBundle.folderPath
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
	return fs.sessions.Get(sessionID)
}

func (fs *FactoryService) requireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	session := fs.sessionByID(sessionID)
	handle := liveSessionHandle(session)
	if session == nil || handle == nil || handle.runtime == nil {
		return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	return session, nil
}

func (fs *FactoryService) sessionFactory(sessionID string) (factory.Factory, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).runtime.factory, nil
}

func (fs *FactoryService) sessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).runtime.runtimeCfg, nil
}

func (fs *FactoryService) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	return fs.requireCoordinator().SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (c *runtimeFactoryCoordinator) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	fs := c.service
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	return activeFactory.SubmitWorkRequest(ctx, request)
}

func (fs *FactoryService) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	return fs.requireCoordinator().MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (c *runtimeFactoryCoordinator) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	fs := c.service
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return interfaces.OperatorMoveResult{}, err
	}
	return activeFactory.MoveWork(ctx, workID, stateName, interfaces.WorkStateChangeSourceAPI, requestID)
}

// MoveWork applies a synchronous operator relocation on the current service-owned runtime.
func (fs *FactoryService) MoveWork(ctx context.Context, workID, stateName string, source interfaces.WorkStateChangeSource, requestID string) (interfaces.OperatorMoveResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return interfaces.OperatorMoveResult{}, fmt.Errorf("factory service runtime is not available")
	}
	return activeFactory.MoveWork(ctx, workID, stateName, source, requestID)
}

func (fs *FactoryService) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	return fs.requireCoordinator().SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (c *runtimeFactoryCoordinator) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	fs := c.service
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	stream, err := activeFactory.SubscribeFactoryEvents(ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID})
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

func (fs *FactoryService) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return fs.requireCoordinator().GetEngineStateSnapshotForSession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	fs := c.service
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snapshot, nil
}

func (fs *FactoryService) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return fs.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return c.service.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (s *runtimeFactoryDefinitionService) GetCurrentFactoryForSession(_ context.Context, sessionID string) (factoryapi.Factory, error) {
	fs := s.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	rootDir := factorysessions.SessionFactoryRootDir(fs.factoryRootDir, session)
	factoryName := factorysessions.FactoryName(rootDir, runtimeCfg)
	versionRootDir := rootDir
	if persistRoot := sessionFactoryPersistRoot(fs.factoryRootDir, session); persistRoot != "" {
		if pointerName, err := configpersist.ReadCurrentFactoryPointer(persistRoot); err == nil {
			pointerFactoryName := factoryapi.FactoryName(pointerName)
			if session.IsDefault || pointerFactoryName == factoryName {
				factoryName = pointerFactoryName
			}
		}
		if sameFactoryDir(persistRoot, rootDir) {
			versionRootDir = persistRoot
		}
	}
	serialized, err := fs.serializeNamedFactory(factoryName, runtimeCfg, true)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return fs.withCurrentFactoryVersion(versionRootDir, serialized.Name, serialized)
}

func (fs *FactoryService) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	return fs.requireCoordinator().OpenFactorySession(ctx, request)
}

func (c *runtimeFactoryCoordinator) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	fs := c.service
	var target *FactorySessionTargetRef
	if request.Target != nil {
		targetName := ""
		if request.Target.Name != nil {
			targetName = strings.TrimSpace(*request.Target.Name)
		}
		target = &FactorySessionTargetRef{
			Kind: FactorySessionTargetKind(request.Target.Kind),
			Name: targetName,
		}
	}
	validateOnly := request.ValidateOnly != nil && *request.ValidateOnly
	initNewFactory := request.InitNewFactory != nil && *request.InitNewFactory
	if validateOnly && initNewFactory {
		return factoryapi.OpenFactorySessionResponse{}, factorysessions.NewValidationError(
			factorysessions.ValidationReasonRequired,
			"initNewFactory",
			fmt.Errorf("initNewFactory cannot be combined with validateOnly"),
		)
	}
	result, err := fs.OpenFactorySessionFromFolder(ctx, request.FolderPath, target, validateOnly, initNewFactory)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	response := factoryapi.OpenFactorySessionResponse{}
	if result.InitsNewFactory {
		initsNewFactory := true
		response.InitsNewFactory = &initsNewFactory
		if folderPath := strings.TrimSpace(result.FolderPath); folderPath != "" {
			response.FolderPath = &folderPath
		}
	}
	if len(result.Targets) > 0 {
		targets := factorysessions.TargetsResponse(result.Targets)
		response.Targets = &targets
	}
	if result.SessionID != "" {
		session, err := fs.requireSession(result.SessionID)
		if err != nil {
			return factoryapi.OpenFactorySessionResponse{}, err
		}
		summary := factorysessions.SummaryResponse(session)
		response.Session = &summary
	}
	return response, nil
}

func (fs *FactoryService) CloseFactorySession(ctx context.Context, sessionID string) error {
	return fs.requireCoordinator().CloseFactorySession(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) CloseFactorySession(_ context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("factory session id is required")
	}
	fs := c.service
	return fs.stopFactorySession(sessionID)
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
	return fs.requireCoordinator().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (c *runtimeFactoryCoordinator) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	fs := c.service
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	if initNewFactory {
		return fs.initNewFactoryAndOpenSession(ctx, folderPath)
	}

	targets, err := fs.discoverFactorySessionTargets(folderPath)
	if err != nil {
		if validateOnly {
			if reason, _, ok := factorysessions.ValidationReasonFromError(err); ok && reason == factorysessions.ValidationReasonNotRunnable {
				resolved, resolveErr := factorysessions.ResolveSessionFolder(folderPath)
				if resolveErr != nil {
					return nil, resolveErr
				}
				return &FactorySessionOpenResult{
					InitsNewFactory: true,
					FolderPath:      resolved,
				}, nil
			}
		}
		return nil, err
	}

	selectedTarget, err := factorysessions.SelectTarget(targets, target)
	if err != nil {
		return nil, err
	}
	if selectedTarget == nil {
		return &FactorySessionOpenResult{Targets: factorysessions.CloneTargets(targets)}, nil
	}
	if validateOnly {
		return &FactorySessionOpenResult{Targets: factorysessions.CloneTargets(targets)}, nil
	}

	sessionID, err := fs.openFactorySessionForTarget(ctx, *selectedTarget)
	if err != nil {
		return nil, err
	}
	return &FactorySessionOpenResult{SessionID: sessionID}, nil
}

func (fs *FactoryService) initNewFactoryAndOpenSession(
	ctx context.Context,
	folderPath string,
) (*FactorySessionOpenResult, error) {
	resolvedFolder, err := factorysessions.ResolveSessionFolder(folderPath)
	if err != nil {
		return nil, err
	}

	targets, discoverErr := fs.discoverFactorySessionTargets(folderPath)
	if discoverErr == nil {
		return nil, factorysessions.NewValidationError(
			factorysessions.ValidationReasonNotRunnable,
			"folderPath",
			fmt.Errorf("folder %q already exposes runnable factory targets", resolvedFolder),
		)
	}
	reason, _, ok := factorysessions.ValidationReasonFromError(discoverErr)
	if !ok || reason != factorysessions.ValidationReasonNotRunnable {
		return nil, discoverErr
	}

	if err := factorysessions.ValidateInitNewFactoryNestedDir(resolvedFolder); err != nil {
		return nil, err
	}

	nestedFactoryDir := filepath.Join(resolvedFolder, interfaces.FactoryDir)
	if err := initcmd.Init(initcmd.InitConfig{
		Dir:         nestedFactoryDir,
		Diagnostics: io.Discard,
	}); err != nil {
		return nil, factorysessions.NewValidationError(
			factorysessions.ValidationReasonUnreadable,
			"folderPath",
			fmt.Errorf("initialize factory scaffold: %w", err),
		)
	}

	targets, err = fs.discoverFactorySessionTargets(resolvedFolder)
	if err != nil {
		return nil, fmt.Errorf("discover initialized factory targets: %w", err)
	}
	selectedTarget, err := factorysessions.SelectTarget(targets, nil)
	if err != nil {
		return nil, err
	}
	if selectedTarget == nil {
		return nil, fmt.Errorf("initialized factory folder %q did not resolve to a runnable target", resolvedFolder)
	}

	sessionID, err := fs.openFactorySessionForTarget(ctx, *selectedTarget)
	if err != nil {
		return nil, err
	}
	return &FactorySessionOpenResult{SessionID: sessionID}, nil
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
		FactoryDir: runtimeBundle.dir,
		FolderPath: runtimeBundle.dir,
		Project:    filepath.Base(runtimeBundle.dir),
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
	replacementHandle := fs.startLiveRuntime(serviceCtx, replacement)
	if err := fs.waitForLiveRuntimeStart(ctx, replacementHandle); err != nil {
		_ = fs.stopLiveRuntime(replacementHandle)
		return nil, fmt.Errorf("start replacement runtime: %w", err)
	}
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) != interfaces.RuntimeModeService {
		return replacementHandle, nil
	}
	if err := fs.startLiveRuntimeSidecars(serviceCtx, replacementHandle); err != nil {
		_ = fs.stopLiveRuntime(replacementHandle)
		return nil, fmt.Errorf("start replacement runtime sidecars: %w", err)
	}
	return replacementHandle, nil
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

	restoreCurrentSidecars := false
	serviceMode := runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService
	if serviceMode {
		fs.stopLiveRuntimeSidecars(handle)
		restoreCurrentSidecars = true
		defer func() {
			if restoreCurrentSidecars {
				fs.restoreLiveRuntimeSidecars(runState)
			}
		}()
	}

	replacementHandle, err := fs.startReplacementSessionRuntime(ctx, serviceCtx, replacement)
	if err != nil {
		return err
	}

	fs.publishFactoryChangeEvent(ctx, handle, replacement)
	restoreCurrentSidecars = false
	executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir)
	if replacement.runtimeCfg != nil {
		if runtimeBaseDir := strings.TrimSpace(replacement.runtimeCfg.RuntimeBaseDir()); runtimeBaseDir != "" {
			executionBaseDir = runtimeBaseDir
		}
	}
	fs.closeSessionResponseStreams(session)
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		session.ID,
		replacement.dir,
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

func (fs *FactoryService) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	return fs.requireCoordinator().GetFactorySessionSyncPreflight(ctx, sessionID, reconnect)
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

func (c *runtimeFactoryCoordinator) GetFactorySessionSyncPreflight(
	_ context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	fs := c.service
	response := newFactorySessionSyncPreflightResponse(sessionID, reconnect)
	if strings.HasPrefix(sessionID, "dur-sess-") {
		response.ReasonCode = factoryapi.SessionNotFound
		return response, nil
	}

	resolved, err := fs.resolveSessionSyncPreflightTarget(sessionID)
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	if resolved.session == nil {
		response.ReasonCode = factoryapi.SessionNotFound
		return response, nil
	}
	session := resolved.session

	response.BackendScopeId = stringPointer(factorySessionBackendScopeID(fs, session))
	response.LogicalSessionKeyId = stringPointer(factorySessionLogicalSessionKeyID(session))
	response.FactorySessionId = stringPointer(session.ID)
	response.StreamGenerationId = stringPointer(factorySessionStreamGenerationID(fs, session))
	if resolved.remapped {
		response.ReasonCode = factoryapi.LogicalSessionRemap
		return response, nil
	}

	if !response.ReconnectCursor.Provided {
		response.ReasonCode = factoryapi.Ok
		response.CheckpointReusable = true
		return response, nil
	}

	eventHistory := liveSessionHandle(session).runtime.eventHistory
	eventsSnapshot := []factoryapi.FactoryEvent(nil)
	if eventHistory != nil {
		eventsSnapshot = eventHistory.Events()
	}
	_, err = events.BuildReconnectReplay(
		eventsSnapshot,
		*reconnect,
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
	session  *factorysessions.LiveSession
	remapped bool
}

func (fs *FactoryService) resolveSessionSyncPreflightTarget(sessionID string) (sessionSyncPreflightTarget, error) {
	if fs == nil {
		return sessionSyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	if session, err := fs.requireSession(sessionID); err == nil {
		return sessionSyncPreflightTarget{session: session}, nil
	} else if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		return sessionSyncPreflightTarget{}, err
	}

	if strings.TrimSpace(sessionID) != defaultFactorySessionID {
		return sessionSyncPreflightTarget{}, nil
	}

	if session := fs.preflightDefaultSessionSuccessor(); session != nil {
		return sessionSyncPreflightTarget{session: session, remapped: true}, nil
	}
	return sessionSyncPreflightTarget{}, nil
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
		BackendScopeID:   strings.TrimSpace(liveSessionBundle(session).backendScopeID),
		RuntimeStartedAt: liveSessionBundle(session).startedAtUTC,
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
		checkpointStore = fs.javascriptCheckpointStore(session)
		projectionCtx.JavaScriptCheckpoints = checkpointStore.List()
	}
	projectionCtx.JavaScript, err = fs.projectJavaScriptRuntimeState(session, checkpointStore, snapshot.TickCount)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	projectionCtx.Enabled = factorysessions.EnabledTransitionsForSnapshot(ctx, snapshot, runtimeCfg)
	return projectionCtx, nil
}

func (fs *FactoryService) projectJavaScriptRuntimeState(
	session *factorysessions.LiveSession,
	checkpointStore *factorysessions.JavaScriptCheckpointStore,
	selectedTick int,
) (*interfaces.FactorySessionJavaScriptRuntimeState, error) {
	state := (*interfaces.FactorySessionJavaScriptRuntimeState)(nil)
	handle := liveSessionHandle(session)
	if handle != nil && handle.runtime != nil && handle.runtime.eventHistory != nil {
		worldState, err := projections.ReconstructFactoryWorldState(handle.runtime.eventHistory.Events(), selectedTick)
		if err != nil {
			return nil, err
		}
		state = worldState.JavaScriptRuntime
	}
	return factorysessions.JavaScriptRuntimeStateFromCheckpoints(checkpointStore, state), nil
}

func (fs *FactoryService) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	return fs.requireCoordinator().GetFactorySessionResult(ctx, sessionID)
}

func (c *runtimeFactoryCoordinator) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	fs := c.service
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.FactorySessionLiveResult{}, err
	}
	projectionCtx, err := fs.buildSessionProjectionContext(ctx, session)
	if err != nil {
		return factoryapi.FactorySessionLiveResult{}, err
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(projectionCtx.FactoryCfg) {
		return factoryapi.FactorySessionLiveResult{}, fmt.Errorf("%w", apisurface.ErrFactorySessionResultUnavailable)
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

func (fs *FactoryService) closeSessionResponseStreams(session *factorysessions.LiveSession) {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return
	}
	streams.Close()
}

func (fs *FactoryService) closeSessionResponseStreamDispatch(
	session *factorysessions.LiveSession,
	dispatchID string,
) bool {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return false
	}
	return streams.CloseDispatch(dispatchID)
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

func (fs *FactoryService) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return nil, responsestream.ErrSubscriptionClosed
	}
	return streams.Subscribe(dispatchID, afterSequence)
}

func (fs *FactoryService) SessionResponseStreamDispatchIDs(sessionID string) ([]string, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return nil, nil
	}
	return streams.DispatchIDs(), nil
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
		return strings.TrimSpace(bundle.backendScopeID)
	}
	return ""
}

func factorySessionLogicalSessionKeyID(session *factorysessions.LiveSession) string {
	if session == nil {
		return ""
	}
	folderPath := filepath.Clean(strings.TrimSpace(session.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	targetKind := strings.TrimSpace(string(session.Target.Kind))
	targetName := strings.TrimSpace(session.Target.Name)
	if targetKind == "" {
		targetKind = string(factorysessions.TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
}

func factorySessionStreamGenerationID(fs *FactoryService, session *factorysessions.LiveSession) string {
	factorySessionID := ""
	if session != nil {
		factorySessionID = strings.TrimSpace(session.ID)
	}
	backendScopeID := factorySessionBackendScopeID(fs, session)
	switch {
	case backendScopeID != "" && factorySessionID != "":
		return backendScopeID + "::" + factorySessionID
	case factorySessionID != "":
		return factorySessionID
	default:
		return backendScopeID
	}
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
	svc := &FactoryService{sessions: sessions}
	return func(sessionID string) workerprovider.InferenceProgressPublisher {
		return svc.inferenceProgressPublisher(sessionID, logger)
	}
}

func newSessionDispatchCompletionObserverFactory(
	sessions *factorysessions.Registry,
) dispatchCompletionObserverFactory {
	if sessions == nil {
		return nil
	}
	svc := &FactoryService{sessions: sessions}
	return func(sessionID string) func(string) {
		normalizedSessionID := strings.TrimSpace(sessionID)
		if normalizedSessionID == "" {
			normalizedSessionID = defaultFactorySessionID
		}
		return func(dispatchID string) {
			session := sessions.Get(normalizedSessionID)
			if session == nil && normalizedSessionID == defaultFactorySessionID {
				session = sessions.Get(defaultFactorySessionID)
			}
			svc.closeSessionResponseStreamDispatch(session, dispatchID)
		}
	}
}

func (fs *FactoryService) inferenceProgressPublisher(
	sessionID string,
	logger *zap.Logger,
) workerprovider.InferenceProgressPublisher {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	sessions := fs.sessions
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		normalizedSessionID = defaultFactorySessionID
	}
	return func(fragment workerprovider.InferenceProgressFragment) {
		dispatchID := strings.TrimSpace(fragment.DispatchID)
		var session *factorysessions.LiveSession
		defer func() {
			if recovered := recover(); recovered != nil {
				emitSessionResponseStreamDegraded(
					session,
					normalizedSessionID,
					dispatchID,
					"PUBLISH_PANIC",
					logger,
					fmt.Errorf("panic during internal provider progress publication: %v", recovered),
				)
			}
		}()
		session = sessions.Get(normalizedSessionID)
		if session == nil && normalizedSessionID == defaultFactorySessionID {
			session = sessions.Get(defaultFactorySessionID)
		}
		stream := fs.sessionResponseStream(session, fragment.DispatchID)
		if stream == nil {
			emitSessionResponseStreamDegraded(session, normalizedSessionID, dispatchID, "STREAM_UNAVAILABLE", logger, nil)
			return
		}
		publisher := responsestream.NewPublisher(stream, func(summary responsestream.CompactionSummary) {
			emitSessionResponseStreamCompaction(session, normalizedSessionID, dispatchID, summary)
		})
		event := mapInferenceProgressFragment(fragment)
		stored := publisher.Publish(event)
		emitSessionResponseStreamPublished(session, normalizedSessionID, stored)
	}
}

func emitSessionResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(event.DispatchID),
		Reason:     string(event.Kind),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamPublished, fields)
}

func emitSessionResponseStreamCompaction(
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
	if handle := liveSessionHandle(session); handle != nil && handle.runtime != nil && handle.runtime.logger != nil {
		handle.runtime.logger.Warn("session response stream compacted internal provider progress",
			zap.String("session_id", sessionID),
			zap.String("dispatch_id", dispatchID),
			zap.String("compaction_reason", string(summary.Reason)),
			zap.Int("dropped_sequence_count", summary.DroppedSequenceCount),
			zap.Int64("first_retained_sequence", summary.FirstRetainedSequence),
			zap.Int64("last_dropped_sequence", summary.LastDroppedSequence),
		)
	}
}

func emitSessionResponseStreamDegraded(
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
	if handle := liveSessionHandle(session); handle != nil && handle.runtime != nil && handle.runtime.logger != nil {
		log = handle.runtime.logger
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
	if handle == nil || handle.runtime == nil {
		return
	}
	if fields.DispatchID == "" {
		fields.DispatchID = sessionID
	}
	if err := handle.runtime.metricsEmitter().Counter(context.Background(), name, 1, fields); err != nil {
		handle.runtime.runtimeLogger().Warn("session response stream metric emission failed",
			zap.String("metric_name", name),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
}

func sortFactorySessionSummaries(summaries []factoryapi.FactorySessionSummary) {
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].IsDefault != summaries[j].IsDefault {
			return summaries[i].IsDefault
		}
		return summaries[i].Id < summaries[j].Id
	})
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
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.durableExecutionService().Pause(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) ResumeDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.durableExecutionService().Resume(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) CancelDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.durableExecutionService().Cancel(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) TerminateDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.durableExecutionService().Terminate(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) ApproveDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	approve, err := factorysession.ApproveRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.durableExecutionService().Approve(ctx, sessionID, approve)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	retry, err := factorysession.RetryDispatchRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.durableExecutionService().RetryDispatch(ctx, sessionID, retry)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	interrupt, err := factorysession.InterruptDispatchRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.durableExecutionService().InterruptDispatch(ctx, sessionID, interrupt)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}
func (fs *FactoryService) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.applyLiveLifecycleControl(
		ctx,
		sessionID,
		factorysessionexecution.LifecycleControlPause,
		control,
	)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.applyLiveLifecycleControl(
		ctx,
		sessionID,
		factorysessionexecution.LifecycleControlResume,
		control,
	)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) applyLiveLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if fs == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("factory service is required")
	}
	if err := ctx.Err(); err != nil {
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	if _, err := factorysessionexecution.NormalizeControlRequest(control); err != nil {
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		fs.observeLiveLifecycleControl(sessionID, operation, control, "", "", err)
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	snapshot, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("get engine state snapshot: %w", err)
	}

	currentStatus := factorysessionexecution.LifecycleStatusFromFactoryRuntimeState(snapshot.FactoryState)
	outcome := factorysessionexecution.EvaluateLifecycleControl(operation, currentStatus)
	if outcome == factorysessionexecution.LifecycleControlOutcomeInvalidState ||
		outcome == factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		controlErr := &factorysessionexecution.ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message: fmt.Sprintf(
				"%s rejected for session %s in status %s",
				operation,
				sessionID,
				currentStatus,
			),
		}
		fs.observeLiveLifecycleControl(sessionID, operation, control, outcome, currentStatus, controlErr)
		return factorysessionexecution.LifecycleControlResult{}, controlErr
	}

	resultStatus := currentStatus
	if outcome == factorysessionexecution.LifecycleControlOutcomeAccepted {
		switch operation {
		case factorysessionexecution.LifecycleControlPause:
			if err := activeFactory.Pause(ctx); err != nil {
				return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("pause live factory session: %w", err)
			}
			resultStatus = factorysessionexecution.LifecycleStatusPaused
		case factorysessionexecution.LifecycleControlResume:
			if err := activeFactory.Resume(ctx); err != nil {
				return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("resume live factory session: %w", err)
			}
			resultStatus = factorysessionexecution.LifecycleStatusRunning
		default:
			return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("unsupported live lifecycle operation %s", operation)
		}
	}

	result := factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    resultStatus,
		Links:     factorysession.LiveLifecycleControlLinksForSession(sessionID),
	}
	fs.observeLiveLifecycleControl(sessionID, operation, control, outcome, resultStatus, nil)
	return result, nil
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
	bundle := liveSessionHandle(session).runtime
	if bundle == nil {
		return
	}
	bundle.emitMetricCounter(runtimeMetricLifecycleControl, 1, metrics.Fields{
		Outcome: outcomeClass,
		Reason:  string(operation),
	})
}
