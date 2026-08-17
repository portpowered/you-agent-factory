package http

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionshttp "github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
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
		Runtime: runtime, FactoryStatus: factoryStatus,
		Sessions: sessions, Invocation: invocation,
		FactoryDefinitions: factoryDefinitions, FactoryValidation: factoryValidation,
		WorkflowPreview:  workflowPreview,
		DurableExecution: durableExecution, DurableLifecycle: durableLifecycle,
		DurableListing: durableListing, DurableResponseEvents: durableResponseEvents,
		DurableLister: durableLister, LiveSessionLister: liveSessionLister,
		WorkerPrompts:   workerPrompts,
		SessionRequests: sessionRequests,
	}, logger)
	workRoot := work.AdmissionContentService(contentStaging, requestPreparation)
	workAdapter := workhttp.NewAdapterFromRoles(workRoot, workRoot, workAPI, workRead)
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
		recordingshttp.NewLegacyAdapterWithLive(
			factorysessionmapping.NewDurableHistoryBridge(durableResponseEvents),
			factorysessionshttp.NewDurableRequestPreparation(sessionRequests),
			workAPI,
		),
		handler, workAdapter, modelsHTTP, providerSessionsHTTP, nil, logger,
	)
}
