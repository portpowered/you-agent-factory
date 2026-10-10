package apiserver_test

import (
	"context"
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionshttp "github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	recordingshttp "github.com/portpowered/infinite-you/pkg/transports/http/recordings"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

func newAPIServerFromRoles(
	runtime currentFactoryTestAPI,
	factoryStatus apisurface.FactoryStatusAPI,
	sessions apisurface.LiveSessionAPI,
	workAPI apisurface.WorkAPI,
	workRead apisurface.WorkReadAPI,
	invocation apisurface.InvocationAPI,
	modelsHTTP *modelshttp.Handler,
	factoryDefinitions apisurface.FactorySaveAPI,
	factoryValidation factorydefinitions.SubmittedDefinitionValidationOperation,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	durableExecution apisurface.DurableSessionExecutionAPI,
	durableLifecycle apisurface.DurableSessionLifecycleAPI,
	durableListing apisurface.DurableSessionListingAPI,
	durableResponseEvents apisurface.DurableSessionProjectionAPI,
	durableLister api.DurableExecutionSessionLister,
	liveSessionLister factorysessionshttp.LiveSessionListReader,
	providerSessions providersessions.Service,
	workerPrompts workers.PromptTemplates,
	contentStaging work.ContentStagingService,
	requestPreparation work.RequestPreparationService,
	sessionRequests factorysessionshttp.RequestPreparation,
	logger *zap.Logger,
	sessionsRoots ...factorysessions.Service,
) *api.Server {
	var sessionsRoot factorysessions.Service
	if len(sessionsRoots) > 0 {
		sessionsRoot = sessionsRoots[0]
	}
	var inspection factorysessions.SessionInspectionService
	if sessionsRoot != nil {
		inspection, _ = sessionsRoot.(factorysessions.SessionInspectionService)
	}
	if workAPI != nil {
		sessionsRoot = liveEventsTestRoot{Service: sessionsRoot, source: workAPI}
	}
	listRoot := sessionsRoot
	if durableLister != nil {
		listRoot = testDurableListRoot{Service: sessionsRoot, lister: durableLister}
	}
	var liveControl factorysessions.LiveControlService
	if sessions != nil {
		liveControl = testLiveControl{live: sessions}
	}
	handler := factorysessionshttp.NewHandler(
		factorysessionshttp.NewLifecycleHandler(sessionsRoot, liveControl, nil, sessionRequests, logger),
		factorysessionshttp.NewReadHandler(listRoot, sessions, liveSessionLister, sessionRequests, logger),
		factorysessionshttp.NewAuthoringHandler(currentFactoryTestReader{runtime}, factoryDefinitions, factoryValidation, workflowPreview, workerPrompts, logger),
		factorysessionshttp.NewInvocationHandler(invocation, logger),
		factorysessionshttp.NewObservationHandler(sessionsRoot, sessions, factoryStatus, sessionRequests, logger),
	)
	workRoot := &completeWorkTestRoot{
		submission: workAPI, read: workRead, staging: contentStaging, preparation: requestPreparation,
		invocationInput: func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
			return work.PreparedInvocationInput{}, errWorkFixtureUnavailable
		},
	}
	var providerSessionsHTTP *providersessionshttp.Handler
	if providerSessions != nil {
		providerSessionsHTTP = providersessionshttp.NewHandler(
			providersessionshttp.NewAdapter(providerSessions), logger,
		)
	}
	return api.NewServerWithRecordings(
		recordingshttp.NewAdapterWithSessions(nil, sessionsRoot, inspection),
		handler, workhttp.NewAdapter(workRoot),
		modelsHTTP, providerSessionsHTTP, nil, logger,
	)
}

type liveEventsTestRoot struct {
	factorysessions.Service
	source apisurface.WorkAPI
}

func (root liveEventsTestRoot) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return root.source.SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (root liveEventsTestRoot) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) error {
	return root.source.ProbeFactoryEventsForSession(ctx, sessionID, reconnect)
}

type canonicalOpenTestRoot struct{ factorysessions.Service }

func (canonicalOpenTestRoot) Start(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	return factorysessions.SessionStartResult{SessionID: "opened", Mode: factorysessions.SessionOperationModeLive,
		Live: &factorysessions.SessionOpenResult{SessionID: "opened", FolderPath: request.FolderPath}}, nil
}

// completeWorkTestRoot supplies all Work operations explicitly; unexpected
// fixture operations fail instead of relying on an embedded nil service.
type completeWorkTestRoot struct {
	invocationInput func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error)
	submission      apisurface.WorkAPI
	read            apisurface.WorkReadAPI
	staging         work.ContentStagingService
	preparation     work.RequestPreparationService
}

var errWorkFixtureUnavailable = errors.New("work service is unavailable")
var _ work.Service = (*completeWorkTestRoot)(nil)

func (r *completeWorkTestRoot) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if r.submission == nil {
		return work.WorkRequestSubmitResult{}, errWorkFixtureUnavailable
	}
	return r.submission.SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (r *completeWorkTestRoot) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	if r.submission == nil {
		return work.OperatorMoveResult{}, errWorkFixtureUnavailable
	}
	return r.submission.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (r *completeWorkTestRoot) ListWork(ctx context.Context, sessionID string, options work.ListOptions) (work.ListResult, error) {
	if r.read == nil {
		return work.ListResult{}, errWorkFixtureUnavailable
	}
	return r.read.ListWork(ctx, sessionID, options)
}

func (r *completeWorkTestRoot) GetWork(ctx context.Context, sessionID, workID string) (work.ReadModel, error) {
	if r.read == nil {
		return work.ReadModel{}, errWorkFixtureUnavailable
	}
	return r.read.GetWork(ctx, sessionID, workID)
}

func (r *completeWorkTestRoot) ResolveWorkerSessionWork(ctx context.Context, sessionID, workID string) (work.WorkerSessionWork, error) {
	reader, ok := r.read.(interface {
		ResolveWorkerSessionWork(context.Context, string, string) (work.WorkerSessionWork, error)
	})
	if !ok {
		return work.WorkerSessionWork{}, errWorkFixtureUnavailable
	}
	return reader.ResolveWorkerSessionWork(ctx, sessionID, workID)
}

func (r *completeWorkTestRoot) MoveWorkAndRead(ctx context.Context, sessionID, workID, stateName, requestID string) (work.ReadModel, error) {
	if r.read == nil {
		return work.ReadModel{}, errWorkFixtureUnavailable
	}
	return r.read.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
}

func (r *completeWorkTestRoot) PrepareWorkRequest(ctx context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
	if r.preparation == nil {
		return work.WorkRequest{}, errWorkFixtureUnavailable
	}
	return r.preparation.PrepareWorkRequest(ctx, input)
}

func (r *completeWorkTestRoot) StageContent(ctx context.Context, request work.StageContentRequest) (work.StageContentResult, error) {
	if r.staging == nil {
		return work.StageContentResult{}, errWorkFixtureUnavailable
	}
	return r.staging.StageContent(ctx, request)
}

func (r *completeWorkTestRoot) PrepareContent(ctx context.Context, items []work.StagedSubmissionItem) ([]work.WorkContentPart, error) {
	if r.staging == nil {
		return nil, errWorkFixtureUnavailable
	}
	return r.staging.PrepareContent(ctx, items)
}

func (r *completeWorkTestRoot) ResolveContent(ctx context.Context, ref string) (work.ResolvedStagedContent, error) {
	if r.staging == nil {
		return work.ResolvedStagedContent{}, errWorkFixtureUnavailable
	}
	return r.staging.ResolveContent(ctx, ref)
}

func (r *completeWorkTestRoot) CleanupContent(ctx context.Context, ref string) error {
	if r.staging == nil {
		return errWorkFixtureUnavailable
	}
	return r.staging.CleanupContent(ctx, ref)
}
func (*completeWorkTestRoot) MaterializeContentURL(context.Context, string) (string, work.ContentCleanup, error) {
	return "", nil, errWorkFixtureUnavailable
}
func (*completeWorkTestRoot) MaterializeWorkerOutput(context.Context, work.MaterializeWorkerOutputRequest) (work.MaterializeWorkerOutputResult, error) {
	return work.MaterializeWorkerOutputResult{}, errWorkFixtureUnavailable
}
func (r *completeWorkTestRoot) PrepareInvocationInput(ctx context.Context, request work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
	return r.invocationInput(ctx, request)
}
func (*completeWorkTestRoot) ResolvePrimaryResult(context.Context, work.PrimaryResultSelectionInput) (work.PrimaryResultSelection, error) {
	return work.PrimaryResultSelection{}, errWorkFixtureUnavailable
}

// Current Factory fixtures retain their existing public read method; the HTTP
// adapter consumes only definition reads, without a runtime or event service.
type currentFactoryTestAPI interface {
	GetCurrentFactory(context.Context) (factoryapi.Factory, error)
}

type currentFactoryTestReader struct{ source currentFactoryTestAPI }

func (reader currentFactoryTestReader) GetCurrentNamedFactory(ctx context.Context) (factoryapi.Factory, error) {
	return reader.source.GetCurrentFactory(ctx)
}

type testDurableListRoot struct {
	factorysessions.Service
	lister factorysessionshttp.DurableExecutionSessionLister
}

func (root testDurableListRoot) ListSessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	return root.lister.ListSessions(ctx, request)
}

// testLiveControl migrates legacy response fixtures onto the explicit control role.
type testLiveControl struct {
	factorysessions.LiveControlService
	live apisurface.LiveSessionAPI
}

func (root testLiveControl) PauseLiveFactorySession(ctx context.Context, id string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	response, err := root.live.PauseLiveFactorySession(ctx, id, request)
	return factorysession.LifecycleControlResultFromAPI(response), err
}
func (root testLiveControl) ResumeLiveFactorySession(ctx context.Context, id string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	response, err := root.live.ResumeLiveFactorySession(ctx, id, request)
	return factorysession.LifecycleControlResultFromAPI(response), err
}

func (root testLiveControl) GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error) {
	return factorysessions.SessionProjection{}, factorysessions.ErrSessionNotFound
}
