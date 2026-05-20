package service

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const defaultFactorySessionID = "~default"

type liveFactorySession struct {
	id     string
	handle *liveRuntimeHandle
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
	fs.sessions.upsert(&liveFactorySession{
		id:     sessionID,
		handle: handle,
	}, selectSession)
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

func (fs *FactoryService) sessionByID(sessionID string) *liveFactorySession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.get(sessionID)
}

func (fs *FactoryService) openFactorySession(ctx context.Context, factoryDir string) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, factoryDir)
	if err != nil {
		return "", err
	}
	sessionID := newFactorySessionID()
	if err := fs.startBackgroundSession(ctx, sessionID, replacement); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *FactoryService) startBackgroundSession(ctx context.Context, sessionID string, runtimeBundle *replacementFactoryRuntime) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	if runtimeBundle == nil {
		return fmt.Errorf("runtime bundle is required")
	}
	parentCtx := ctx
	if runState := fs.currentRunState(); runState != nil && runState.ctx != nil {
		parentCtx = runState.ctx
	}
	handle := fs.startLiveRuntime(parentCtx, runtimeBundle)
	if err := fs.waitForLiveRuntimeStart(ctx, handle); err != nil {
		_ = fs.stopLiveRuntime(handle)
		return fmt.Errorf("start runtime session: %w", err)
	}
	if fs.cfg != nil && runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService {
		if err := fs.startLiveRuntimeSidecars(parentCtx, handle); err != nil {
			_ = fs.stopLiveRuntime(handle)
			return fmt.Errorf("start runtime session sidecars: %w", err)
		}
	}
	fs.registerLiveSession(sessionID, handle, false)
	return nil
}

func (fs *FactoryService) stopFactorySession(sessionID string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	runState := fs.currentRunState()
	if runState != nil && runState.sessionID == sessionID {
		return fmt.Errorf("stopping the selected runtime session is not supported")
	}
	session := fs.sessionByID(sessionID)
	if session == nil || session.handle == nil {
		return fmt.Errorf("runtime session %q not found", sessionID)
	}
	if err := fs.stopLiveRuntime(session.handle); err != nil && err != context.Canceled {
		return err
	}
	fs.unregisterLiveSession(sessionID)
	return nil
}
