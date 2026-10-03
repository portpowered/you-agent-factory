package service

import (
	"context"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
)

type cursorScopeState struct {
	mu      sync.RWMutex
	memory  map[string]persistedCursor
	durable durableCursorRecorder
}

type cursorScopes struct {
	files  CursorPersistenceFileSystem
	mu     sync.Mutex
	states map[scriptpollers.CursorScope]*cursorScopeState
}

var _ scriptpollers.CursorScopes = (*cursorScopes)(nil)

// NewCursorScopes stores the required filesystem without performing IO.
func NewCursorScopes(files CursorPersistenceFileSystem) scriptpollers.CursorScopes {
	return &cursorScopes{files: files, states: make(map[scriptpollers.CursorScope]*cursorScopeState)}
}

func (s *cursorScopes) GetCursor(
	ctx context.Context,
	scope scriptpollers.CursorScope,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	state := s.state(scope)
	if strings.TrimSpace(scope.BaseDir) != "" {
		return state.durable.GetCursor(ctx, request)
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if strings.TrimSpace(request.InstanceID) == "" || strings.TrimSpace(request.InstanceID) != request.InstanceID {
		return automations.GetCursorResult{}, invalidCursorOperationError(scriptpollers.GetCursorOperation, "malformed instance identity")
	}
	current, exists := state.memory[request.InstanceID]
	if !exists {
		return automations.GetCursorResult{}, &automations.Error{
			Op: scriptpollers.GetCursorOperation, Code: automations.ErrorCodeNotFound, Err: automations.ErrNotFound,
		}
	}
	if request.ExpectedCursor != "" && request.ExpectedCursor != current.Cursor {
		return automations.GetCursorResult{}, scriptpollers.CursorConflictError(scriptpollers.GetCursorOperation)
	}
	return automations.GetCursorResult{
		AutomationID: current.AutomationID, InstanceID: current.InstanceID,
		Cursor: current.Cursor, Checkpoint: current.Checkpoint,
	}, nil
}

func (s *cursorScopes) CommitCursor(
	ctx context.Context,
	scope scriptpollers.CursorScope,
	request scriptpollers.CommitCursorRequest,
) error {
	state := s.state(scope)
	if strings.TrimSpace(scope.BaseDir) != "" {
		return state.durable.CommitCursor(ctx, request)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	instanceID, err := commitCursorInstanceID(request)
	if err != nil {
		return err
	}
	current, exists := state.memory[instanceID]
	if err := validateExpectedCursor(request.ExpectedCursor, current, exists); err != nil {
		return err
	}
	state.memory[instanceID] = persistedCursor{
		AutomationID: strings.TrimSpace(request.AutomationID), InstanceID: instanceID,
		Cursor: request.Cursor, Checkpoint: request.Checkpoint,
	}
	return nil
}

// state allocates only runtime-keyed records and a path/lock resource. All
// scopes retain the same injected behavior owner and filesystem effect.
func (s *cursorScopes) state(scope scriptpollers.CursorScope) *cursorScopeState {
	scope.BaseDir = strings.TrimSpace(scope.BaseDir)
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.states[scope]; state != nil {
		return state
	}
	state := &cursorScopeState{
		memory:  make(map[string]persistedCursor),
		durable: durableCursorRecorder{dir: cursorStateDir(scope.BaseDir), files: s.files},
	}
	s.states[scope] = state
	return state
}
