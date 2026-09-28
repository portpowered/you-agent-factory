package apiserver_test

import (
	"context"

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
	recordingshttp "github.com/portpowered/infinite-you/pkg/transports/http/recordings"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func newAPIServerFromRoles(
	runtime apisurface.RuntimeAPI,
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
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		SessionsRoot: sessionsRoot,
		Runtime:      runtime, FactoryStatus: factoryStatus,
		Sessions: sessions, Invocation: invocation,
		FactoryDefinitions: factoryDefinitions, FactoryValidation: factoryValidation,
		WorkflowPreview: workflowPreview,
		DurableLister:   durableLister, LiveSessionLister: liveSessionLister,
		WorkerPrompts:   workerPrompts,
		SessionRequests: sessionRequests,
	}, logger)
	workRoot := work.AdmissionContentService(contentStaging, requestPreparation)
	var providerSessionsHTTP *providersessionshttp.Handler
	if providerSessions != nil {
		providerSessionsHTTP = providersessionshttp.NewHandler(
			providersessionshttp.NewAdapter(providerSessions), logger,
		)
	}
	return api.NewServerWithRecordings(
		recordingshttp.NewAdapterWithSessions(nil, sessionsRoot, inspection),
		handler, workhttp.NewAdapterFromRoles(workRoot, workRoot, workAPI, workRead),
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
