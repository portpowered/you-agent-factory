package composition

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func NewRuntimeAPI(
	runtime factoryruntime.APIFactory,
	definitions factorydefinitions.Service,
) apisurface.RuntimeAPI {
	return apisurface.NewRuntimeAPI(
		runtime,
		factorydefinitionmapping.New(definitions),
	)
}

func NewLiveSessionAPI(
	sessions factorysessions.Service,
) apisurface.LiveSessionAPI {
	return factorysessionmapping.NewLiveAPI(sessions)
}

func NewFactoryDefinitionAPI(service factorydefinitions.Service) apisurface.FactorySaveAPI {
	definitions := factorydefinitionmapping.New(service)
	return factorydefinitionmapping.NewAPI(definitions, definitions)
}

func NewInvocationAPI(invocations factorysessionmapping.SessionInvoker) apisurface.InvocationAPI {
	return factorysessionmapping.NewInvocationAPI(invocations)
}

func NewDurableAPI(
	execution factorysessionmapping.DurableExecution,
	sessions factorysessions.Service,
) apisurface.DurableSessionAPI {
	return factorysessionmapping.NewDurableAPI(
		execution,
		sessions,
	)
}
