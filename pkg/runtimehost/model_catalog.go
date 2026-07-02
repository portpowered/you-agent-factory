// backendsizecheck:ignore-file model catalog owns packaged TTS invocation wait and metrics paths until dedicated service seams split.
// pkgmaintcheck:ignore-file-lines model catalog owns packaged TTS invocation wait and metrics paths until dedicated service seams split.
package runtimehost

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
	"github.com/portpowered/infinite-you/pkg/config/factoryrun"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workcontent"
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

// NewModelPullMetricsHostAdapter adapts host pull metrics recorders for pkg/models/service.
func NewModelPullMetricsHostAdapter(recorder ModelPullMetricsRecorder) modelsservice.PullMetricsRecorder {
	if recorder == nil {
		return nil
	}
	return modelPullMetricsHostAdapter{inner: recorder}
}

// CloneMetricLabels returns a defensive copy of metric labels.
func CloneMetricLabels(labels map[string]string) map[string]string {
	return cloneMetricLabels(labels)
}

func wireModelServiceCollaborator(fs *Host, cfg *Config) apisurface.ModelAPI {
	if cfg != nil && cfg.ModelAPI != nil {
		return cfg.ModelAPI
	}
	return modelsservice.NewFromHost(modelServiceHost{Host: fs})
}

// ProvideModelServiceCollaborator constructs the model-domain collaborator for a
// built Host shell.
func ProvideModelServiceCollaborator(
	shell HostShell,
	cfg *Config,
) apisurface.ModelAPI {
	return wireModelServiceCollaborator(shell.Host, cfg)
}

// AttachModelServiceCollaborator assigns the model-domain collaborator on the
// service shell and returns the assembled Host.
func AttachModelServiceCollaborator(
	shell HostShell,
	modelAPI apisurface.ModelAPI,
) *Host {
	if shell.Host != nil {
		shell.Host.modelService = modelAPI
	}
	return shell.Host
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

type sessionInvocationWaitInput struct {
	RequestID        string
	TraceID          string
	InputSource      invocations.InputSourceLabel
	InvocationReturn *interfaces.InvocationReturnConfig
	FactoryConfig    *interfaces.FactoryConfig
	TimeoutMillis    *int64
}

const invocationInputSourceStructuredArgs invocations.InputSourceLabel = "signature_args"

type resolvedSessionInvocationInput struct {
	Source              invocations.InputSourceLabel
	Content             []interfaces.WorkContentPart
	NormalizedArguments *invocations.NormalizedArguments
}

func (fs *Host) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("factory session runtime config is unavailable")
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	sourceHint := sessionInvocationSourceHint(request)
	fs.recordInvocationMetric(invocationMetricNormalizationAttempts, invocationMetricLabels(factoryCfg, sourceHint))

	resolved, err := resolveSessionInvocationInput(factoryCfg, request)
	if err != nil {
		fs.recordInvocationMetric(
			invocationMetricNormalizationFailure,
			mergeMetricLabels(invocationMetricLabels(factoryCfg, sourceHint), invocationErrorMetricLabels(err)),
		)
		fs.logInvocationArgumentFailure(sessionID, sourceHint, factoryCfg, nil, err, "normalization_failure")
		return apisurface.FactoryInvocationResult{}, err
	}
	fs.recordInvocationMetric(invocationMetricNormalizationSuccess, invocationMetricLabels(factoryCfg, resolved.Source))
	if err := validateSessionInvocationInterpolation(factoryCfg, resolved.NormalizedArguments); err != nil {
		return apisurface.FactoryInvocationResult{}, fs.handleSessionInvocationInterpolationFailure(
			sessionID,
			factoryCfg,
			resolved,
			err,
		)
	}

	submitResult, err := fs.submitSessionInvocationRequest(ctx, sessionID, request, factoryCfg, resolved)
	if err != nil {
		fs.logInvocationFailure(
			sessionID,
			resolved.Source,
			factoryCfg.InvocationReturn,
			invocationMetricLabels(factoryCfg, resolved.Source),
			"runtime_failure",
			err,
		)
		return apisurface.FactoryInvocationResult{}, err
	}
	fs.recordSuccessfulSessionInvocation(sessionID, factoryCfg, resolved.Source, submitResult)
	return fs.waitForSessionInvocationResult(
		ctx,
		sessionID,
		sessionInvocationWaitInput{
			RequestID:        submitResult.RequestID,
			TraceID:          submitResult.TraceID,
			InputSource:      resolved.Source,
			InvocationReturn: factoryCfg.InvocationReturn,
			FactoryConfig:    factoryCfg,
			TimeoutMillis:    request.TimeoutMillis,
		},
	)
}

func (fs *Host) handleSessionInvocationInterpolationFailure(
	sessionID string,
	factoryCfg *interfaces.FactoryConfig,
	resolved resolvedSessionInvocationInput,
	err error,
) error {
	fs.recordInvocationMetric(
		invocationMetricInterpolationFailure,
		mergeMetricLabels(invocationMetricLabels(factoryCfg, resolved.Source), invocationErrorMetricLabels(err)),
	)
	fs.logInvocationArgumentFailure(sessionID, resolved.Source, factoryCfg, resolved.NormalizedArguments, err, "interpolation_failure")
	return normalizeSessionInvocationError(err)
}

func (fs *Host) submitSessionInvocationRequest(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
	factoryCfg *interfaces.FactoryConfig,
	resolved resolvedSessionInvocationInput,
) (interfaces.WorkRequestSubmitResult, error) {
	workTypeName, err := factoryrun.DefaultHandlingWorkTypeName(factoryCfg)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, fmt.Errorf("resolve invocation work type: %w", err)
	}
	requestID := strings.TrimSpace(stringValue(request.RequestId))
	submitRequest := interfaces.SubmitRequest{
		RequestID:           requestID,
		WorkTypeID:          workTypeName,
		Content:             resolved.Content,
		InvocationArguments: invocationArgumentsForSession(factoryCfg, resolved.NormalizedArguments),
	}
	return fs.SubmitWorkRequestForSession(
		ctx,
		sessionID,
		factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitRequest}),
	)
}

func (fs *Host) recordSuccessfulSessionInvocation(
	sessionID string,
	factoryCfg *interfaces.FactoryConfig,
	source invocations.InputSourceLabel,
	submitResult interfaces.WorkRequestSubmitResult,
) {
	fs.recordInvocationMetric(invocationMetricAttempts, invocationMetricLabels(factoryCfg, source))
	if policyModeForInvocation(factoryCfg.InvocationReturn) == invocationPolicyModeFallback {
		fs.recordInvocationMetric(invocationMetricFallbackPolicyUsed, invocationMetricLabels(factoryCfg, source))
	}
	if tts.IsPackagedFactory(factoryCfg) {
		fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactoryAttempts, source, nil)
		fs.logger.Info(
			"packaged tts invocation submitted",
			packagedTTSInvocationLogFields(
				sessionID,
				source,
				factoryCfg.InvocationReturn,
				factoryCfg,
				zap.String("request_id", submitResult.RequestID),
				zap.String("trace_id", submitResult.TraceID),
				zap.String("readiness_outcome", tts.FailureClassLoading),
			)...,
		)
	}
	fs.logger.Info(
		"factory session invocation submitted",
		invocationLogFields(
			sessionID,
			source,
			factoryCfg.InvocationReturn,
			factoryCfg,
			zap.String("request_id", submitResult.RequestID),
			zap.String("trace_id", submitResult.TraceID),
		)...,
	)
}

func resolveSessionInvocationInput(
	cfg *interfaces.FactoryConfig,
	request factoryapi.InvocationRequest,
) (resolvedSessionInvocationInput, error) {
	content, err := sessionInvocationCompatibilityContent(request)
	if err != nil {
		return resolvedSessionInvocationInput{}, err
	}
	directArgs, err := sessionInvocationStructuredArgs(request)
	if err != nil {
		return resolvedSessionInvocationInput{}, err
	}
	argsProvided := request.Args != nil
	signature := invocationSignatureFromFactoryConfig(cfg)

	if !argsProvided {
		return resolveCompatibilitySessionInvocationInput(content)
	}
	return resolveStructuredSessionInvocationInput(signature, directArgs, content)
}

func resolveCompatibilitySessionInvocationInput(
	content []interfaces.WorkContentPart,
) (resolvedSessionInvocationInput, error) {
	if len(content) == 0 {
		return resolvedSessionInvocationInput{}, &apisurface.RequestValidationError{
			Message: "content is required when args are omitted",
		}
	}
	normalized, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		CompatibilityContent: content,
	})
	if err != nil {
		return resolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	if normalized.CompatibilityInput == nil {
		return resolvedSessionInvocationInput{}, &apisurface.RequestValidationError{
			Message: "content did not resolve to one logical invocation input",
		}
	}
	return resolvedSessionInvocationInput{
		Source:              normalized.CompatibilityInput.Source,
		Content:             normalized.CompatibilityInput.Content,
		NormalizedArguments: &normalized,
	}, nil
}

func resolveStructuredSessionInvocationInput(
	signature *interfaces.InvocationSignatureConfig,
	directArgs []invocations.NamedArgumentInput,
	content []interfaces.WorkContentPart,
) (resolvedSessionInvocationInput, error) {
	if signature == nil {
		return resolvedSessionInvocationInput{}, &invocations.ArgumentError{
			Code:     invocations.ArgumentErrorCodeInvalidActiveSignature,
			Message:  "named arguments require a factory invocationSignature",
			Argument: firstStructuredArgumentKey(directArgs),
		}
	}

	normalized, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:            signature,
		DirectArgs:           directArgs,
		CompatibilityContent: content,
	})
	if err != nil {
		return resolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	source := invocationInputSourceStructuredArgs
	if normalized.CompatibilityInput != nil {
		source = normalized.CompatibilityInput.Source
	}
	return resolvedSessionInvocationInput{
		Source:              source,
		Content:             nil,
		NormalizedArguments: &normalized,
	}, nil
}

func sessionInvocationCompatibilityContent(request factoryapi.InvocationRequest) ([]interfaces.WorkContentPart, error) {
	if request.Content == nil {
		if request.SourceKind != nil && *request.SourceKind != factoryapi.InvocationInputSourceKindText {
			return nil, &apisurface.RequestValidationError{Message: "sourceKind must be text"}
		}
		return nil, nil
	}
	if request.SourceKind == nil || *request.SourceKind != factoryapi.InvocationInputSourceKindText {
		return nil, &apisurface.RequestValidationError{Message: "sourceKind must be text"}
	}
	return workcontent.PartsFromGenerated(request.Content), nil
}

func sessionInvocationStructuredArgs(request factoryapi.InvocationRequest) ([]invocations.NamedArgumentInput, error) {
	if request.Args == nil {
		return nil, nil
	}
	directArgs, err := invocations.NamedArgumentInputsFromAnyMap(*request.Args)
	if err != nil {
		return nil, &apisurface.RequestValidationError{Message: err.Error()}
	}
	return directArgs, nil
}

func firstStructuredArgumentKey(arguments []invocations.NamedArgumentInput) string {
	if len(arguments) == 0 {
		return ""
	}
	return strings.TrimSpace(arguments[0].Key)
}

func normalizeSessionInvocationError(err error) error {
	var validationErr *invocations.TextContentValidationError
	if errors.As(err, &validationErr) {
		return &apisurface.RequestValidationError{
			Message: validationErr.Message,
		}
	}
	return err
}

func validateSessionInvocationInterpolation(
	cfg *interfaces.FactoryConfig,
	normalized *invocations.NormalizedArguments,
) error {
	return invocations.ValidateInvocationInterpolation(cfg, invocationArgumentsForSession(cfg, normalized))
}

func invocationArgumentsForSession(
	cfg *interfaces.FactoryConfig,
	normalized *invocations.NormalizedArguments,
) *interfaces.InvocationArguments {
	if cfg == nil {
		return nil
	}
	return invocations.RuntimeInvocationArguments(cfg.InvocationSignature, normalized)
}

func invocationSignatureFromFactoryConfig(cfg *interfaces.FactoryConfig) *interfaces.InvocationSignatureConfig {
	if cfg == nil {
		return nil
	}
	return cfg.InvocationSignature
}

func (fs *Host) waitForSessionInvocationResult(
	ctx context.Context,
	sessionID string,
	input sessionInvocationWaitInput,
) (apisurface.FactoryInvocationResult, error) {
	waitCtx, cancel := invocationWaitContext(ctx, input.TimeoutMillis)
	defer cancel()

	result := apisurface.FactoryInvocationResult{
		RequestID: input.RequestID,
		TraceID:   input.TraceID,
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	packagedTTSInvocation := fs.isPackagedTTSSession(sessionID)
	loggedPackagedLoading := false

	for {
		tick := fs.processInvocationWaitTick(
			waitCtx,
			sessionID,
			input,
			result,
			packagedTTSInvocation,
			loggedPackagedLoading,
		)
		loggedPackagedLoading = tick.loggedLoading
		if tick.done {
			return tick.result, tick.err
		}

		select {
		case <-waitCtx.Done():
			return fs.invocationWaitTimedOut(sessionID, input, result, waitCtx.Err()), nil
		case <-ticker.C:
		}
	}
}

func (fs *Host) handleInvocationWaitError(
	result apisurface.FactoryInvocationResult,
	err error,
) (apisurface.FactoryInvocationResult, error) {
	if statusResult, ok := invocationContextResult(result, err); ok {
		return statusResult, nil
	}
	return apisurface.FactoryInvocationResult{}, err
}

func (fs *Host) handleInvocationSelectionSuccess(
	sessionID string,
	input sessionInvocationWaitInput,
	selection invocations.PrimaryResultSelection,
) apisurface.FactoryInvocationResult {
	resultType := primaryResultMetricType(selection.PrimaryResult)
	result := apisurface.FactoryInvocationResult{
		RequestID:     input.RequestID,
		TraceID:       input.TraceID,
		Status:        factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: selection.PrimaryResult,
	}
	fs.recordInvocationMetric(invocationMetricSuccess, invocationMetricLabels(input.FactoryConfig, input.InputSource))
	fs.recordInvocationMetric(
		invocationMetricResultType,
		mergeMetricLabels(
			invocationMetricLabels(input.FactoryConfig, input.InputSource),
			map[string]string{"result_type": resultType},
		),
	)
	fs.logger.Info(
		"factory session invocation completed",
		invocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			input.FactoryConfig,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(result.Status)),
			zap.String("resolved_work_id", selection.WorkID),
			zap.String("resolved_work_type", selection.WorkTypeName),
			zap.String("resolved_work_name", selection.WorkName),
			zap.String("resolved_terminal_state", selection.TerminalState),
			zap.String("result_type", resultType),
		)...,
	)
	return result
}

func (fs *Host) handleInvocationPrimaryResultFailure(
	sessionID string,
	input sessionInvocationWaitInput,
	primaryErr *invocations.PrimaryResultError,
) apisurface.FactoryInvocationResult {
	result := apisurface.FactoryInvocationResult{
		RequestID: input.RequestID,
		TraceID:   input.TraceID,
		Status:    factoryapi.InvocationTerminalStatusFailed,
		ErrorCode: string(primaryErr.Code),
		Message:   primaryErr.Message,
		SessionID: primaryErr.Context.SessionID,
		WorkID:    primaryErr.Context.WorkID,
		WorkName:  primaryErr.Context.WorkName,
		WorkState: primaryErr.Context.WorkState,
	}
	failureClass := invocationFailureClassForPrimaryResultError(primaryErr.Code)
	fs.recordInvocationMetric(invocationMetricFailure, invocationMetricLabels(input.FactoryConfig, input.InputSource))
	if primaryErr.Code == invocations.PrimaryResultErrorCodeUnresolved {
		fs.recordInvocationMetric(invocationMetricUnresolvedPrimary, invocationMetricLabels(input.FactoryConfig, input.InputSource))
	}
	fs.logger.Warn(
		"factory session invocation failed",
		invocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			input.FactoryConfig,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(result.Status)),
			zap.String("error_code", result.ErrorCode),
			zap.String("failure_class", failureClass),
		)...,
	)
	return result
}

func invocationFailureClassForPrimaryResultError(code invocations.PrimaryResultErrorCode) string {
	switch code {
	case invocations.PrimaryResultErrorCodeFailed:
		return "failed"
	case invocations.PrimaryResultErrorCodePaused:
		return "paused"
	case invocations.PrimaryResultErrorCodeInterrupted:
		return "interrupted"
	case invocations.PrimaryResultErrorCodeBlocked:
		return "blocked"
	case invocations.PrimaryResultErrorCodeNeedsHuman:
		return "needs_human"
	default:
		return "unresolved_primary"
	}
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

func invocationContextResult(
	result apisurface.FactoryInvocationResult,
	err error,
) (apisurface.FactoryInvocationResult, bool) {
	if err == nil {
		return apisurface.FactoryInvocationResult{}, false
	}
	return invocationContextTerminalResult(result, err), errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func invocationContextTerminalResult(
	result apisurface.FactoryInvocationResult,
	err error,
) apisurface.FactoryInvocationResult {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.Status = factoryapi.InvocationTerminalStatusTimedOut
		result.ErrorCode = string(factoryapi.INVOCATIONTIMEDOUT)
		result.Message = "invocation timed out while waiting for primary result"
	case errors.Is(err, context.Canceled):
		result.Status = factoryapi.InvocationTerminalStatusCanceled
		result.ErrorCode = string(factoryapi.INVOCATIONCANCELED)
		result.Message = "invocation was canceled while waiting for primary result"
	default:
		result.Status = factoryapi.InvocationTerminalStatusFailed
		result.ErrorCode = string(factoryapi.INVOCATIONRUNTIMEFAILURE)
		result.Message = strings.TrimSpace(err.Error())
	}
	return result
}

const (
	invocationPolicySubmittedWorkTerminal = "SUBMITTED_WORK_TERMINAL"
	invocationPolicyExplicit              = "EXPLICIT"

	invocationPolicyModeAuthored = "authored"
	invocationPolicyModeFallback = "fallback"
)

func (fs *Host) logInvocationTerminalResult(
	sessionID string,
	input sessionInvocationWaitInput,
	result apisurface.FactoryInvocationResult,
) {
	failureClass := "runtime_failure"
	switch result.Status {
	case factoryapi.InvocationTerminalStatusTimedOut:
		failureClass = "timeout"
	case factoryapi.InvocationTerminalStatusCanceled:
		failureClass = "cancellation"
	case factoryapi.InvocationTerminalStatusFailed:
		switch strings.TrimSpace(result.ErrorCode) {
		case string(invocations.PrimaryResultErrorCodeFailed):
			failureClass = "failed"
		case string(invocations.PrimaryResultErrorCodeUnresolved):
			failureClass = "unresolved_primary"
		case string(invocations.PrimaryResultErrorCodePaused):
			failureClass = "paused"
		case string(invocations.PrimaryResultErrorCodeInterrupted):
			failureClass = "interrupted"
		}
	}
	fs.logger.Warn(
		"factory session invocation failed",
		invocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
			input.FactoryConfig,
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(result.Status)),
			zap.String("error_code", result.ErrorCode),
			zap.String("failure_class", failureClass),
		)...,
	)
}

func (fs *Host) logInvocationFailure(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.InvocationReturnConfig,
	labels map[string]string,
	failureClass string,
	err error,
) {
	fs.recordInvocationMetric(invocationMetricFailure, labels)
	fs.logger.Warn(
		"factory session invocation failed",
		invocationLogFields(
			sessionID,
			source,
			cfg,
			nil,
			zap.String("status", string(factoryapi.InvocationTerminalStatusFailed)),
			zap.String("error_code", string(factoryapi.INVOCATIONRUNTIMEFAILURE)),
			zap.String("failure_class", failureClass),
			zap.Error(err),
		)...,
	)
}

func (fs *Host) logInvocationArgumentFailure(
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

func (fs *Host) recordInvocationMetric(name string, labels map[string]string) {
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

func sessionInvocationSourceHint(request factoryapi.InvocationRequest) invocations.InputSourceLabel {
	if request.Args != nil {
		return invocationInputSourceStructuredArgs
	}
	return invocations.InputSourceLabel(invocations.ArgumentSourceKindCompatibilityContent)
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

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

type invocationWaitTickResult struct {
	done          bool
	result        apisurface.FactoryInvocationResult
	err           error
	loggedLoading bool
}

func invocationWaitContext(ctx context.Context, timeoutMillis *int64) (context.Context, context.CancelFunc) {
	if timeoutMillis != nil && *timeoutMillis > 0 {
		return context.WithTimeout(ctx, time.Duration(*timeoutMillis)*time.Millisecond)
	}
	return ctx, func() {}
}

func (fs *Host) isPackagedTTSSession(sessionID string) bool {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	return err == nil && runtimeCfg != nil && tts.IsPackagedFactory(runtimeCfg.FactoryConfig())
}

func (fs *Host) processInvocationWaitTick(
	waitCtx context.Context,
	sessionID string,
	input sessionInvocationWaitInput,
	result apisurface.FactoryInvocationResult,
	packagedTTSInvocation bool,
	loggedPackagedLoading bool,
) invocationWaitTickResult {
	snapshot, err := fs.GetEngineStateSnapshotForSession(waitCtx, sessionID)
	if err != nil {
		waitResult, waitErr := fs.handleInvocationWaitError(result, err)
		return invocationWaitTickResult{done: true, result: waitResult, err: waitErr}
	}

	worldState, err := fs.sessionInvocationWorldState(waitCtx, sessionID, snapshot.TickCount)
	if err != nil {
		waitResult, waitErr := fs.handleInvocationWaitError(result, err)
		return invocationWaitTickResult{done: true, result: waitResult, err: waitErr}
	}

	activeWork := snapshotHasActiveWork(snapshot)
	if packagedTTSInvocation && activeWork && !loggedPackagedLoading {
		fs.logPackagedTTSInvocationLoading(sessionID, input)
		loggedPackagedLoading = true
	}

	selection, selectionErr := invocations.ResolvePrimaryResult(invocations.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: input.InvocationReturn,
		WorldState:       worldState,
	})
	if selectionErr == nil {
		if packagedTTSInvocation {
			fs.logPackagedTTSInvocationCompleted(sessionID, input, selection)
		}
		return invocationWaitTickResult{
			done:   true,
			result: fs.handleInvocationSelectionSuccess(sessionID, input, selection),
		}
	}

	primaryErr, ok := selectionErr.(*invocations.PrimaryResultError)
	if !ok {
		return invocationWaitTickResult{done: true, err: selectionErr}
	}

	if classified := classifyInvocationMissingPrimaryResultFromSnapshot(sessionID, snapshot, input); classified != nil {
		return invocationWaitTickResult{
			done:   true,
			result: fs.handleInvocationPrimaryResultFailure(sessionID, input, classified),
		}
	}
	if classified, ok := invocations.ClassifyInvocationControlState(sessionID, snapshot.FactoryState, invocations.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: input.InvocationReturn,
		WorldState:       worldState,
	}); ok {
		return invocationWaitTickResult{
			done:   true,
			result: fs.handleInvocationPrimaryResultFailure(sessionID, input, classified),
		}
	}
	if classified, ok := invocations.ClassifyMissingPrimaryResult(invocations.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: input.InvocationReturn,
		WorldState:       worldState,
	}); ok {
		return invocationWaitTickResult{
			done:   true,
			result: fs.handleInvocationPrimaryResultFailure(sessionID, input, classified),
		}
	}

	if _, exists := worldState.WorkRequestsByID[input.RequestID]; exists && !activeWork {
		return invocationWaitTickResult{
			done:   true,
			result: fs.resolveInvocationWaitTerminal(sessionID, input, worldState, packagedTTSInvocation, primaryErr),
		}
	}

	return invocationWaitTickResult{loggedLoading: loggedPackagedLoading}
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

func (fs *Host) resolveInvocationWaitTerminal(
	sessionID string,
	input sessionInvocationWaitInput,
	worldState interfaces.FactoryWorldState,
	packagedTTSInvocation bool,
	primaryErr *invocations.PrimaryResultError,
) apisurface.FactoryInvocationResult {
	if packagedTTSInvocation {
		if _, failure := tts.ClassifyInvocationWait(worldState, input.RequestID, false); failure != nil {
			return fs.handlePackagedTTSInvocationFailure(sessionID, input, failure)
		}
	}
	if classified, ok := invocations.ClassifyInvocationControlState(sessionID, "", invocations.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: input.InvocationReturn,
		WorldState:       worldState,
	}); ok {
		return fs.handleInvocationPrimaryResultFailure(sessionID, input, classified)
	}
	if classified, ok := invocations.ClassifyFailedInvocation(sessionID, invocations.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: input.InvocationReturn,
		WorldState:       worldState,
	}); ok {
		return fs.handleInvocationPrimaryResultFailure(sessionID, input, classified)
	}
	if classified, ok := invocations.ClassifyMissingPrimaryResult(invocations.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: input.InvocationReturn,
		WorldState:       worldState,
	}); ok {
		return fs.handleInvocationPrimaryResultFailure(sessionID, input, classified)
	}
	return fs.handleInvocationPrimaryResultFailure(sessionID, input, primaryErr)
}

func (fs *Host) invocationWaitTimedOut(
	sessionID string,
	input sessionInvocationWaitInput,
	result apisurface.FactoryInvocationResult,
	waitErr error,
) apisurface.FactoryInvocationResult {
	terminalResult := invocationContextTerminalResult(result, waitErr)
	fs.recordInvocationMetric(invocationMetricFailure, invocationMetricLabels(input.FactoryConfig, input.InputSource))
	fs.logInvocationTerminalResult(sessionID, input, terminalResult)
	return terminalResult
}

func (fs *Host) recordPackagedTTSInvocationMetric(
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

func (fs *Host) handlePackagedTTSInvocationFailure(
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
	fs.recordInvocationMetric(invocationMetricFailure, invocationMetricLabels(input.FactoryConfig, input.InputSource))
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

func (fs *Host) logPackagedTTSInvocationLoading(
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

func (fs *Host) logPackagedTTSInvocationCompleted(
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
