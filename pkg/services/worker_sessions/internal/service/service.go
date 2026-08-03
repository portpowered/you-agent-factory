// Package service implements the Worker Sessions W1+W2 registry:
// reservation, immutable Get, deterministic filtered List, and supervised
// Start with exactly-once terminal classification, over a synchronized
// process-local map.
package service

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ErrMissingExecution reports that New was constructed without the one
// required directly injected workers.WorkstationExecutionService.
var ErrMissingExecution = errors.New("worker sessions: execution service is required")

type registry struct {
	mu        sync.RWMutex
	sessions  map[string]workersessions.Session
	execution workers.WorkstationExecutionService
	logger    logging.Logger
}

// Compile-time proof that production registry seals the W1+W2 root contract
// (Reserve + Get + List + Start) without exposing the mutable map or a
// broader API.
var _ workersessions.Service = (*registry)(nil)

// New constructs the process-local Worker Session registry from the one
// directly injected workers.WorkstationExecutionService that Start hands
// attempts to. A nil logger falls back to logging.NoopLogger. A nil
// execution is rejected: Start has no meaningful behavior without it.
func New(execution workers.WorkstationExecutionService, logger logging.Logger) (workersessions.Service, error) {
	if execution == nil {
		return nil, ErrMissingExecution
	}
	return &registry{
		sessions:  make(map[string]workersessions.Session),
		execution: execution,
		logger:    logging.EnsureLogger(logger),
	}, nil
}

func (r *registry) Reserve(_ context.Context, req workersessions.ReserveRequest) (workersessions.Session, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session reserve rejected", "sessionID", req.ID, "outcome", "invalid")
		return workersessions.Session{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[req.ID]; exists {
		r.logger.Info("worker session reserve", "sessionID", req.ID, "outcome", "duplicate")
		return workersessions.Session{}, workersessions.ErrSessionAlreadyExists
	}
	session := workersessions.Session{ID: req.ID, State: workersessions.StateReserved}
	r.sessions[req.ID] = session
	r.logger.Info("worker session reserve", "sessionID", req.ID, "outcome", "reserved")
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
	return cloneSession(session), nil
}

func (r *registry) List(_ context.Context, req workersessions.ListRequest) (workersessions.ListResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session list rejected", "outcome", "invalid")
		return workersessions.ListResult{}, err
	}

	r.mu.RLock()
	matched := make([]workersessions.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		if matchesFilter(session, req.Filter) {
			matched = append(matched, cloneSession(session))
		}
	}
	r.mu.RUnlock()

	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	r.logger.Info("worker session list", "outcome", "success", "filter_state_count", len(req.Filter.States), "result_count", len(matched))
	return workersessions.ListResult{Sessions: matched}, nil
}

func matchesFilter(session workersessions.Session, filter workersessions.Filter) bool {
	if len(filter.States) == 0 {
		return true
	}
	return slices.Contains(filter.States, session.State)
}

// Start validates req, then establishes or reuses one stable Worker Session
// identity, observably persisted in StateReserved before any Workers call,
// transitions StateStarting before handoff, and hands a detached clone of
// req.Execution to the one injected workers.WorkstationExecutionService so
// the caller retains exclusive ownership of req.Execution's reference-backed
// fields. Start is synchronous: it blocks until the attempt commits its
// exactly-once absorbing terminal outcome, classified from the Workers
// WorkResult first and the adapter error second, and returns the committed
// detached snapshot.
func (r *registry) Start(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	attemptID := req.Execution.Execution.Dispatch.DispatchID

	if err := req.Validate(); err != nil {
		r.logger.Info("worker session start rejected", "sessionID", req.ID, "attemptID", attemptID, "outcome", "invalid")
		return workersessions.StartResult{}, err
	}

	r.reserveIfAbsent(req.ID)
	if _, err := r.transitionToStarting(req.ID); err != nil {
		r.logger.Info("worker session start rejected", "sessionID", req.ID, "attemptID", attemptID, "outcome", "not_startable")
		return workersessions.StartResult{}, err
	}
	r.logger.Info("worker session start", "sessionID", req.ID, "attemptID", attemptID, "outcome", "handoff", "state", string(workersessions.StateStarting))

	handoff := workers.WorkstationDispatchRequest{
		WorkstationName: req.Execution.WorkstationName,
		Execution:       workers.CloneWorkstationExecutionRequest(req.Execution.Execution),
	}
	dispatchResult, dispatchErr := r.execution.DispatchWorkstation(ctx, handoff)
	terminal := classifyTerminal(dispatchErr, dispatchResult)

	finalState := workersessions.StateFailed
	if terminal.Outcome == workersessions.TerminalOutcomeCompleted {
		finalState = workersessions.StateCompleted
	}

	final, committed := r.commitTerminal(req.ID, finalState, terminal)
	if committed {
		r.logger.Info(
			"worker session start terminal",
			"sessionID", req.ID,
			"attemptID", attemptID,
			"outcome", string(finalState),
			"state", string(finalState),
			"cause", causeKindString(terminal.Cause),
		)
	}

	return workersessions.StartResult{Session: final}, nil
}

// reserveIfAbsent stores id as a new StateReserved session when it is not
// already registered, in its own locked critical section distinct from
// transitionToStarting. This makes a brand-new identity's RESERVED state a
// genuine, observable map write (visible to a concurrent Get/List) before
// Start ever transitions it to StateStarting or calls Workers. An identity
// already registered, in any state, is left untouched here; conflicts are
// reported by the following transitionToStarting call.
func (r *registry) reserveIfAbsent(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[id]; exists {
		return
	}
	r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateReserved}
}

// transitionToStarting atomically moves id from StateReserved to
// StateStarting. Only one caller can win this transition for a given id: a
// concurrent Start racing to claim the same newly reserved or already
// reserved identity, or an identity in any other state, sees
// ErrSessionNotStartable and makes no mutation and no Workers call.
func (r *registry) transitionToStarting(id string) (workersessions.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateReserved {
		return workersessions.Session{}, workersessions.ErrSessionNotStartable
	}

	session.State = workersessions.StateStarting
	session.Result = nil
	r.sessions[id] = session
	return cloneSession(session), nil
}

// commitTerminal stores the exactly-once terminal outcome for id and reports
// whether this call is the one that committed it. The commit requires the
// one allowed W2 predecessor state, StateStarting: a missing identity, an
// already-terminal identity (for example because a duplicate or racing
// callback reaches commitTerminal for the same identity), or any other
// nonterminal state (StateReserved, StatePaused) is left completely
// unchanged and returned as-is, and committed reports false. This makes the
// terminal write itself absorbing regardless of how many callers reach
// commitTerminal for one identity, and prevents it from fabricating a
// terminal outcome for an identity that never actually reached handoff.
// Only the caller for which committed is true may emit the terminal
// effect/log for this identity.
func (r *registry) commitTerminal(id string, state workersessions.State, result workersessions.TerminalResult) (workersessions.Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.sessions[id]
	if !exists || existing.State != workersessions.StateStarting {
		return cloneSession(existing), false
	}

	session := existing
	session.State = state
	session.Result = cloneTerminalResult(&result)
	r.sessions[id] = session
	return cloneSession(session), true
}

// cloneSession returns a detached copy of session: mutating the returned
// value, or its Result, never affects registry-owned state.
func cloneSession(session workersessions.Session) workersessions.Session {
	session.Result = cloneTerminalResult(session.Result)
	return session
}

func cloneTerminalResult(result *workersessions.TerminalResult) *workersessions.TerminalResult {
	if result == nil {
		return nil
	}
	clone := *result
	if result.Cause != nil {
		cause := *result.Cause
		clone.Cause = &cause
	}
	return &clone
}

func causeKindString(cause *workersessions.FailureCause) string {
	if cause == nil {
		return ""
	}
	return string(cause.Kind)
}
