package http

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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	recordingshttp "github.com/portpowered/infinite-you/pkg/transports/http/recordings"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

// newServerFromRoles is a compatibility construction helper for transport
// tests that exercise individual generated operations with narrow fakes.
// Production composition injects the service-owned adapter through NewServer.
func newServerFromRoles(
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
	durableLister DurableExecutionSessionLister,
	liveSessionLister factorysessionshttp.LiveSessionListReader,
	providerSessions providersessions.Service,
	workerPrompts workers.PromptTemplates,
	contentStaging work.ContentStagingService,
	requestPreparation work.RequestPreparationService,
	sessionRequests factorysessionshttp.RequestPreparation,
	logger *zap.Logger,
) *Server {
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		CurrentFactory: currentFactoryTestReader{runtime}, FactoryStatus: factoryStatus,
		Sessions: sessions, Invocation: invocation,
		FactoryDefinitions: factoryDefinitions, FactoryValidation: factoryValidation,
		WorkflowPreview: workflowPreview,
		DurableLister:   durableLister, LiveSessionLister: liveSessionLister,
		WorkerPrompts:   workerPrompts,
		SessionRequests: sessionRequests,
	}, logger)
	workRoot := &completeWorkTestRoot{
		submission: workAPI, read: workRead, staging: contentStaging, preparation: requestPreparation,
		invocationInput: func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
			return work.PreparedInvocationInput{}, errWorkFixtureUnavailable
		},
	}
	workAdapter := workhttp.NewAdapter(workRoot)
	if factoryDefinitions != nil {
		workAdapter = workAdapter.WithSessionScope(func(ctx context.Context, sessionID string) error {
			_, err := factoryDefinitions.GetCurrentFactoryForSession(ctx, sessionID)
			return err
		})
	}
	var providerSessionsHTTP *providersessionshttp.Handler
	if providerSessions != nil {
		if logger == nil {
			logger = zap.NewNop()
		}
		providerSessionsHTTP = providersessionshttp.NewHandler(
			providersessionshttp.NewAdapter(providerSessions), logger,
		)
	}
	return NewServerWithRecordings(
		recordingshttp.NewAdapterWithSessions(nil, roleTestSessionsRoot{source: workAPI}, nil),
		handler, workAdapter, modelsHTTP, providerSessionsHTTP, nil, logger,
	)
}

type roleTestSessionsRoot struct {
	factorysessions.Service
	source apisurface.WorkAPI
}

func (root roleTestSessionsRoot) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return root.source.SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (root roleTestSessionsRoot) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) error {
	return root.source.ProbeFactoryEventsForSession(ctx, sessionID, reconnect)
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
