// Package runners defines the Workers-private registry for common Runner
// implementations. Peer services consume only the Workers root service.
package runners

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	ScriptIdentity    = "script"
	InferenceIdentity = "inference"
)

// Registration explicitly associates one canonical identity and metadata
// snapshot with its common Workers Runner implementation.
type Registration struct {
	Identity string
	Metadata workers.RunnerMetadata
	Runner   workers.Runner
}

// ResolutionRequest carries the explicit selection and optional capabilities
// one Workers-private consumer requires.
type ResolutionRequest struct {
	Identity             string
	RequiredCapabilities []workers.RunnerOptionalCapability
}

// Binding is one resolved registry entry. Metadata collections are detached
// from registry state on every resolution.
type Binding struct {
	Identity string
	Metadata workers.RunnerMetadata
	Runner   workers.Runner
}

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

// InferenceLocalInvoker is the Models-root local invocation edge required by
// one Inference Runner registration.
type InferenceLocalInvoker interface {
	InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error)
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
	Models   InferenceLocalInvoker
	Delegate workers.Runner
}

// Service resolves immutable runner registrations without executing or
// probing their implementations.
type Service interface {
	Resolve(ResolutionRequest) (Binding, error)
}
