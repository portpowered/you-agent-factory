package workers

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

// RuntimeOpeningRequest contains only Worker execution selection for one
// opened Factory Session runtime.
type RuntimeOpeningRequest struct {
	RunnerID                          string
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
// private to the Workers implementation.
type RuntimeService interface {
	Service

	Close(context.Context) error
	WithCommandRunners(CommandRunner, CommandRunner) (RuntimeService, error)
	WithProgressPublisher(
		CommandRunner,
		ProgressPublisher,
		bool,
		logging.Logger,
	) (RuntimeService, error)
	ProviderCommandInjected() bool
}
