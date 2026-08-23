package wire

import (
	"context"
	"fmt"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"go.uber.org/zap"
)

type noopAutomationCommandRunner struct{}

func provideAutomationsCommandRunner(
	edges serviceedges.Edges,
) (platformprocess.CommandRunner, error) {
	if edges.ScriptCommandRunner != nil {
		return edges.ScriptCommandRunner, nil
	}
	return providePlatformProcessCommandRunner(edges)
}

func (noopAutomationCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

// AutomationsRootFromEdges constructs the published Automations Root through
// the same process-owned Automations wiring used by InjectBundle.
func AutomationsRootFromEdges(
	edges serviceedges.Edges,
	workflowID string,
	defaultFactoryDir string,
) (automations.Root, error) {
	hostedSourceInputs, err := provideAutomationHostedSourceInputs(edges)
	if err != nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: %w", err)
	}

	commandRunner := platformprocess.CommandRunner(noopAutomationCommandRunner{})
	if edges.ScriptCommandRunner != nil {
		commandRunner = edges.ScriptCommandRunner
	}

	ports, err := factorydefinitionswire.InvocationPolicyPortsFromNestedOwner()
	if err != nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: %w", err)
	}

	root, err := automationswire.NewRoot(
		zap.NewNop(),
		platformclock.Real{},
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedSourceInputs,
		workerswire.ResolveTemplateFields,
		ports.WorkstationExecution,
	)
	if err != nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: construct root: %w", err)
	}
	if root.Operations == nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: constructed root has no operations")
	}
	return root, nil
}
