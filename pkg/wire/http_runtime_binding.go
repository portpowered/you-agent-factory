package wire

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/costs"
	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	costshttp "github.com/portpowered/infinite-you/pkg/services/costs/transports/http"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionshttp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorydefinition"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessionshttp "github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	work "github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionshttp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/http"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	generatedhttpclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

const metricsCLIHTTPTimeout = 5 * time.Minute

func provideCostsCLI() costscli.Operation {
	return costscli.NewOperation(func(server string) (costscli.Client, error) {
		return generatedhttpclient.NewClientWithResponses(
			server,
			generatedhttpclient.WithHTTPClient(&http.Client{Timeout: costscli.DefaultRequestTimeout}),
		)
	})
}

func provideCostsReportCLI() visualizationcli.CostReportOperation {
	readReport := costscli.NewReportOperation(func(server string) (costscli.Client, error) {
		return generatedhttpclient.NewClientWithResponses(
			server,
			generatedhttpclient.WithHTTPClient(&http.Client{Timeout: costscli.DefaultRequestTimeout}),
		)
	})
	return func(ctx context.Context, request visualizationcli.MetricsCostReportRequest) (generatedhttpclient.CostsReport, error) {
		return readReport(ctx, costscli.CostsConfig{
			Server:         request.Server,
			SessionID:      request.SessionID,
			RequestTimeout: costscli.DefaultRequestTimeout,
		})
	}
}

func provideMetricsCLI() visualizationcli.Operation {
	return provideMetricsCLIWithHTTPTransport(nil)
}

// provideMetricsCLIWithHTTPTransport keeps the metrics request policy in Wire
// while allowing package-owned tests to observe or control the external HTTP
// boundary without changing the generated client or public CLI contracts.
func provideMetricsCLIWithHTTPTransport(transport http.RoundTripper) visualizationcli.Operation {
	return visualizationcli.NewOperation(func(server string) (visualizationcli.Client, error) {
		return generatedhttpclient.NewClientWithResponses(
			server,
			generatedhttpclient.WithHTTPClient(newMetricsCLIHTTPClient(transport)),
		)
	})
}

func newMetricsCLIHTTPClient(transport http.RoundTripper) *http.Client {
	return &http.Client{Transport: transport, Timeout: metricsCLIHTTPTimeout}
}

// httpRuntimeBinding is the Wire-owned operation that builds the owner HTTP
// adapters for one opened runtime and returns the generated route shell.
// Runtime opening invokes this operation directly; no transport application
// binder or central mapping graph is involved.
type httpRuntimeBinding func(factorysessionwire.OpenedApplicationRuntime) (http.Handler, error)

// provideHTTPRuntimeBindingWithMetrics is the production Wire-owned live HTTP
// composition path. It builds each owner adapter for the opened runtime and
// hands the generated route shell only those prebuilt adapters.
func provideHTTPRuntimeBindingWithMetrics(
	factoryStatusProjector factoryruntime.FactoryStatusProjector,
	providerSessionsHTTP *providersessionshttp.Handler,
	modelsContent work.ContentPreparation,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	sessionRequests factorysessionshttp.RequestPreparation,
	metricsQuery factoryvisualization.RuntimeMetricsQuery,
	costsQuery costs.CostsQuery,
) (httpRuntimeBinding, error) {
	if factoryStatusProjector == nil || providerSessionsHTTP == nil || modelsContent == nil || validation == nil || invocationWorkType == nil || sessionRequests == nil || metricsQuery == nil || costsQuery == nil {
		return nil, errors.New("construct HTTP runtime binding: owner adapters and boundary policies are required")
	}
	return func(opened factorysessionwire.OpenedApplicationRuntime) (http.Handler, error) {
		return newHTTPRuntimeHandlerWithMetrics(opened, factoryStatusProjector, providerSessionsHTTP, modelsContent, validation, invocationWorkType, sessionRequests, metricsQuery, costsQuery)
	}, nil
}

func newHTTPRuntimeHandlerWithMetrics(
	opened factorysessionwire.OpenedApplicationRuntime,
	factoryStatusProjector factoryruntime.FactoryStatusProjector,
	providerSessionsHTTP *providersessionshttp.Handler,
	modelsContent work.ContentPreparation,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	sessionRequests factorysessionshttp.RequestPreparation,
	metricsQuery factoryvisualization.RuntimeMetricsQuery,
	costsQuery costs.CostsQuery,
) (http.Handler, error) {
	if err := validateOpenedHTTPRuntime(opened); err != nil {
		return nil, err
	}
	modelsHandler, err := newHTTPModelsHandler(opened, modelsContent)
	if err != nil {
		return nil, err
	}
	runtimeAPI, definitionMapping, err := newHTTPRuntimeDefinitionAPIs(opened)
	if err != nil {
		return nil, err
	}
	sessionsHandler, definitionsAPI, err := newHTTPSessionsHandler(
		opened, factoryStatusProjector, runtimeAPI, definitionMapping,
		validation, invocationWorkType, sessionRequests,
	)
	if err != nil {
		return nil, err
	}
	recordingsAdapter := newHTTPRecordingsAdapter(opened, sessionRequests)
	workerSessionsHandler := newHTTPWorkerSessionsHandler(opened)
	metricsScopeResolver := factorysessionwire.NewRuntimeMetricsScopeResolver(opened.FactorySessions)
	if metricsScopeResolver == nil {
		return nil, errors.New("bind HTTP runtime: Factory Sessions metrics scope resolver is unavailable")
	}
	costsHandler, err := newHTTPCostsHandler(opened, costsQuery, metricsScopeResolver)
	if err != nil {
		return nil, err
	}
	workHandler := workhttpAdapter(opened, definitionsAPI, invocationWorkType)
	factoryDefinitionsHandler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: opened.FactoryDefinitions}, opened.Logger,
	)
	return newHTTPRuntimeServer(
		recordingsAdapter, sessionsHandler, workHandler, modelsHandler,
		providerSessionsHTTP, factoryDefinitionsHandler, opened, metricsQuery,
		metricsScopeResolver, costsHandler, workerSessionsHandler,
	), nil
}

func validateOpenedHTTPRuntime(opened factorysessionwire.OpenedApplicationRuntime) error {
	if opened.FactoryRuntime == nil || opened.FactoryDefinitions == nil || opened.FactorySessions == nil || opened.LiveControl == nil {
		return errors.New("bind HTTP mappings: opened Factory Session roles are required")
	}
	return nil
}

func newHTTPModelsHandler(
	opened factorysessionwire.OpenedApplicationRuntime,
	modelsContent work.ContentPreparation,
) (*modelshttp.Handler, error) {
	modelInvoker := opened.ModelInvoker
	if modelInvoker == nil {
		modelInvoker = opened.Workers
	}
	modelsAdapter := modelshttp.NewAdapter(opened.Models, modelInvoker, modelsContent, opened.ModelsScope)
	modelsHandler := modelshttp.NewHandler(modelsAdapter, opened.Logger)
	if modelsHandler == nil {
		return nil, errors.New("bind HTTP runtime: Models service, invoker, content preparation, and logger are required")
	}
	return modelsHandler, nil
}

func newHTTPRuntimeDefinitionAPIs(
	opened factorysessionwire.OpenedApplicationRuntime,
) (apisurface.RuntimeAPI, *factorydefinitionmapping.Service, error) {
	definitionMapping := factorydefinitionmapping.New(opened.FactoryDefinitions)
	runtimeAPI := apisurface.NewRuntimeAPI(opened.FactoryRuntime, definitionMapping)
	if _, ok := opened.FactoryRuntime.(interface {
		SubscribeFactoryEvents(
			context.Context,
			*factorydefinitions.FactoryEventReconnectCursor,
			factorydefinitions.FactoryEventReconnectScope,
		) (*factorydefinitions.FactoryEventStream, error)
	}); !ok {
		return nil, nil, errors.New("bind HTTP mappings: Factory Runtime event subscription is required")
	}
	return runtimeAPI, definitionMapping, nil
}

func newHTTPSessionsHandler(
	opened factorysessionwire.OpenedApplicationRuntime,
	factoryStatusProjector factoryruntime.FactoryStatusProjector,
	runtimeAPI apisurface.RuntimeAPI,
	definitionMapping *factorydefinitionmapping.Service,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	sessionRequests factorysessionshttp.RequestPreparation,
) (*factorysessionshttp.Handler, *factorydefinitionmapping.API, error) {
	statusSessions, ok := opened.FactorySessions.(factorysessionshttp.FactoryStatusSessionReader)
	if !ok {
		return nil, nil, errors.New("bind HTTP mappings: Factory Sessions session-scoped status observation is required")
	}
	liveGateway, ok := opened.FactorySessions.(factorysessionmapping.LiveGateway)
	if !ok {
		return nil, nil, errors.New("bind HTTP mappings: Factory Sessions live result gateway is required")
	}
	statusAPI := factorysessionshttp.NewFactoryStatusAPI(statusSessions, factoryStatusProjector)
	definitionsAPI := factorydefinitionmapping.NewAPI(definitionMapping, definitionMapping)
	liveAPI := factorysessionmapping.NewLiveAPI(opened.LiveControl, liveGateway)
	deletion, _ := opened.FactorySessions.(factorysessions.LiveDeletionService)
	invocationAPI := factorysessionmapping.NewInvocationAPI(opened.FactorySessions)
	durableAPI := factorysessionmapping.NewDurableAPI(opened.FactorySessions)
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		SessionsRoot: opened.FactorySessions, LiveControl: opened.LiveControl,
		SessionDeletion: deletion,
		Runtime:         runtimeAPI, FactoryStatus: statusAPI,
		Sessions: liveAPI, Invocation: invocationAPI, FactoryDefinitions: definitionsAPI,
		FactoryValidation: validation, WorkflowPreview: opened.WorkflowPreview,
		DurableExecution: durableAPI, DurableLifecycle: durableAPI,
		DurableListing: durableAPI, DurableResponseEvents: durableAPI,
		DurableLister:     opened.FactorySessions,
		LiveSessionLister: factorysessionshttp.ReadProjectionSessionListReader{Reader: opened.LiveControl},
		WorkerPrompts:     opened.WorkerPrompts, InvocationWorkType: invocationWorkType,
		SessionRequests: sessionRequests,
	}, opened.Logger)
	return handler, definitionsAPI, nil
}

func newHTTPRecordingsAdapter(
	opened factorysessionwire.OpenedApplicationRuntime,
	sessionRequests factorysessionshttp.RequestPreparation,
) *recordingshttp.Adapter {
	legacyDurable := factorysessionmapping.NewDurableAPI(opened.FactorySessions)
	return recordingshttp.NewAdapterWithLegacyFallback(
		opened.Recordings,
		factorysessionmapping.NewDurableHistoryBridge(legacyDurable),
		factorysessionshttp.NewDurableRequestPreparation(sessionRequests),
		opened.FactorySessions,
	)
}

func newHTTPWorkerSessionsHandler(
	opened factorysessionwire.OpenedApplicationRuntime,
) *workersessionshttp.Handler {
	if opened.WorkerSessions == nil {
		return nil
	}
	resolver := newWorkerSessionsFactorySessionScopeResolver(opened.FactorySessions)
	adapter := workersessionshttp.NewAdapterWithStartAndContinueAndInterruptAndControl(
		opened.WorkerSessions, opened.WorkerSessions, opened.WorkerSessions,
		opened.WorkerSessions, opened.WorkerSessions, opened.Work, resolver,
	)
	if adapter == nil {
		return nil
	}
	fleet := workersessionswire.NewFleetObservationService(func(ctx context.Context) ([]workersessions.Service, error) {
		return workerSessionObservationSources(ctx, opened)
	})
	return workersessionshttp.NewHandler(adapter.WithTopLevelObservationService(fleet), opened.Logger)
}

func workerSessionObservationSources(
	ctx context.Context,
	opened factorysessionwire.OpenedApplicationRuntime,
) ([]workersessions.Service, error) {
	sources := make([]workersessions.Service, 0, 1)
	if opened.FactorySessions == nil {
		if opened.WorkerSessions != nil {
			sources = append(sources, opened.WorkerSessions)
		}
		return sources, nil
	}
	projections, err := opened.FactorySessions.ListFactorySessions(ctx)
	if err != nil {
		return nil, err
	}
	provider, ok := opened.FactorySessions.(interface {
		WorkerSessionsObservationForSession(string) workersessions.Service
	})
	if !ok {
		return sources, nil
	}
	ids := make([]string, 0, len(projections))
	for _, projection := range projections {
		id := strings.TrimSpace(projection.Context.FactorySessionID)
		if id == "" && projection.Context.Session != nil {
			id = strings.TrimSpace(projection.Context.Session.ID)
		}
		if id == "" || containsString(ids, id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if observation := provider.WorkerSessionsObservationForSession(id); observation != nil {
			sources = append(sources, observation)
		}
	}
	// The per-Factory-Session decorators carry canonical dispatch projection
	// and durable recording health. Add the broad process-local service only
	// after them so FleetObservationService's duplicate merge starts from the
	// stronger Factory-scoped identity facts.
	if opened.WorkerSessions != nil {
		sources = append(sources, opened.WorkerSessions)
	}
	return sources, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true

		}
	}
	return false
}

func newHTTPCostsHandler(
	opened factorysessionwire.OpenedApplicationRuntime,
	costsQuery costs.CostsQuery,
	metricsScopeResolver factorysessions.RuntimeMetricsScopeResolver,
) (*costshttp.Handler, error) {
	costsAdapter := costshttp.NewAdapter(
		costsQuery,
		opened.Resources.Diagnostics.MetricsRootDir,
		opened.OperatorSettingsPath,
		metricsScopeResolver,
	)
	costsHandler := costshttp.NewHandler(costsAdapter, opened.Logger)
	if costsHandler == nil {
		return nil, errors.New("bind HTTP runtime: Costs query and runtime paths are required")
	}
	return costsHandler, nil
}

func newHTTPRuntimeServer(
	recordingsAdapter *recordingshttp.Adapter,
	sessionsHandler *factorysessionshttp.Handler,
	workHandler *workhttp.Adapter,
	modelsHandler *modelshttp.Handler,
	providerSessionsHTTP *providersessionshttp.Handler,
	factoryDefinitionsHandler *factorydefinitionshttp.Handler,
	opened factorysessionwire.OpenedApplicationRuntime,
	metricsQuery factoryvisualization.RuntimeMetricsQuery,
	metricsScopeResolver factorysessions.RuntimeMetricsScopeResolver,
	costsHandler *costshttp.Handler,
	workerSessionsHandler *workersessionshttp.Handler,
) http.Handler {
	metricsAdapter := factoryvisualizationhttp.NewMetricsAdapter(
		metricsQuery,
		metricsScopeResolver,
		opened.Resources.Diagnostics.MetricsRootDir,
	)
	metricsHandler := factoryvisualizationhttp.NewMetricsHandler(metricsAdapter, opened.Logger)
	var shutdown transporthttp.ShutdownOperation
	if opened.Cancellation != nil {
		shutdown = opened.Cancellation.Cancel
	}
	return transporthttp.NewServerWithRecordingsAndMetricsAndCosts(
		recordingsAdapter, sessionsHandler, workHandler, modelsHandler, providerSessionsHTTP,
		factoryDefinitionsHandler, opened.Logger, metricsHandler, costsHandler, shutdown, workerSessionsHandler,
	).Handler()
}

func workhttpAdapter(
	opened factorysessionwire.OpenedApplicationRuntime,
	factoryDefinitionsAPI apisurface.FactorySaveAPI,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
) *workhttp.Adapter {
	return workhttp.NewAdapterWithSessionScope(opened.Work, func(ctx context.Context, sessionID string) error {
		_, err := factoryDefinitionsAPI.GetCurrentFactoryForSession(ctx, sessionID)
		return err
	}).WithDefaultWorkTypeResolver(newDefaultWorkTypeResolver(factoryDefinitionsAPI, invocationWorkType))
}
