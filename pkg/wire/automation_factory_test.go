package wire

import (
	"context"
	"testing"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
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

func TestProvideAutomationFactoryConstructsThroughAutomationsWire(t *testing.T) {
	t.Parallel()

	store, err := automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}
	hostedSources, err := provideAutomationHostedSourcesFactory(serviceedges.Edges{
		HostedLinearCheckpointStore: store,
	})
	if err != nil {
		t.Fatalf("provideAutomationHostedSourcesFactory() error = %v", err)
	}
	hostedPollers := hostedSources(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		nil,
		"",
	)

	invocationPolicy, err := factorydefinitionswire.NewInvocationPolicy()
	if err != nil {
		t.Fatalf("NewInvocationPolicy() error = %v", err)
	}

	factory := provideAutomationFactory(serviceedges.Edges{}, invocationPolicy)
	service := factory(
		zap.NewNop(),
		platformclock.Real{},
		automationFactoryCommandRunner{},
		"wire-automation-factory",
		t.TempDir(),
		hostedPollers,
	)
	if service == nil {
		t.Fatal("provideAutomationFactory() returned nil service")
	}
	var published automations.Service = service
	if published == nil {
		t.Fatal("constructed service is not assignable to automations.Service")
	}
}
