package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
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

func (fs *FactoryService) activeFactoryDirectory() string {
	if fs == nil {
		return ""
	}
	if fs.cfg != nil {
		if dir := strings.TrimSpace(fs.cfg.Dir); dir != "" {
			return dir
		}
	}
	return fs.factoryRootDir
}

func (fs *FactoryService) registerLiveSession(sessionID string, handle *liveRuntimeHandle, selectSession bool) {
	if fs == nil || fs.sessions == nil || sessionID == "" || handle == nil {
		return
	}
	factoryDir := fs.activeFactoryDirectory()
	folderPath := fs.factoryRootDir
	if folderPath == "" {
		folderPath = factoryDir
	}
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		factoryDir,
		folderPath,
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		handle,
		sessionID == defaultFactorySessionID,
		filepath.Base(folderPath),
	), selectSession)
	if selectSession && handle.runtime != nil {
		fs.swapActiveRuntime(handle.runtime)
	}
}

func (fs *FactoryService) unregisterLiveSession(sessionID string) {
	if fs == nil || fs.sessions == nil {
		return
	}
	fs.sessions.Remove(sessionID)
	current := fs.sessions.Current()
	if current == nil {
		fs.clearActiveRuntime()
		return
	}
	if handle := liveSessionHandle(current); handle != nil && handle.runtime != nil {
		fs.swapActiveRuntime(handle.runtime)
		return
	}
	fs.clearActiveRuntime()
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
	serialized, err := fs.serializeNamedFactory(factorysessions.FactoryName(rootDir, runtimeCfg), runtimeCfg, true)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return fs.withCurrentFactoryVersion(rootDir, serialized.Name, serialized)
}

func (fs *FactoryService) SaveCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}

	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	current, err := fs.GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir, sanitized, err := fs.prepareEditableFactoryDefinitionSave(factorysessions.SessionFactoryRootDir(fs.factoryRootDir, session), current, request)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return fs.saveDefaultCurrentFactoryForSession(ctx, sessionID, session, sessionRootDir, current, request, sanitized)
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireFreshEditableFactoryVersionAtRoot(request.Version, sessionRootDir, current.Name); err != nil {
		return factoryapi.Factory{}, err
	}
	nextVersion := nextEditableFactoryVersion(current.Version, factory.EnsureClock(fs.clock).Now().UTC())
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	factoryDir, err := fs.replaceEditableFactoryDefinition(sessionRootDir, request.Name, payload)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	replacement, err := fs.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, factoryDir, sessionID, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.replaceSessionRuntime(ctx, session, string(request.Name), replacement); err != nil {
		return factoryapi.Factory{}, err
	}

	return fs.GetCurrentFactoryForSession(ctx, sessionID)
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
	result, err := fs.OpenFactorySessionFromFolder(ctx, request.FolderPath, target, request.ValidateOnly != nil && *request.ValidateOnly)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	response := factoryapi.OpenFactorySessionResponse{}
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
) (*FactorySessionOpenResult, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}

	targets, err := fs.discoverFactorySessionTargets(folderPath)
	if err != nil {
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

func (fs *FactoryService) startBackgroundSession(ctx context.Context, sessionID string, runtimeBundle *replacementFactoryRuntime) error {
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
	runtimeBundle *replacementFactoryRuntime,
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
	fs.sessions.Upsert(factorysessions.NewLiveSession(
		sessionID,
		target.FactoryDir,
		target.FolderPath,
		target.Ref,
		handle,
		false,
		target.Project,
	), false)
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
			fs.swapActiveRuntime(successorHandle.runtime)
		} else {
			fs.clearRunState()
			fs.clearActiveRuntime()
		}
	}

	fs.unregisterLiveSession(sessionID)
	if err := fs.stopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
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
	replacement *replacementFactoryRuntime,
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
		fs.swapActiveRuntime(replacement)
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
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, fs.cfg.WorkstationLoader)
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
