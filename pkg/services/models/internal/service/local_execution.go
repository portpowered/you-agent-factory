package service

import (
	"context"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	"sync"
)

// ScopedLocalExecution owns scope admission for legacy catalog asset pulls.
// Inference and host leases are owned by the current joined Models operation.
type ScopedLocalExecution interface {
	PullModelForScope(context.Context, models.PullModelRequest) (models.PullResult, error)
	CloseScope(models.RuntimeScopeRef)
	Close()
}

type scopedLocalExecution struct {
	scopes       runtimescopes.Service
	assets       scopedassets.Service
	mu           sync.Mutex
	closedScopes map[models.RuntimeScopeRef]bool
	closed       bool
}

func NewScopedLocalExecution(scopes runtimescopes.Service, assets scopedassets.Service) (ScopedLocalExecution, error) {
	return &scopedLocalExecution{scopes: scopes, assets: assets, closedScopes: make(map[models.RuntimeScopeRef]bool)}, nil
}

func (s *scopedLocalExecution) admitInvocation(ctx context.Context, scope models.RuntimeScopeRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closedScopes[scope] {
		return models.ErrRuntimeScopeClosed
	}
	return ctx.Err()
}
func (s *scopedLocalExecution) CloseScope(scope models.RuntimeScopeRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closedScopes[scope] = true
}
func (s *scopedLocalExecution) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	clear(s.closedScopes)
}
