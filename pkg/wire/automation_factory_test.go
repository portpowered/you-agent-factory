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

func TestProvideAutomationsServiceConstructsThroughAutomationsWire(t *testing.T) {
	t.Parallel()

	ports, err := factorydefinitionswire.InvocationPolicyPortsFromNestedOwner()
	if err != nil {
		t.Fatalf("InvocationPolicyPortsFromNestedOwner() error = %v", err)
	}

	service, err := provideAutomationsService(
		serviceedges.Edges{},
		zap.NewNop(),
		platformclock.Real{},
		automationFactoryCommandRunner{},
		ports.WorkstationExecution,
	)
	if service == nil {
		t.Fatal("provideAutomationsService() returned nil service")
	}
	var published automations.Service = service
	if published == nil {
		t.Fatal("constructed service is not assignable to automations.Service")
	}
}
