package wire

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/initializer"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	agentmessageshttp "github.com/portpowered/infinite-you/pkg/services/agent_messages/transports/http"
	agentmessageswire "github.com/portpowered/infinite-you/pkg/services/agent_messages/wire"
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

// provideHTTPRuntimeBindingWithMetrics selects host facts over completed HTTP roles.
func provideHTTPRuntimeBindingWithMetrics(
	root *factorysessionwire.Root,
	recordingsAdapter *recordingshttp.Adapter,
	sessionsHandler *factorysessionshttp.Handler,
	factoryDefinitionsHandler *factorydefinitionshttp.Handler,
	workHandler *workhttp.Adapter,
	providerSessionsHTTP *providersessionshttp.Handler,
	modelsHandler *modelshttp.Handler,
	metricsHandler *factoryvisualizationhttp.MetricsHandler,
	costsHandler *costshttp.Handler,
	workerSessionsHandler *workersessionshttp.Handler,
	recoverOwners recordings.WorkerOwnerRecoveryOperation,
	endpointBinding workersessions.ExecutionEndpointBinding,
	logger *zap.Logger,
	messages *agentmessageswire.ConfiguredService,
	messagesHTTP *agentmessageshttp.Handler,
) httpRuntimeBinding {
	return func(sessionID string, cancellation initializer.InvocationCancellation) (http.Handler, error) {
		presentation, err := root.SessionPresentation(sessionID)
		if err != nil {
			return nil, err
		}
		messagesHTTP := messagesHTTP.ForProfile(messages.Activate(context.Background(), presentation.OperatorSettingsPath))
		if recoverOwners != nil {
			if err := recoverOwners(context.Background()); err != nil {
				return nil, err
			}
		}
		handler, err := newHTTPRuntimeHandlerWithMetrics(root, recordingsAdapter, sessionsHandler, factoryDefinitionsHandler, workHandler, sessionID, cancellation, providerSessionsHTTP, modelsHandler, metricsHandler, costsHandler, workerSessionsHandler, logger, messagesHTTP)
		if err != nil {
			return nil, err
		}
		return workerExecutionEndpointHandler{Handler: handler, bind: endpointBinding}, nil
	}
}

// Only a host exposing the canonical Worker Sessions admission routes may
// advertise an execution endpoint. Workflow-only hosts retain their own shell.
type workerExecutionEndpointHandler struct {
	http.Handler
	bind workersessions.ExecutionEndpointBinding
}

func (handler workerExecutionEndpointHandler) ObserveHostBinding(binding platformhttpserver.Binding) func() {
	endpoint := "http://" + net.JoinHostPort(binding.Host, fmt.Sprint(binding.Port))
	return handler.bind(endpoint)
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
	messagesHTTP *agentmessageshttp.Handler,
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
		costsHandler, workerSessionsHandler, logger, messagesHTTP,
	), nil
}

func provideHTTPDefinitionMapping(service factorydefinitions.Service) *factorydefinitionmapping.Service {
	return factorydefinitionmapping.New(service)
}
func provideHTTPDefinitionsAPI(mapping *factorydefinitionmapping.Service) *factorydefinitionmapping.API {
	return factorydefinitionmapping.NewAPI(mapping, mapping)
}
func provideHTTPRecordingsAdapter(service recordings.Service, root *factorysessionwire.Root, inspection factorysessions.SessionInspectionService) *recordingshttp.Adapter {
	return recordingshttp.NewAdapterWithSessions(service, root, inspection)
}
func provideHTTPMetricsScope(root *factorysessionwire.Root) factorysessions.RuntimeMetricsScopeResolver {
	return factorysessionwire.NewRuntimeMetricsScopeResolver(root)
}
func provideHTTPStatusAPI(root *factorysessionwire.Root, projector factoryruntime.FactoryStatusProjector) apisurface.FactoryStatusAPI {
	return factorysessionshttp.NewFactoryStatusAPI(root, projector)
}
func provideHTTPLiveAPI(root *factorysessionwire.Root) *factorysessionmapping.LiveAPI {
	return factorysessionmapping.NewLiveAPI(root, root)
}
func provideHTTPInvocationAPI(root *factorysessionwire.Root) *factorysessionmapping.InvocationAPI {
	return factorysessionmapping.NewInvocationAPI(root)
}
func provideHTTPLifecycleHandler(root *factorysessionwire.Root, requests factorysessionshttp.RequestPreparation, logger *zap.Logger) *factorysessionshttp.LifecycleHandler {
	return factorysessionshttp.NewLifecycleHandler(root, root, root, requests, logger)
}
func provideHTTPReadHandler(root *factorysessionwire.Root, live *factorysessionmapping.LiveAPI, requests factorysessionshttp.RequestPreparation, logger *zap.Logger) *factorysessionshttp.ReadHandler {
	return factorysessionshttp.NewReadHandler(root, live, factorysessionshttp.ReadProjectionSessionListReader{Reader: root}, requests, logger)
}
func provideHTTPAuthoringHandler(mapping *factorydefinitionmapping.Service, definitions *factorydefinitionmapping.API, validation factorydefinitions.SubmittedDefinitionValidationOperation, preview factoryruntime.WorkflowPreviewOperation, prompts workers.PromptTemplates, logger *zap.Logger) *factorysessionshttp.AuthoringHandler {
	return factorysessionshttp.NewAuthoringHandler(mapping, definitions, validation, preview, prompts, logger)
}
func provideHTTPInvocationHandler(invocation *factorysessionmapping.InvocationAPI, logger *zap.Logger) *factorysessionshttp.InvocationHandler {
	return factorysessionshttp.NewInvocationHandler(invocation, logger)
}
func provideHTTPObservationHandler(root *factorysessionwire.Root, live *factorysessionmapping.LiveAPI, status apisurface.FactoryStatusAPI, requests factorysessionshttp.RequestPreparation, logger *zap.Logger) *factorysessionshttp.ObservationHandler {
	return factorysessionshttp.NewObservationHandler(root, live, status, requests, logger)
}

type httpDefinitionsTopology struct {
	factorydefinitions.SubmittedDefinitionValidationOperation
}

func provideHTTPDefinitionsTopology(service factorydefinitions.Service) httpDefinitionsTopology {
	return httpDefinitionsTopology{factorydefinitionshttp.NewTopologyValidation(service)}
}
func provideHTTPDefinitionsHandler(service factorydefinitions.Service, validation httpDefinitionsTopology, logger *zap.Logger) *factorydefinitionshttp.Handler {
	return factorydefinitionshttp.NewHandler(service, validation, logger)
}
func provideHTTPModelsAdapter(service models.Service, root *factorysessionwire.Root, content work.ContentPreparation) *modelshttp.Adapter {
	return modelshttp.NewSessionAdapter(service, root, content, modelshttp.ModelsScope, modelshttp.SessionID)
}
func provideHTTPCostsAdapter(query costs.CostsQuery, resolver factorysessions.RuntimeMetricsScopeResolver) *costshttp.Adapter {
	return costshttp.NewAdapter(query, costshttp.RuntimePaths, resolver)
}
func provideHTTPMetricsAdapter(query factoryvisualization.RuntimeMetricsQuery, resolver factorysessions.RuntimeMetricsScopeResolver) *factoryvisualizationhttp.MetricsAdapter {
	return factoryvisualizationhttp.NewMetricsAdapter(query, resolver, factoryvisualizationhttp.MetricsRoot)
}
func provideHTTPWorkerSessionScope(root *factorysessionwire.Root) workersessionshttp.SessionScopeResolver {
	return newWorkerSessionsFactorySessionScopeResolver(root)
}
func provideHTTPWorkerSessionController(logs workersessions.Service, sources workersessionswire.ObservationServiceCatalog) workerSessionControlRouter {
	return workerSessionControlRouter{archived: logs, sources: sources}
}
func provideHTTPWorkerSessionsAdapter(logs workersessions.Service, controller workerSessionControlRouter, workService work.Service, attribution recordings.WorkerWorkAttributionReader, resolver workersessionshttp.SessionScopeResolver) *workersessionshttp.Adapter {
	return workersessionshttp.NewAdapterWithStartAndContinueAndInterruptAndControl(logs, logs, logs, controller, logs, workService, attribution, resolver)
}
func provideHTTPWorkerSessionsHandler(adapter *workersessionshttp.Adapter, fleet *workersessionswire.FleetObservationService, logs workersessions.Service, logger *zap.Logger) *workersessionshttp.Handler {
	return workersessionshttp.NewHandler(adapter.WithTopLevelObservationService(fleet).WithLogsService(logs), logger)
}
func provideWorkerExecutionEndpointBinding(logs workersessions.Service) (workersessions.ExecutionEndpointBinding, error) {
	owner, ok := logs.(interface {
		ExecutionEndpointBinding() workersessions.ExecutionEndpointBinding
	})
	if !ok {
		return nil, errors.New("construct HTTP runtime binding: Worker Sessions execution endpoint binding is required")
	}
	return owner.ExecutionEndpointBinding(), nil
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
	messagesHTTP *agentmessageshttp.Handler,
) http.Handler {
	var shutdown transporthttp.ShutdownOperation
	if cancellation != nil {
		shutdown = cancellation.Cancel
	}
	handler := transporthttp.NewServerWithRecordingsAndMetricsAndCosts(
		recordingsAdapter, sessionsHandler, workHandler, modelsHandler, providerSessionsHTTP,
		factoryDefinitionsHandler, logger, metricsHandler, costsHandler, shutdown, messagesHTTP, workerSessionsHandler,
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

type httpDefaultWorkTypeResolver func(context.Context, string) (string, error)

func provideHTTPDefaultWorkTypeResolver(root *factorysessionwire.Root, policy factorydefinitions.InvocationWorkTypeService) httpDefaultWorkTypeResolver {
	return newDefaultWorkTypeResolver(root, policy)
}

func provideHTTPWorkAdapter(workService work.Service, definitions *factorydefinitionmapping.API, defaultType httpDefaultWorkTypeResolver) *workhttp.Adapter {
	return workhttp.NewAdapterWithSessionScope(workService, func(ctx context.Context, sessionID string) error {
		_, err := definitions.GetCurrentFactoryForSession(ctx, sessionID)
		return err
	}).WithDefaultWorkTypeResolver(defaultType)
}
