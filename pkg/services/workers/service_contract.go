package workers

import (
	"context"
	"errors"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

// Service is the singular Workers root contract. Cross-service peers depend on
// this interface for Workers authority for the published runtime-build,
// workstation-dispatch, and Runner-neutral slices.
//
// Additive CTR-WRK slices publish those operations on this same Service using
// plain Workers-owned request, result, value, and typed-error contracts rather
// than concrete types from service/, construction/, executor/, or similar
// implementation packages. Nested IMP-WRK runner/subservice moves, provider/*
// migrations, Wire/root, CLI-manifest, CTR-PROV, and OpenAPI package-motion
// changes remain out of scope for the root-contract packet.
//
// Existing model invocation remains on this root so peers keep one Workers
// authority surface. Approved peer root contracts (Models, Work) may appear in
// signatures where the aggregate already requires them.
type Service interface {
	// InvokeModel executes one model operation through the configured Worker
	// path. Request and result use Models-owned plain contracts rather than
	// Workers implementation structs.
	InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error)

	// BuildRuntime is the published runtime-build slice. Peers supply a plain
	// RuntimeBuildRequest covering runner selection, mock/worker opening
	// options, and role assembly inputs, and receive a detached
	// RuntimeBuildResult with immutable assembled bindings, or a typed
	// Workers failure such as ErrInvalidRuntimeBuildRequest,
	// ErrMissingRunnerSelection, ErrUnknownRunnerSelection,
	// ErrRuntimeAssemblyRejected, or ErrIncompleteRuntimeAssembly. Callers do
	// not supply Factory Session implementation types, provider factory
	// constructors, or Petri/JavaScript internals on the request shape.
	BuildRuntime(context.Context, RuntimeBuildRequest) (RuntimeBuildResult, error)

	// DispatchWorkstation is the published workstation-dispatch slice. Peers
	// supply a plain WorkstationDispatchRequest covering workstation
	// execution identity and resolved runner selection facts, and receive a
	// detached WorkstationDispatchResult with execution/work outcomes, or a
	// typed Workers failure such as ErrInvalidWorkstationDispatchRequest,
	// ErrWorkstationDispatchRoutingRejected, ErrWorkstationDispatchCancelled,
	// ErrWorkstationDispatchSaturated, or ErrIncompleteWorkstationDispatch.
	// The request stays free of provider-native command/decode/session types
	// and concrete provider identity switches.
	DispatchWorkstation(context.Context, WorkstationDispatchRequest) (WorkstationDispatchResult, error)
}

// ModelInvoker is the narrow Workers role that exposes only direct model
// invocation. Prefer Service for the singular cross-service Workers seam;
// ModelInvoker remains available for callers that intentionally bind only this
// capability.
type ModelInvoker interface {
	InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error)
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
// peers may supply when assembling immutable execution bindings. It stays
// free of Factory Session implementation types and provider factory
// constructors.
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
	RunnerID string
	Opening  RuntimeBuildOpeningOptions
	Roles    []RuntimeBuildRoleRequest
}

// AssembledRuntimeBinding is one detached immutable role/binding fact peers
// can consume without importing Workers construction or executor packages.
type AssembledRuntimeBinding struct {
	RoleName        string
	RoleKind        RuntimeBuildRoleKind
	RunnerSelection ResolvedRunnerSelection
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

// ErrRuntimeAssemblyRejected reports that Workers rejected the supplied
// assembly-shaped input.
var ErrRuntimeAssemblyRejected = errors.New("Workers runtime assembly rejected")

// ErrIncompleteRuntimeAssembly reports that Workers could not complete assembly
// from the supplied runtime-build request.
var ErrIncompleteRuntimeAssembly = errors.New("Workers runtime assembly incomplete")

// WorkstationDispatchRequest is the plain Workers-owned workstation-dispatch
// input covering workstation execution identity and resolved runner selection
// facts peers already consume. It stays free of provider-native
// command/decode/session types and concrete provider identity switches.
type WorkstationDispatchRequest struct {
	DispatchID      string
	TransitionID    string
	WorkstationName string
	WorkerType      string
	RunnerSelection ResolvedRunnerSelection
}

// WorkstationDispatchResult carries detached execution/work success facts for
// one workstation-dispatch operation peers can consume from Workers root types
// only.
type WorkstationDispatchResult struct {
	DispatchID      string
	TransitionID    string
	WorkstationName string
	RunnerSelection ResolvedRunnerSelection
	Outcome         WorkOutcome
	Output          string
	Error           string
}

// ErrInvalidWorkstationDispatchRequest reports a malformed or empty
// workstation-dispatch request peers can distinguish without parsing free-form
// dispatch details.
var ErrInvalidWorkstationDispatchRequest = errors.New("invalid Workers workstation-dispatch request")

// ErrWorkstationDispatchRoutingRejected reports that Workers rejected runner
// routing or selection for a workstation-dispatch request.
var ErrWorkstationDispatchRoutingRejected = errors.New("Workers workstation-dispatch routing rejected")

// ErrWorkstationDispatchCancelled reports that a workstation-dispatch was
// cancelled before a detached execution result was produced.
var ErrWorkstationDispatchCancelled = errors.New("Workers workstation-dispatch cancelled")

// ErrWorkstationDispatchSaturated reports that Workers could not accept a
// workstation-dispatch because dispatch capacity was saturated.
var ErrWorkstationDispatchSaturated = errors.New("Workers workstation-dispatch saturated")

// ErrIncompleteWorkstationDispatch reports that Workers could not complete
// workstation dispatch from the supplied request.
var ErrIncompleteWorkstationDispatch = errors.New("Workers workstation dispatch incomplete")
