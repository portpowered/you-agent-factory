// backendsizecheck:ignore-file model catalog owns packaged TTS invocation wait and metrics paths until dedicated service seams split.
// pkgmaintcheck:ignore-file-lines model catalog owns packaged TTS invocation wait and metrics paths until dedicated service seams split.
package service

import (
	"context"
	"errors"
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

// InvocationMetric records one emitted invocation counter together with its
// low-cardinality dimensions.
type InvocationMetric struct {
	Name   string
	Labels map[string]string
}

// InvocationMetricsRecorder receives invocation counter emissions from CLI and
// session-runtime boundaries. Implementations should treat each call as a
// single counter increment.
type InvocationMetricsRecorder interface {
	RecordInvocationMetric(InvocationMetric)
}

// ModelPullMetricsRecorder receives managed-runtime pull counter emissions.
// Implementations should treat each call as a single counter increment.
type ModelPullMetricsRecorder interface {
	RecordModelPullMetric(InvocationMetric)
}

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

type sessionInvocationWaitInput struct {
	RequestID        string
	TraceID          string
	InputSource      invocations.InputSourceLabel
	InvocationReturn *interfaces.InvocationReturnConfig
	FactoryConfig    *interfaces.FactoryConfig
	TimeoutMillis    *int64
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
	telemetry := serviceSessionInvocationTelemetry{service: fs}
	return invocations.NewSessionOwner(invocations.SessionOwnerDependencies{
		FactoryConfig: fs.sessionInvocationFactoryConfig,
		SubmitWork:    fs.submitSessionInvocationWork,
		Observe:       fs.observeSessionInvocation,
		Metrics:       telemetry,
		Logger:        telemetry,
		SpecialCase:   serviceSessionInvocationSpecialCase{service: fs},
	})
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
	legacyInput := sessionInvocationWaitInput{
		RequestID:        input.RequestID,
		TraceID:          input.TraceID,
		InputSource:      input.InputSource,
		InvocationReturn: input.InvocationReturn,
		FactoryConfig:    input.FactoryConfig,
		TimeoutMillis:    input.TimeoutMillis,
	}
	return invocations.SessionInvocationObservation{
		WorldState: worldState, FactoryState: snapshot.FactoryState,
		ActiveWork:           snapshotHasActiveWork(snapshot),
		MissingPrimaryResult: classifyInvocationMissingPrimaryResultFromSnapshot(sessionID, snapshot, legacyInput),
	}, nil
}

type serviceSessionInvocationTelemetry struct {
	service *FactoryService
}

func (t serviceSessionInvocationTelemetry) NormalizationAttempt(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel) {
	t.service.recordInvocationMetric(invocationMetricNormalizationAttempts, invocationMetricLabels(cfg, source))
}

func (t serviceSessionInvocationTelemetry) NormalizationFailure(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel, err error) {
	t.service.recordInvocationMetric(invocationMetricNormalizationFailure, mergeMetricLabels(invocationMetricLabels(cfg, source), invocationErrorMetricLabels(err)))
}

func (t serviceSessionInvocationTelemetry) NormalizationSuccess(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel) {
	t.service.recordInvocationMetric(invocationMetricNormalizationSuccess, invocationMetricLabels(cfg, source))
}

func (t serviceSessionInvocationTelemetry) InterpolationFailure(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel, err error) {
	t.service.recordInvocationMetric(invocationMetricInterpolationFailure, mergeMetricLabels(invocationMetricLabels(cfg, source), invocationErrorMetricLabels(err)))
}

func (t serviceSessionInvocationTelemetry) SubmissionFailure(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel, _ error) {
	t.service.recordInvocationMetric(invocationMetricFailure, invocationMetricLabels(cfg, source))
}

func (t serviceSessionInvocationTelemetry) InvocationSubmitted(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel) {
	t.service.recordInvocationMetric(invocationMetricAttempts, invocationMetricLabels(cfg, source))
	if policyModeForInvocation(cfg.InvocationReturn) == invocationPolicyModeFallback {
		t.service.recordInvocationMetric(invocationMetricFallbackPolicyUsed, invocationMetricLabels(cfg, source))
	}
	if tts.IsPackagedFactory(cfg) {
		t.service.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactoryAttempts, source, nil)
	}
}

func (t serviceSessionInvocationTelemetry) InvocationCompleted(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel, result []interfaces.WorkContentPart) {
	t.service.recordInvocationMetric(invocationMetricSuccess, invocationMetricLabels(cfg, source))
	t.service.recordInvocationMetric(invocationMetricResultType, mergeMetricLabels(
		invocationMetricLabels(cfg, source), map[string]string{"result_type": primaryResultMetricType(result)},
	))
}

func (t serviceSessionInvocationTelemetry) InvocationFailed(cfg *interfaces.FactoryConfig, source invocations.InputSourceLabel, errorCode string) {
	t.service.recordInvocationMetric(invocationMetricFailure, invocationMetricLabels(cfg, source))
	if errorCode == string(invocations.PrimaryResultErrorCodeUnresolved) {
		t.service.recordInvocationMetric(invocationMetricUnresolvedPrimary, invocationMetricLabels(cfg, source))
	}
}

func (t serviceSessionInvocationTelemetry) LogArgumentFailure(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.FactoryConfig,
	normalized *invocations.NormalizedArguments,
	err error,
	failureClass string,
) {
	t.service.logInvocationArgumentFailure(sessionID, source, cfg, normalized, err, failureClass)
}

func (t serviceSessionInvocationTelemetry) LogSubmissionFailure(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.FactoryConfig,
	err error,
) {
	t.service.logger.Warn(
		"factory session invocation failed",
		invocationLogFields(
			sessionID,
			source,
			cfg.InvocationReturn,
			nil,
			zap.String("status", string(factoryapi.InvocationTerminalStatusFailed)),
			zap.String("error_code", string(factoryapi.INVOCATIONRUNTIMEFAILURE)),
			zap.String("failure_class", "runtime_failure"),
			zap.Error(err),
		)...,
	)
}

func (t serviceSessionInvocationTelemetry) LogInvocationSubmitted(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.FactoryConfig,
	result interfaces.WorkRequestSubmitResult,
) {
	if tts.IsPackagedFactory(cfg) {
		t.service.logger.Info(
			"packaged tts invocation submitted",
			packagedTTSInvocationLogFields(
				sessionID,
				source,
				cfg.InvocationReturn,
				cfg,
				zap.String("request_id", result.RequestID),
				zap.String("trace_id", result.TraceID),
				zap.String("readiness_outcome", tts.FailureClassLoading),
			)...,
		)
	}
	t.service.logger.Info(
		"factory session invocation submitted",
		invocationLogFields(
			sessionID,
			source,
			cfg.InvocationReturn,
			cfg,
			zap.String("request_id", result.RequestID),
			zap.String("trace_id", result.TraceID),
		)...,
	)
}

func (t serviceSessionInvocationTelemetry) LogInvocationCompleted(sessionID string, input invocations.SessionInvocationWaitInput, selection invocations.PrimaryResultSelection) {
	t.service.logger.Info(
		"factory session invocation completed",
		invocationLogFields(sessionID, input.InputSource, input.InvocationReturn, input.FactoryConfig,
			zap.String("request_id", input.RequestID), zap.String("trace_id", input.TraceID),
			zap.String("status", string(factoryapi.InvocationTerminalStatusCompleted)),
			zap.String("resolved_work_id", selection.WorkID), zap.String("resolved_work_type", selection.WorkTypeName),
			zap.String("resolved_work_name", selection.WorkName), zap.String("resolved_terminal_state", selection.TerminalState),
			zap.String("result_type", primaryResultMetricType(selection.PrimaryResult)),
		)...,
	)
}

func (t serviceSessionInvocationTelemetry) LogInvocationFailed(sessionID string, input invocations.SessionInvocationWaitInput, result invocations.FactoryInvocationResult, failureClass string) {
	t.service.logger.Warn(
		"factory session invocation failed",
		invocationLogFields(sessionID, input.InputSource, input.InvocationReturn, input.FactoryConfig,
			zap.String("request_id", input.RequestID), zap.String("trace_id", input.TraceID),
			zap.String("status", string(result.Status)), zap.String("error_code", result.ErrorCode),
			zap.String("failure_class", failureClass),
		)...,
	)
}

type serviceSessionInvocationSpecialCase struct{ service *FactoryService }

func (s serviceSessionInvocationSpecialCase) Active(cfg *interfaces.FactoryConfig) bool {
	return tts.IsPackagedFactory(cfg)
}

func (s serviceSessionInvocationSpecialCase) InvocationActive(sessionID string, input invocations.SessionInvocationWaitInput) {
	s.service.logPackagedTTSInvocationLoading(sessionID, serviceWaitInput(input))
}

func (s serviceSessionInvocationSpecialCase) InvocationCompleted(sessionID string, input invocations.SessionInvocationWaitInput, selection invocations.PrimaryResultSelection) {
	s.service.logPackagedTTSInvocationCompleted(sessionID, serviceWaitInput(input), selection)
}

func (s serviceSessionInvocationSpecialCase) TerminalFailure(worldState interfaces.FactoryWorldState, requestID string) *invocations.SessionInvocationSpecialFailure {
	_, failure := tts.ClassifyInvocationWait(worldState, requestID, false)
	if failure == nil {
		return nil
	}
	return &invocations.SessionInvocationSpecialFailure{ErrorCode: failure.ErrorCode, Message: failure.Message, FailureClass: failure.FailureClass}
}

func (s serviceSessionInvocationSpecialCase) InvocationFailed(sessionID string, input invocations.SessionInvocationWaitInput, failure invocations.SessionInvocationSpecialFailure) {
	s.service.handlePackagedTTSInvocationFailure(sessionID, serviceWaitInput(input), &tts.InvocationFailure{
		ErrorCode: failure.ErrorCode, Message: failure.Message, FailureClass: failure.FailureClass,
	})
}

func serviceWaitInput(input invocations.SessionInvocationWaitInput) sessionInvocationWaitInput {
	return sessionInvocationWaitInput{
		RequestID: input.RequestID, TraceID: input.TraceID, InputSource: input.InputSource,
		InvocationReturn: input.InvocationReturn, FactoryConfig: input.FactoryConfig, TimeoutMillis: input.TimeoutMillis,
	}
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

const (
	invocationPolicySubmittedWorkTerminal = "SUBMITTED_WORK_TERMINAL"
	invocationPolicyExplicit              = "EXPLICIT"

	// InvocationMetricNormalizationAttempts is the exported metric name for
	// invocation input normalization attempts recorded by the service layer.
	InvocationMetricNormalizationAttempts = "invocation.normalization_attempts"

	invocationMetricNormalizationAttempts = InvocationMetricNormalizationAttempts
	invocationMetricNormalizationSuccess  = "invocation.normalization_success"
	invocationMetricNormalizationFailure  = "invocation.normalization_failure"
	invocationMetricInterpolationFailure  = "invocation.interpolation_failure"
	invocationMetricAttempts              = "invocation.attempts"
	invocationMetricSuccess               = "invocation.success"
	invocationMetricFailure               = "invocation.failure"
	invocationMetricUnresolvedPrimary     = "invocation.unresolved_primary"
	invocationMetricFallbackPolicyUsed    = "invocation.fallback_policy_used"
	invocationMetricResultType            = "invocation.result_type"

	invocationPolicyModeAuthored = "authored"
	invocationPolicyModeFallback = "fallback"
)

func (fs *FactoryService) logInvocationArgumentFailure(
	sessionID string,
	source invocations.InputSourceLabel,
	factoryCfg *interfaces.FactoryConfig,
	normalized *invocations.NormalizedArguments,
	err error,
	failureClass string,
) {
	fields := []zap.Field{
		zap.String("status", string(factoryapi.InvocationTerminalStatusFailed)),
		zap.String("failure_class", failureClass),
		zap.Error(err),
	}
	var argumentErr *invocations.ArgumentError
	if errors.As(err, &argumentErr) {
		if code := strings.TrimSpace(string(argumentErr.Code)); code != "" {
			fields = append(fields, zap.String("error_code", code))
		}
		if parameter := strings.TrimSpace(argumentErr.Parameter); parameter != "" {
			fields = append(fields, zap.String("argument_name", parameter))
		}
		if argument := strings.TrimSpace(argumentErr.Argument); argument != "" {
			fields = append(fields, zap.String("argument_key", argument))
		}
		if sourceKind := strings.TrimSpace(string(argumentErr.SourceKind)); sourceKind != "" {
			fields = append(fields, zap.String("argument_source_kind", sourceKind))
		} else if kinds := invocationArgumentSourceKinds(normalized, argumentErr.Parameter); kinds != "" {
			fields = append(fields, zap.String("argument_source_kind", kinds))
		}
		if redacted, valueCount := invocationArgumentRedactionState(normalized, argumentErr.Parameter); redacted || valueCount > 0 {
			fields = append(fields,
				zap.Bool("argument_value_redacted", redacted),
				zap.Int("argument_value_count", valueCount),
			)
		}
	}
	fs.logger.Warn(
		"factory session invocation argument failure",
		invocationLogFields(sessionID, source, factoryCfg.InvocationReturn, factoryCfg, fields...)...,
	)
}

func (fs *FactoryService) recordInvocationMetric(name string, labels map[string]string) {
	if fs == nil || fs.cfg == nil || fs.cfg.InvocationMetricsRecorder == nil {
		return
	}
	fs.cfg.InvocationMetricsRecorder.RecordInvocationMetric(InvocationMetric{
		Name:   name,
		Labels: cloneMetricLabels(labels),
	})
}

func invocationArgumentSourceKinds(normalized *invocations.NormalizedArguments, parameter string) string {
	argument := invocationNormalizedArgument(normalized, parameter)
	if argument == nil || len(argument.Sources) == 0 {
		return ""
	}
	kinds := make([]string, 0, len(argument.Sources))
	seen := map[string]struct{}{}
	for _, source := range argument.Sources {
		kind := strings.TrimSpace(string(source.Kind))
		if kind == "" {
			continue
		}
		if _, exists := seen[kind]; exists {
			continue
		}
		seen[kind] = struct{}{}
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ",")
}

func invocationArgumentRedactionState(normalized *invocations.NormalizedArguments, parameter string) (bool, int) {
	argument := invocationNormalizedArgument(normalized, parameter)
	if argument == nil {
		return false, 0
	}
	redacted := argument.Sensitive
	for _, source := range argument.Sources {
		if source.Redact {
			redacted = true
			break
		}
	}
	return redacted, len(argument.Values)
}

func invocationNormalizedArgument(normalized *invocations.NormalizedArguments, parameter string) *invocations.NormalizedArgument {
	if normalized == nil || strings.TrimSpace(parameter) == "" {
		return nil
	}
	argument, ok := normalized.Arguments[strings.TrimSpace(parameter)]
	if !ok {
		return nil
	}
	return &argument
}

func invocationLogFields(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.InvocationReturnConfig,
	factoryCfg *interfaces.FactoryConfig,
	extra ...zap.Field,
) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("input_source", string(source)),
		zap.String("invocation_return_policy", invocationPolicyName(cfg)),
		zap.String("invocation_return_policy_mode", policyModeForInvocation(cfg)),
		zap.String("policy_resolution_path", invocationPolicyResolutionPath(cfg)),
	}
	fields = append(fields, invocationFactoryLogFields(factoryCfg)...)
	return append(fields, extra...)
}

func invocationMetricLabels(factoryCfg *interfaces.FactoryConfig, source invocations.InputSourceLabel) map[string]string {
	labels := map[string]string{"input_source": string(source)}
	for key, value := range invocationFactoryMetricLabels(factoryCfg) {
		labels[key] = value
	}
	return labels
}

func mergeMetricLabels(parts ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, part := range parts {
		for key, value := range part {
			merged[key] = value
		}
	}
	return merged
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

func invocationFactoryMetricLabels(factoryCfg *interfaces.FactoryConfig) map[string]string {
	if factoryCfg == nil {
		return nil
	}
	labels := map[string]string{}
	if factoryName := strings.TrimSpace(factoryCfg.Name); factoryName != "" {
		labels["factory_name"] = factoryName
	}
	if factoryProject := strings.TrimSpace(factoryCfg.Project); factoryProject != "" {
		labels["factory_project"] = factoryProject
	}
	if signatureHash := invocations.InvocationSignatureHash(factoryCfg.InvocationSignature); signatureHash != "" {
		labels["signature_hash"] = signatureHash
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func invocationFactoryLogFields(factoryCfg *interfaces.FactoryConfig) []zap.Field {
	if factoryCfg == nil {
		return nil
	}
	fields := []zap.Field{}
	if factoryName := strings.TrimSpace(factoryCfg.Name); factoryName != "" {
		fields = append(fields, zap.String("factory_name", factoryName))
	}
	if factoryProject := strings.TrimSpace(factoryCfg.Project); factoryProject != "" {
		fields = append(fields, zap.String("factory_project", factoryProject))
	}
	if signatureHash := invocations.InvocationSignatureHash(factoryCfg.InvocationSignature); signatureHash != "" {
		fields = append(fields, zap.String("signature_hash", signatureHash))
	}
	return fields
}

func invocationErrorMetricLabels(err error) map[string]string {
	var argumentErr *invocations.ArgumentError
	if !errors.As(err, &argumentErr) {
		return nil
	}
	if code := strings.TrimSpace(string(argumentErr.Code)); code != "" {
		return map[string]string{"error_code": code}
	}
	return nil
}

func invocationPolicyName(cfg *interfaces.InvocationReturnConfig) string {
	policy := strings.TrimSpace(invocationPolicyFromConfig(cfg))
	if policy == "" {
		return invocationPolicySubmittedWorkTerminal
	}
	return policy
}

func policyModeForInvocation(cfg *interfaces.InvocationReturnConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Policy) == "" {
		return invocationPolicyModeFallback
	}
	return invocationPolicyModeAuthored
}

func invocationPolicyResolutionPath(cfg *interfaces.InvocationReturnConfig) string {
	if invocationPolicyName(cfg) == invocationPolicyExplicit {
		return "explicit_scoped_terminal_match"
	}
	return "submitted_work_terminal"
}

func invocationPolicyFromConfig(cfg *interfaces.InvocationReturnConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.Policy
}

func primaryResultMetricType(parts []interfaces.WorkContentPart) string {
	if len(parts) == 0 {
		return "empty"
	}
	types := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(string(part.Type.Normalized()))
		if partType == "" {
			partType = "unknown"
		}
		types[partType] = struct{}{}
	}
	if len(types) == 1 {
		for partType := range types {
			return partType
		}
	}
	names := make([]string, 0, len(types))
	for partType := range types {
		names = append(names, partType)
	}
	sort.Strings(names)
	return "mixed:" + strings.Join(names, "+")
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
	input sessionInvocationWaitInput,
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

func (fs *FactoryService) recordPackagedTTSInvocationMetric(
	name string,
	source invocations.InputSourceLabel,
	extra map[string]string,
) {
	labels := mergeMetricLabels(
		invocationMetricLabels(nil, source),
		map[string]string{"packaged_factory": tts.PackagedFactoryName},
		extra,
	)
	fs.recordInvocationMetric(name, labels)
}

func (fs *FactoryService) handlePackagedTTSInvocationFailure(
	sessionID string,
	input sessionInvocationWaitInput,
	failure *tts.InvocationFailure,
) apisurface.FactoryInvocationResult {
	result := apisurface.FactoryInvocationResult{
		RequestID: input.RequestID,
		TraceID:   input.TraceID,
		Status:    factoryapi.InvocationTerminalStatusFailed,
		ErrorCode: failure.ErrorCode,
		Message:   failure.Message,
	}
	fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactoryFailure, input.InputSource, map[string]string{
		"failure_class": failure.FailureClass,
	})
	if failure.FailureClass == tts.FailureClassModelNotReady {
		fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactoryNotReady, input.InputSource, nil)
	}
	fs.logger.Warn(
		"packaged tts invocation failed",
		packagedTTSInvocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			input.FactoryConfig,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(result.Status)),
			zap.String("error_code", result.ErrorCode),
			zap.String("failure_class", failure.FailureClass),
			zap.String("readiness_outcome", failure.FailureClass),
		)...,
	)
	return result
}

func (fs *FactoryService) logPackagedTTSInvocationLoading(
	sessionID string,
	input sessionInvocationWaitInput,
) {
	fs.logger.Info(
		"packaged tts invocation loading",
		packagedTTSInvocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			input.FactoryConfig,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("readiness_outcome", tts.FailureClassLoading),
		)...,
	)
}

func (fs *FactoryService) logPackagedTTSInvocationCompleted(
	sessionID string,
	input sessionInvocationWaitInput,
	selection invocations.PrimaryResultSelection,
) {
	fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactorySuccess, input.InputSource, map[string]string{
		"readiness_outcome": tts.FailureClassSuccess,
	})
	fs.logger.Info(
		"packaged tts invocation completed",
		packagedTTSInvocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			input.FactoryConfig,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(factoryapi.InvocationTerminalStatusCompleted)),
			zap.String("resolved_work_id", selection.WorkID),
			zap.String("resolved_work_type", selection.WorkTypeName),
			zap.String("readiness_outcome", tts.FailureClassSuccess),
		)...,
	)
}

func packagedTTSInvocationLogFields(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.InvocationReturnConfig,
	factoryCfg *interfaces.FactoryConfig,
	extra ...zap.Field,
) []zap.Field {
	fields := invocationLogFields(sessionID, source, cfg, factoryCfg,
		zap.String("packaged_factory_name", tts.PackagedFactoryName),
		zap.String("tts_backend", tts.BackendRuntimeLabel()),
	)
	return append(fields, extra...)
}
