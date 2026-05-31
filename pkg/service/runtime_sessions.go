package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

const (
	defaultFactorySessionID        = factorysessions.DefaultSessionID
	FactorySessionTargetKindDefault = factorysessions.TargetKindDefault
	FactorySessionTargetKindNamed   = factorysessions.TargetKindNamed
)

type (
	FactorySessionTargetKind = factorysessions.TargetKind
	FactorySessionTargetRef  = factorysessions.TargetRef
	FactorySessionTarget     = factorysessions.Target
	FactorySessionOpenResult = factorysessions.OpenResult
	liveFactorySession         = factorysessions.LiveSession
)

func liveSessionHandle(session *factorysessions.LiveSession) *liveRuntimeHandle {
	if session == nil {
		return nil
	}
	handle, _ := session.Handle.(*liveRuntimeHandle)
	return handle
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
	factoryDir := strings.TrimSpace(target.FactoryDir)
	if factoryDir == "" && handle.runtime != nil {
		factoryDir = handle.runtime.dir
	}
	folderPath := strings.TrimSpace(target.FolderPath)
	if folderPath == "" {
		folderPath = fs.factoryRootDir
	}
	if folderPath == "" {
		folderPath = factoryDir
	}
	targetRef := target.Ref
	if targetRef.Kind == "" {
		targetRef = FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault}
	}
	project := strings.TrimSpace(target.Project)
	if project == "" {
		project = filepath.Base(folderPath)
	}
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		factoryDir,
		folderPath,
		targetRef,
		handle,
		sessionID == defaultFactorySessionID,
		project,
	), selectSession)
	if selectSession && handle.runtime != nil {
		fs.syncActiveSessionDir(handle.runtime)
		if sessionID == defaultFactorySessionID {
			fs.clearStartupBundle()
		}
	}
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
	fs.sessions.Remove(sessionID)
	current := fs.sessions.Current()
	if current == nil {
		fs.resetActiveSessionDir()
		return
	}
	if handle := liveSessionHandle(current); handle != nil && handle.runtime != nil {
		fs.syncActiveSessionDir(handle.runtime)
		return
	}
	fs.resetActiveSessionDir()
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
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	return activeFactory.SubmitWorkRequest(ctx, request)
}

func (fs *FactoryService) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
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

func (fs *FactoryService) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string) (*interfaces.FactoryEventStream, error) {
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	stream, err := activeFactory.SubscribeFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

func (fs *FactoryService) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
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

func (fs *FactoryService) GetCurrentFactoryForSession(_ context.Context, sessionID string) (factoryapi.Factory, error) {
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

func (fs *FactoryService) ListFactorySessions(_ context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if fs == nil || fs.sessions == nil {
		return factoryapi.ListFactorySessionsResponse{}, nil
	}
	return factoryapi.ListFactorySessionsResponse{Sessions: factorysessions.ListSummaries(fs.sessions)}, nil
}

func (fs *FactoryService) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
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

func (fs *FactoryService) CloseFactorySession(_ context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("factory session id is required")
	}
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

	if err := initcmd.Init(initcmd.InitConfig{
		Dir:         resolvedFolder,
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
	if fs.cfg != nil && runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService {
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
			successorHandle := liveSessionHandle(successor)
			fs.setRunState(runState.ctx, successor.ID, successorHandle)
			fs.syncActiveSessionDir(successorHandle.runtime)
		} else {
			fs.clearRunState()
			fs.resetActiveSessionDir()
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

//nolint:contextcheck // The request context bounds the save/startup wait, while the long-lived service runtime context owns the replacement session runtime and sidecars after the request returns.
func (fs *FactoryService) replaceSessionRuntime(
	ctx context.Context,
	session *factorysessions.LiveSession,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	if session == nil {
		return fmt.Errorf("%w: session handle is unavailable", apisurface.ErrFactorySessionNotFound)
	}
	handle := liveSessionHandle(session)
	if handle == nil {
		return fmt.Errorf("%w: session handle is unavailable", apisurface.ErrFactorySessionNotFound)
	}
	runState := fs.currentRunState()
	serviceCtx := ctx
	if runState != nil && runState.ctx != nil {
		serviceCtx = runState.ctx
	}
	isActiveSession := runState != nil && runState.sessionID == session.ID

	restoreCurrentSidecars := false
	serviceMode := fs.cfg != nil && runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService
	if serviceMode {
		fs.stopLiveRuntimeSidecars(handle)
		restoreCurrentSidecars = true
		defer func() {
			if restoreCurrentSidecars {
				fs.restoreLiveRuntimeSidecars(&serviceRunState{ctx: serviceCtx, runtime: handle})
			}
		}()
	}

	replacementHandle := fs.startLiveRuntime(serviceCtx, replacement)
	if err := fs.waitForLiveRuntimeStart(ctx, replacementHandle); err != nil {
		_ = fs.stopLiveRuntime(replacementHandle)
		return fmt.Errorf("start replacement runtime: %w", err)
	}
	if serviceMode {
		if err := fs.startLiveRuntimeSidecars(serviceCtx, replacementHandle); err != nil {
			_ = fs.stopLiveRuntime(replacementHandle)
			return fmt.Errorf("start replacement runtime sidecars: %w", err)
		}
	}

	fs.publishFactoryChangeEvent(ctx, handle, replacement)
	restoreCurrentSidecars = false
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		session.ID,
		replacement.dir,
		session.FolderPath,
		session.Target,
		replacementHandle,
		session.IsDefault,
		session.Project,
	), isActiveSession)
	if isActiveSession {
		fs.syncActiveSessionDir(replacement)
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
) (factorysessions.Target, bool) {
	if fs == nil {
		return factorysessions.Target{}, false
	}
	loaded, err := configload.LoadRuntimeConfigFromFactoryDir(factoryDir, fs.cfg.WorkstationLoader)
	if err != nil {
		return factorysessions.Target{}, false
	}

	project := ""
	if cfg := loaded.FactoryConfig(); cfg != nil {
		project = strings.TrimSpace(cfg.Project)
		if project == "" {
			project = strings.TrimSpace(cfg.Name)
		}
	}
	return factorysessions.BuildTargetFromConfig(folderPath, factoryDir, ref, project), true
}
