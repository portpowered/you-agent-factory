package wire

import (
	"context"
	"fmt"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"go.uber.org/zap"
)

type automationsRootPeer interface {
	Root() automations.Root
}

type noopAutomationCommandRunner struct{}

func (noopAutomationCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

// AutomationsRootFromEdges constructs the published Automations Root through
// the same process-owned Automations wiring used by InjectBundle.
func AutomationsRootFromEdges(
	edges serviceedges.Edges,
	workflowID string,
	defaultFactoryDir string,
) (automations.Root, error) {
	hostedPollers, err := provideAutomationHostedPollers(edges, zap.NewNop())
	if err != nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: %w", err)
	}
	if hostedPollers == nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: hosted sources returned nil pollers")
	}

	commandRunner := workers.CommandRunner(noopAutomationCommandRunner{})
	if edges.ScriptCommandRunner != nil {
		commandRunner = workers.AdaptCommandRunner(edges.ScriptCommandRunner)
	}

	ports, err := factorydefinitionswire.InvocationPolicyPortsFromNestedOwner()
	if err != nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: %w", err)
	}

	service, err := automationswire.NewService(
		zap.NewNop(),
		platformclock.Real{},
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		workerswire.ResolveTemplateFields,
		ports.WorkstationExecution,
	)
	if err != nil || service == nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: construct service: %w", err)
	}
	peer, ok := service.(automationsRootPeer)
	if !ok {
		return automations.Root{}, fmt.Errorf("compose Automations root: service does not expose Root()")
	}
	return peer.Root(), nil
}
