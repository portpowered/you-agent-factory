package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

const defaultFactorySessionID = "~default"

type FactorySessionTargetKind string

const (
	FactorySessionTargetKindDefault FactorySessionTargetKind = "default"
	FactorySessionTargetKindNamed   FactorySessionTargetKind = "named"
)

type FactorySessionTargetRef struct {
	Kind FactorySessionTargetKind
	Name string
}

type FactorySessionTarget struct {
	Ref        FactorySessionTargetRef
	Label      string
	FolderPath string
	FactoryDir string
	Project    string
}

type FactorySessionOpenResult struct {
	SessionID string
	Targets   []FactorySessionTarget
}

type liveFactorySession struct {
	id         string
	factoryDir string
	folderPath string
	handle     *liveRuntimeHandle
	isDefault  bool
	project    string
	target     FactorySessionTargetRef
}

type liveRuntimeSessionManager struct {
	mu         sync.RWMutex
	selectedID string
	sessions   map[string]*liveFactorySession
}

func newLiveRuntimeSessionManager() *liveRuntimeSessionManager {
	return &liveRuntimeSessionManager{
		sessions: make(map[string]*liveFactorySession),
	}
}

func (m *liveRuntimeSessionManager) upsert(session *liveFactorySession, selectSession bool) {
	if m == nil || session == nil || session.id == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.id] = session
	if selectSession || m.selectedID == "" {
		m.selectedID = session.id
	}
}

func (m *liveRuntimeSessionManager) current() *liveFactorySession {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if session, ok := m.sessions[m.selectedID]; ok {
		return session
	}
	return nil
}

func (m *liveRuntimeSessionManager) get(id string) *liveFactorySession {
	if m == nil || id == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *liveRuntimeSessionManager) remove(id string) {
	if m == nil || id == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.selectedID != id {
		return
	}
	m.selectedID = ""
	if len(m.sessions) == 0 {
		return
	}
	ids := make([]string, 0, len(m.sessions))
	for sessionID := range m.sessions {
		ids = append(ids, sessionID)
	}
	sort.Strings(ids)
	m.selectedID = ids[0]
}

func (m *liveRuntimeSessionManager) count() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

func (m *liveRuntimeSessionManager) ids() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func newFactorySessionID() string {
	return uuid.NewString()
}

func (fs *FactoryService) registerLiveSession(sessionID string, handle *liveRuntimeHandle, selectSession bool) {
	if fs == nil || fs.sessions == nil || sessionID == "" || handle == nil {
		return
	}
	fs.sessions.upsert(newLiveFactorySession(
		sessionID,
		fs.factoryRootDir,
		fs.factoryRootDir,
		FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
		handle,
		sessionID == defaultFactorySessionID,
		filepath.Base(fs.factoryRootDir),
	), selectSession)
	if selectSession && handle.runtime != nil {
		fs.swapActiveRuntime(handle.runtime)
	}
}

func (fs *FactoryService) unregisterLiveSession(sessionID string) {
	if fs == nil || fs.sessions == nil {
		return
	}
	fs.sessions.remove(sessionID)
	current := fs.sessions.current()
	if current == nil || current.handle == nil || current.handle.runtime == nil {
		fs.clearActiveRuntime()
		return
	}
	fs.swapActiveRuntime(current.handle.runtime)
}

func (fs *FactoryService) currentSession() *liveFactorySession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.current()
}

func (fs *FactoryService) defaultSession() *liveFactorySession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.get(defaultFactorySessionID)
}

func (fs *FactoryService) sessionByID(sessionID string) *liveFactorySession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.get(sessionID)
}

func (fs *FactoryService) requireSession(sessionID string) (*liveFactorySession, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	session := fs.sessionByID(sessionID)
	if session == nil || session.handle == nil || session.handle.runtime == nil {
		return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	return session, nil
}

func (fs *FactoryService) sessionFactory(sessionID string) (factory.Factory, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return session.handle.runtime.factory, nil
}

func (fs *FactoryService) sessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return session.handle.runtime.runtimeCfg, nil
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

func (fs *FactoryService) GetCurrentNamedFactoryForSession(_ context.Context, sessionID string) (factoryapi.Factory, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return fs.serializeNamedFactory(sessionFactoryName(sessionFactoryRootDir(fs, session), runtimeCfg), runtimeCfg, true)
}

func (fs *FactoryService) GetEditableFactoryDefinitionForSession(ctx context.Context, sessionID string) (factoryapi.EditableFactoryDefinition, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	current, err := fs.GetCurrentNamedFactoryForSession(ctx, sessionID)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	version, err := fs.currentFactoryDefinitionVersionAtRoot(sessionFactoryRootDir(fs, session), current.Name)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	return factoryapi.EditableFactoryDefinition{
		FactoryDefinition: current,
		Version:           version,
	}, nil
}

func (fs *FactoryService) SaveEditableFactoryDefinitionForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.SaveEditableFactoryDefinitionRequest,
) (factoryapi.EditableFactoryDefinition, error) {
	if fs == nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("factory service is required")
	}

	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	current, err := fs.GetCurrentNamedFactoryForSession(ctx, sessionID)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return factoryapi.EditableFactoryDefinition{}, ErrCurrentNamedFactoryNotFound
	}
	if request.FactoryDefinition.Name != current.Name {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("%w: editable save must preserve current factory name %q", ErrInvalidNamedFactoryName, current.Name)
	}
	if err := apisurface.ValidateWritableNamedFactoryName(request.FactoryDefinition.Name); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if err := validateEditableFactoryTopology(request.FactoryDefinition); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	payload, err := json.Marshal(request.FactoryDefinition)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("marshal editable factory payload: %w", err)
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	sessionRootDir := sessionFactoryRootDir(fs, session)
	if err := fs.requireFreshEditableFactoryVersionAtRoot(request.BaseVersion, sessionRootDir, current.Name); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	factoryDir, err := factoryconfig.ReplaceNamedFactory(sessionRootDir, string(request.FactoryDefinition.Name), payload)
	if err != nil {
		switch {
		case errors.Is(err, factoryconfig.ErrInvalidNamedFactory):
			return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
		default:
			return factoryapi.EditableFactoryDefinition{}, err
		}
	}

	replacement, err := fs.buildReplacementFactoryRuntime(ctx, sessionRootDir, factoryDir, sessionID)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, request.FactoryDefinition.Name, err)
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if err := fs.replaceSessionRuntime(ctx, session, string(request.FactoryDefinition.Name), replacement); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	return fs.GetEditableFactoryDefinitionForSession(ctx, sessionID)
}

func (fs *FactoryService) ListFactorySessions(_ context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if fs == nil || fs.sessions == nil {
		return factoryapi.ListFactorySessionsResponse{}, nil
	}
	sessionIDs := fs.sessions.ids()
	summaries := make([]factoryapi.FactorySessionSummary, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session := fs.sessionByID(sessionID)
		if session == nil {
			continue
		}
		summaries = append(summaries, sessionSummaryResponse(session))
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].IsDefault != summaries[j].IsDefault {
			return summaries[i].IsDefault
		}
		return summaries[i].Id < summaries[j].Id
	})
	return factoryapi.ListFactorySessionsResponse{Sessions: summaries}, nil
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
	result, err := fs.OpenFactorySessionFromFolder(ctx, request.FolderPath, target)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	response := factoryapi.OpenFactorySessionResponse{}
	if len(result.Targets) > 0 {
		targets := make([]factoryapi.FactorySessionTarget, 0, len(result.Targets))
		for _, sessionTarget := range result.Targets {
			targets = append(targets, factorySessionTargetResponse(sessionTarget))
		}
		response.Targets = &targets
	}
	if result.SessionID != "" {
		session, err := fs.requireSession(result.SessionID)
		if err != nil {
			return factoryapi.OpenFactorySessionResponse{}, err
		}
		summary := sessionSummaryResponse(session)
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
	sessionID := newFactorySessionID()
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
) (*FactorySessionOpenResult, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}

	targets, err := fs.discoverFactorySessionTargets(folderPath)
	if err != nil {
		return nil, err
	}

	selectedTarget, err := selectFactorySessionTarget(targets, target)
	if err != nil {
		return nil, err
	}
	if selectedTarget == nil {
		return &FactorySessionOpenResult{Targets: cloneFactorySessionTargets(targets)}, nil
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
	sessionID := newFactorySessionID()
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
	if runState := fs.currentRunState(); runState != nil && runState.ctx != nil {
		ctx = runState.ctx
	}
	handle := fs.startLiveRuntime(ctx, runtimeBundle)
	if err := fs.waitForLiveRuntimeStart(ctx, handle); err != nil {
		_ = fs.stopLiveRuntime(handle)
		return fmt.Errorf("start runtime session: %w", err)
	}
	if fs.cfg != nil && runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService {
		if err := fs.startLiveRuntimeSidecars(ctx, handle); err != nil {
			_ = fs.stopLiveRuntime(handle)
			return fmt.Errorf("start runtime session sidecars: %w", err)
		}
	}
	fs.sessions.upsert(newLiveFactorySession(
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
	if session == nil || session.handle == nil {
		return fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}

	runState := fs.currentRunState()
	if runState != nil && runState.sessionID == sessionID {
		successor := fs.nextLiveSessionAfterStop(sessionID)
		if successor != nil {
			fs.setRunState(runState.ctx, successor.id, successor.handle)
			fs.swapActiveRuntime(successor.handle.runtime)
		} else {
			fs.clearRunState()
			fs.clearActiveRuntime()
		}
	}

	fs.unregisterLiveSession(sessionID)
	if err := fs.stopLiveRuntime(session.handle); err != nil && !errors.Is(err, context.Canceled) {
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
	session *liveFactorySession,
	name string,
	replacement *replacementFactoryRuntime,
) error {
	if session == nil || session.handle == nil {
		return fmt.Errorf("%w: session handle is unavailable", apisurface.ErrFactorySessionNotFound)
	}
	runState := fs.currentRunState()
	serviceCtx := ctx
	if runState != nil && runState.ctx != nil {
		serviceCtx = runState.ctx
	}
	isActiveSession := runState != nil && runState.sessionID == session.id

	restoreCurrentSidecars := false
	serviceMode := fs.cfg != nil && runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService
	if serviceMode {
		fs.stopLiveRuntimeSidecars(session.handle)
		restoreCurrentSidecars = true
		defer func() {
			if restoreCurrentSidecars {
				fs.restoreLiveRuntimeSidecars(&serviceRunState{ctx: serviceCtx, runtime: session.handle})
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

	fs.publishFactoryChangeEvent(ctx, session.handle, replacement)
	restoreCurrentSidecars = false
	fs.sessions.upsert(newLiveFactorySession(
		session.id,
		replacement.dir,
		session.folderPath,
		session.target,
		replacementHandle,
		session.isDefault,
		session.project,
	), isActiveSession)
	if isActiveSession {
		fs.swapActiveRuntime(replacement)
		fs.setRunState(serviceCtx, session.id, replacementHandle)
	}
	if err := fs.stopLiveRuntime(session.handle); err != nil && !errors.Is(err, context.Canceled) {
		fs.logger.Warn("prior session runtime shutdown failed", zap.Error(err), zap.String("session_id", session.id))
	}
	return nil
}

func sessionFactoryRootDir(fs *FactoryService, session *liveFactorySession) string {
	if session == nil {
		return ""
	}
	rootDir := session.folderPath
	if session.folderPath == "" {
		return rootDir
	}
	if session.factoryDir == "" || !sameFactoryDir(session.factoryDir, session.folderPath) {
		return rootDir
	}
	serviceRoot := filepath.Clean(fs.factoryRootDir)
	if serviceRoot != "" && filepath.Dir(session.factoryDir) == serviceRoot {
		return serviceRoot
	}
	return rootDir
}

func (fs *FactoryService) nextLiveSessionAfterStop(sessionID string) *liveFactorySession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	for _, id := range fs.sessions.ids() {
		if id == sessionID {
			continue
		}
		next := fs.sessionByID(id)
		if next != nil && next.handle != nil {
			return next
		}
	}
	return nil
}

func (fs *FactoryService) discoverFactorySessionTargets(folderPath string) ([]FactorySessionTarget, error) {
	resolvedFolder, err := resolveFactorySessionFolder(folderPath)
	if err != nil {
		return nil, err
	}

	targets := make([]FactorySessionTarget, 0, 4)
	if target, ok := fs.loadFactorySessionTarget(resolvedFolder, resolvedFolder, FactorySessionTargetRef{
		Kind: FactorySessionTargetKindDefault,
	}); ok {
		targets = append(targets, target)
	}

	childEntries, err := os.ReadDir(resolvedFolder)
	if err != nil {
		return nil, fmt.Errorf("read factory session folder %s: %w", resolvedFolder, err)
	}
	for _, entry := range childEntries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if err := factoryconfig.ValidateNamedFactoryName(name); err != nil {
			continue
		}
		targetDir := filepath.Join(resolvedFolder, name)
		target, ok := fs.loadFactorySessionTarget(resolvedFolder, targetDir, FactorySessionTargetRef{
			Kind: FactorySessionTargetKindNamed,
			Name: name,
		})
		if ok {
			targets = append(targets, target)
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		left := targets[i]
		right := targets[j]
		if left.Ref.Kind != right.Ref.Kind {
			return left.Ref.Kind == FactorySessionTargetKindDefault
		}
		return left.Ref.Name < right.Ref.Name
	})
	if len(targets) == 0 {
		return nil, fmt.Errorf("folder %q does not expose any runnable factory targets", resolvedFolder)
	}
	return targets, nil
}

func (fs *FactoryService) loadFactorySessionTarget(
	folderPath string,
	factoryDir string,
	ref FactorySessionTargetRef,
) (FactorySessionTarget, bool) {
	if fs == nil {
		return FactorySessionTarget{}, false
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, fs.cfg.WorkstationLoader)
	if err != nil {
		return FactorySessionTarget{}, false
	}

	label := "default"
	if ref.Kind == FactorySessionTargetKindNamed {
		label = ref.Name
	}
	project := ""
	if cfg := loaded.FactoryConfig(); cfg != nil {
		project = strings.TrimSpace(cfg.Project)
		if project == "" {
			project = strings.TrimSpace(cfg.Name)
		}
	}

	return FactorySessionTarget{
		Ref:        ref,
		Label:      label,
		FolderPath: folderPath,
		FactoryDir: factoryDir,
		Project:    project,
	}, true
}

func resolveFactorySessionFolder(folderPath string) (string, error) {
	trimmed := strings.TrimSpace(folderPath)
	if trimmed == "" {
		return "", fmt.Errorf("factory session folder is required")
	}
	resolved, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve factory session folder %q: %w", folderPath, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat factory session folder %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("factory session folder %q must be a directory", resolved)
	}
	return resolved, nil
}

func selectFactorySessionTarget(
	targets []FactorySessionTarget,
	ref *FactorySessionTargetRef,
) (*FactorySessionTarget, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("factory session target list is empty")
	}
	if ref == nil {
		if len(targets) == 1 {
			target := targets[0]
			return &target, nil
		}
		return nil, nil
	}

	normalized := FactorySessionTargetRef{
		Kind: ref.Kind,
		Name: strings.TrimSpace(ref.Name),
	}
	switch normalized.Kind {
	case FactorySessionTargetKindDefault:
		normalized.Name = ""
	case FactorySessionTargetKindNamed:
		if normalized.Name == "" {
			return nil, fmt.Errorf("named factory session target requires a name")
		}
	default:
		return nil, fmt.Errorf("unsupported factory session target kind %q", normalized.Kind)
	}

	for i := range targets {
		if targets[i].Ref == normalized {
			target := targets[i]
			return &target, nil
		}
	}
	return nil, fmt.Errorf("factory session target %q was not found", factorySessionTargetDisplayName(normalized))
}

func factorySessionTargetDisplayName(ref FactorySessionTargetRef) string {
	if ref.Kind == FactorySessionTargetKindDefault {
		return "default"
	}
	return ref.Name
}

func cloneFactorySessionTargets(targets []FactorySessionTarget) []FactorySessionTarget {
	if len(targets) == 0 {
		return nil
	}
	cloned := make([]FactorySessionTarget, len(targets))
	copy(cloned, targets)
	return cloned
}

func newLiveFactorySession(
	sessionID string,
	factoryDir string,
	folderPath string,
	target FactorySessionTargetRef,
	handle *liveRuntimeHandle,
	isDefault bool,
	project string,
) *liveFactorySession {
	return &liveFactorySession{
		id:         sessionID,
		factoryDir: factoryDir,
		folderPath: folderPath,
		handle:     handle,
		isDefault:  isDefault,
		project:    project,
		target:     target,
	}
}

func sessionSummaryResponse(session *liveFactorySession) factoryapi.FactorySessionSummary {
	return factoryapi.FactorySessionSummary{
		FactoryDir: session.factoryDir,
		FolderPath: session.folderPath,
		Id:         session.id,
		IsDefault:  session.isDefault,
		Project:    session.project,
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(session.target.Kind),
			Name: stringPointerOrNil(session.target.Name),
		},
	}
}

func factorySessionTargetResponse(target FactorySessionTarget) factoryapi.FactorySessionTarget {
	return factoryapi.FactorySessionTarget{
		FactoryDir: target.FactoryDir,
		FolderPath: target.FolderPath,
		Label:      target.Label,
		Project:    target.Project,
		Ref: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(target.Ref.Kind),
			Name: stringPointerOrNil(target.Ref.Name),
		},
	}
}

func stringPointerOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func sessionFactoryName(rootDir string, runtimeCfg *factoryconfig.LoadedFactoryConfig) factoryapi.FactoryName {
	if runtimeCfg == nil {
		return apisurface.DefaultCurrentFactoryName
	}
	factoryDir := runtimeCfg.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if sameFactoryDir(factoryDir, cleanRoot) {
		return apisurface.DefaultCurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		name := filepath.Base(factoryDir)
		if err := factoryconfig.ValidateNamedFactoryName(name); err == nil {
			return factoryapi.FactoryName(name)
		}
	}
	cfg := runtimeCfg.FactoryConfig()
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Name); name != "" {
			return factoryapi.FactoryName(name)
		}
		if project := strings.TrimSpace(cfg.Project); project != "" {
			return factoryapi.FactoryName(project)
		}
	}
	return factoryapi.FactoryName("factory")
}
