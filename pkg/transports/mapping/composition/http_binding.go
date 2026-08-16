package composition

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// HTTPBinding contains the representation-only roles bound to one opened
// Factory Session. The binding is immutable and contains no constructors or
// application service lookup.
type HTTPBinding struct {
	Runtime            apisurface.RuntimeAPI
	FactoryStatus      apisurface.FactoryStatusAPI
	Sessions           apisurface.LiveSessionAPI
	Invocation         apisurface.InvocationAPI
	FactoryDefinitions apisurface.FactorySaveAPI
	Durable            apisurface.DurableSessionAPI
}

// HTTPBinder is the stable mapping operation constructed by Wire. Bind only
// attaches one opened Factory Session's exact service-root roles to the
// process-scoped representation operations.
type HTTPBinder struct {
	statusProjector factoryruntime.FactoryStatusProjector
	content         work.ContentPreparation
}

type runtimeHTTPSubmitter interface {
	SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error)
}

type runtimeHTTPEventSubscriber interface {
	SubscribeFactoryEvents(
		context.Context,
		*factorydefinitions.FactoryEventReconnectCursor,
		factorydefinitions.FactoryEventReconnectScope,
	) (*factorydefinitions.FactoryEventStream, error)
}

func NewHTTPBinder(
	statusProjector factoryruntime.FactoryStatusProjector,
	content work.ContentPreparation,
) (*HTTPBinder, error) {
	if statusProjector == nil || content == nil {
		return nil, fmt.Errorf("construct HTTP mapping binder: Factory Runtime status projection and Work content preparation are required")
	}
	return &HTTPBinder{statusProjector: statusProjector, content: content}, nil
}

func (binder *HTTPBinder) Bind(
	runtime factoryruntime.Service,
	definitions factorydefinitions.Service,
	sessions factorysessions.Service,
	liveControl factorysessions.LiveControlService,
) (HTTPBinding, error) {
	if runtime == nil || definitions == nil || sessions == nil || liveControl == nil ||
		binder == nil || binder.content == nil {
		return HTTPBinding{}, fmt.Errorf("bind HTTP mappings: opened Factory Session roles are required")
	}
	if _, ok := runtime.(runtimeHTTPSubmitter); !ok {
		return HTTPBinding{}, fmt.Errorf("bind HTTP mappings: Factory Runtime work submission is required")
	}
	if _, ok := runtime.(runtimeHTTPEventSubscriber); !ok {
		// TODO(P5B): bind event reads from Recordings and remove this
		// compatibility capability check from the HTTP mapping boundary.
		return HTTPBinding{}, fmt.Errorf("bind HTTP mappings: Factory Runtime event subscription is required")
	}
	var durableExecution factorysessionmapping.DurableExecution = sessions
	durable := NewDurableAPI(durableExecution)
	return HTTPBinding{
		Runtime:            NewRuntimeAPI(runtime, definitions),
		FactoryStatus:      newFactoryStatusAPI(runtime, sessions),
		Sessions:           NewLiveSessionAPI(liveControl, sessions),
		Invocation:         NewInvocationAPI(sessions),
		FactoryDefinitions: NewFactoryDefinitionAPI(definitions),
		Durable:            durable,
	}, nil
}
