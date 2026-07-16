package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
	workerinference "github.com/portpowered/infinite-you/pkg/workers/inference"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
)

// AgentExecutor implements WorkstationRequestExecutor for MODEL_WORKER types.
// It reads prompt/output inputs resolved by WorkstationExecutor, calls the
// configured Provider for inference, and maps the response to a WorkResult.
type AgentExecutor struct {
	providerExecutor providerexecution.Executor
	runtimeConfig    interfaces.RuntimeDefinitionLookup
	logger           logging.Logger
	retryConfig      providerRetryConfig
}

var _ WorkstationRequestExecutor = (*AgentExecutor)(nil)

// AgentExecutorOption configures an AgentExecutor.
type AgentExecutorOption func(*AgentExecutor)

func WithLogger(logger logging.Logger) AgentExecutorOption {
	return func(ae *AgentExecutor) {
		ae.logger = logging.EnsureLogger(logger)
	}
}

// NewAgentExecutor creates an AgentExecutor from runtime-loaded config and a Provider.
func NewAgentExecutor(runtimeConfig interfaces.RuntimeDefinitionLookup, provider Provider, opts ...AgentExecutorOption) *AgentExecutor {
	return newAgentExecutor(runtimeConfig, providerexecution.NewExecutor(provider), opts...)
}

// NewAgentExecutorWithRunner creates an AgentExecutor from runtime-loaded config
// and the shared runner execution contract.
func NewAgentExecutorWithRunner(runtimeConfig interfaces.RuntimeDefinitionLookup, runner Runner, opts ...AgentExecutorOption) *AgentExecutor {
	return newAgentExecutor(runtimeConfig, providerexecution.NewProviderExecutor(runnerProviderAdapter{inner: runner}), opts...)
}

func newAgentExecutor(runtimeConfig interfaces.RuntimeDefinitionLookup, executor providerexecution.Executor, opts ...AgentExecutorOption) *AgentExecutor {
	ae := &AgentExecutor{
		providerExecutor: executor,
		runtimeConfig:    runtimeConfig,
		logger:           logging.NoopLogger{},
		retryConfig:      newProviderRetryConfig(),
	}
	for _, opt := range opts {
		opt(ae)
	}
	return ae
}

// Execute calls the Provider with one rendered workstation request, parses the
// response against OutputSchema if present, and returns a WorkResult.
func (ae *AgentExecutor) Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	start := time.Now()
	workerType := workerTypeForExecutionRequest(request)
	workerDef, ok := ae.runtimeConfig.Worker(workerType)
	if !ok {
		return missingWorkerWorkResult(request.Dispatch, workerType, time.Since(start)), nil
	}
	workerDef = effectiveWorkerDefinition(request, workerDef)

	workstationDef, _ := ae.runtimeConfig.Workstation(inferenceWorkstationType(request))
	req := inferenceRequestForExecutionRequest(request, workerDef, workstationDef)
	diagnostics := workDiagnosticsForInferenceRequest(req)

	resp, retryCount, err := ae.inferWithRetry(ctx, req)
	if err != nil {
		return inferenceErrorWorkResult(request.Dispatch, err, diagnostics, retryCount, start), nil
	}
	diagnostics = withInferenceResponseDiagnostics(diagnostics, resp, retryCount)

	if goal.UsesGoalRoutingDecisionEnvelope(workstationDef) {
		return goalRoutingEnvelopeWorkResult(request, resp, diagnostics, retryCount, start), nil
	}
	if goal.UsesDecisionEnvelopeOutcome(workstationDef) {
		return decisionEnvelopeWorkResult(request, resp, diagnostics, retryCount, start), nil
	}

	outcome := ae.evaluateOutcome(resp, workerDef)
	shapedContent, err := ae.canonicalInferenceOutput(resp.Content, workerDef, request.ModelOperation)
	if err != nil {
		return workerexecution.WorkResult{
			DispatchID:      request.Dispatch.DispatchID,
			TransitionID:    request.Dispatch.TransitionID,
			Outcome:         workerexecution.OutcomeFailed,
			Output:          resp.Content,
			Error:           err.Error(),
			ProviderSession: workerexecution.CloneProviderSessionMetadata(resp.ProviderSession),
			Diagnostics:     diagnostics,
			Metrics:         agentWorkMetrics(start, retryCount),
		}, nil
	}
	if shapedContent != "" {
		resp.Content = shapedContent
	}
	return ae.workResultForInferenceResponse(request, resp, outcome, diagnostics, retryCount, start)
}

func effectiveWorkerDefinition(request workerexecution.WorkstationExecutionRequest, workerDef *workerconfig.Config) *workerconfig.Config {
	if workerDef == nil || (request.Model == "" && request.ModelProvider == "") {
		return workerDef
	}
	effective := *workerDef
	if request.Model != "" {
		effective.Model = request.Model
	}
	if request.ModelProvider != "" {
		effective.ModelProvider = request.ModelProvider
	}
	return &effective
}

func (ae *AgentExecutor) canonicalInferenceOutput(raw string, workerDef *workerconfig.Config, operationName string) (string, error) {
	operationName = strings.TrimSpace(operationName)
	if operationName == "" || workerDef == nil {
		return raw, nil
	}
	var operation workerconfig.ModelOperation
	var ok bool
	for _, candidate := range workerDef.Operations {
		if strings.TrimSpace(candidate.Name) == operationName {
			operation = candidate
			ok = true
			break
		}
	}
	if !ok {
		return raw, nil
	}
	parts, err := workerinference.WorkContentFromInferenceOutput(raw, operation)
	if err != nil {
		return "", fmt.Errorf("inference output shaping failed: %w", err)
	}
	if len(parts) == 0 {
		return raw, nil
	}
	return workerinference.MarshalWorkContentOutput(parts)
}

func decisionEnvelopeWorkResult(
	request workerexecution.WorkstationExecutionRequest,
	resp workerexecution.InferenceResponse,
	diagnostics *workerexecution.WorkDiagnostics,
	retryCount int,
	start time.Time,
) workerexecution.WorkResult {
	result := goal.WorkResultFromDecisionEnvelopeJSONOrFailed(
		request.Dispatch.DispatchID,
		request.Dispatch.TransitionID,
		resp.Content,
	)
	result.ProviderSession = workerexecution.CloneProviderSessionMetadata(resp.ProviderSession)
	result.Diagnostics = diagnostics
	result.Metrics = agentWorkMetrics(start, retryCount)
	return result
}

func goalRoutingEnvelopeWorkResult(
	request workerexecution.WorkstationExecutionRequest,
	resp workerexecution.InferenceResponse,
	diagnostics *workerexecution.WorkDiagnostics,
	retryCount int,
	start time.Time,
) workerexecution.WorkResult {
	result := goal.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		request.Dispatch.DispatchID,
		request.Dispatch.TransitionID,
		resp.Content,
	)
	result.ProviderSession = workerexecution.CloneProviderSessionMetadata(resp.ProviderSession)
	result.Diagnostics = diagnostics
	result.Metrics = agentWorkMetrics(start, retryCount)
	return result
}

func workerTypeForExecutionRequest(request workerexecution.WorkstationExecutionRequest) string {
	if request.WorkerType != "" {
		return request.WorkerType
	}
	return request.Dispatch.WorkerType
}

func missingWorkerWorkResult(dispatch work.WorkDispatch, workerType string, duration time.Duration) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "worker config not found: " + workerType,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func inferenceErrorWorkResult(dispatch work.WorkDispatch, err error, diagnostics *workerexecution.WorkDiagnostics, retryCount int, start time.Time) workerexecution.WorkResult {
	providerErr := workerprovider.NormalizeProviderExecutionError(err)
	failureMetadata := workerprovider.WorkFailureMetadataFromError(providerErr)
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         workerexecution.OutcomeFailed,
		Error:           formatAgentProviderError(err),
		FailureMetadata: workerexecution.CloneWorkFailureMetadata(failureMetadata),
		ProviderSession: providerSessionFromError(providerErr),
		Diagnostics:     mergeWorkDiagnostics(withInferenceErrorDiagnostics(diagnostics, err, retryCount), providerDiagnosticsFromError(providerErr)),
		Metrics:         agentWorkMetrics(start, retryCount),
	}
}

func (ae *AgentExecutor) workResultForInferenceResponse(request workerexecution.WorkstationExecutionRequest, resp workerexecution.InferenceResponse, outcome workerexecution.WorkOutcome, diagnostics *workerexecution.WorkDiagnostics, retryCount int, start time.Time) (workerexecution.WorkResult, error) {
	metrics := agentWorkMetrics(start, retryCount)
	if request.OutputSchema != "" {
		ae.logger.Info("parsing output against schema", "schema", request.OutputSchema)
		parseFailure := ""
		if _, parseErr := parseOutputAgainstSchema(resp.Content, []byte(request.OutputSchema)); parseErr != nil {
			parseFailure = parseErr.Error()
		}
		if parseFailure != "" {
			return workerexecution.WorkResult{
				DispatchID:      request.Dispatch.DispatchID,
				TransitionID:    request.Dispatch.TransitionID,
				Outcome:         workerexecution.OutcomeFailed,
				Output:          resp.Content,
				Error:           "output parse failed: " + parseFailure,
				ProviderSession: workerexecution.CloneProviderSessionMetadata(resp.ProviderSession),
				Diagnostics:     diagnostics,
				Metrics:         metrics,
			}, nil
		}
	}

	return workerexecution.WorkResult{
		DispatchID:      request.Dispatch.DispatchID,
		TransitionID:    request.Dispatch.TransitionID,
		Outcome:         outcome,
		Output:          resp.Content,
		ProviderSession: workerexecution.CloneProviderSessionMetadata(resp.ProviderSession),
		Diagnostics:     diagnostics,
		Metrics:         metrics,
	}, nil
}

func agentWorkMetrics(start time.Time, retryCount int) workerexecution.WorkMetrics {
	return workerexecution.WorkMetrics{
		Duration:   time.Since(start),
		RetryCount: retryCount,
	}
}

func inferenceRequestForExecutionRequest(request workerexecution.WorkstationExecutionRequest, workerDef *workerconfig.Config, workstationDef *interfaces.FactoryWorkstationConfig) workerexecution.ProviderInferenceRequest {
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:                     work.CloneWorkDispatch(request.Dispatch),
		WorkerType:                   request.WorkerType,
		WorkstationType:              inferenceWorkstationType(request),
		RunnerID:                     request.RunnerID,
		ProjectID:                    request.ProjectID,
		InputTokens:                  cloneRawInputTokens(request.InputTokens),
		ModelOperation:               request.ModelOperation,
		ModelBindings:                workerexecution.CloneResolvedModelOperationBindings(request.ModelBindings),
		SystemPrompt:                 request.SystemPrompt,
		UserMessage:                  request.UserMessage,
		OutputSchema:                 request.OutputSchema,
		ToolExecutionMode:            workerexecution.RunnerToolExecutionModeRequired,
		RequiredOptionalCapabilities: requiredRunnerOptionalCapabilities(request),
		EnvVars:                      cloneEnvVars(request.EnvVars),
		Worktree:                     request.Worktree,
		WorkingDirectory:             request.WorkingDirectory,
	}
	if workerDef != nil {
		req.Model = workerDef.Model
		req.ModelProvider = modelProviderForExecution(workerDef.ModelProvider, workerexecution.ResolvedRunnerSelection{
			RunnerID: request.RunnerID,
			Source:   request.RunnerSelectionSource,
		})
		req.ModelLocality = workerDef.ModelLocality
		req.SessionID = workerDef.SessionID
		if workerDef.SessionID != "" {
			req.RequiredOptionalCapabilities = append(req.RequiredOptionalCapabilities, workerexecution.RunnerOptionalCapabilitySessionResume)
		}
	}
	if req.ModelProvider == string(modelprovider.OpenCode) {
		workstationAgent := ""
		workerAgent := ""
		if workstationDef != nil {
			workstationAgent = workstationDef.OpenCodeAgent
		}
		if workerDef != nil {
			workerAgent = workerDef.OpenCodeAgent
		}
		req.OpenCodeAgent = workerrunner.ResolveOpenCodeAgent(workstationAgent, workerAgent)
	}
	return req
}

func modelProviderForExecution(workerModelProvider string, selection workerexecution.ResolvedRunnerSelection) string {
	if selection.Source == workerexecution.RunnerSelectionSourceWorkstation || selection.Source == workerexecution.RunnerSelectionSourceFactory {
		if provider := modelProviderForRunnerID(selection.RunnerID); provider != "" {
			return provider
		}
	}
	if workerModelProvider != "" {
		return workerModelProvider
	}
	return modelProviderForRunnerID(selection.RunnerID)
}

func modelProviderForRunnerID(runnerID string) string {
	switch workerrunner.NormalizeRunnerID(runnerID) {
	case workerexecution.RunnerIDCodex:
		return string(modelprovider.Codex)
	case workerexecution.RunnerIDGemini:
		return string(modelprovider.Gemini)
	case workerexecution.RunnerIDKiro:
		return string(modelprovider.Kiro)
	case workerexecution.RunnerIDCursorCLI:
		return string(modelprovider.Cursor)
	case workerexecution.RunnerIDOpenCode:
		return string(modelprovider.OpenCode)
	case workerexecution.RunnerIDPi:
		return string(modelprovider.Pi)
	default:
		return ""
	}
}

func inferenceWorkstationType(request workerexecution.WorkstationExecutionRequest) string {
	if request.WorkstationType != "" {
		return request.WorkstationType
	}
	return request.Dispatch.WorkstationName
}

func providerSessionFromError(providerErr *workerprovider.ProviderError) *workerexecution.ProviderSessionMetadata {
	if providerErr == nil {
		return nil
	}
	return workerexecution.CloneProviderSessionMetadata(providerErr.ProviderSession)
}

func providerDiagnosticsFromError(providerErr *workerprovider.ProviderError) *workerexecution.WorkDiagnostics {
	if providerErr == nil {
		return nil
	}
	return providerErr.Diagnostics
}

func formatAgentProviderError(err error) string {
	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) {
		message := strings.TrimSpace(providerErr.Message)
		if providerErr.Type == workerexecution.WorkFailureTypeTimeout && message == "execution timeout" {
			return message
		}
		if message != "" {
			return fmt.Sprintf("%s: %s", providerErr.Error(), message)
		}
		return providerErr.Error()
	}
	return "provider error: " + err.Error()
}

const (
	defaultProviderMaxRetries     = 2
	defaultProviderInitialBackoff = 100 * time.Millisecond
)

type retrySleepFunc func(context.Context, time.Duration) error
type retryJitterFunc func(time.Duration) time.Duration

type providerRetryConfig struct {
	maxRetries     int
	initialBackoff time.Duration
	sleep          retrySleepFunc
	jitter         retryJitterFunc
}

func newProviderRetryConfig() providerRetryConfig {
	return providerRetryConfig{
		maxRetries:     defaultProviderMaxRetries,
		initialBackoff: defaultProviderInitialBackoff,
		sleep:          sleepWithContext,
		jitter:         newLockedRetryJitter(),
	}
}

func (ae *AgentExecutor) inferWithRetry(ctx context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, int, error) {
	logger := logging.EnsureLogger(ae.logger)
	retryCount := 0
	if ae.providerExecutor == nil {
		return workerexecution.InferenceResponse{}, retryCount, workerprovider.NewProviderError(
			workerexecution.WorkFailureTypeMisconfigured,
			"provider execution requires a provider",
			nil,
		)
	}

	for {
		result, err := ae.providerExecutor.Execute(ctx, providerexecution.ExecutionInput{
			Request: req,
			Attempt: retryCount + 1,
		})
		if err == nil {
			return result.Response, retryCount, nil
		}

		providerErr := workerprovider.NormalizeProviderExecutionError(err)
		if providerErr == nil {
			return workerexecution.InferenceResponse{}, retryCount, err
		}

		decision := workerprovider.WorkFailureDecisionFromProviderError(providerErr)
		if !decision.Retryable || retryCount >= ae.retryConfig.maxRetries {
			return workerexecution.InferenceResponse{}, retryCount, providerErr
		}

		baseDelay := ae.retryConfig.initialBackoff << retryCount
		delay := baseDelay + ae.retryConfig.jitter(baseDelay)
		retryCount++

		logger.Warn("provider inference failed; retrying",
			WorkLogFields(req.Dispatch.Execution,
				"model_provider", req.ModelProvider,
				"model", req.Model,
				"retry_count", retryCount,
				"max_retries", ae.retryConfig.maxRetries,
				"provider_error_type", string(providerErr.Type),
				"backoff_ms", delay.Milliseconds())...)

		if err := ae.retryConfig.sleep(ctx, delay); err != nil {
			return workerexecution.InferenceResponse{}, retryCount, err
		}
	}
}

type providerRunnerAdapter struct {
	executor providerexecution.Executor
}

type runnerProviderAdapter struct {
	inner Runner
}

func (a runnerProviderAdapter) Infer(ctx context.Context, request workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	if a.inner == nil {
		return workerexecution.InferenceResponse{}, workerprovider.NewProviderError(
			workerexecution.WorkFailureTypeMisconfigured,
			"runner requires an implementation",
			nil,
		)
	}
	return a.inner.Execute(ctx, request)
}

// RunnerFromProvider adapts a legacy provider implementation onto the shared
// runner execution contract.
func RunnerFromProvider(provider Provider) Runner {
	return providerRunnerAdapter{executor: providerexecution.NewExecutor(provider)}
}

func (a providerRunnerAdapter) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	if a.executor == nil {
		return workerexecution.RunnerExecutionResult{}, workerprovider.NewProviderError(
			workerexecution.WorkFailureTypeMisconfigured,
			"runner requires a provider implementation",
			nil,
		)
	}
	result, err := a.executor.Execute(ctx, providerexecution.ExecutionInput{Request: request, Attempt: 1})
	return result.Response, err
}

func requiredRunnerOptionalCapabilities(request workerexecution.WorkstationExecutionRequest) []workerexecution.RunnerOptionalCapability {
	capabilities := make([]workerexecution.RunnerOptionalCapability, 0, 5)
	if request.OutputSchema != "" {
		capabilities = append(capabilities, workerexecution.RunnerOptionalCapabilityStructuredOutput)
	}
	if request.WorkingDirectoryAuthored && request.WorkingDirectory != "" {
		capabilities = append(capabilities, workerexecution.RunnerOptionalCapabilityWorkingDirectory)
	}
	if shouldRequireWorktreeRunnerCapability(request) {
		capabilities = append(capabilities, workerexecution.RunnerOptionalCapabilityWorktree)
	}
	for _, token := range cloneInputTokens(request.InputTokens) {
		if tokenHasImageContent(token) {
			capabilities = append(capabilities, workerexecution.RunnerOptionalCapabilityImageInput)
			break
		}
	}
	return capabilities
}

func shouldRequireWorktreeRunnerCapability(request workerexecution.WorkstationExecutionRequest) bool {
	if request.Worktree == "" {
		return false
	}
	if request.WorkingDirectory != "" && workerrunner.NormalizeRunnerID(request.RunnerID) == workerexecution.RunnerIDCodex {
		return false
	}
	return true
}

func tokenHasImageContent(token factorytoken.Token) bool {
	for _, part := range token.Color.Content {
		if part.Type == work.WorkContentPartTypeImage {
			return true
		}
	}
	return false
}

// evaluateOutcome determines the WorkOutcome based on stop token evaluation.
// When no stop token is configured, all successful provider responses are ACCEPTED.
// When a stop token is configured, the output is checked: found → ACCEPTED,
// <CONTINUE> → CONTINUE, otherwise → REJECTED.
func (ae *AgentExecutor) evaluateOutcome(resp workerexecution.InferenceResponse, workerDef *workerconfig.Config) workerexecution.WorkOutcome {
	if workerDef.StopToken == "" {
		ae.logger.Info("no stop token configured; defaulting to ACCEPTED outcome")
		return workerexecution.OutcomeAccepted
	}
	if workerprovider.ContainsStopToken(resp.Content, workerDef.StopToken) {
		ae.logger.Info("stop token found in output; returning ACCEPTED outcome", "stop_token", workerDef.StopToken)
		return workerexecution.OutcomeAccepted
	}
	if strings.Contains(resp.Content, "<CONTINUE>") {
		return workerexecution.OutcomeContinue
	}
	return workerexecution.OutcomeRejected
}

// parseOutputAgainstSchema parses the response content as JSON and validates
// it can be unmarshalled into TokenColor structs. The schema parameter is
// reserved for future schema validation; for MVP, we just validate JSON.
func parseOutputAgainstSchema(content string, _ []byte) ([]factorytoken.Color, error) {
	// Try parsing as array of token colors first.
	var colors []factorytoken.Color
	if err := json.Unmarshal([]byte(content), &colors); err == nil {
		return colors, nil
	}

	// Try parsing as a single token color.
	var color factorytoken.Color
	if err := json.Unmarshal([]byte(content), &color); err != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", err)
	}

	return []factorytoken.Color{color}, nil
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newLockedRetryJitter() retryJitterFunc {
	randomizer := rand.New(rand.NewSource(time.Now().UnixNano()))
	var mu sync.Mutex

	return func(baseDelay time.Duration) time.Duration {
		if baseDelay <= 0 {
			return 0
		}

		maxJitter := baseDelay / 2
		if maxJitter <= 0 {
			return 0
		}

		mu.Lock()
		defer mu.Unlock()
		return time.Duration(randomizer.Int63n(int64(maxJitter) + 1))
	}
}

// Compile-time check.
var _ WorkstationRequestExecutor = (*AgentExecutor)(nil)
