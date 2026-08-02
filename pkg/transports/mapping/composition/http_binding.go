package composition

import (
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
	invocations factorysessionmapping.SessionInvoker,
	execution factorysessions.ExecutionService,
) (HTTPBinding, error) {
	if runtime == nil || definitions == nil || sessions == nil || invocations == nil ||
		execution == nil ||
		binder == nil || binder.content == nil {
		return HTTPBinding{}, fmt.Errorf("bind HTTP mappings: opened Factory Session roles are required")
	}
	legacyObservation, ok := runtime.(factoryruntime.APIFactory)
	if !ok {
		return HTTPBinding{}, fmt.Errorf("bind HTTP mappings: legacy Factory Runtime observation is required")
	}
	durable := NewDurableAPI(execution, sessions)
	return HTTPBinding{
		Runtime:            NewRuntimeAPI(legacyObservation, definitions),
		FactoryStatus:      newFactoryStatusAPI(runtime, sessions),
		Sessions:           NewLiveSessionAPI(sessions),
		Invocation:         NewInvocationAPI(invocations),
		FactoryDefinitions: NewFactoryDefinitionAPI(definitions),
		Durable:            durable,
	}, nil
}
