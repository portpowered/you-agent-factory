package internal

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
	active       map[string]*runtimeActivationState
	failed       map[string]*runtimeActivationCleanupState
	activating   map[string]bool
	deactivating map[string]bool
}

var _ factoryruntime.Service = (*Root)(nil)

type runtimeActivationState struct {
	request factoryruntime.RuntimeActivationRequest
	service factoryruntime.Service
	view    factoryruntime.RuntimeActivationView
	close   func(context.Context) error
}

type runtimeActivationCleanupState struct {
	runtimeID string
	close     func(context.Context) error
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
		active:        make(map[string]*runtimeActivationState),
		failed:        make(map[string]*runtimeActivationCleanupState),
		activating:    make(map[string]bool),
		deactivating:  make(map[string]bool),
	}, nil
}

// Activate validates and atomically publishes one initialized Runtime. The
// activation operation is the only construction-time route to a live
// delegate; failed operations never become observable through this root. If
// failed-start cleanup also fails, the cleanup remains explicitly retryable
// through Deactivate for the same Runtime ID.
func (r *Root) Activate(
	ctx context.Context,
	request factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationResult, error) {
	if err := validateActivationContext(ctx); err != nil {
		return factoryruntime.RuntimeActivationResult{}, err
	}
	normalized, err := request.Normalize()
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
	r.mu.RLock()
	active := r.active[normalized.RuntimeID]
	if active == nil {
		r.mu.RUnlock()
		return factoryruntime.RuntimeActivationResult{}, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorFailed,
			RuntimeID: normalized.RuntimeID,
			Message:   "activate Factory Runtime: published state is unavailable",
		}
	}
	view := active.view
	r.mu.RUnlock()
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: normalized.RuntimeID,
		State:     factoryruntime.RuntimeLifecycleStateActive,
		Binding:   view.Binding,
		Runtime:   view,
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
	if active := r.active[request.RuntimeID]; active != nil {
		kind := factoryruntime.RuntimeActivationErrorConflict
		message := "activate Factory Runtime: Runtime identity is already active"
		if reflect.DeepEqual(active.request, request) {
			kind = factoryruntime.RuntimeActivationErrorAlreadyActive
			message = "activate Factory Runtime: Runtime is already active"
		}
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      kind,
			RuntimeID: request.RuntimeID,
			Message:   message,
		}
	}
	if _, ok := r.failed[request.RuntimeID]; ok {
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorConflict,
			RuntimeID: request.RuntimeID,
			Message:   "activate Factory Runtime: failed activation cleanup is pending",
		}
	}
	if r.activating[request.RuntimeID] || r.deactivating[request.RuntimeID] {
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
	r.activating[request.RuntimeID] = true
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
			r.retainFailedCleanup(request.RuntimeID, activation)
			operationErr = fmt.Errorf("%w; unwind activation: %v", operationErr, cleanupErr)
		}
		r.clearActivating(request.RuntimeID)
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
			r.retainFailedCleanup(request.RuntimeID, activation)
			operationErr = fmt.Errorf("activation returned no Runtime service; unwind activation: %w", cleanupErr)
		} else {
			operationErr = fmt.Errorf("activation returned no Runtime service")
		}
		r.clearActivating(request.RuntimeID)
		return &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorFailed,
			RuntimeID: request.RuntimeID,
			Message:   "activate Factory Runtime: initialization returned no Runtime service",
			Cause:     operationErr,
		}
	}
	bindingService := &boundRuntimeService{root: r, runtimeID: request.RuntimeID}
	binding := factoryruntime.NewRuntimeBinding(
		request.RuntimeID,
		bindingService,
		func(closeCtx context.Context) (factoryruntime.RuntimeDeactivationResult, error) {
			return r.deactivateRuntime(closeCtx, request.RuntimeID)
		},
	)
	r.mu.Lock()
	r.activating[request.RuntimeID] = false
	view := factoryruntime.RuntimeActivationView{
		RuntimeID:        request.RuntimeID,
		FactorySessionID: request.FactorySessionID,
		Binding:          binding,
		Service:          activation.Service,
	}
	r.active[request.RuntimeID] = &runtimeActivationState{
		request: request,
		service: activation.Service,
		view:    view,
		close:   activation.Close,
	}
	r.mu.Unlock()
	return nil
}

func (r *Root) retainFailedCleanup(
	runtimeID string,
	activation *factoryruntime.RuntimeActivation,
) {
	if r == nil || activation == nil || activation.Close == nil {
		return
	}
	r.mu.Lock()
	r.failed[runtimeID] = &runtimeActivationCleanupState{
		runtimeID: runtimeID,
		close:     activation.Close,
	}
	r.mu.Unlock()
}

func (r *Root) clearActivating(runtimeID string) {
	r.mu.Lock()
	delete(r.activating, runtimeID)
	r.mu.Unlock()
}

// Deactivate closes the active Runtime's owned resources before removing its
// delegate. A failed cleanup leaves the Runtime published so callers can retry
// cleanup without losing the only reference to the active state. It also
// retries cleanup retained from a failed activation that never became active.
func (r *Root) Deactivate(
	ctx context.Context,
	request factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	if !request.Binding.IsZero() {
		return request.Binding.Deactivate(ctx)
	}
	return r.deactivateRuntime(ctx, strings.TrimSpace(request.RuntimeID))
}

func (r *Root) deactivateRuntime(
	ctx context.Context,
	runtimeID string,
) (factoryruntime.RuntimeDeactivationResult, error) {
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
			r.abortDeactivation(runtimeID)
			return factoryruntime.RuntimeDeactivationResult{}, &factoryruntime.RuntimeActivationError{
				Kind:      factoryruntime.RuntimeActivationErrorDeactivationFailed,
				RuntimeID: runtimeID,
				Message:   "deactivate Factory Runtime: owned cleanup failed",
				Cause:     err,
			}
		}
	}
	r.completeDeactivation(runtimeID)
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
	if active, ok := r.active[runtimeID]; ok {
		if r.activating[runtimeID] || r.deactivating[runtimeID] {
			return nil, &factoryruntime.RuntimeActivationError{
				Kind:      factoryruntime.RuntimeActivationErrorConflict,
				RuntimeID: runtimeID,
				Message:   "deactivate Factory Runtime: lifecycle transition is already in progress",
			}
		}
		r.deactivating[runtimeID] = true
		return active.close, nil
	}
	if failed, ok := r.failed[runtimeID]; ok {
		if r.activating[runtimeID] || r.deactivating[runtimeID] {
			return nil, &factoryruntime.RuntimeActivationError{
				Kind:      factoryruntime.RuntimeActivationErrorConflict,
				RuntimeID: runtimeID,
				Message:   "deactivate Factory Runtime: lifecycle transition is already in progress",
			}
		}
		r.deactivating[runtimeID] = true
		return failed.close, nil
	}
	if r.activating[runtimeID] || r.deactivating[runtimeID] {
		return nil, &factoryruntime.RuntimeActivationError{
			Kind:      factoryruntime.RuntimeActivationErrorConflict,
			RuntimeID: runtimeID,
			Message:   "deactivate Factory Runtime: lifecycle transition is already in progress",
		}
	}
	return nil, &factoryruntime.RuntimeActivationError{
		Kind:      factoryruntime.RuntimeActivationErrorNotActive,
		RuntimeID: runtimeID,
		Message:   "deactivate Factory Runtime: Runtime is not active",
	}
}

func (r *Root) abortDeactivation(runtimeID string) {
	r.mu.Lock()
	delete(r.deactivating, runtimeID)
	r.mu.Unlock()
}

func (r *Root) completeDeactivation(runtimeID string) {
	r.mu.Lock()
	delete(r.active, runtimeID)
	delete(r.failed, runtimeID)
	delete(r.deactivating, runtimeID)
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

// SubmitWorkRequest preserves the migration-only Factory Sessions ingress
// while routing the request through the activated Runtime delegate. The
// process root remains the canonical Service authority; this narrow bridge is
// retained for the legacy HTTP mapping until that representation migrates.
func (r *Root) SubmitWorkRequest(
	ctx context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	service, ok := r.delegate().(factoryruntime.APIFactory)
	if !ok {
		return work.WorkRequestSubmitResult{}, factoryruntime.ErrNotRunning
	}
	return service.SubmitWorkRequest(ctx, request)
}

// SubscribeFactoryEvents preserves the migration-only Factory Sessions event
// stream through the activated Runtime delegate for the legacy HTTP mapping.
func (r *Root) SubscribeFactoryEvents(
	ctx context.Context,
	reconnect *interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	service, ok := r.delegate().(factoryruntime.APIFactory)
	if !ok {
		return nil, factoryruntime.ErrNotRunning
	}
	return service.SubscribeFactoryEvents(ctx, reconnect, scope)
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
	if len(r.active) != 1 {
		return nil
	}
	for _, active := range r.active {
		return active.service
	}
	return nil
}

func (r *Root) serviceForRuntime(runtimeID string) factoryruntime.Service {
	if r == nil || r.orchestration == nil || r.instanceHost == nil || r.dispatchPlan == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	active := r.active[runtimeID]
	if active == nil {
		return nil
	}
	return active.service
}

// boundRuntimeService is the detached capability returned in RuntimeBinding.
// It keeps the Runtime identity private and rechecks the process root before
// every operation, so a binding cannot continue to operate after its owner has
// been successfully deactivated.
type boundRuntimeService struct {
	root      *Root
	runtimeID string
}

var _ factoryruntime.Service = (*boundRuntimeService)(nil)

func (service *boundRuntimeService) target() factoryruntime.Service {
	if service == nil {
		return nil
	}
	return service.root.serviceForRuntime(service.runtimeID)
}

func (service *boundRuntimeService) ControlPause(ctx context.Context, req factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	if target := service.target(); target != nil {
		return target.ControlPause(ctx, req)
	}
	return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) ControlResume(ctx context.Context, req factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	if target := service.target(); target != nil {
		return target.ControlResume(ctx, req)
	}
	return factoryruntime.ResumeResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) ControlTerminate(ctx context.Context, req factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	if target := service.target(); target != nil {
		return target.ControlTerminate(ctx, req)
	}
	return factoryruntime.TerminateResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) ControlWaitToComplete(req factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	if target := service.target(); target != nil {
		return target.ControlWaitToComplete(req)
	}
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}

func (service *boundRuntimeService) ControlMoveWork(ctx context.Context, req factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	if target := service.target(); target != nil {
		return target.ControlMoveWork(ctx, req)
	}
	return factoryruntime.MoveWorkResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) Observe(ctx context.Context, req factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	if !validObservationScope(req.Scope) {
		return factoryruntime.ObserveResult{}, factoryruntime.ErrInvalidObservationScope
	}
	if target := service.target(); target != nil {
		return target.Observe(ctx, req)
	}
	return factoryruntime.ObserveResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) PlanDispatch(ctx context.Context, req factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	if target := service.target(); target != nil {
		return target.PlanDispatch(ctx, req)
	}
	return factoryruntime.PlanDispatchResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) AcceptDispatchResult(ctx context.Context, req factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	if req.CorrelationID == "" {
		return factoryruntime.AcceptDispatchResultResult{}, factoryruntime.ErrUnknownDispatchCorrelation
	}
	if target := service.target(); target != nil {
		return target.AcceptDispatchResult(ctx, req)
	}
	return factoryruntime.AcceptDispatchResultResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) InvokeWorker(ctx context.Context, req factoryruntime.InvokeWorkerRequest) (factoryruntime.InvokeWorkerResult, error) {
	if err := req.Validate(); err != nil {
		return factoryruntime.InvokeWorkerResult{}, err
	}
	if target := service.target(); target != nil {
		return target.InvokeWorker(ctx, req)
	}
	return factoryruntime.InvokeWorkerResult{}, factoryruntime.ErrNotRunning
}

func (service *boundRuntimeService) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	target, ok := service.target().(factoryruntime.APIFactory)
	if !ok {
		return work.WorkRequestSubmitResult{}, factoryruntime.ErrNotRunning
	}
	return target.SubmitWorkRequest(ctx, request)
}

func (service *boundRuntimeService) SubscribeFactoryEvents(
	ctx context.Context,
	reconnect *interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	target, ok := service.target().(factoryruntime.APIFactory)
	if !ok {
		return nil, factoryruntime.ErrNotRunning
	}
	return target.SubscribeFactoryEvents(ctx, reconnect, scope)
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
