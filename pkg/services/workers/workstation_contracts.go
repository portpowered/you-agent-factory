package workers

import (
	"context"
	"errors"
	"fmt"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

// ModelInvoker executes one model operation through the configured Worker
// path.
type ModelInvoker interface {
	InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error)
}

// WorkstationPoolLifecycleOutcome describes an idempotent workstation-pool
// lifecycle transition.
type WorkstationPoolLifecycleOutcome string

// WorkstationDispatchTerminalOutcome classifies the one terminal result
// committed for an accepted workstation dispatch.
type WorkstationDispatchTerminalOutcome string

// WorkstationDispatchCancelOutcome describes an idempotent explicit
// cancellation request.
type WorkstationDispatchCancelOutcome string

const (
	WorkstationPoolLifecycleOutcomeStarted        WorkstationPoolLifecycleOutcome = "STARTED"
	WorkstationPoolLifecycleOutcomeAlreadyRunning WorkstationPoolLifecycleOutcome = "ALREADY_RUNNING"
	WorkstationPoolLifecycleOutcomeStopped        WorkstationPoolLifecycleOutcome = "STOPPED"
	WorkstationPoolLifecycleOutcomeAlreadyStopped WorkstationPoolLifecycleOutcome = "ALREADY_STOPPED"

	WorkstationDispatchTerminalOutcomeCompleted WorkstationDispatchTerminalOutcome = "COMPLETED"
	WorkstationDispatchTerminalOutcomeFailed    WorkstationDispatchTerminalOutcome = "FAILED"
	WorkstationDispatchTerminalOutcomeCanceled  WorkstationDispatchTerminalOutcome = "CANCELED"

	WorkstationDispatchCancelOutcomeCanceled        WorkstationDispatchCancelOutcome = "CANCELED"
	WorkstationDispatchCancelOutcomeAlreadyCanceled WorkstationDispatchCancelOutcome = "ALREADY_CANCELED"
	WorkstationDispatchCancelOutcomeAlreadyTerminal WorkstationDispatchCancelOutcome = "ALREADY_TERMINAL"

	// DefaultWorkstationCapacity preserves bounded behavior for bindings that
	// predate explicit workstation admission limits.
	DefaultWorkstationCapacity = 1
	// DefaultWorkstationQueueCapacity bounds waiting work for legacy bindings.
	DefaultWorkstationQueueCapacity = 1
)

var (
	// ErrInvalidWorkstationPoolStart reports malformed workstation bindings.
	ErrInvalidWorkstationPoolStart = errors.New("invalid Workers workstation-pool start request")
	// ErrInvalidWorkstationDispatch reports a malformed workstation dispatch.
	ErrInvalidWorkstationDispatch = errors.New("invalid Workers workstation dispatch request")
	// ErrWorkstationPoolUnavailable reports a pool that has not been started.
	ErrWorkstationPoolUnavailable = errors.New("Workers workstation pool is unavailable")
	// ErrWorkstationPoolStopped reports a terminal pool that cannot be restarted.
	ErrWorkstationPoolStopped = errors.New("Workers workstation pool is stopped")
	// ErrUnknownWorkstationRoute reports a route outside the started snapshot.
	ErrUnknownWorkstationRoute = errors.New("Workers workstation route is unknown")
	// ErrMissingWorkstationBinding reports a configured route without an executor.
	ErrMissingWorkstationBinding = errors.New("Workers workstation executor binding is missing")
	// ErrWorkstationSaturated reports a route whose running and waiting capacity
	// are both occupied.
	ErrWorkstationSaturated = errors.New("Workers workstation route is saturated")
	// ErrInvalidWorkstationCancellation reports a cancellation without an ID.
	ErrInvalidWorkstationCancellation = errors.New("invalid Workers workstation cancellation request")
	// ErrUnknownWorkstationDispatch reports cancellation for an unaccepted ID.
	ErrUnknownWorkstationDispatch = errors.New("Workers workstation dispatch is unknown")
	// ErrWorkstationDispatchCanceled classifies the canonical cancelled terminal
	// result from caller, explicit, or pool-stop cancellation.
	ErrWorkstationDispatchCanceled = errors.New("Workers workstation dispatch is canceled")
	// ErrWorkstationDispatchAlreadyTerminal reports late cancellation after a
	// non-cancelled terminal result was committed.
	ErrWorkstationDispatchAlreadyTerminal = errors.New("Workers workstation dispatch is already terminal")
)

// WorkstationPoolStartRequest supplies the detached runtime bindings that are
// available to the workstation pool. Only workstation bindings are accepted.
type WorkstationPoolStartRequest struct {
	Bindings []AssembledRuntimeBinding
}

// WorkstationPoolStartResult reports whether start activated the pool or
// observed an already-running pool.
type WorkstationPoolStartResult struct {
	Outcome WorkstationPoolLifecycleOutcome
}

// WorkstationPoolStopResult reports whether stop performed the terminal
// transition or observed an already-stopped pool.
type WorkstationPoolStopResult struct {
	Outcome WorkstationPoolLifecycleOutcome
}

// WorkstationRouteRequest names a configured workstation route.
type WorkstationRouteRequest struct {
	WorkstationName string
}

// WorkstationRouteResult reports availability in the active route snapshot.
type WorkstationRouteResult struct {
	WorkstationName string
	Available       bool
}

// WorkstationDispatchRequest carries one detached execution request to the
// executor binding assembled for the named workstation.
type WorkstationDispatchRequest struct {
	WorkstationName string
	Execution       WorkstationExecutionRequest
}

// WorkstationDispatchResult attributes one executor result to its original
// dispatch and workstation route.
type WorkstationDispatchResult struct {
	DispatchID      string
	WorkstationName string
	TerminalOutcome WorkstationDispatchTerminalOutcome
	Result          WorkResult
}

// WorkstationDispatchCancelRequest identifies one accepted dispatch.
type WorkstationDispatchCancelRequest struct {
	DispatchID string
}

// WorkstationDispatchCancelResult reports whether cancellation was newly
// committed, already committed, or arrived after another terminal result.
type WorkstationDispatchCancelResult struct {
	DispatchID string
	Outcome    WorkstationDispatchCancelOutcome
}

// RuntimeBuildRoleKind identifies the kind of role peers ask Workers to
// assemble during a runtime build.
type RuntimeBuildRoleKind string

const (
	// RuntimeBuildRoleKindWorker assembles one worker-role binding.
	RuntimeBuildRoleKindWorker RuntimeBuildRoleKind = "worker"
	// RuntimeBuildRoleKindWorkstation assembles one workstation-role binding.
	RuntimeBuildRoleKindWorkstation RuntimeBuildRoleKind = "workstation"
)

// RuntimeBuildOpeningOptions carries Workers-owned opening selection facts
// peers may supply when assembling immutable execution bindings.
type RuntimeBuildOpeningOptions struct {
	MockWorkers                       *MockWorkersConfig
	InvocationSkipPermissionsOverride *bool
	SkipBuiltInPrerequisiteValidation bool
}

// RuntimeBuildRoleRequest names one role peers want assembled into a detached
// runtime binding.
type RuntimeBuildRoleRequest struct {
	Name string
	Kind RuntimeBuildRoleKind
}

// RuntimeBuildRequest is the plain Workers-owned runtime-build input covering
// execution selection and role-assembly facts peers need.
type RuntimeBuildRequest struct {
	RunnerID                   string
	RequiredRunnerCapabilities []RunnerOptionalCapability
	Opening                    RuntimeBuildOpeningOptions
	Roles                      []RuntimeBuildRoleRequest
}

// AssembledRuntimeBinding is one detached immutable role/binding fact peers
// can consume without importing Workers construction or executor packages.
type AssembledRuntimeBinding struct {
	RoleName        string
	RoleKind        RuntimeBuildRoleKind
	RunnerSelection ResolvedRunnerSelection
	Executor        WorkstationRequestExecutor
	// Capacity is the maximum concurrent executor calls for this workstation.
	// Zero selects DefaultWorkstationCapacity for compatibility.
	Capacity int
	// QueueCapacity is the maximum accepted dispatches waiting for a slot.
	// Zero selects DefaultWorkstationQueueCapacity for compatibility.
	QueueCapacity int
}

// RuntimeBuildResult carries detached assembled-binding success facts for one
// runtime-build operation.
type RuntimeBuildResult struct {
	RunnerSelection ResolvedRunnerSelection
	Bindings        []AssembledRuntimeBinding
}

// ErrInvalidRuntimeBuildRequest reports a malformed or empty runtime-build
// request peers can distinguish without parsing free-form construction details.
var ErrInvalidRuntimeBuildRequest = errors.New("invalid Workers runtime-build request")

// ErrMissingRunnerSelection reports that a runtime-build request omitted the
// runner selection peers must supply.
var ErrMissingRunnerSelection = errors.New("Workers runtime-build missing runner selection")

// ErrUnknownRunnerSelection reports that a runtime-build request named a runner
// identity Workers does not recognize.
var ErrUnknownRunnerSelection = errors.New("Workers runtime-build unknown runner selection")

// ErrUnsupportedRunnerCapability reports that a selected runner cannot satisfy
// one explicitly required optional capability.
var ErrUnsupportedRunnerCapability = errors.New("Workers runner capability unsupported")

// UnsupportedRunnerCapabilityError carries detached, customer-safe selection
// context for one unsupported required capability.
type UnsupportedRunnerCapabilityError struct {
	RunnerID   string
	Capability RunnerOptionalCapability
}

func (e *UnsupportedRunnerCapabilityError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"runner %q does not support capability %q",
		e.RunnerID,
		e.Capability,
	)
}

func (e *UnsupportedRunnerCapabilityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return ErrUnsupportedRunnerCapability
}

// ErrInvalidRunnerRegistration reports that a private Workers runner
// registration contains malformed identity, metadata, capabilities, or a nil
// implementation.
var ErrInvalidRunnerRegistration = errors.New("invalid Workers runner registration")

// ErrConflictingRunnerRegistration reports that a registration's explicit
// identity disagrees with its metadata identity.
var ErrConflictingRunnerRegistration = errors.New("conflicting Workers runner registration")

// ErrDuplicateRunnerRegistration reports that registry construction received
// more than one registration for the same canonical runner identity.
var ErrDuplicateRunnerRegistration = errors.New("duplicate Workers runner registration")

// ErrRuntimeAssemblyRejected reports that Workers rejected the supplied
// assembly-shaped input.
var ErrRuntimeAssemblyRejected = errors.New("Workers runtime assembly rejected")

// ErrIncompleteRuntimeAssembly reports that Workers could not complete assembly
// from the supplied runtime-build request.
var ErrIncompleteRuntimeAssembly = errors.New("Workers runtime assembly incomplete")

// Service is the aggregate customer-facing Worker execution boundary.
// Provider factories, command runners, and workstation builders remain
// implementation details or explicit Worker subservices.
type Service interface {
	ModelInvoker

	// BuildRuntime assembles detached execution bindings from explicit
	// Workers-owned inputs.
	BuildRuntime(context.Context, RuntimeBuildRequest) (RuntimeBuildResult, error)
	// StartWorkstationPool activates one immutable workstation-route snapshot.
	StartWorkstationPool(context.Context, WorkstationPoolStartRequest) (WorkstationPoolStartResult, error)
	// StopWorkstationPool permanently stops workstation admission and activity.
	StopWorkstationPool(context.Context) (WorkstationPoolStopResult, error)
	// WorkstationRoute reports whether a route belongs to the active snapshot.
	WorkstationRoute(context.Context, WorkstationRouteRequest) (WorkstationRouteResult, error)
	// DispatchWorkstation executes through the binding for the requested route.
	DispatchWorkstation(context.Context, WorkstationDispatchRequest) (WorkstationDispatchResult, error)
	// CancelWorkstationDispatch cancels queued or running workstation work.
	CancelWorkstationDispatch(context.Context, WorkstationDispatchCancelRequest) (WorkstationDispatchCancelResult, error)
}
