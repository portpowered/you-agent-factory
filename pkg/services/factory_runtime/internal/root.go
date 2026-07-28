package internal

import (
	"context"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
)

// Root retains process-scoped Factory Runtime dependencies. It is inert until a
// hosted runtime binds an active factory.Service delegate.
type Root struct {
	orchestration orchestration.Service
	instanceHost  instancehost.Service
	dispatchPlan  dispatchplanning.Service
	active        factoryruntime.Service
}

var _ factoryruntime.Service = (*Root)(nil)

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
	return &Root{
		orchestration: orchestrationwire.New(newID, workflows, workflowRuntime),
		instanceHost:  instanceHost,
		dispatchPlan:  dispatchplanningwire.New(workersPublisher, workersCanceler),
	}, nil
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

func (r *Root) CaptureCheckpoint(
	ctx context.Context,
	req factoryruntime.CaptureCheckpointRequest,
) (factoryruntime.CaptureCheckpointResult, error) {
	if strings.TrimSpace(req.CheckpointID) == "" {
		return factoryruntime.CaptureCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
	}
	if r != nil && r.active != nil {
		return r.active.CaptureCheckpoint(ctx, req)
	}
	return factoryruntime.CaptureCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
}

func (r *Root) LoadCheckpoint(ctx context.Context, req factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	if strings.TrimSpace(req.CheckpointID) == "" {
		return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
	}
	if r != nil && r.active != nil {
		return r.active.LoadCheckpoint(ctx, req)
	}
	return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
}

func (r *Root) RestoreCheckpoint(
	ctx context.Context,
	req factoryruntime.RestoreCheckpointRequest,
) (factoryruntime.RestoreCheckpointResult, error) {
	if strings.TrimSpace(req.Checkpoint.CheckpointID) == "" {
		return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
	}
	if r != nil && r.active != nil {
		return r.active.RestoreCheckpoint(ctx, req)
	}
	switch {
	case req.Checkpoint.SchemaVersion <= 0 || len(req.Checkpoint.Payload) == 0:
		return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrCorruptCheckpoint
	case req.Checkpoint.SchemaVersion != 1:
		return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrIncompatibleCheckpoint
	default:
		return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
	}
}

// BindActiveService attaches the hosted runtime delegate that serves published
// control, observation, and dispatch-plan operations on the wire-constructed
// root.
func (r *Root) BindActiveService(service factoryruntime.Service) {
	if r == nil {
		return
	}
	r.active = service
}

func (r *Root) delegate() factoryruntime.Service {
	if r == nil || r.orchestration == nil || r.instanceHost == nil || r.dispatchPlan == nil {
		return nil
	}
	return r.active
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
