package apisurface_test

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type runtimeAPIFactory struct {
	factory.Service
}

func (f *runtimeAPIFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{}, nil
}

type currentFactoryReader struct{ value factoryapi.Factory }

func (r currentFactoryReader) GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error) {
	return r.value, nil
}

func TestRuntimeAPIComposesFactoryRuntimeEventsAndDefinition(t *testing.T) {
	runtimeFactory := &runtimeAPIFactory{}
	wantFactory := factoryapi.Factory{Name: "current"}
	api := apisurface.NewRuntimeAPI(runtimeFactory, currentFactoryReader{value: wantFactory})

	if _, err := api.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{}); err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if got, err := api.GetCurrentFactory(context.Background()); err != nil || got.Name != wantFactory.Name {
		t.Fatalf("GetCurrentFactory = (%q, %v)", got.Name, err)
	}
}
