// Package runners defines the Workers-private registry for common Runner
// implementations. Peer services consume only the Workers root service.
package runners

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
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

// ScriptConfig is the private registry construction input for one configured
// Script Runner. The implementation translates it into its own immutable state.
type ScriptConfig struct {
	Command          string
	Args             []string
	FactoryDirectory string
}

// ScriptDependencies are the exact effects projected into one Script Runner.
type ScriptDependencies struct {
	CommandRunner workers.CommandRunner
	FactoryDocs   workers.FactoryDocsLoader
	Now           func() time.Time
	Publish       workers.ProgressPublisher
	Record        workers.ScriptEventRecorder
}

// InferenceConfig is the private registry construction input for one configured
// Inference Runner. The implementation translates it into its own immutable state.
type InferenceConfig struct {
	Worker    models.LocalWorker
	Resources []models.LocalResource
	Scope     models.RuntimeScopeRef
}

// InferenceDependencies are the exact effects projected into one Inference Runner.
type InferenceDependencies struct {
	Models   inference.LocalInvoker
	Delegate workers.Runner
}

// AgentDependencies are the exact peer-service and observation capabilities
// projected into one Agent Runner.
type AgentDependencies struct {
	Providers providers.Service
	Publish   workers.ProgressPublisher
}

// MockConfig is the private registry construction input for one configured
// mock Runner. Production Workers wire must not register this strategy.
type MockConfig struct {
	WorkersConfig *workers.MockWorkersConfig
}

// MockDependencies are optional effects for mock script execution. Omitting
// Next confines mock accept/reject behavior to the Workers testing feature path.
type MockDependencies struct {
	Next workers.CommandRunner
}

// Service owns the immutable process-scoped runner registry and request-scoped
// strategy dispatch. Resolve performs selection only; Execute runs one attempt.
type Service interface {
	Resolve(ResolutionRequest) (Binding, error)
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
}
