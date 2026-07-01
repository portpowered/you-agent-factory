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

// ModelService owns direct model catalog, pull, and invocation operations.
// FactoryService keeps these methods only as a phase-one compatibility facade.
type ModelService interface {
	ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error)
	GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error)
	PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error)
	InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error)
}

type modelServiceDependencies struct {
	runtimeConfig           func() *factoryconfig.LoadedFactoryConfig
	modelAssetPuller        func() modelAssetPuller
	modelHost               func() modelhost.Host
	modelInvocationExecutor func(*factoryconfig.LoadedFactoryConfig, *interfaces.FactoryConfig, string) (workers.WorkstationRequestExecutor, error)
	factoryRunnerID         func() string
	logger                  *zap.Logger
	modelPullMetrics        func() ModelPullMetricsRecorder
}

type runtimeModelService struct {
	deps         modelServiceDependencies
	modelService *modelsservice.Service
}

var _ ModelService = (*runtimeModelService)(nil)
var _ apisurface.ModelAPI = (*runtimeModelService)(nil)

func newModelService(deps modelServiceDependencies) ModelService {
	return &runtimeModelService{
		deps: deps,
		modelService: modelsservice.New(modelsservice.Dependencies{
			RuntimeConfig: deps.runtimeConfig,
			ModelHost:     deps.modelHost,
			ModelAssetPuller: func() localmodels.AssetPuller {
				if deps.modelAssetPuller == nil {
					return nil
				}
				return deps.modelAssetPuller()
			},
			Logger: func() *zap.Logger { return deps.logger },
			ModelPullMetrics: func() modelsservice.PullMetricsRecorder {
				if deps.modelPullMetrics == nil {
					return nil
				}
				recorder := deps.modelPullMetrics()
				if recorder == nil {
					return nil
				}
				return modelPullMetricsAdapter{inner: recorder}
			},
		}),
	}
}

type modelPullMetricsAdapter struct {
	inner ModelPullMetricsRecorder
}

func (a modelPullMetricsAdapter) RecordModelPullMetric(metric modelsservice.PullMetric) {
	a.inner.RecordModelPullMetric(InvocationMetric{
		Name:   metric.Name,
		Labels: metric.Labels,
	})
}

func newFactoryModelService(fs *FactoryService) ModelService {
	if fs == nil {
		return newModelService(modelServiceDependencies{})
	}
	return newModelService(modelServiceDependencies{
		runtimeConfig:           fs.currentRuntimeConfig,
		modelAssetPuller:        fs.modelAssetPuller,
		modelHost:               fs.modelHost,
		modelInvocationExecutor: fs.modelInvocationExecutor,
		factoryRunnerID:         fs.factoryRunnerID,
		logger:                  fs.logger,
		modelPullMetrics:        fs.modelPullMetricsRecorder,
	})
}

func (fs *FactoryService) requireModelService() ModelService {
	if fs == nil {
		return newFactoryModelService(nil)
	}
	fs.modelInitOnce.Do(func() {
		if fs.modelService == nil {
			fs.modelService = newFactoryModelService(fs)
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

const directModelInvocationTransitionID = "direct-model-invocation"

func (fs *FactoryService) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return fs.requireModelService().InvokeModel(ctx, modelName, request)
}

func (s *runtimeModelService) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	if s == nil || s.modelService == nil {
		return modelsservice.New(modelsservice.Dependencies{}).ListModels(ctx)
	}
	return s.modelService.ListModels(ctx)
}

func (s *runtimeModelService) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	if s == nil || s.modelService == nil {
		return modelsservice.New(modelsservice.Dependencies{}).GetModel(ctx, modelName)
	}
	return s.modelService.GetModel(ctx, modelName)
}

func (s *runtimeModelService) modelHost() modelhost.Host {
	if s == nil || s.deps.modelHost == nil {
		return nil
	}
	return s.deps.modelHost()
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
	if bundle := fs.currentRuntimeBundle(); bundle != nil && bundle.modelHost != nil {
		return bundle.modelHost
	}
	return nil
}

func (s *runtimeModelService) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	if s == nil || s.modelService == nil {
		return modelsservice.New(modelsservice.Dependencies{}).PullModel(ctx, modelName)
	}
	return s.modelService.PullModel(ctx, modelName)
}

func (s *runtimeModelService) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	runtimeCfg := s.currentRuntimeConfig()
	if runtimeCfg == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("factory service runtime is not available")
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	if factoryCfg == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("factory config is not available")
	}

	workerDef, operation, err := localmodels.SelectInvocationWorker(runtimeCfg, modelName, request.Operation)
	failureContext := apisurface.InferenceFailureContext{
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(request.Operation),
	}
	if err != nil {
		if failure, ok := apisurface.ClassifyInferenceFailure(err, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, err
	}
	failureContext.WorkerName = workerDef.Name
	failureContext.ModelName = workerDef.Model
	managed, readinessErr := modelhost.EnsureInvocationReady(ctx, s.modelHost(), runtimeCfg, workerDef.Model)
	s.recordManagedRuntimeInvocationReadiness(modelName, managed, readinessErr)
	if readinessErr != nil {
		if failure, ok := apisurface.ClassifyInferenceFailure(readinessErr, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, readinessErr
	}

	inputContent := workcontent.PartsFromGenerated(request.Content)
	inputTokens := []interfaces.Token{{
		ID: "direct-model-invocation-input",
		Color: interfaces.TokenColor{
			Content: inputContent,
		},
	}}
	workstationDef := invocations.DirectInferenceWorkstationConfig(
		request.Operation,
		invocations.OperationBindingsFromGenerated(request.Bindings),
	)
	resolvedBindings, err := invocations.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	executor, err := s.modelInvocationExecutor(runtimeCfg, factoryCfg, workerDef.Name)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	workstationRequest := directModelInvocationWorkstationRequest(workerDef, request, inputTokens, resolvedBindings, s.factoryRunnerID())

	result, err := executor.Execute(ctx, workstationRequest)
	if err != nil {
		if failure, ok := apisurface.ClassifyInferenceFailure(err, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, err
	}
	if result.Outcome == interfaces.OutcomeFailed {
		if failure, ok := apisurface.ClassifyInferenceWorkResultFailure(result, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, fmt.Errorf("provider execution failed: %s", strings.TrimSpace(result.Error))
	}

	outputContent, err := invocations.WorkContentFromInferenceOutput(result.Output, operation)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	streamFile, streamContentType, err := directModelInvocationStream(outputContent, request.Options)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	return apisurface.ModelInvocationResult{
		ModelName:         workerDef.Model,
		Worker:            workerDef.Name,
		Operation:         strings.TrimSpace(request.Operation),
		ProviderLocality:  workerDef.ModelLocality,
		Content:           outputContent,
		Bindings:          interfaces.CloneResolvedModelOperationBindings(resolvedBindings),
		StreamFile:        streamFile,
		StreamContentType: streamContentType,
	}, nil
}

func directModelInvocationWorkstationRequest(
	workerDef *interfaces.WorkerConfig,
	request factoryapi.ModelInvocationRequest,
	inputTokens []interfaces.Token,
	resolvedBindings []interfaces.ResolvedModelOperationBinding,
	factoryRunnerID string,
) interfaces.WorkstationExecutionRequest {
	selection := interfaces.ResolveRunnerSelection("", factoryRunnerID, workerDef.ModelProvider)
	inputContent := workcontent.PartsFromGenerated(request.Content)
	return interfaces.WorkstationExecutionRequest{
		Dispatch: interfaces.WorkDispatch{
			DispatchID:      directModelInvocationTransitionID,
			TransitionID:    directModelInvocationTransitionID,
			WorkerType:      workerDef.Name,
			WorkstationName: directModelInvocationTransitionID,
			InputTokens:     workers.InputTokens(inputTokens...),
		},
		WorkerType:            workerDef.Name,
		WorkstationType:       directModelInvocationTransitionID,
		RunnerID:              selection.RunnerID,
		RunnerSelectionSource: selection.Source,
		InputTokens:           workers.InputTokens(inputTokens...),
		ModelOperation:        strings.TrimSpace(request.Operation),
		ModelBindings:         resolvedBindings,
		SystemPrompt:          workerDef.Body,
		UserMessage:           invocations.InferenceOperationUserMessage(request.Operation, inputContent, resolvedBindings),
	}
}

type sessionInvocationWaitInput struct {
	RequestID        string
	TraceID          string
	InputSource      invocations.InputSourceLabel
	InvocationReturn *interfaces.InvocationReturnConfig
	TimeoutMillis    *int64
}

func (fs *FactoryService) InvokeFactorySession(
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

	resolved, err := resolveSessionInvocationInput(request)
	if err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}

	workTypeName, err := factoryrun.DefaultHandlingWorkTypeName(runtimeCfg.FactoryConfig())
	if err != nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("resolve invocation work type: %w", err)
	}

	requestID := strings.TrimSpace(stringValue(request.RequestId))
	submitRequest := interfaces.SubmitRequest{
		RequestID:  requestID,
		WorkTypeID: workTypeName,
		Content:    resolved.Content,
	}
	submitResult, err := fs.SubmitWorkRequestForSession(
		ctx,
		sessionID,
		factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitRequest}),
	)
	if err != nil {
		fs.logInvocationFailure(
			sessionID,
			resolved.Source,
			runtimeCfg.FactoryConfig().InvocationReturn,
			inputMetricLabels(resolved.Source),
			"runtime_failure",
			err,
		)
		return apisurface.FactoryInvocationResult{}, err
	}
	fs.recordInvocationMetric(invocationMetricAttempts, inputMetricLabels(resolved.Source))
	if policyModeForInvocation(runtimeCfg.FactoryConfig().InvocationReturn) == invocationPolicyModeFallback {
		fs.recordInvocationMetric(invocationMetricFallbackPolicyUsed, inputMetricLabels(resolved.Source))
	}
	if tts.IsPackagedFactory(runtimeCfg.FactoryConfig()) {
		fs.recordPackagedTTSInvocationMetric(tts.MetricPackagedFactoryAttempts, resolved.Source, nil)
		fs.logger.Info(
			"packaged tts invocation submitted",
			packagedTTSInvocationLogFields(
				sessionID,
				resolved.Source,
				runtimeCfg.FactoryConfig().InvocationReturn,
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
			resolved.Source,
			runtimeCfg.FactoryConfig().InvocationReturn,
			zap.String("request_id", submitResult.RequestID),
			zap.String("trace_id", submitResult.TraceID),
		)...,
	)
	return fs.waitForSessionInvocationResult(
		ctx,
		sessionID,
		sessionInvocationWaitInput{
			RequestID:        submitResult.RequestID,
			TraceID:          submitResult.TraceID,
			InputSource:      resolved.Source,
			InvocationReturn: runtimeCfg.FactoryConfig().InvocationReturn,
			TimeoutMillis:    request.TimeoutMillis,
		},
	)
}

func resolveSessionInvocationInput(request factoryapi.InvocationRequest) (invocations.ResolvedInput, error) {
	if request.SourceKind != factoryapi.InvocationInputSourceKindText {
		return invocations.ResolvedInput{}, &apisurface.RequestValidationError{
			Message: "sourceKind must be text",
		}
	}

	content := workcontent.PartsFromGenerated(&request.Content)
	resolved, err := invocations.ResolveAPITextInputContent(content)
	if err != nil {
		var validationErr *invocations.TextContentValidationError
		if errors.As(err, &validationErr) {
			return invocations.ResolvedInput{}, &apisurface.RequestValidationError{
				Message: validationErr.Message,
			}
		}
		return invocations.ResolvedInput{}, err
	}
	return resolved, nil
}

func (fs *FactoryService) waitForSessionInvocationResult(
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

func (fs *FactoryService) handleInvocationWaitError(
	result apisurface.FactoryInvocationResult,
	err error,
) (apisurface.FactoryInvocationResult, error) {
	if statusResult, ok := invocationContextResult(result, err); ok {
		return statusResult, nil
	}
	return apisurface.FactoryInvocationResult{}, err
}

func (fs *FactoryService) handleInvocationSelectionSuccess(
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
	fs.recordInvocationMetric(invocationMetricSuccess, inputMetricLabels(input.InputSource))
	fs.recordInvocationMetric(
		invocationMetricResultType,
		mergeMetricLabels(
			inputMetricLabels(input.InputSource),
			map[string]string{"result_type": resultType},
		),
	)
	fs.logger.Info(
		"factory session invocation completed",
		invocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
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

func (fs *FactoryService) handleInvocationPrimaryResultFailure(
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
	fs.recordInvocationMetric(invocationMetricFailure, inputMetricLabels(input.InputSource))
	failureClass := invocationFailureClassForPrimaryResultError(primaryErr.Code)
	if primaryErr.Code == invocations.PrimaryResultErrorCodeUnresolved {
		fs.recordInvocationMetric(invocationMetricUnresolvedPrimary, inputMetricLabels(input.InputSource))
	}
	fs.logger.Warn(
		"factory session invocation failed",
		invocationLogFields(
			sessionID,
			input.InputSource,
			input.InvocationReturn,
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

	invocationMetricAttempts           = "invocation.attempts"
	invocationMetricSuccess            = "invocation.success"
	invocationMetricFailure            = "invocation.failure"
	invocationMetricUnresolvedPrimary  = "invocation.unresolved_primary"
	invocationMetricFallbackPolicyUsed = "invocation.fallback_policy_used"
	invocationMetricResultType         = "invocation.result_type"

	invocationPolicyModeAuthored = "authored"
	invocationPolicyModeFallback = "fallback"
)

func (fs *FactoryService) logInvocationTerminalResult(
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
			zap.String("request_id", input.RequestID),
			zap.String("trace_id", input.TraceID),
			zap.String("status", string(result.Status)),
			zap.String("error_code", result.ErrorCode),
			zap.String("failure_class", failureClass),
		)...,
	)
}

func (fs *FactoryService) logInvocationFailure(
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
			zap.String("status", string(factoryapi.InvocationTerminalStatusFailed)),
			zap.String("error_code", string(factoryapi.INVOCATIONRUNTIMEFAILURE)),
			zap.String("failure_class", failureClass),
			zap.Error(err),
		)...,
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

func invocationLogFields(
	sessionID string,
	source invocations.InputSourceLabel,
	cfg *interfaces.InvocationReturnConfig,
	extra ...zap.Field,
) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("input_source", string(source)),
		zap.String("invocation_return_policy", invocationPolicyName(cfg)),
		zap.String("invocation_return_policy_mode", policyModeForInvocation(cfg)),
		zap.String("policy_resolution_path", invocationPolicyResolutionPath(cfg)),
	}
	return append(fields, extra...)
}

func inputMetricLabels(source invocations.InputSourceLabel) map[string]string {
	return map[string]string{"input_source": string(source)}
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

func (s *runtimeModelService) currentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if s == nil || s.deps.runtimeConfig == nil {
		return nil
	}
	return s.deps.runtimeConfig()
}

func (s *runtimeModelService) modelAssetPuller() modelAssetPuller {
	if s == nil || s.deps.modelAssetPuller == nil {
		return newModelAssetPuller("")
	}
	return s.deps.modelAssetPuller()
}

func (s *runtimeModelService) modelInvocationExecutor(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error) {
	if s == nil || s.deps.modelInvocationExecutor == nil {
		return nil, fmt.Errorf("model invocation executor is not configured")
	}
	return s.deps.modelInvocationExecutor(runtimeCfg, factoryCfg, workerName)
}

func (s *runtimeModelService) factoryRunnerID() string {
	if s == nil || s.deps.factoryRunnerID == nil {
		return ""
	}
	return s.deps.factoryRunnerID()
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
		modelDomain = localModelDomain{
			resources:      bundle.modelResources,
			assets:         bundle.modelAssets,
			runtime:        bundle.localModelRuntime,
			host:           bundle.modelHost,
			manager:        bundle.localModels,
			leaseExecution: bundle.leaseExecution,
		}
		workflowContext = runtime.WorkflowContext(bundle.factory)
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

func directModelInvocationStream(content []interfaces.WorkContentPart, options *factoryapi.ModelInvocationOptions) (string, string, error) {
	if options == nil || options.ResponseMode == nil || *options.ResponseMode != factoryapi.AUDIOSTREAM {
		return "", "", nil
	}
	for _, part := range content {
		if part.Type.Normalized() != interfaces.WorkContentPartTypeAudio || strings.TrimSpace(part.File) == "" {
			continue
		}
		contentType := strings.TrimSpace(part.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return part.File, contentType, nil
	}
	return "", "", fmt.Errorf("%w: invocation did not produce audio output", apisurface.ErrModelInvocationUnsupportedMode)
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

func (fs *FactoryService) isPackagedTTSSession(sessionID string) bool {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	return err == nil && runtimeCfg != nil && tts.IsPackagedFactory(runtimeCfg.FactoryConfig())
}

func (fs *FactoryService) processInvocationWaitTick(
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

func (fs *FactoryService) resolveInvocationWaitTerminal(
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

func (fs *FactoryService) invocationWaitTimedOut(
	sessionID string,
	input sessionInvocationWaitInput,
	result apisurface.FactoryInvocationResult,
	waitErr error,
) apisurface.FactoryInvocationResult {
	terminalResult := invocationContextTerminalResult(result, waitErr)
	fs.recordInvocationMetric(invocationMetricFailure, inputMetricLabels(input.InputSource))
	fs.logInvocationTerminalResult(sessionID, input, terminalResult)
	return terminalResult
}

func (fs *FactoryService) recordPackagedTTSInvocationMetric(
	name string,
	source invocations.InputSourceLabel,
	extra map[string]string,
) {
	labels := mergeMetricLabels(
		inputMetricLabels(source),
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
	fs.recordInvocationMetric(invocationMetricFailure, inputMetricLabels(input.InputSource))
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
	extra ...zap.Field,
) []zap.Field {
	fields := invocationLogFields(sessionID, source, cfg,
		zap.String("packaged_factory_name", tts.PackagedFactoryName),
		zap.String("tts_backend", tts.BackendRuntimeLabel()),
	)
	return append(fields, extra...)
}
