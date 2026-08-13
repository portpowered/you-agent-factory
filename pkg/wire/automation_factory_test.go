package wire

import (
	"context"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type automationFactoryCommandRunner struct{}

func (automationFactoryCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func TestProvideAutomationsRootConstructsThroughAutomationsWire(t *testing.T) {
	t.Parallel()

	ports, err := factorydefinitionswire.InvocationPolicyPortsFromNestedOwner()
	if err != nil {
		t.Fatalf("InvocationPolicyPortsFromNestedOwner() error = %v", err)
	}

	hostedSourceInputs, err := provideAutomationHostedSourceInputs(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideAutomationHostedSourceInputs() error = %v", err)
	}
	root, err := provideAutomationsRoot(
		hostedSourceInputs,
		zap.NewNop(),
		platformclock.Real{},
		automationFactoryCommandRunner{},
		ports.WorkstationExecution,
	)
	if err != nil {
		t.Fatalf("provideAutomationsRoot() error = %v", err)
	}
	var published automations.Service = root
	if published == nil {
		t.Fatal("constructed root is not assignable to automations.Service")
	}
	if root.Runtime == nil {
		t.Fatal("constructed root has no runtime capability")
	}
}
