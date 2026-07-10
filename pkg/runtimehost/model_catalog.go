package runtimehost

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

func (fs *Host) requireModelService() apisurface.ModelAPI {
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

func (fs *Host) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	return fs.requireModelService().ListModels(ctx)
}

func (fs *Host) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	return fs.requireModelService().GetModel(ctx, modelName)
}

type modelAssetPuller = localmodels.AssetPuller

func newModelAssetPuller(cacheDir string) modelAssetPuller {
	return localmodels.NewAssetPuller(cacheDir)
}

func (fs *Host) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	return fs.requireModelService().PullModel(ctx, modelName)
}

func (fs *Host) modelAssetPuller() modelAssetPuller {
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

func (fs *Host) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return fs.requireModelService().InvokeModel(ctx, modelName, request)
}

// modelServiceHost adapts Host runtime seams for pkg/models/service wiring.
type modelServiceHost struct {
	*Host
}

var _ modelsservice.Host = modelServiceHost{}

func (h modelServiceHost) RuntimeConfig() func() *factoryconfig.LoadedFactoryConfig {
	if h.Host == nil {
		return func() *factoryconfig.LoadedFactoryConfig { return nil }
	}
	return h.Host.currentRuntimeConfig
}

func (h modelServiceHost) ModelHost() func() modelhost.Host {
	if h.Host == nil {
		return func() modelhost.Host { return nil }
	}
	return h.Host.modelHost
}

func (h modelServiceHost) ModelAssetPuller() func() localmodels.AssetPuller {
	if h.Host == nil {
		return func() localmodels.AssetPuller { return nil }
	}
	return h.Host.modelAssetPuller
}

func (h modelServiceHost) Logger() func() *zap.Logger {
	if h.Host == nil {
		return func() *zap.Logger { return nil }
	}
	return func() *zap.Logger { return h.Host.logger }
}

func (h modelServiceHost) ModelPullMetrics() func() modelsservice.PullMetricsRecorder {
	if h.Host == nil {
		return func() modelsservice.PullMetricsRecorder { return nil }
	}
	return func() modelsservice.PullMetricsRecorder {
		recorder := h.Host.modelPullMetricsRecorder()
		if recorder == nil {
			return nil
		}
		return modelPullMetricsHostAdapter{inner: recorder}
	}
}

func (h modelServiceHost) ModelInvocationExecutor() modelsservice.ModelInvocationExecutor {
	if h.Host == nil {
		return nil
	}
	return h.Host.modelInvocationExecutor
}

func (h modelServiceHost) FactoryRunnerID() func() string {
	if h.Host == nil {
		return func() string { return "" }
	}
	return h.Host.factoryRunnerID
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

func wireModelServiceCollaborator(fs *Host, cfg *Config) apisurface.ModelAPI {
	if cfg != nil && cfg.ModelAPI != nil {
		return cfg.ModelAPI
	}
	return modelsservice.NewFromHost(modelServiceHost{Host: fs})
}

func (fs *Host) modelHost() modelhost.Host {
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

func (fs *Host) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	return fs.sessionInvocationOwner().InvokeFactorySession(ctx, sessionID, request)
}

func (fs *Host) sessionInvocationOwner() invocations.SessionInvoker {
	if fs.sessionInvoker != nil {
		return fs.sessionInvoker
	}
	return invocations.NewSessionOwner(invocations.SessionOwnerDependencies{
		FactoryConfig: fs.sessionInvocationFactoryConfig,
		SubmitWork:    fs.submitOwnedSessionInvocationWork,
		Observe:       fs.observeSessionInvocation,
		Telemetry:     fs.sessionInvocationTelemetry(),
		SpecialCase:   hostSessionInvocationSpecialCase{},
	})
}

func (fs *Host) sessionInvocationTelemetry() invocations.SessionInvocationTelemetry {
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

func (fs *Host) recordSessionInvocationLog(record invocations.SessionInvocationLogRecord) {
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

func (fs *Host) sessionInvocationFactoryConfig(sessionID string) (*interfaces.FactoryConfig, error) {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil || runtimeCfg == nil {
		return nil, err
	}
	return runtimeCfg.FactoryConfig(), nil
}

func (fs *Host) submitOwnedSessionInvocationWork(ctx context.Context, sessionID string, request interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
	return fs.SubmitWorkRequestForSession(ctx, sessionID, factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{request}))
}

func (fs *Host) observeSessionInvocation(ctx context.Context, sessionID string, input invocations.SessionInvocationWaitInput) (invocations.SessionInvocationObservation, error) {
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

type hostSessionInvocationSpecialCase struct{}

func (s hostSessionInvocationSpecialCase) Active(cfg *interfaces.FactoryConfig) bool {
	return tts.IsPackagedFactory(cfg)
}
func (s hostSessionInvocationSpecialCase) TerminalFailure(worldState interfaces.FactoryWorldState, requestID string) *invocations.SessionInvocationSpecialFailure {
	_, failure := tts.ClassifyInvocationWait(worldState, requestID, false)
	if failure == nil {
		return nil
	}
	return &invocations.SessionInvocationSpecialFailure{ErrorCode: failure.ErrorCode, Message: failure.Message, FailureClass: failure.FailureClass}
}
func (fs *Host) sessionInvocationWorldState(
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

func (fs *Host) recordInvocationMetric(name string, labels map[string]string) {
	if fs == nil || fs.cfg == nil || fs.cfg.InvocationMetricsRecorder == nil {
		return
	}
	fs.cfg.InvocationMetricsRecorder.RecordInvocationMetric(InvocationMetric{
		Name:   name,
		Labels: labels,
	})
}

func (fs *Host) modelInvocationExecutor(runtimeCfg *factoryconfig.LoadedFactoryConfig, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
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

func (fs *Host) factoryRunnerID() string {
	if fs == nil {
		return ""
	}
	return fs.coordinatorPolicy().runnerID
}

func (fs *Host) providerOverride() workers.Provider {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().providerOverride
}

func (fs *Host) providerCommandRunnerOverride() workers.CommandRunner {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().providerCommandRunnerOverride
}

func (fs *Host) commandRunnerOverride() workers.CommandRunner {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().commandRunnerOverride
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
