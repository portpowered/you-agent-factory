// Package service implements the Worker Sessions W1 registry: reservation,
// immutable Get, and deterministic filtered List over a synchronized
// process-local map.
package service

import (
	"context"
	"slices"
	"sort"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type registry struct {
	mu       sync.RWMutex
	sessions map[string]workersessions.Session
	logger   logging.Logger
}

// Compile-time proof that production registry seals the W1 root contract
// (Reserve + Get + List) without exposing the mutable map or a broader API.
var _ workersessions.Service = (*registry)(nil)

// New constructs the process-local Worker Session registry. A nil logger
// falls back to logging.NoopLogger.
func New(logger logging.Logger) workersessions.Service {
	return &registry{
		sessions: make(map[string]workersessions.Session),
		logger:   logging.EnsureLogger(logger),
	}
}

func (r *registry) Reserve(_ context.Context, req workersessions.ReserveRequest) (workersessions.Session, error) {
	if err := req.Validate(); err != nil {
		return workersessions.Session{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[req.ID]; exists {
		return workersessions.Session{}, workersessions.ErrSessionAlreadyExists
	}
	session := workersessions.Session{ID: req.ID, State: workersessions.StateReserved}
	r.sessions[req.ID] = session
	return session, nil
}

func (r *registry) Get(_ context.Context, req workersessions.GetRequest) (workersessions.Session, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session get rejected", "sessionID", req.ID, "outcome", "invalid")
		return workersessions.Session{}, err
	}

	r.mu.RLock()
	session, exists := r.sessions[req.ID]
	r.mu.RUnlock()

	if !exists {
		r.logger.Info("worker session get", "sessionID", req.ID, "outcome", "not_found")
		return workersessions.Session{}, workersessions.ErrSessionNotFound
	}
	r.logger.Info("worker session get", "sessionID", req.ID, "outcome", "found", "state", string(session.State))
	return session, nil
}

func (r *registry) List(_ context.Context, req workersessions.ListRequest) (workersessions.ListResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ListResult{}, err
	}

	r.mu.RLock()
	matched := make([]workersessions.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		if matchesFilter(session, req.Filter) {
			matched = append(matched, session)
		}
	}
	r.mu.RUnlock()

	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	return workersessions.ListResult{Sessions: matched}, nil
}

func matchesFilter(session workersessions.Session, filter workersessions.Filter) bool {
	if len(filter.States) == 0 {
		return true
	}
	return slices.Contains(filter.States, session.State)
}
