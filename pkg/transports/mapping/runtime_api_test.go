package apisurface_test

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type runtimeAPIFactory struct {
	factory.Factory
	submitted string
	snapshot  *interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net]
}

func (f *runtimeAPIFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	f.submitted = request.RequestID
	return work.WorkRequestSubmitResult{}, nil
}

func (f *runtimeAPIFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{}, nil
}

func (f *runtimeAPIFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net], error) {
	return f.snapshot, nil
}

type currentFactoryReader struct{ value factoryapi.Factory }

func (r currentFactoryReader) GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error) {
	return r.value, nil
}

func TestRuntimeAPIComposesFactoryRuntimeAndDefinition(t *testing.T) {
	runtimeFactory := &runtimeAPIFactory{snapshot: &interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net]{}}
	wantFactory := factoryapi.Factory{Name: "current"}
	api := apisurface.NewRuntimeAPI(runtimeFactory, currentFactoryReader{value: wantFactory})

	if _, err := api.SubmitWorkRequest(context.Background(), work.WorkRequest{RequestID: "request-1"}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if got, err := api.GetEngineStateSnapshot(context.Background()); err != nil || got != runtimeFactory.snapshot {
		t.Fatalf("GetEngineStateSnapshot = (%v, %v)", got, err)
	}
	if got, err := api.GetCurrentFactory(context.Background()); err != nil || got.Name != wantFactory.Name {
		t.Fatalf("GetCurrentFactory = (%q, %v)", got.Name, err)
	}
	if runtimeFactory.submitted != "request-1" {
		t.Fatalf("submitted request = %q", runtimeFactory.submitted)
	}
}
