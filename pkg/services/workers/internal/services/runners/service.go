// Package runners defines the Workers-private registry for common Runner
// implementations. Peer services consume only the Workers root service.
package runners

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	ScriptIdentity    = "script"
	InferenceIdentity = "inference"
	AgentIdentity     = "agent"
	MockIdentity      = "mock"
)

// Strategy is the private common runner contract owned by this subservice.
// Transitional workstation and runtime_assembly code may still name the
// workers.Runner alias; new production execution must enter through Service.Execute.
type Strategy = workers.Runner

// AttemptRequest is one request-scoped strategy input. Mutable prompt,
// environment, process, provider, model, and Worktree facts belong here and
// must not be retained by the registry after Execute returns.
type AttemptRequest = workers.RunnerExecutionRequest

// AttemptResult is the detached strategy outcome of one Execute call.
type AttemptResult = workers.RunnerExecutionResult

// Registration explicitly associates one canonical identity and metadata
// snapshot with its private Strategy implementation.
type Registration struct {
	Identity string
	Metadata workers.RunnerMetadata
	Runner   Strategy
}

// ResolutionRequest carries the explicit selection and optional capabilities
// one Workers-private consumer requires. Resolve never executes or probes a
// strategy implementation.
type ResolutionRequest struct {
	Identity             string
	RequiredCapabilities []workers.RunnerOptionalCapability
}

// Binding is one resolved registry entry. Metadata collections are detached
// from registry state on every resolution. Runner remains available for
// transitional runtime_assembly consumers; Workers root must use Service.Execute.
type Binding struct {
	Identity string
	Metadata workers.RunnerMetadata
	Runner   Strategy
}

// ExecuteRequest selects one immutable registration and carries the
// request-scoped attempt input for a single strategy call.
type ExecuteRequest struct {
	Identity             string
	RequiredCapabilities []workers.RunnerOptionalCapability
	Attempt              AttemptRequest
}

// ExecuteResult is the detached outcome of one private strategy attempt.
type ExecuteResult = AttemptResult

// Service owns the immutable process-scoped runner registry and request-scoped
// strategy dispatch. Resolve performs selection only; Execute runs one attempt.
type Service interface {
	Resolve(ResolutionRequest) (Binding, error)
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
}
