package wire

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/initializer"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
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
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessionshttp "github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	work "github.com/portpowered/infinite-you/pkg/services/work"
	workhttp "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionshttp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/http"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	generatedhttpclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	recordingshttp "github.com/portpowered/infinite-you/pkg/transports/http/recordings"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

const metricsCLIHTTPTimeout = 5 * time.Minute

func provideHTTPWorkerPrompts(service workers.Service) (workers.PromptTemplates, error) {
	prompts, ok := service.(workers.PromptTemplates)
	if !ok || prompts == nil {
		return nil, errors.New("construct HTTP worker prompts: Workers prompt templates are required")
	}
	return prompts, nil
}

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

// httpRuntimeBinding selects one canonical live session and host cancellation.
type httpRuntimeBinding func(string, initializer.InvocationCancellation) (http.Handler, error)

// provideHTTPRuntimeBindingWithMetrics is the production Wire-owned live HTTP
// composition path. Session-independent adapters are constructed once; the
// returned operation selects host facts and cancellation for the route shell.
func provideHTTPRuntimeBindingWithMetrics(
	root *factorysessionwire.Root,
	inspection factorysessions.SessionInspectionService,
	definitions factorydefinitions.Service,
	workService work.Service,
	modelService models.Service,
	recordingsService recordings.Service,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	workerPrompts workers.PromptTemplates,
	factoryStatusProjector factoryruntime.FactoryStatusProjector,
	providerSessionsHTTP *providersessionshttp.Handler,
	modelsContent work.ContentPreparation,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
	sessionRequests factorysessionshttp.RequestPreparation,
	metricsQuery factoryvisualization.RuntimeMetricsQuery,
	costsQuery costs.CostsQuery,
	logs workersessions.Service,
	recoverOwners recordings.WorkerOwnerRecoveryOperation,
	writer recordings.WorkerRecordingWriter,
	clock factoryruntime.Clock,
	snapshots *workersessionswire.HistorySnapshotBudget,
	attribution recordings.WorkerWorkAttributionReader,
	logger *zap.Logger,
) (httpRuntimeBinding, error) {
	if root == nil || inspection == nil || definitions == nil || workService == nil || modelService == nil || recordingsService == nil || workflowPreview == nil || workerPrompts == nil || factoryStatusProjector == nil || providerSessionsHTTP == nil || modelsContent == nil || validation == nil || invocationWorkType == nil || sessionRequests == nil || metricsQuery == nil || costsQuery == nil || logs == nil || clock == nil || attribution == nil || logger == nil || snapshots == nil {
		return nil, errors.New("construct HTTP runtime binding: owner adapters and boundary policies are required")
	}
	definitionMapping := factorydefinitionmapping.New(definitions)
	definitionsAPI := factorydefinitionmapping.NewAPI(definitionMapping, definitionMapping)
	recordingsAdapter := recordingshttp.NewAdapterWithSessions(recordingsService, root, inspection)
	workHandler := workhttpAdapter(root, workService, definitionsAPI, invocationWorkType)
	metricsScopeResolver := factorysessionwire.NewRuntimeMetricsScopeResolver(root)
	statusAPI := factorysessionshttp.NewFactoryStatusAPI(root, factoryStatusProjector)
	liveAPI := factorysessionmapping.NewLiveAPI(root, root)
	invocationAPI := factorysessionmapping.NewInvocationAPI(root)
	sessionsHandler := newHTTPSessionsHandler(
		root, logger, definitionMapping, definitionsAPI, statusAPI, liveAPI, invocationAPI,
		validation, sessionRequests, workflowPreview, workerPrompts,
	)
	factoryDefinitionsHandler := factorydefinitionshttp.NewHandler(
		definitions, factorydefinitionshttp.NewTopologyValidation(definitions), logger,
	)
	modelsHandler := modelshttp.NewHandler(modelshttp.NewSessionAdapter(modelService, root, modelsContent, modelshttp.ModelsScope, modelshttp.SessionID), logger)
	costsHandler := costshttp.NewHandler(costshttp.NewAdapter(costsQuery, costshttp.RuntimePaths, metricsScopeResolver), logger)
	metricsHandler := factoryvisualizationhttp.NewMetricsHandler(factoryvisualizationhttp.NewMetricsAdapter(metricsQuery, metricsScopeResolver, factoryvisualizationhttp.MetricsRoot), logger)
	workerSessionsHandler := newHTTPWorkerSessionsHandler(root, workService, logs, writer, clock, snapshots, attribution, logger)
	return func(sessionID string, cancellation initializer.InvocationCancellation) (http.Handler, error) {
		if recoverOwners != nil {
			if err := recoverOwners(context.Background()); err != nil {
				return nil, err
			}
		}
		return newHTTPRuntimeHandlerWithMetrics(root, recordingsAdapter, sessionsHandler, factoryDefinitionsHandler, workHandler, sessionID, cancellation, providerSessionsHTTP, modelsHandler, metricsHandler, costsHandler, workerSessionsHandler, logger)
	}, nil
}

func newHTTPRuntimeHandlerWithMetrics(
	root *factorysessionwire.Root,
	recordingsAdapter *recordingshttp.Adapter,
	sessionsHandler *factorysessionshttp.Handler,
	factoryDefinitionsHandler *factorydefinitionshttp.Handler,
	workHandler *workhttp.Adapter,
	sessionID string,
	cancellation initializer.InvocationCancellation,
	providerSessionsHTTP *providersessionshttp.Handler,
	modelsHandler *modelshttp.Handler,
	metricsHandler *factoryvisualizationhttp.MetricsHandler,
	costsHandler *costshttp.Handler,
	workerSessionsHandler *workersessionshttp.Handler,
	logger *zap.Logger,
) (http.Handler, error) {
	if root == nil {
		return nil, errors.New("bind HTTP mappings: Factory Sessions root is required")
	}
	presentation, err := root.SessionPresentation(sessionID)
	if err != nil {
		return nil, err
	}
	return newHTTPRuntimeServer(
		recordingsAdapter, sessionsHandler, workHandler, modelsHandler,
		providerSessionsHTTP, factoryDefinitionsHandler, presentation, sessionID, cancellation, metricsHandler,
		costsHandler, workerSessionsHandler, logger,
	), nil
}

func newHTTPSessionsHandler(
	root *factorysessionwire.Root,
	logger *zap.Logger,
	currentFactory apisurface.CurrentFactoryReader,
	definitionsAPI *factorydefinitionmapping.API,
	statusAPI apisurface.FactoryStatusAPI,
	liveAPI *factorysessionmapping.LiveAPI,
	invocationAPI *factorysessionmapping.InvocationAPI,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	sessionRequests factorysessionshttp.RequestPreparation,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	workerPrompts workers.PromptTemplates,
) *factorysessionshttp.Handler {
	handler := factorysessionshttp.NewHandler(
		factorysessionshttp.NewLifecycleHandler(root, root, root, sessionRequests, logger),
		factorysessionshttp.NewReadHandler(root, liveAPI, factorysessionshttp.ReadProjectionSessionListReader{Reader: root}, sessionRequests, logger),
		factorysessionshttp.NewAuthoringHandler(currentFactory, definitionsAPI, validation, workflowPreview, workerPrompts, logger),
		factorysessionshttp.NewInvocationHandler(invocationAPI, logger),
		factorysessionshttp.NewObservationHandler(root, liveAPI, statusAPI, sessionRequests, logger),
	)
	return handler
}

func newHTTPWorkerSessionsHandler(
	root *factorysessionwire.Root,
	workService work.Service,
	logs workersessions.Service,
	writer recordings.WorkerRecordingWriter,
	clock platformclock.Source,
	snapshots *workersessionswire.HistorySnapshotBudget,
	attribution recordings.WorkerWorkAttributionReader,
	logger *zap.Logger,
) *workersessionshttp.Handler {
	resolver := newWorkerSessionsFactorySessionScopeResolver(root)
	sources := func(ctx context.Context) ([]workersessions.Service, error) {
		hostID, _ := workersessionshttp.RuntimeHostSession(ctx)
		return workerSessionObservationSources(ctx, root, root.WorkerSessionsObservationForSession(hostID))
	}
	controller := workerSessionControlRouter{
		archived: logs,
		sources:  sources,
	}
	adapter := workersessionshttp.NewAdapterWithStartAndContinueAndInterruptAndControl(
		logs, logs, logs,
		controller, logs, workService, attribution, resolver,
	)
	captured, _ := writer.(recordings.WorkerCapturedActivityReader)
	fleet := workersessionswire.NewFleetObservationService(sources, captured, clock, logging.NewZapLogger(logger, false), snapshots, logs)
	return workersessionshttp.NewHandler(adapter.WithTopLevelObservationService(fleet).WithLogsService(logs), logger)
}

func workerSessionObservationSources(
	ctx context.Context,
	root interface {
		ListLiveSessionIDs() []string
		WorkerSessionsObservationForSession(string) workersessions.ObservationService
	},
	current workersessions.Service,
) ([]workersessions.Service, error) {
	sources := make([]workersessions.Service, 0, 1)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Discovery needs only live identities, not session or Work projections.
	// Building those projections here makes every fleet read wait on unrelated
	// runtime snapshots before it can reach the Worker Session registries.
	liveIDs := root.ListLiveSessionIDs()
	ids := make([]string, 0, len(liveIDs))
	for _, liveID := range liveIDs {
		id := strings.TrimSpace(liveID)
		if id == "" || containsString(ids, id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if observation := root.WorkerSessionsObservationForSession(id); observation != nil {
			sources = append(sources, observation)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Runtime-owned registries are authoritative for Factory Worker Sessions.
	// Keep the selected host registry last so duplicate restored identities do
	// not hide the successor runtime's canonical session attribution.
	if current != nil {
		sources = append(sources, current)
	}
	return sources, nil
}

// workerSessionControlRouter resolves top-level controls through the same
// ordered runtime-owned service sources used by Worker Session observations.
// Factory Runtime owns sessions opened while dispatching Work, so the
// process-default registry is only selected when it owns the exact identity.
type workerSessionControlRouter struct {
	sources  func(context.Context) ([]workersessions.Service, error)
	archived workersessions.Service
}

func (router workerSessionControlRouter) owner(
	ctx context.Context,
	request workersessions.ControlRequest,
) (workersessions.Service, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if router.sources == nil {
		return nil, workersessions.ErrSessionNotFound
	}
	sources, err := router.sources(ctx)
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		if source == nil {
			continue
		}
		_, err := source.Get(ctx, workersessions.GetRequest{ID: request.ID})
		if err == nil {
			return source, nil
		} else if !errors.Is(err, workersessions.ErrSessionNotFound) {
			return nil, err
		}
	}
	return nil, workersessions.ErrSessionNotFound
}

func (router workerSessionControlRouter) control(
	ctx context.Context,
	request workersessions.ControlRequest,
	action workersessions.ControlAction,
) (workersessions.ControlResult, error) {
	owner, err := router.owner(ctx, request)
	// Force outcomes may replay their exact committed request after the owner
	// has exited. The archived service validates that tuple without restoring
	// control authority; a fresh request still fails live-owner validation.
	archivedRequest := request.RequestID == "" || (request.Force && action == workersessions.ControlActionTerminate)
	if errors.Is(err, workersessions.ErrSessionNotFound) && archivedRequest &&
		(action == workersessions.ControlActionCancel || action == workersessions.ControlActionTerminate) && router.archived != nil {
		owner, err = router.archived, nil
	}
	if err != nil {
		return workersessions.ControlResult{}, err
	}
	switch action {
	case workersessions.ControlActionPause:
		return owner.Pause(ctx, request)
	case workersessions.ControlActionResume:
		return owner.Resume(ctx, request)
	case workersessions.ControlActionCancel:
		return owner.Cancel(ctx, request)
	case workersessions.ControlActionTerminate:
		return owner.Terminate(ctx, request)
	default:
		return workersessions.ControlResult{}, errors.New("unsupported Worker Session control action")
	}
}

func (router workerSessionControlRouter) Pause(
	ctx context.Context,
	request workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	return router.control(ctx, request, workersessions.ControlActionPause)
}

func (router workerSessionControlRouter) Resume(
	ctx context.Context,
	request workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	return router.control(ctx, request, workersessions.ControlActionResume)
}

func (router workerSessionControlRouter) Cancel(
	ctx context.Context,
	request workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	return router.control(ctx, request, workersessions.ControlActionCancel)
}

func (router workerSessionControlRouter) Terminate(
	ctx context.Context,
	request workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	return router.control(ctx, request, workersessions.ControlActionTerminate)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true

		}
	}
	return false
}

func newHTTPRuntimeServer(
	recordingsAdapter *recordingshttp.Adapter,
	sessionsHandler *factorysessionshttp.Handler,
	workHandler *workhttp.Adapter,
	modelsHandler *modelshttp.Handler,
	providerSessionsHTTP *providersessionshttp.Handler,
	factoryDefinitionsHandler *factorydefinitionshttp.Handler,
	presentation factorysessionwire.SessionPresentation,
	sessionID string,
	cancellation initializer.InvocationCancellation,
	metricsHandler *factoryvisualizationhttp.MetricsHandler,
	costsHandler *costshttp.Handler,
	workerSessionsHandler *workersessionshttp.Handler,
	logger *zap.Logger,
) http.Handler {
	var shutdown transporthttp.ShutdownOperation
	if cancellation != nil {
		shutdown = cancellation.Cancel
	}
	handler := transporthttp.NewServerWithRecordingsAndMetricsAndCosts(
		recordingsAdapter, sessionsHandler, workHandler, modelsHandler, providerSessionsHTTP,
		factoryDefinitionsHandler, logger, metricsHandler, costsHandler, shutdown, workerSessionsHandler,
	).Handler()
	modelsScope := presentation.ModelsScope
	metricsRoot := presentation.MetricsRootDir
	settingsPath := presentation.OperatorSettingsPath
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := workersessionshttp.WithRuntimeHostSession(r.Context(), sessionID)
		ctx = modelshttp.WithRuntimeSelection(ctx, sessionID, modelsScope)
		ctx = costshttp.WithRuntimePaths(ctx, metricsRoot, settingsPath)
		ctx = factoryvisualizationhttp.WithMetricsRoot(ctx, metricsRoot)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func workhttpAdapter(
	root *factorysessionwire.Root,
	workService work.Service,
	factoryDefinitionsAPI apisurface.FactorySaveAPI,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
) *workhttp.Adapter {
	return workhttp.NewAdapterWithSessionScope(workService, func(ctx context.Context, sessionID string) error {
		_, err := factoryDefinitionsAPI.GetCurrentFactoryForSession(ctx, sessionID)
		return err
	}).WithDefaultWorkTypeResolver(newDefaultWorkTypeResolver(root, invocationWorkType))
}
