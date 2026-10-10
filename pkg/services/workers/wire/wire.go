// Package wire exposes focused, inert Workers construction providers.
package wire

import (
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/prompting"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
	worktree "github.com/portpowered/infinite-you/pkg/services/workers/internal/worktree"
)

type ExecuteCapability = workersinternal.ExecuteCapability
type Registry = runners.Service
type Harness = agentrun.HarnessAdapter
type ScriptConfig = runnerswire.ScriptRunnerConfig
type InferenceConfig = runnerswire.InferenceRunnerConfig

// NewService captures completed execution behavior without constructing children.
func NewService(execute ExecuteCapability) (workers.Service, error) {
	return workersinternal.NewRoot(execute)
}

var (
	NewWorktree               = worktree.New
	NewPlatformGitCommander   = worktree.NewPlatformGitCommander
	NewFactoryDocsLoader      = workerprompting.NewFactoryDocsLoader
	NewExecutor               = invocation.NewExecutor
	NewLibraryHarnessAdapter  = agentrun.NewLibraryHarnessAdapter
	NewExecute                = executeservice.NewWithProviderOverride
	NewAgentRunner            = runnerswire.NewAgentRunner
	NewInferenceRunner        = runnerswire.NewInferenceRunner
	NewProductionRegistry     = runnerswire.NewProductionRegistry
	NewMockProductionRegistry = runnerswire.NewMockProductionRegistry
)

// NewScriptRunner adapts the Platform command boundary to the private Script
// execution contract, preserving its actual streaming capability and config.
func NewScriptRunner(config ScriptConfig, command platformprocess.CommandRunner,
	docs workers.FactoryDocsLoader, now func() time.Time,
	publish workers.ProgressPublisher, record workers.ScriptEventRecorder,
) (workers.Runner, error) {
	if strings.TrimSpace(config.Command) == "" && !config.RequestSelected {
		return nil, workers.NewProviderError(workers.WorkFailureTypeMisconfigured, "script command is required", nil)
	}
	streaming, ok := workerprocess.AdaptPlatformCommandRunner(command).(workerprocess.StreamingCommandRunner)
	if !ok {
		return nil, workers.NewProviderError(workers.WorkFailureTypeMisconfigured, "script command runner must support streaming", nil)
	}
	return runnerswire.NewScriptRunner(config, streaming, docs, now, publish, record), nil
}
