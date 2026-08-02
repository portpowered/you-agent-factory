package apiserver_test

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
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
	durableProjection apisurface.DurableSessionProjectionAPI,
	durableLister api.DurableExecutionSessionLister,
	liveSessionLister factorysessionshttp.LiveSessionListReader,
	providerSessions providersessions.Service,
	workerPrompts workers.PromptTemplates,
	contentStaging work.ContentStagingService,
	requestPreparation work.RequestPreparationService,
	sessionRequests factorysessionshttp.RequestPreparation,
	logger *zap.Logger,
) *api.Server {
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		Runtime: runtime, FactoryStatus: factoryStatus,
		Sessions: sessions, SessionEvents: workAPI, Invocation: invocation,
		FactoryDefinitions: factoryDefinitions, FactoryValidation: factoryValidation,
		WorkflowPreview:  workflowPreview,
		DurableExecution: durableExecution, DurableLifecycle: durableLifecycle,
		DurableListing: durableListing, DurableProjection: durableProjection,
		DurableLister: durableLister, LiveSessionLister: liveSessionLister,
		WorkerPrompts:   workerPrompts,
		SessionRequests: sessionRequests,
	}, logger)
	workRoot := work.AdmissionContentService(contentStaging, requestPreparation)
	return api.NewServer(handler, workhttp.NewAdapterFromRoles(workRoot, workRoot, workAPI, workRead), modelsHTTP, providerSessions, logger)
}
