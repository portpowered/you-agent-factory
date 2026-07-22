package apisurface

import (
	"context"
	"fmt"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// CurrentFactoryReader supplies the durable current Factory definition.
type CurrentFactoryReader interface {
	GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error)
}

// runtimeAPI is the compatibility unscoped runtime view backed by the selected
// canonical Factory Session runtime.
type runtimeAPI struct {
	runtime     factory.APIFactory
	definitions CurrentFactoryReader
}

var _ RuntimeAPI = (*runtimeAPI)(nil)
var _ factory.APIFactory = (*runtimeAPI)(nil)

// NewRuntimeAPI composes the legacy unscoped API from canonical services.
func NewRuntimeAPI(runtime factory.APIFactory, definitions CurrentFactoryReader) RuntimeAPI {
	return &runtimeAPI{runtime: runtime, definitions: definitions}
}

func (a *runtimeAPI) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (result work.WorkRequestSubmitResult, err error) {
	if a == nil || a.runtime == nil {
		return result, factorysessions.ErrRuntimeNotAvailable
	}
	return a.runtime.SubmitWorkRequest(ctx, request)
}

func (a *runtimeAPI) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (stream *interfaces.FactoryEventStream, err error) {
	if a == nil || a.runtime == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	stream, err = a.runtime.SubscribeFactoryEvents(ctx, reconnect, scope)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

func (a *runtimeAPI) GetEngineStateSnapshot(ctx context.Context) (snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net], err error) {
	if a == nil || a.runtime == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	snapshot, err = a.runtime.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snapshot, nil
}

func (a *runtimeAPI) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	if a == nil || a.definitions == nil {
		return factoryapi.Factory{}, fmt.Errorf("Factory Definition service is required")
	}
	return a.definitions.GetCurrentNamedFactory(ctx)
}
