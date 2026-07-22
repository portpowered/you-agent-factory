package composition

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

func NewWorkAPI(workService work.Service, sessions factorysessions.Service) apisurface.WorkAPI {
	return workAPI{work: workService, sessions: sessions}
}

type workAPI struct {
	work     work.Service
	sessions factorysessions.Service
}

func (a workAPI) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return a.work.SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (a workAPI) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	return a.work.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (a workAPI) ListWork(ctx context.Context, sessionID string, options work.ListOptions) (work.ListResult, error) {
	return a.work.ListWork(ctx, sessionID, options)
}

func (a workAPI) GetWork(ctx context.Context, sessionID, id string) (work.ReadModel, error) {
	return a.work.GetWork(ctx, sessionID, id)
}

func (a workAPI) MoveWorkAndRead(ctx context.Context, sessionID, id, stateName, requestID string) (work.ReadModel, error) {
	return a.work.MoveWorkAndRead(ctx, sessionID, id, stateName, requestID)
}

func (a workAPI) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return a.sessions.SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (a workAPI) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) error {
	return a.sessions.ProbeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (a workAPI) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*factoryruntime.StateSnapshot, error) {
	return a.sessions.GetEngineStateSnapshotForSession(ctx, sessionID)
}

func NewFactoryDefinitionAPI(service factorydefinitions.Service) apisurface.FactorySaveAPI {
	definitions := factorydefinitionmapping.New(service)
	return factorydefinitionmapping.NewAPI(definitions, definitions)
}

func NewInvocationAPI(invocations factorysessions.SessionInvoker) apisurface.InvocationAPI {
	return factorysessionmapping.NewInvocationAPI(invocations)
}

func NewDurableAPI(
	execution factorysessions.ExecutionService,
	sessions factorysessions.Service,
) apisurface.DurableSessionAPI {
	return factorysessionmapping.NewDurableAPI(
		execution,
		sessions,
	)
}
