package wire

import (
	"context"
	"fmt"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

// AutomationsRootFromEdges constructs the published Automations Root through the
// same AutomationFactory wiring used by InjectBundle / root.BuildProcess.
func AutomationsRootFromEdges(
	edges serviceedges.Edges,
	workflowID string,
	defaultFactoryDir string,
) (automations.Root, error) {
	hostedSourcesFactory, err := provideAutomationHostedSourcesFactory(edges)
	if err != nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: %w", err)
	}
	hostedClock := edges.HostedClock
	if hostedClock == nil {
		hostedClock = clockwork.NewRealClock()
	}
	hostedPollers := hostedSourcesFactory(
		zap.NewNop(),
		hostedClock,
		edges.HostedHTTPClient,
		edges.HostedSecretResolver,
		edges.HostedLinearEndpoint,
	)
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

	service := provideAutomationFactory(edges, ports.WorkstationExecution)(
		zap.NewNop(),
		platformclock.Real{},
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
	)
	if service == nil {
		return automations.Root{}, fmt.Errorf("compose Automations root: automation factory returned nil service")
	}
	peer, ok := service.(automationsRootPeer)
	if !ok {
		return automations.Root{}, fmt.Errorf("compose Automations root: service does not expose Root()")
	}
	return peer.Root(), nil
}
