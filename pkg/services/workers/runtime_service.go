package workers

import (
	"context"
)

// RuntimeOpeningRequest contains only Worker execution selection for one
// opened Factory Session runtime.
type RuntimeOpeningRequest struct {
	RunnerID                          string
	Worktree                          string
	WorkerReasoningEffort             string
	MockWorkers                       *MockWorkersConfig
	InvocationSkipPermissionsOverride *bool
	SkipBuiltInPrerequisiteValidation bool
}

// ScriptEventRecorder receives one Worker-owned script execution event.
type ScriptEventRecorder func(ScriptEvent)

// InferenceEventRecorder receives one Worker-owned provider inference event.
type InferenceEventRecorder func(InferenceEvent)

// ModelEventRecorder receives one Worker-owned model execution event.
type ModelEventRecorder func(ModelEvent)

// AgentRunEventRecorder receives one Worker-owned agent-run event.
type AgentRunEventRecorder func(AgentRunResponseEvent)

// RuntimeService is the Worker service role used while constructing a Factory
// Runtime. Provider factories, executor builders, and runner decorators remain
// private to the Workers implementation. The provider command runner, script
// command runner, and progress publisher are explicit, immutable construction
// dependencies for the runtime's lifetime; there is no supported operation
// that replaces them after construction.
type RuntimeService interface {
	Service

	Close(context.Context) error
}

// SessionBuildFactory constructs an independent Workers runtime for one
// Factory Session build, reaching the same canonical Workers construction
// boundary used to build the Factory Session's own runtime. A nil argument
// preserves the provider command runner, script command runner, or progress
// publisher the Factory Session's own runtime was constructed with; a
// non-nil argument supplies the final per-build value explicitly.
// SessionBuildFactory never derives configuration by reading an already-built
// RuntimeService's dependencies.
type SessionBuildFactory func(
	providerCommandRunner CommandRunner,
	scriptCommandRunner CommandRunner,
	progressPublisher ProgressPublisher,
) (RuntimeService, error)
