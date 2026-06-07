package service

import (
	"context"
	"encoding/json"
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
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	"go.uber.org/zap"
)

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
	modelInvocationExecutor func(*factoryconfig.LoadedFactoryConfig, *interfaces.FactoryConfig, string) (workers.WorkstationRequestExecutor, error)
	factoryRunnerID         func() string
}

type runtimeModelService struct {
	deps modelServiceDependencies
}

var _ ModelService = (*runtimeModelService)(nil)
var _ apisurface.ModelAPI = (*runtimeModelService)(nil)

func newModelService(deps modelServiceDependencies) ModelService {
	return &runtimeModelService{deps: deps}
}

func newFactoryModelService(fs *FactoryService) ModelService {
	if fs == nil {
		return newModelService(modelServiceDependencies{})
	}
	return newModelService(modelServiceDependencies{
		runtimeConfig:           fs.currentRuntimeConfig,
		modelAssetPuller:        fs.modelAssetPuller,
		modelInvocationExecutor: fs.modelInvocationExecutor,
		factoryRunnerID:         fs.factoryRunnerID,
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
	_ = ctx
	return localmodels.ListModels(s.currentRuntimeConfig())
}

func (s *runtimeModelService) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	_ = ctx
	return localmodels.GetModel(s.currentRuntimeConfig(), modelName)
}

func (s *runtimeModelService) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	return localmodels.PullModel(s.modelAssetPuller(), ctx, s.currentRuntimeConfig(), modelName)
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
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	if err := s.modelAssetPuller().EnsureModelAvailable(ctx, runtimeCfg, workerDef); err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	inputContent := workcontent.PartsFromGenerated(request.Content)
	inputTokens := []interfaces.Token{{
		ID: "direct-model-invocation-input",
		Color: interfaces.TokenColor{
			Content: inputContent,
		},
	}}
	workstationDef := &interfaces.FactoryWorkstationConfig{
		Type:              interfaces.WorkstationTypeInvoke,
		Operation:         strings.TrimSpace(request.Operation),
		OperationBindings: modelInvocationBindingsFromGenerated(request.Bindings),
	}
	resolvedBindings, err := workerexecutor.ResolveModelOperationBindings(workstationDef, workerDef, inputTokens)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	executor, err := s.modelInvocationExecutor(runtimeCfg, factoryCfg, workerDef.Name)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	selection := interfaces.ResolveRunnerSelection("", s.factoryRunnerID(), workerDef.ModelProvider)
	workstationRequest := interfaces.WorkstationExecutionRequest{
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
		UserMessage:           directModelInvocationUserMessage(request.Operation, inputContent, resolvedBindings),
	}

	result, err := executor.Execute(ctx, workstationRequest)
	if err != nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("provider execution failed: %w", err)
	}
	if result.Outcome == interfaces.OutcomeFailed {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("provider execution failed: %s", strings.TrimSpace(result.Error))
	}

	outputContent, err := directModelInvocationOutputContent(result.Output, operation)
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

func (fs *FactoryService) handleInvocationUnresolvedPrimary(
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
	}
	fs.recordInvocationMetric(invocationMetricFailure, inputMetricLabels(input.InputSource))
	fs.recordInvocationMetric(invocationMetricUnresolvedPrimary, inputMetricLabels(input.InputSource))
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
			zap.String("failure_class", "unresolved_primary"),
		)...,
	)
	return result
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
		if strings.TrimSpace(result.ErrorCode) == string(invocations.PrimaryResultErrorCodeUnresolved) {
			failureClass = "unresolved_primary"
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
	var modelResources *localModelResourceLimiter
	var localModels *managedLocalModelManager
	var workflowContext *factory_context.FactoryContext
	if bundle != nil {
		modelResources = bundle.modelResources
		localModels = bundle.localModels
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
		fs.providerCommandRunnerOverride(),
		fs.commandRunnerOverride(),
		nil,
		nil,
		nil,
		time.Now,
		modelResources,
		localModels,
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

func modelInvocationBindingsFromGenerated(values *[]factoryapi.WorkstationOperationBinding) []interfaces.ModelOperationBinding {
	if values == nil || len(*values) == 0 {
		return nil
	}
	bindings := make([]interfaces.ModelOperationBinding, 0, len(*values))
	for _, binding := range *values {
		current := interfaces.ModelOperationBinding{
			Slot:           strings.TrimSpace(binding.Slot),
			Config:         workcontent.PartsFromGenerated(binding.Config),
			DefaultContent: workcontent.PartsFromGenerated(binding.DefaultContent),
		}
		if binding.Selector != nil {
			current.Selector = &interfaces.ModelOperationBindingSelector{
				Slot:  stringValue(binding.Selector.Slot),
				Label: stringValue(binding.Selector.Label),
				Type:  stringValue(binding.Selector.Type),
				Role:  stringValue(binding.Selector.Role),
			}
		}
		bindings = append(bindings, current)
	}
	return bindings
}

func directModelInvocationUserMessage(operation string, inputContent []interfaces.WorkContentPart, bindings []interfaces.ResolvedModelOperationBinding) string {
	payload := struct {
		Operation string                                     `json:"operation"`
		Input     []interfaces.WorkContentPart               `json:"input,omitempty"`
		Bindings  []interfaces.ResolvedModelOperationBinding `json:"bindings,omitempty"`
	}{
		Operation: strings.TrimSpace(operation),
		Input:     append([]interfaces.WorkContentPart(nil), inputContent...),
		Bindings:  interfaces.CloneResolvedModelOperationBindings(bindings),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(operation)
	}
	return string(encoded)
}

func directModelInvocationOutputContent(raw string, operation interfaces.ModelOperation) ([]interfaces.WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var content factoryapi.WorkContent
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil {
		return workcontent.PartsFromGenerated(&content), nil
	}
	var envelope struct {
		Content factoryapi.WorkContent `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && envelope.Content != nil {
		return workcontent.PartsFromGenerated(&envelope.Content), nil
	}
	if modelOperationHasOnlyTextOutputs(operation) {
		return []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: raw,
		}}, nil
	}
	return nil, fmt.Errorf("model invocation response is not valid WorkContent JSON for operation %q", strings.TrimSpace(operation.Name))
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

func modelOperationHasOnlyTextOutputs(operation interfaces.ModelOperation) bool {
	if len(operation.Outputs) == 0 {
		return true
	}
	for _, output := range operation.Outputs {
		if len(output.ContentTypes) == 0 {
			return false
		}
		for _, contentType := range output.ContentTypes {
			if strings.TrimSpace(contentType) != interfaces.ModelOperationContentTypeText {
				return false
			}
		}
	}
	return true
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
