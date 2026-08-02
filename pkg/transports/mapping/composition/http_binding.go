package composition

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
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
) (HTTPBinding, error) {
	if runtime == nil || definitions == nil || sessions == nil ||
		binder == nil || binder.content == nil {
		return HTTPBinding{}, fmt.Errorf("bind HTTP mappings: opened Factory Session roles are required")
	}
	legacyObservation, ok := runtime.(factoryruntime.APIFactory)
	if !ok {
		return HTTPBinding{}, fmt.Errorf("bind HTTP mappings: legacy Factory Runtime observation is required")
	}
	durable := NewDurableAPI(sessions, sessions)
	return HTTPBinding{
		Runtime:            NewRuntimeAPI(legacyObservation, definitions),
		FactoryStatus:      newFactoryStatusAPI(runtime, sessions),
		Sessions:           NewLiveSessionAPI(sessions),
		Invocation:         NewInvocationAPI(rootInvocationAdapter{root: sessions}),
		FactoryDefinitions: NewFactoryDefinitionAPI(definitions),
		Durable:            durable,
	}, nil
}

type rootInvocationAdapter struct{ root factorysessions.Service }

func (adapter rootInvocationAdapter) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	result, err := adapter.root.InvokeFactorySession(ctx, sessionID, request)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	return factorydefinitions.FactoryInvocationResult{
		RequestID: result.RequestID, TraceID: result.TraceID,
		Status:        factorydefinitions.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult, ErrorCode: result.ErrorCode,
		Message: result.Message, SessionID: result.SessionID, WorkID: result.WorkID,
		WorkName: result.WorkName, WorkState: result.WorkState,
	}, nil
}
