package http

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

// newServerFromRoles is a compatibility construction helper for transport
// tests that exercise individual generated operations with narrow fakes.
// Production composition injects the service-owned adapter through NewServer.
func newServerFromRoles(
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
	durableProjection apisurface.DurableSessionProjectionAPI,
	durableLister DurableExecutionSessionLister,
	liveSessionLister factorysessionshttp.LiveSessionListReader,
	providerSessions providersessions.Service,
	workerPrompts workers.PromptTemplates,
	contentStaging work.ContentStagingService,
	requestPreparation work.RequestPreparationService,
	sessionRequests factorysessionshttp.RequestPreparation,
	logger *zap.Logger,
) *Server {
	workService := work.AdmissionContentService(contentStaging, requestPreparation)
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		Runtime: runtime, FactoryStatus: factoryStatus,
		Sessions: sessions, Work: workAPI, WorkRead: workRead, Invocation: invocation,
		FactoryDefinitions: factoryDefinitions, FactoryValidation: factoryValidation,
		WorkflowPreview:  workflowPreview,
		DurableExecution: durableExecution, DurableLifecycle: durableLifecycle,
		DurableListing: durableListing, DurableProjection: durableProjection,
		DurableLister: durableLister, LiveSessionLister: liveSessionLister,
		WorkerPrompts: workerPrompts,
		WorkHTTP:      workhttp.NewAdapterFromRoles(nil, workService, workAPI, workRead, nil),
		WorkHTTPFactory: func(
			primary work.Service,
			admission work.Service,
			submission apisurface.WorkAPI,
			read apisurface.WorkReadAPI,
		) factorysessionshttp.WorkHTTP {
			return workhttp.NewAdapterFromRoles(primary, admission, submission, read, nil)
		},
		WorkService:     workService,
		SessionRequests: sessionRequests,
	}, logger)
	return NewServer(handler, modelsHTTP, providerSessions, logger, nil)
}
