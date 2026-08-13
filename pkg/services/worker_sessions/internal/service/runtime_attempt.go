package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Compile-time proof that the production registry exposes the optional
// Runtime-owned durable opening bridge without widening the Worker Sessions
// root Service contract.
var _ workersessions.RuntimeAttemptService = (*registry)(nil)

type runtimeAttempt struct {
	registry   *registry
	workerID   string
	dispatchID string
	attemptID  string
	once       sync.Once
}

// BeginRuntimeAttempt opens the Worker Session observation and recording
// window, then returns control to Factory Runtime. It intentionally stops
// before registerInvocationSupervision or any Workers boundary call: Runtime
// has already admitted the detached attempt and remains responsible for its
// execution, cancellation, and terminal race.
func (r *registry) BeginRuntimeAttempt(
	ctx context.Context,
	req workersessions.RuntimeAttemptRequest,
) (workersessions.RuntimeAttempt, error) {
	if r == nil {
		return nil, workersessions.ErrStartAdmissionFailed
	}
	if err := (workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
	}).Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logicalDispatchID := strings.TrimSpace(req.Execution.Execution.Dispatch.DispatchID)
	attemptID := strings.TrimSpace(req.AttemptID)
	if attemptID == "" {
		attemptID = logicalDispatchID
	}

	// A dispatch may be observed under one stable Worker Session owner only.
	// Check before opening so a conflict cannot leave a durable opening record
	// with no Runtime handle capable of closing it.
	r.mu.RLock()
	ownerID, owned := r.dispatchOwners[logicalDispatchID]
	r.mu.RUnlock()
	if owned && ownerID != req.ID && attemptID == logicalDispatchID {
		return nil, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}

	// Lifecycle records use the physical attempt identity while provider
	// observations continue to use the logical dispatch identity. Clone the
	// nested request before replacing that one identity so caller-owned request
	// data remains detached at the service boundary.
	execution := req.Execution
	execution.Execution = workers.CloneWorkstationExecutionRequest(req.Execution.Execution)
	execution.Execution.Dispatch.DispatchID = attemptID
	prepared, err := r.prepareInvocation(
		context.WithoutCancel(ctx),
		workersessions.InvokeSessionRequest{ID: req.ID, Execution: execution},
		invocationPreparationOptions{runtimeOwned: true},
	)
	if err != nil {
		return nil, err
	}
	if prepared.terminal {
		if prepared.failure != nil {
			return nil, prepared.failure
		}
		return nil, workersessions.ErrStartAdmissionFailed
	}
	if !r.transitionToRunning(req.ID) {
		return nil, workersessions.ErrStartAdmissionFailed
	}

	r.mu.Lock()
	if r.runtimeAttempts == nil {
		r.runtimeAttempts = make(map[string]struct{})
	}
	if r.dispatchOwners == nil {
		r.dispatchOwners = make(map[string]string)
	}
	if ownerID, exists := r.dispatchOwners[logicalDispatchID]; exists && ownerID != req.ID && attemptID == logicalDispatchID {
		r.mu.Unlock()
		r.terminalizeInvocationBeforeAdmission(context.WithoutCancel(ctx), req.ID, attemptID)
		return nil, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}
	r.dispatchOwners[logicalDispatchID] = req.ID
	r.runtimeAttempts[req.ID] = struct{}{}
	r.mu.Unlock()

	return &runtimeAttempt{
		registry:   r,
		workerID:   req.ID,
		dispatchID: logicalDispatchID,
		attemptID:  attemptID,
	}, nil
}

// Complete commits the one terminal Worker Session observation. Runtime has
// already normalized cancellation and execution outcomes before invoking this
// hook; Worker Sessions only classifies and durably publishes the detached
// lifecycle result. The handle is idempotent because a late duplicate callback
// must not rewrite the terminal record or close recording twice.
func (a *runtimeAttempt) Complete(
	ctx context.Context,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) error {
	if a == nil || a.registry == nil {
		return errors.New("worker sessions: runtime attempt is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.once.Do(func() {
		r := a.registry
		r.associateProviderSessionFromResult(a.workerID, a.dispatchID, result)
		state, terminal := dispatchedTerminal("", result, dispatchErr)
		final, committed := r.commitTerminal(a.workerID, state, terminal)
		if committed {
			r.logTerminal(a.workerID, a.attemptID, final)
			r.publishTerminalRecordOrLog(
				context.WithoutCancel(ctx),
				a.workerID,
				a.attemptID,
				state,
				*final.Result,
			)
		}
		r.mu.Lock()
		delete(r.runtimeAttempts, a.workerID)
		r.mu.Unlock()
	})
	return nil
}
