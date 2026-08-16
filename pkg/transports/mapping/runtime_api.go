package apisurface

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// CurrentFactoryReader supplies the durable current Factory definition.
type CurrentFactoryReader interface {
	GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error)
}

type runtimeWorkSubmitter interface {
	SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error)
}

// runtimeEventSubscriber is a P5B compatibility capability. Canonical event
// reads will move to Recordings; mapping code must not depend on APIFactory as a
// broad runtime host contract while that migration is pending.
type runtimeEventSubscriber interface {
	SubscribeFactoryEvents(
		context.Context,
		*interfaces.FactoryEventReconnectCursor,
		interfaces.FactoryEventReconnectScope,
	) (*interfaces.FactoryEventStream, error)
}

// runtimeAPI is the compatibility unscoped runtime view backed by the selected
// canonical Factory Session runtime.
type runtimeAPI struct {
	runtime     factoryruntime.Service
	submitter   runtimeWorkSubmitter
	events      runtimeEventSubscriber
	definitions CurrentFactoryReader
}

var _ RuntimeAPI = (*runtimeAPI)(nil)

// NewRuntimeAPI composes the compatibility unscoped API from canonical
// services. The runtime identity is supplied as the opaque-root Service; the
// two narrow capabilities are retained only for routes awaiting the Work and
// Recordings migrations.
func NewRuntimeAPI(runtime factoryruntime.Service, definitions CurrentFactoryReader) RuntimeAPI {
	var submitter runtimeWorkSubmitter
	var events runtimeEventSubscriber
	if runtime != nil {
		submitter, _ = runtime.(runtimeWorkSubmitter)
		events, _ = runtime.(runtimeEventSubscriber)
	}
	return &runtimeAPI{runtime: runtime, submitter: submitter, events: events, definitions: definitions}
}

func (a *runtimeAPI) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (result work.WorkRequestSubmitResult, err error) {
	if a == nil || a.runtime == nil || a.submitter == nil {
		return result, factorysessions.ErrRuntimeNotAvailable
	}
	return a.submitter.SubmitWorkRequest(ctx, request)
}

func (a *runtimeAPI) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (stream *interfaces.FactoryEventStream, err error) {
	if a == nil || a.runtime == nil || a.events == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	stream, err = a.events.SubscribeFactoryEvents(ctx, reconnect, scope)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

func (a *runtimeAPI) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	if a == nil || a.definitions == nil {
		return factoryapi.Factory{}, fmt.Errorf("Factory Definition service is required")
	}
	return a.definitions.GetCurrentNamedFactory(ctx)
}
