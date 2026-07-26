package workers

import (
	"context"
	"errors"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

// ModelInvoker executes one model operation through the configured Worker
// path.
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
}
