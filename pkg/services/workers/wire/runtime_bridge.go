package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	modelrecording "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution/recording"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	runnermockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
)

// NewMockCommandRunner decorates a command edge with configured mock behavior.
func NewMockCommandRunner(
	config *workers.MockWorkersConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	next platformprocess.CommandRunner,
	files workers.AgentToolFileSystem,
) platformprocess.CommandRunner {
	return workerprocess.ProjectPlatformCommandRunner(&runnermockworker.MockWorkerCommandRunner{
		Config:        config,
		RuntimeConfig: runtimeConfig,
		Next:          workerprocess.AdaptPlatformCommandRunner(next),
		Files:         files,
	})
}

// LocalRuntimeHooks returns Workers-owned recording hooks for the Models runtime.
func LocalRuntimeHooks() workers.LocalRuntimeHooks {
	return modelrecording.Hooks()
}

// ResolveTemplateFields exposes the Workers-owned template resolver for composition.
func ResolveTemplateFields(
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	workflowContext *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	return workersinternal.ResolveTemplateFields(
		workingDirectory,
		environment,
		tokens,
		workflowContext,
		worktree,
	)
}
