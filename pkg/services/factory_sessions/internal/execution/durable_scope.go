package factorysessionexecution

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/workersettings"
	"go.uber.org/zap"
)

type durableScope struct {
	facts         durableexecution.ScopeFacts
	store         runtimepersist.Store
	clock         factory.Clock
	logger        *zap.Logger
	retiring      bool
	controlReplay map[string]controlReplayRecord
}

type durableScopeKey struct {
	runtimeID string
	sessionID string
}

// Acquire allocates an execution handle's state, retaining the fixed behavior
// constructed by Wire. Canonical IDs are local to the selected runtime: two
// default openings must both probe and persist ~default in their own stores.
func (s *JavaScriptRuntimeService) Acquire(ctx context.Context, facts durableexecution.ScopeFacts, clock factory.Clock, logger *zap.Logger) (durableexecution.Service, func(context.Context) error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	id, err := NormalizeSessionID(facts.FactorySessionID)
	if err != nil {
		return nil, nil, err
	}
	key := durableScopeKey{runtimeID: facts.RuntimeID, sessionID: id}
	s.runLifecycleMu.Lock()
	if s.runClosed {
		s.runLifecycleMu.Unlock()
		return nil, nil, ErrDurableExecutionClosed
	}
	if s.acquiredScopes[key] != nil {
		s.runLifecycleMu.Unlock()
		return nil, nil, errors.New("durable Factory Session scope is already acquired")
	}
	owned := s.allocateScopeState(facts)
	if s.acquiredScopes == nil {
		s.acquiredScopes = make(map[durableScopeKey]*JavaScriptRuntimeService)
	}
	s.acquiredScopes[key] = owned
	s.runLifecycleMu.Unlock()
	releaseOwned, acquireErr := owned.acquire(ctx, facts, clock, logger)
	release := func(ctx context.Context) error {
		return s.releaseRuntimeScope(ctx, key, owned, releaseOwned)
	}
	if acquireErr == nil {
		acquireErr = s.ensureOpen()
	}
	if acquireErr != nil {
		return nil, release, acquireErr
	}
	return owned, release, nil
}

// allocateScopeState copies only fixed behavior and replaces every mutable
// runtime map/lock with owned state. Request selections are immutable facts.
func (s *JavaScriptRuntimeService) allocateScopeState(facts durableexecution.ScopeFacts) *JavaScriptRuntimeService {
	// All synchronization and mutable runtime data live behind this pointer.
	// Copying the fixed behavior retains collaborators without reconstruction.
	owned := *s
	owned.durableRuntimeState = &durableRuntimeState{
		sessions:      make(map[string]*runtimeSessionState),
		startReplay:   make(map[string]startReplayRecord),
		startInflight: make(map[string]*startInflightFlight),
		controlReplay: make(map[string]controlReplayRecord),
	}
	owned.projectRoot = facts.ProjectRoot
	owned.childExecutorMode = facts.ChildExecutorMode
	owned.workerPresetIDs = clonePresentedEventIDs(facts.WorkerPresetIDs)
	owned.workerSettings = *workersettings.Clone(&facts.WorkerSettings)
	owned.scopePersistence = &ScopePersistence{stores: s.scopePersistence.stores, scopes: make(map[string]*durableScope)}
	owned.persistence = owned.scopePersistence
	if s.workerExecution != nil {
		owned.workerExecution = owned.newChildWorkerExecutionBinding(s.workerExecution.execute, nil, facts.RuntimeID, "", nil, nil, nil, nil, nil)
	}
	return &owned
}

func (s *JavaScriptRuntimeService) releaseRuntimeScope(ctx context.Context, key durableScopeKey, owned *JavaScriptRuntimeService, release func(context.Context) error) error {
	if release != nil {
		if err := release(ctx); err != nil {
			return err
		}
	}
	s.runLifecycleMu.Lock()
	defer s.runLifecycleMu.Unlock()
	if s.acquiredScopes[key] == owned {
		delete(s.acquiredScopes, key)
	}
	return nil
}

// ScopePersistence routes the fixed runtime's persistence operations by the
// explicit session key, including probes that precede runtime activation.
// It owns resources and selections; it never constructs execution behavior.
type ScopePersistence struct {
	mu     sync.RWMutex
	stores func(string) (runtimepersist.Store, error)
	scopes map[string]*durableScope
}

func NewScopePersistence(stores func(string) (runtimepersist.Store, error)) *ScopePersistence {
	return &ScopePersistence{stores: stores, scopes: make(map[string]*durableScope)}
}

func (p *ScopePersistence) scope(id string) *durableScope {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if scope := p.scopes[id]; scope != nil {
		copied := *scope
		return &copied
	}
	return nil
}

func (p *ScopePersistence) Save(id string, snapshot []byte) error {
	scope := p.scope(id)
	if scope == nil {
		return ErrSessionNotFound
	}
	if scope.store == nil {
		return nil
	}
	return scope.store.Save(id, snapshot)
}

func (p *ScopePersistence) Load(id string) ([]byte, error) {
	scope := p.scope(id)
	if scope == nil || scope.store == nil {
		return nil, fs.ErrNotExist
	}
	return scope.store.Load(id)
}

func (p *ScopePersistence) SnapshotPath(id string) string {
	if scope := p.scope(id); scope != nil && scope.store != nil {
		return persistedSnapshotPath(scope.store, scope.facts.ProjectRoot, id)
	}
	return ""
}

// Acquire retains a unique registration even when store acquisition fails.
// The caller owns the returned release before interpreting the error.
func (s *JavaScriptRuntimeService) acquire(
	ctx context.Context,
	facts durableexecution.ScopeFacts,
	clock factory.Clock,
	logger *zap.Logger,
) (func(context.Context) error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := NormalizeSessionID(facts.FactorySessionID)
	if err != nil {
		return nil, err
	}
	facts.FactorySessionID = id
	facts.ProjectRoot = strings.TrimSpace(facts.ProjectRoot)
	if facts.ProjectRoot == "" {
		return nil, NewValidationError("projectRoot", "projectRoot is required")
	}
	if err := validateChildExecutorMode(facts.ChildExecutorMode); err != nil {
		return nil, err
	}
	facts.WorkerPresetIDs = clonePresentedEventIDs(facts.WorkerPresetIDs)
	facts.WorkerSettings = *workersettings.Clone(&facts.WorkerSettings)
	scope := &durableScope{facts: facts, clock: clock, logger: logger, controlReplay: make(map[string]controlReplayRecord)}
	p := s.scopePersistence
	p.mu.Lock()
	if p.scopes[id] != nil {
		p.mu.Unlock()
		return nil, errors.New("durable Factory Session scope is already acquired")
	}
	p.scopes[id] = scope
	p.mu.Unlock()
	release := func(ctx context.Context) error { return s.releaseScope(ctx, scope) }
	choice, err := PersistenceChoiceForPolicy(facts.Persistence, facts.ProjectRoot, p.stores)
	if err != nil {
		return release, err
	}
	store, err := choice.resolve()
	if err != nil {
		return release, err
	}
	p.mu.Lock()
	scope.store = store
	p.mu.Unlock()
	return release, ctx.Err()
}

func (s *JavaScriptRuntimeService) releaseScope(ctx context.Context, scope *durableScope) error {
	p := s.scopePersistence
	id := scope.facts.FactorySessionID
	s.runLifecycleMu.Lock()
	p.mu.Lock()
	if p.scopes[id] != scope {
		p.mu.Unlock()
		s.runLifecycleMu.Unlock()
		return nil
	}
	scope.retiring = true
	p.mu.Unlock()
	s.mu.RLock()
	state := s.sessions[id]
	var cancel context.CancelFunc
	var done chan struct{}
	if state != nil {
		cancel, done = state.runCancel, state.runDone
	}
	s.mu.RUnlock()
	s.runLifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		joinContext, cancelJoin := context.WithTimeout(ctx, durableExecutionShutdownTimeout)
		defer cancelJoin()
		select {
		case <-done:
		case <-joinContext.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrDurableExecutionShutdownTimeout
		}
	}
	// Keep the routing registration until the run has finished terminal Save.
	// Compare registration identity on every retry and stale release.
	s.mu.Lock()
	defer s.mu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scopes[id] != scope {
		return nil
	}
	if state == s.sessions[id] {
		if state != nil && state.responseEvents != nil {
			s.responseStreams.Close(state.responseEvents)
		}
		delete(s.sessions, id)
		delete(s.pendingPetriHistory, id)
	}
	for requestID, replay := range s.startReplay {
		if replay.sessionID == id {
			delete(s.startReplay, requestID)
		}
	}
	delete(p.scopes, id)
	return nil
}

func (s *JavaScriptRuntimeService) clockForSession(id string) factory.Clock {
	if s.scopePersistence != nil {
		if scope := s.scopePersistence.scope(id); scope != nil {
			return scope.clock
		}
	}
	return s.clock
}

func (s *JavaScriptRuntimeService) nowForSession(id string) time.Time {
	return s.clockForSession(id).Now().UTC()
}

func (s *JavaScriptRuntimeService) modeForSession(id string) string {
	if s.scopePersistence != nil {
		if scope := s.scopePersistence.scope(id); scope != nil {
			return scope.facts.ChildExecutorMode
		}
	}
	return s.childExecutorMode
}

func (s *JavaScriptRuntimeService) settingsForSession(id string) factory.JavaScriptWorkerSettings {
	if s.scopePersistence != nil {
		if scope := s.scopePersistence.scope(id); scope != nil {
			return scope.facts.WorkerSettings
		}
	}
	return s.workerSettings
}

func (s *JavaScriptRuntimeService) loggerForSession(id string) *zap.Logger {
	if s.scopePersistence != nil {
		if scope := s.scopePersistence.scope(id); scope != nil {
			return scope.logger
		}
	}
	return s.persistenceWarningLogger
}

// The caller holds s.mu for this session's control replay transaction.
func (s *JavaScriptRuntimeService) controlReplayForSession(id string) map[string]controlReplayRecord {
	if s.scopePersistence != nil {
		if scope := s.scopePersistence.scope(id); scope != nil {
			return scope.controlReplay
		}
	}
	return s.controlReplay
}

func (s *JavaScriptRuntimeService) beginScopeRunAdmission(id string) (*durableRunAdmission, error) {
	admission, err := s.beginRunAdmission()
	if err != nil {
		return nil, err
	}
	if s.scopePersistence != nil {
		scope := s.scopePersistence.scope(id)
		if scope == nil || scope.retiring {
			admission.release()
			return nil, ErrSessionNotFound
		}
	}
	return admission, nil
}
