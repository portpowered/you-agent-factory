package wire_test

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
)

type wireSourceStub struct{}

func (wireSourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return &factorydefinitions.FactoryEventStream{
		Events: make(chan factorydefinitions.FactoryEvent),
	}, nil
}

func (wireSourceStub) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return &factoryruntime.StateSnapshot{}, nil
}

type wireProjectionStub struct{}

func (wireProjectionStub) ReconstructFactoryWorldState(
	[]factorydefinitions.FactoryEvent,
	int,
) (factorydefinitions.FactoryWorldState, error) {
	return factorydefinitions.FactoryWorldState{}, nil
}

func (wireProjectionStub) SimpleDashboardRenderData(
	factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (wireProjectionStub) ProjectActiveThrottlePauses(
	factorydefinitions.InitialStructurePayload,
	[]factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

type wireClock struct{}

func (wireClock) Now() time.Time { return time.Unix(1, 0) }

type wireSink struct{}

func (wireSink) PresentFactoryView(activationlifecycle.View) {}

func TestNewServiceConstructsActivationLifecycleOwner(t *testing.T) {
	t.Parallel()

	service, err := activationlifecyclewire.NewService(
		wireSourceStub{},
		wireProjectionStub{},
		wireClock{},
		wireSink{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil")
	}
	var _ activationlifecycle.Service = service
}
