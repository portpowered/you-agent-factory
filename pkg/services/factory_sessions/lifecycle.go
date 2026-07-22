package factorysessions

import (
	"context"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
)

// RuntimeStop releases one Factory Session runtime sidecar group.
type RuntimeStop = func(context.Context) error

// LifecycleRuntime is the Factory Sessions lifecycle surface consumed by the
// process initializer. It hides the concrete session runtime and registry.
type LifecycleRuntime interface {
	StartLifecycle(context.Context, context.Context) error
	StartWorkerLifecycle(context.Context) (RuntimeStop, error)
	CompleteStartup(context.Context) error
	WaitForRuntime(context.Context) error
	StopLifecycle(context.Context) error
	FailStartup(error) error
	CurrentRuntimeBundle() factory.HostedInstance
}

// ProcessRuntime is the exact state-free Factory Session lifecycle component
// consumed by the Factory Sessions-owned process lifecycle plan. The Factory
// Sessions owner retains the active runtime handle and owns product activation,
// cancellation, worker-stop, and unwind policy.
type ProcessRuntime interface {
	Start(context.Context, context.Context) error
	StartWorkers(context.Context) (RuntimeStop, error)
	RunTransport(context.Context, http.Handler) error
	Stop(context.Context) error
}

// BoundProcessComponents are protocol and presentation components adapted at
// the composition boundary. Their product lifecycle order is deliberately not
// represented here; LifecyclePlanOperation owns that policy.
type BoundProcessComponents struct {
	Transport     lifecycle.Component
	Visualization lifecycle.Component
}

// LifecyclePlanRequest contains the exact opened Factory Session roles needed
// to declare one process lifecycle transaction.
type LifecyclePlanRequest struct {
	Runtime    ProcessRuntime
	Components BoundProcessComponents
	Close      func() error
}

// LifecyclePlanOperation returns the neutral plan consumed by Initializer's
// generic lifecycle manager. This is the sole owner of process component
// ordering and runtime/worker unwind policy.
type LifecyclePlanOperation func(LifecyclePlanRequest) (lifecycle.Plan, error)

// ProcessRuntimeFactory binds one dynamically opened Factory Session runtime
// to the process-scoped lifecycle factory constructed by Wire.
type ProcessRuntimeFactory interface {
	Bind(LifecycleRuntime, RuntimeHostRequest, RuntimeHostObserver, *zap.Logger) (ProcessRuntime, error)
}

// RuntimeHostRequest contains the process-hosting values needed after a
// Factory Session runtime has started. It deliberately excludes the broader
// process configuration and edge aggregates.
type RuntimeHostRequest struct {
	Directory   string
	RuntimeMode interfaces.RuntimeMode
	WorkFile    string
	MockWorkers bool
	Port        int
	AutoPort    bool
}

// RuntimeHostBinding is the endpoint selected by the external HTTP host.
type RuntimeHostBinding struct {
	Port int
}

// RuntimeHostObserver receives the selected endpoint for presentation at the
// process boundary. It does not own listener or server lifecycle.
type RuntimeHostObserver func(RuntimeHostBinding)

// RuntimeHostOperation owns the externally hosted transport phase of a
// started Factory Session. Initializer invokes this operation as the primary
// blocking lifecycle component; it does not implement hosting policy.
type RuntimeHostOperation interface {
	Run(
		context.Context,
		http.Handler,
		LifecycleRuntime,
		*zap.Logger,
		RuntimeHostRequest,
		RuntimeHostObserver,
	) error
}
