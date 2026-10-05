// Package wire constructs completed Factory Runtime owners without activating them.
package wire

import (
	"context"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Construction aliases keep implementation ownership private to Runtime.
type Orchestration = orchestration.Service
type InstanceHost = instancehost.Service
type DispatchPlanning = dispatchplanning.Service
type Lifecycle = factoryhost.LifecycleService

// WorkersPublisher is the publication edge selected at construction.
type WorkersPublisher func(context.Context, workers.WorkstationDispatchRequest) error

// WorkersCanceler is the cancellation edge selected at construction.
type WorkersCanceler func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)

// NewService retains completed collaborators and starts no lifecycle activity.
func NewService(orchestration Orchestration, instanceHost InstanceHost, dispatchPlan DispatchPlanning) (factoryruntime.Root, error) {
	return factoryruntimeinternal.NewRoot(orchestration, instanceHost, dispatchPlan)
}

// NewOrchestration constructs the inert orchestration owner.
func NewOrchestration(mapper *DefinitionMapper, workflows factoryruntime.JavaScriptWorkflowDefinitions, runtime factoryruntime.JavaScriptWorkflowRuntime) Orchestration {
	return orchestrationwire.New(mapper, workflows, runtime)
}

// NewLifecycle constructs the completed lifecycle sequencer.
func NewLifecycle(clock factoryruntime.Clock, scheduler platformclock.TimerSource) (*Lifecycle, error) {
	return factoryhost.NewLifecycleService(clock, scheduler)
}

// NewInstanceHost constructs the single keyed handle authority.
func NewInstanceHost(clock factoryruntime.Clock, scheduler platformclock.TimerSource, lifecycle *Lifecycle) (InstanceHost, error) {
	return instancehostwire.New(clock, scheduler, lifecycle)
}

// NewRuntimeStopOperation exposes the keyed host's clock-aware stop capability.
func NewRuntimeStopOperation(host InstanceHost) factoryruntime.RuntimeStopOperation {
	return host.StopWithClock
}

// NewDispatchPlanning constructs the inert dispatch planner.
func NewDispatchPlanning(publisher WorkersPublisher, canceler WorkersCanceler) DispatchPlanning {
	return dispatchplanningwire.New(dispatchplanning.WorkersPublisher(publisher), dispatchplanning.WorkersCanceler(canceler))
}
