package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	"go.uber.org/zap"
)

func (fs *FactoryService) requireModelService() apisurface.ModelAPI {
	if fs == nil {
		return wireModelServiceCollaborator(nil, nil)
	}
	fs.modelInitOnce.Do(func() {
		if fs.modelService == nil {
			fs.modelService = wireModelServiceCollaborator(fs, fs.cfg)
		}
	})
	return fs.modelService
}

func (fs *FactoryService) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	return fs.requireModelService().ListModels(ctx)
}

func (fs *FactoryService) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	return fs.requireModelService().GetModel(ctx, modelName)
}

type modelAssetPuller = localmodels.AssetPuller

func newModelAssetPuller(cacheDir string) modelAssetPuller {
	return localmodels.NewAssetPuller(cacheDir)
}

func (fs *FactoryService) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	return fs.requireModelService().PullModel(ctx, modelName)
}

func (fs *FactoryService) modelAssetPuller() modelAssetPuller {
	if fs != nil && fs.modelAssets != nil {
		return fs.modelAssets
	}
	cacheDir := ""
	if fs != nil {
		cacheDir = strings.TrimSpace(fs.coordinatorPolicy().modelCacheDir)
	}
	puller := newModelAssetPuller(cacheDir)
	if fs != nil {
		fs.modelAssets = puller
	}
	return puller
}

func (fs *FactoryService) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return fs.requireModelService().InvokeModel(ctx, modelName, request)
}

// modelServiceHost adapts FactoryService runtime seams for pkg/models/service wiring.
type modelServiceHost struct {
	*FactoryService
}

var _ modelsservice.Host = modelServiceHost{}

func (h modelServiceHost) RuntimeConfig() func() *factoryconfig.LoadedFactoryConfig {
	if h.FactoryService == nil {
		return func() *factoryconfig.LoadedFactoryConfig { return nil }
	}
	return h.FactoryService.currentRuntimeConfig
}

func (h modelServiceHost) ModelHost() func() modelhost.Host {
	if h.FactoryService == nil {
		return func() modelhost.Host { return nil }
	}
	return h.FactoryService.modelHost
}

func (h modelServiceHost) ModelAssetPuller() func() localmodels.AssetPuller {
	if h.FactoryService == nil {
		return func() localmodels.AssetPuller { return nil }
	}
	return h.FactoryService.modelAssetPuller
}

func (h modelServiceHost) Logger() func() *zap.Logger {
	if h.FactoryService == nil {
		return func() *zap.Logger { return nil }
	}
	return func() *zap.Logger { return h.FactoryService.logger }
}

func (h modelServiceHost) ModelPullMetrics() func() modelsservice.PullMetricsRecorder {
	if h.FactoryService == nil {
		return func() modelsservice.PullMetricsRecorder { return nil }
	}
	return func() modelsservice.PullMetricsRecorder {
		recorder := h.FactoryService.modelPullMetricsRecorder()
		if recorder == nil {
			return nil
		}
		return modelPullMetricsHostAdapter{inner: recorder}
	}
}

func (h modelServiceHost) ModelInvocationExecutor() modelsservice.ModelInvocationExecutor {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.modelInvocationExecutor
}

func (h modelServiceHost) FactoryRunnerID() func() string {
	if h.FactoryService == nil {
		return func() string { return "" }
	}
	return h.FactoryService.factoryRunnerID
}

type modelPullMetricsHostAdapter struct {
	inner ModelPullMetricsRecorder
}

func (a modelPullMetricsHostAdapter) RecordModelPullMetric(metric modelsservice.PullMetric) {
	a.inner.RecordModelPullMetric(InvocationMetric{
		Name:   metric.Name,
		Labels: metric.Labels,
	})
}

func wireModelServiceCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) apisurface.ModelAPI {
	if cfg != nil && cfg.ModelAPI != nil {
		return cfg.ModelAPI
	}
	return modelsservice.NewFromHost(modelServiceHost{FactoryService: fs})
}

// ProvideModelServiceCollaborator constructs the model-domain collaborator for a
// built FactoryService shell.
func ProvideModelServiceCollaborator(
	shell FactoryServiceShell,
	cfg *FactoryServiceConfig,
) apisurface.ModelAPI {
	return wireModelServiceCollaborator(shell.Service, cfg)
}

// AttachModelServiceCollaborator assigns the model-domain collaborator on the
// service shell and returns the assembled FactoryService.
func AttachModelServiceCollaborator(
	shell FactoryServiceShell,
	modelAPI apisurface.ModelAPI,
) *FactoryService {
	if shell.Service != nil {
		shell.Service.modelService = modelAPI
	}
	return shell.Service
}

func (fs *FactoryService) modelHost() modelhost.Host {
	if fs == nil {
		return nil
	}
	if core := fs.core; core != nil {
		if host := core.ModelHost(); host != nil {
			return host
		}
	}
	if bundle := fs.currentRuntimeBundle(); bundle != nil && bundle.ModelHost != nil {
		return bundle.ModelHost
	}
	return nil
}

func (fs *FactoryService) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	return fs.sessionInvocationOwner().InvokeFactorySession(ctx, sessionID, request)
}

func (fs *FactoryService) sessionInvocationOwner() invocations.SessionInvoker {
	if fs.sessionInvoker != nil {
		return fs.sessionInvoker
	}
	return invocations.NewSessionOwner(invocations.SessionOwnerDependencies{
		FactoryConfig: fs.sessionInvocationFactoryConfig,
		SubmitWork:    fs.submitSessionInvocationWork,
		Observe:       fs.observeSessionInvocation,
		Telemetry:     fs.sessionInvocationTelemetry(),
		SpecialCase:   serviceSessionInvocationSpecialCase{},
	})
}

func (fs *FactoryService) sessionInvocationTelemetry() invocations.SessionInvocationTelemetry {
	return invocations.NewSessionInvocationTelemetry(invocations.SessionInvocationTelemetryDependencies{
		RecordMetric: func(metric invocations.SessionInvocationMetric) {
			fs.recordInvocationMetric(metric.Name, metric.Labels)
		},
		RecordLog: fs.recordSessionInvocationLog,
		Packaged: &invocations.PackagedInvocationTelemetry{
			Active: tts.IsPackagedFactory, FactoryName: tts.PackagedFactoryName, Backend: tts.BackendRuntimeLabel(),
			AttemptsMetric: tts.MetricPackagedFactoryAttempts, SuccessMetric: tts.MetricPackagedFactorySuccess,
			FailureMetric: tts.MetricPackagedFactoryFailure, NotReadyMetric: tts.MetricPackagedFactoryNotReady,
			LoadingClass: tts.FailureClassLoading, SuccessClass: tts.FailureClassSuccess, NotReadyClass: tts.FailureClassModelNotReady,
		},
	})
}

func (fs *FactoryService) recordSessionInvocationLog(record invocations.SessionInvocationLogRecord) {
	if fs == nil || fs.logger == nil {
		return
	}
	fields := make([]zap.Field, 0, len(record.Fields)+1)
	for key, value := range record.Fields {
		fields = append(fields, zap.Any(key, value))
	}
	if record.Error != nil {
		fields = append(fields, zap.Error(record.Error))
	}
	if record.Level == "warn" {
		fs.logger.Warn(record.Message, fields...)
		return
	}
	fs.logger.Info(record.Message, fields...)
}

func (fs *FactoryService) sessionInvocationFactoryConfig(sessionID string) (*interfaces.FactoryConfig, error) {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil {
		return nil, err
	}
	if runtimeCfg == nil {
		return nil, nil
	}
	return runtimeCfg.FactoryConfig(), nil
}

func (fs *FactoryService) submitSessionInvocationWork(
	ctx context.Context,
	sessionID string,
	request interfaces.SubmitRequest,
) (interfaces.WorkRequestSubmitResult, error) {
	return fs.SubmitWorkRequestForSession(
		ctx,
		sessionID,
		factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{request}),
	)
}

func (fs *FactoryService) observeSessionInvocation(
	ctx context.Context,
	sessionID string,
	input invocations.SessionInvocationWaitInput,
) (invocations.SessionInvocationObservation, error) {
	snapshot, err := fs.GetEngineStateSnapshotForSession(ctx, sessionID)
	if err != nil {
		return invocations.SessionInvocationObservation{}, err
	}
	worldState, err := fs.sessionInvocationWorldState(ctx, sessionID, snapshot.TickCount)
	if err != nil {
		return invocations.SessionInvocationObservation{}, err
	}
	return invocations.SessionInvocationObservation{
		WorldState: worldState, FactoryState: snapshot.FactoryState,
		ActiveWork:           snapshotHasActiveWork(snapshot),
		MissingPrimaryResult: classifyInvocationMissingPrimaryResultFromSnapshot(sessionID, snapshot, input),
	}, nil
}

type serviceSessionInvocationSpecialCase struct{}

func (s serviceSessionInvocationSpecialCase) Active(cfg *interfaces.FactoryConfig) bool {
	return tts.IsPackagedFactory(cfg)
}

func (s serviceSessionInvocationSpecialCase) TerminalFailure(worldState interfaces.FactoryWorldState, requestID string) *invocations.SessionInvocationSpecialFailure {
	_, failure := tts.ClassifyInvocationWait(worldState, requestID, false)
	if failure == nil {
		return nil
	}
	return &invocations.SessionInvocationSpecialFailure{ErrorCode: failure.ErrorCode, Message: failure.Message, FailureClass: failure.FailureClass}
}

func (fs *FactoryService) sessionInvocationWorldState(
	ctx context.Context,
	sessionID string,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return interfaces.FactoryWorldState{}, err
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return interfaces.FactoryWorldState{}, err
	}
	return projections.ReconstructFactoryWorldState(events, selectedTick)
}

// InvocationMetricNormalizationAttempts remains an exported compatibility
// alias while metric-name ownership lives in pkg/invocations.
const InvocationMetricNormalizationAttempts = invocations.InvocationMetricNormalizationAttempts

func (fs *FactoryService) recordInvocationMetric(name string, labels map[string]string) {
	if fs == nil || fs.cfg == nil || fs.cfg.InvocationMetricsRecorder == nil {
		return
	}
	fs.cfg.InvocationMetricsRecorder.RecordInvocationMetric(InvocationMetric{
		Name:   name,
		Labels: labels,
	})
}

func cloneMetricLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func (fs *FactoryService) modelInvocationExecutor(runtimeCfg *factoryconfig.LoadedFactoryConfig, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
	if runtimeCfg == nil || factoryCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	logger := logging.NewZapLogger(fs.logger, fs != nil && fs.coordinatorPolicy().verbose)
	bundle := fs.currentRuntimeBundle()
	var modelDomain localModelDomain
	var workflowContext *factory_context.FactoryContext
	if bundle != nil {
		modelDomain = LocalModelDomain{
			Resources:      bundle.ModelResources,
			Assets:         bundle.ModelAssets,
			Runtime:        bundle.LocalModelRuntime,
			Host:           bundle.ModelHost,
			Manager:        bundle.LocalModels,
			LeaseExecution: bundle.LeaseExecution,
		}
		workflowContext = runtime.WorkflowContext(bundle.Factory)
	}
	executor := buildWorkerExecutor(
		runtimeCfg,
		factoryCfg,
		workerName,
		fs.factoryRunnerID(),
		workflowContext,
		logger,
		fs.providerOverride(),
		nil,
		fs.providerCommandRunnerOverride(),
		fs.commandRunnerOverride(),
		nil,
		nil,
		nil,
		nil,
		time.Now,
		modelDomain,
	)
	workstationExecutor, ok := executor.(*workerexecutor.WorkstationExecutor)
	if !ok || workstationExecutor.Executor == nil {
		return nil, fmt.Errorf("model worker %q does not support direct invocation", workerName)
	}
	return workstationExecutor.Executor, nil
}

func (fs *FactoryService) factoryRunnerID() string {
	if fs == nil {
		return ""
	}
	return fs.coordinatorPolicy().runnerID
}

func (fs *FactoryService) providerOverride() workers.Provider {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().providerOverride
}

func (fs *FactoryService) providerCommandRunnerOverride() workers.CommandRunner {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().providerCommandRunnerOverride
}

func (fs *FactoryService) commandRunnerOverride() workers.CommandRunner {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().commandRunnerOverride
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func classifyInvocationMissingPrimaryResultFromSnapshot(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	input invocations.SessionInvocationWaitInput,
) *invocations.PrimaryResultError {
	if snapshot == nil || strings.TrimSpace(input.RequestID) == "" {
		return nil
	}
	tokens := make([]*interfaces.Token, 0, len(snapshot.Marking.Tokens))
	for _, token := range snapshot.Marking.Tokens {
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool {
		leftID, rightID := "", ""
		if tokens[i] != nil {
			leftID = tokens[i].Color.WorkID
		}
		if tokens[j] != nil {
			rightID = tokens[j].Color.WorkID
		}
		if leftID == rightID {
			return tokenPlaceID(tokens[i]) < tokenPlaceID(tokens[j])
		}
		return leftID < rightID
	})
	for _, wantState := range []string{"blocked", "needs-human"} {
		for _, token := range tokens {
			if token == nil || token.Color.DataType == interfaces.DataTypeResource {
				continue
			}
			if strings.TrimSpace(token.Color.RequestID) != strings.TrimSpace(input.RequestID) {
				continue
			}
			if tokenStateName(token.PlaceID) != wantState {
				continue
			}
			return invocations.ClassifyMissingPrimaryResultWorkItem(
				input.RequestID,
				input.InvocationReturn,
				interfaces.FactoryWorkItem{
					ID:          token.Color.WorkID,
					WorkTypeID:  token.Color.WorkTypeID,
					DisplayName: token.Color.Name,
					PlaceID:     token.PlaceID,
				},
				sessionID,
			)
		}
	}
	return nil
}

func tokenStateName(placeID string) string {
	trimmed := strings.TrimSpace(placeID)
	if trimmed == "" {
		return ""
	}
	if _, suffix, ok := strings.Cut(trimmed, ":"); ok {
		return suffix
	}
	return trimmed
}

func tokenPlaceID(token *interfaces.Token) string {
	if token == nil {
		return ""
	}
	return token.PlaceID
}
