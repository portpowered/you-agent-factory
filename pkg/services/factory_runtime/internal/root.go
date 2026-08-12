package internal

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
)

// Root retains process-scoped Factory Runtime dependencies. It is inert until
// an injected activation operation has initialized and published a complete
// Runtime delegate.
type Root struct {
	orchestration orchestration.Service
	instanceHost  instancehost.Service
	dispatchPlan  dispatchplanning.Service
	activation    factoryruntime.RuntimeActivationOperation

	mu           sync.RWMutex
	active       *runtimeActivationState
	activating   bool
	deactivating bool
}

var _ factoryruntime.Root = (*Root)(nil)

type runtimeActivationState struct {
	request factoryruntime.RuntimeActivationRequest
	service factoryruntime.Service
	close   func(context.Context) error
}

// NewRoot constructs the inert Factory Runtime root from construction ports. It
// composes accepted parent-private owners and starts no lifecycle, sidecars,
// Workers publication, or checkpoint recovery activity.
func NewRoot(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowRuntime factoryruntime.JavaScriptWorkflowRuntime,
	clock factoryruntime.Clock,
	workersPublisher dispatchplanning.WorkersPublisher,
	workersCanceler dispatchplanning.WorkersCanceler,
	activation ...factoryruntime.RuntimeActivationOperation,
) (*Root, error) {
	if newID == nil {
		return nil, fmt.Errorf("construct Factory Runtime: ID generator is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("construct Factory Runtime: clock is required")
	}
	if workersPublisher == nil {
		return nil, fmt.Errorf("construct Factory Runtime: Workers publisher is required")
	}
	instanceHost, err := instancehostwire.New(instancehost.Dependencies{Clock: clock})
	if err != nil {
		return nil, err
	}
	var activationOperation factoryruntime.RuntimeActivationOperation
	if len(activation) > 1 {
		return nil, fmt.Errorf("construct Factory Runtime: at most one activation operation is supported")
	}
	if len(activation) == 1 {
		activationOperation = activation[0]
	}
	return &Root{
		orchestration: orchestrationwire.New(newID, workflows, workflowRuntime),
		instanceHost:  instanceHost,
		dispatchPlan:  dispatchplanningwire.New(workersPublisher, workersCanceler),
		activation:    activationOperation,
	}, nil
}

// Activate validates and atomically publishes one initialized Runtime. The
// activation operation is the only construction-time route to a live
// delegate; failed operations never become observable through this root.
func (r *Root) Activate(
	ctx context.Context,
	request factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationResult, error) {
	if err := validateActivationContext(ctx); err != nil {
		return factoryruntime.RuntimeActivationResult{}, err
	}
	normalized, err := factoryruntime.NormalizeRuntimeActivationRequest(request)
	if err != nil {
		return factoryruntime.RuntimeActivationResult{}, err
	}
	if r == nil {
		return factoryruntime.RuntimeActivationResult{}, fmt.Errorf("activate Factory Runtime: root is required")
	}
	operation, err := r.beginActivation(normalized)
	if err != nil {
		return factoryruntime.RuntimeActivationResult{}, err
	}
	activation, operationErr := operation(ctx, normalized)
	if err := r.finishActivation(ctx, normalized, activation, operationErr); err != nil {
		return factoryruntime.RuntimeActivationResult{}, err
	}
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: normalized.RuntimeID,
		State:     factoryruntime.RuntimeLifecycleStateActive,
	}, nil
}

func validateActivationContext(ctx context.Context) error {
	if ctx == nil {
		return &factoryruntime.RuntimeActivationError{
			Kind:    factoryruntime.RuntimeActivationErrorMissingParameters,
			Message: "activate Factory Runtime: context is required",
		}
	}
	return ctx.Err()
}

func (r *Root) beginActivation(
	request factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationOperation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active != nil {
		kind := factoryruntime.RuntimeActivationErrorConflict
		message := "activate Factory Runtime: another Runtime is already active"
		if reflect.DeepEqual(r.active.request, request) {
			kind = factoryruntime.RuntimeActivationErrorAlreadyActive
			message = "activate Factory Runtime: Runtime is already active"
		}
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      kind,
			RuntimeID: request.RuntimeID,
			Message:   message,
		}
	}
	if r.activating || r.deactivating {
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorConflict,
			RuntimeID: request.RuntimeID,
			Message:   "activate Factory Runtime: lifecycle transition is already in progress",
		}
	}
	if r.activation == nil {
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorUnavailable,
			RuntimeID: request.RuntimeID,
			Message:   "activate Factory Runtime: activation operation is unavailable",
		}
	}
	r.activating = true
	return r.activation, nil
}

func (r *Root) finishActivation(
	ctx context.Context,
	request factoryruntime.RuntimeActivationRequest,
	activation *factoryruntime.RuntimeActivation,
	operationErr error,
) error {
	if operationErr != nil {
		cleanupErr := closeActivation(activation, ctx)
		if cleanupErr != nil {
			operationErr = fmt.Errorf("%w; unwind activation: %v", operationErr, cleanupErr)
		}
		r.clearActivating()
		return &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorFailed,
			RuntimeID: request.RuntimeID,
			Message:   "activate Factory Runtime: initialization failed",
			Cause:     operationErr,
		}
	}
	if activation == nil || activation.Service == nil {
		cleanupErr := closeActivation(activation, ctx)
		if cleanupErr != nil {
			operationErr = fmt.Errorf("activation returned no Runtime service; unwind activation: %w", cleanupErr)
		} else {
			operationErr = fmt.Errorf("activation returned no Runtime service")
		}
		r.clearActivating()
		return &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorFailed,
			RuntimeID: request.RuntimeID,
			Message:   "activate Factory Runtime: initialization returned no Runtime service",
			Cause:     operationErr,
		}
	}
	r.mu.Lock()
	r.activating = false
	r.active = &runtimeActivationState{
		request: request,
		service: activation.Service,
		close:   activation.Close,
	}
	r.mu.Unlock()
	return nil
}

func (r *Root) clearActivating() {
	r.mu.Lock()
	r.activating = false
	r.mu.Unlock()
}

// Deactivate closes the active Runtime's owned resources before removing its
// delegate. A failed cleanup leaves the Runtime published so callers can retry
// cleanup without losing the only reference to the active state.
func (r *Root) Deactivate(
	ctx context.Context,
	request factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	runtimeID := strings.TrimSpace(request.RuntimeID)
	if runtimeID == "" {
		return factoryruntime.RuntimeDeactivationResult{}, &factoryruntime.RuntimeActivationError{
			Kind:    factoryruntime.RuntimeActivationErrorMissingParameters,
			Message: "deactivate Factory Runtime: Runtime ID is required",
		}
	}
	if err := validateDeactivationContext(ctx, runtimeID); err != nil {
		return factoryruntime.RuntimeDeactivationResult{}, err
	}
	if r == nil {
		return factoryruntime.RuntimeDeactivationResult{}, fmt.Errorf("deactivate Factory Runtime: root is required")
	}

	closeOwnedResources, err := r.beginDeactivation(runtimeID)
	if err != nil {
		return factoryruntime.RuntimeDeactivationResult{}, err
	}
	if closeOwnedResources != nil {
		if err := closeOwnedResources(ctx); err != nil {
			r.abortDeactivation()
			return factoryruntime.RuntimeDeactivationResult{}, &factoryruntime.RuntimeActivationError{
				Kind:      factoryruntime.RuntimeActivationErrorDeactivationFailed,
				RuntimeID: runtimeID,
				Message:   "deactivate Factory Runtime: owned cleanup failed",
				Cause:     err,
			}
		}
	}
	r.completeDeactivation()
	return factoryruntime.RuntimeDeactivationResult{
		RuntimeID: runtimeID,
		State:     factoryruntime.RuntimeLifecycleStateStopped,
	}, nil
}

func validateDeactivationContext(ctx context.Context, runtimeID string) error {
	if ctx == nil {
		return &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorMissingParameters,
			RuntimeID: runtimeID,
			Message:   "deactivate Factory Runtime: context is required",
		}
	}
	return ctx.Err()
}

func (r *Root) beginDeactivation(runtimeID string) (func(context.Context) error, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil {
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorNotActive,
			RuntimeID: runtimeID,
			Message:   "deactivate Factory Runtime: Runtime is not active",
		}
	}
	if r.activating || r.deactivating {
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorConflict,
			RuntimeID: runtimeID,
			Message:   "deactivate Factory Runtime: lifecycle transition is already in progress",
		}
	}
	if r.active.request.RuntimeID != runtimeID {
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorConflict,
			RuntimeID: runtimeID,
			Message:   "deactivate Factory Runtime: Runtime ID does not match the active Runtime",
		}
	}
	r.deactivating = true
	return r.active.close, nil
}

func (r *Root) abortDeactivation() {
	r.mu.Lock()
	r.deactivating = false
	r.mu.Unlock()
}

func (r *Root) completeDeactivation() {
	r.mu.Lock()
	r.deactivating = false
	r.active = nil
	r.mu.Unlock()
}

func (r *Root) ControlPause(ctx context.Context, req factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	if service := r.delegate(); service != nil {
		return service.ControlPause(ctx, req)
	}
	return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
}

func (r *Root) ControlResume(ctx context.Context, req factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	if service := r.delegate(); service != nil {
		return service.ControlResume(ctx, req)
	}
	return factoryruntime.ResumeResult{}, factoryruntime.ErrNotRunning
}

func (r *Root) ControlTerminate(ctx context.Context, req factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	if service := r.delegate(); service != nil {
		return service.ControlTerminate(ctx, req)
	}
	return factoryruntime.TerminateResult{}, factoryruntime.ErrNotRunning
}

func (r *Root) ControlWaitToComplete(_ factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	if service := r.delegate(); service != nil {
		return service.ControlWaitToComplete(factoryruntime.WaitToCompleteRequest{})
	}
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}

func (r *Root) ControlMoveWork(ctx context.Context, req factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	if service := r.delegate(); service != nil {
		return service.ControlMoveWork(ctx, req)
	}
	return factoryruntime.MoveWorkResult{}, factoryruntime.ErrNotRunning
}

func (r *Root) Observe(ctx context.Context, req factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	if !validObservationScope(req.Scope) {
		return factoryruntime.ObserveResult{}, factoryruntime.ErrInvalidObservationScope
	}
	if service := r.delegate(); service != nil {
		return service.Observe(ctx, req)
	}
	return factoryruntime.ObserveResult{}, factoryruntime.ErrNotRunning
}

func (r *Root) PlanDispatch(ctx context.Context, req factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	if service := r.delegate(); service != nil {
		return service.PlanDispatch(ctx, req)
	}
	return factoryruntime.PlanDispatchResult{}, factoryruntime.ErrNotRunning
}

func (r *Root) AcceptDispatchResult(
	ctx context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) (factoryruntime.AcceptDispatchResultResult, error) {
	if req.CorrelationID == "" {
		return factoryruntime.AcceptDispatchResultResult{}, factoryruntime.ErrUnknownDispatchCorrelation
	}
	if service := r.delegate(); service != nil {
		return service.AcceptDispatchResult(ctx, req)
	}
	return factoryruntime.AcceptDispatchResultResult{}, factoryruntime.ErrNotRunning
}

// InvokeWorker delegates one orchestrator-resolved Worker invocation to the
// hosted runtime, which owns the Worker Sessions service and the canonical
// event ledger the invocation's association must land on.
func (r *Root) InvokeWorker(
	ctx context.Context,
	req factoryruntime.InvokeWorkerRequest,
) (factoryruntime.InvokeWorkerResult, error) {
	if err := req.Validate(); err != nil {
		return factoryruntime.InvokeWorkerResult{}, err
	}
	if service := r.delegate(); service != nil {
		return service.InvokeWorker(ctx, req)
	}
	return factoryruntime.InvokeWorkerResult{}, factoryruntime.ErrNotRunning
}

func (r *Root) delegate() factoryruntime.Service {
	if r == nil || r.orchestration == nil || r.instanceHost == nil || r.dispatchPlan == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.active == nil {
		return nil
	}
	return r.active.service
}

func closeActivation(activation *factoryruntime.RuntimeActivation, ctx context.Context) error {
	if activation == nil || activation.Close == nil {
		return nil
	}
	cleanupContext := context.WithoutCancel(ctx)
	return activation.Close(cleanupContext)
}

func validObservationScope(scope factoryruntime.ObservationScope) bool {
	switch scope {
	case "", factoryruntime.ObservationScopeFull, factoryruntime.ObservationScopeStatus, factoryruntime.ObservationScopeProgress,
		factoryruntime.ObservationScopeDispatches, factoryruntime.ObservationScopeResults, factoryruntime.ObservationScopeResources,
		factoryruntime.ObservationScopeHealth:
		return true
	default:
		return false
	}
}
