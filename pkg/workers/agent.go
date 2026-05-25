package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

// AgentExecutor implements WorkstationRequestExecutor for MODEL_WORKER types.
// It reads prompt/output inputs resolved by WorkstationExecutor, calls the
// configured Provider for inference, and maps the response to a WorkResult.
type AgentExecutor struct {
	runner        Runner
	runtimeConfig interfaces.RuntimeDefinitionLookup
	logger        logging.Logger
	retryConfig   providerRetryConfig
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
	return NewAgentExecutorWithRunner(runtimeConfig, providerRunnerAdapter{inner: provider}, opts...)
}

// NewAgentExecutorWithRunner creates an AgentExecutor from runtime-loaded config
// and the shared runner execution contract.
func NewAgentExecutorWithRunner(runtimeConfig interfaces.RuntimeDefinitionLookup, runner Runner, opts ...AgentExecutorOption) *AgentExecutor {
	ae := &AgentExecutor{
		runner:        runner,
		runtimeConfig: runtimeConfig,
		logger:        logging.NoopLogger{},
		retryConfig:   newProviderRetryConfig(),
	}
	for _, opt := range opts {
		opt(ae)
	}
	return ae
}

// Execute calls the Provider with one rendered workstation request, parses the
// response against OutputSchema if present, and returns a WorkResult.
func (ae *AgentExecutor) Execute(ctx context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	start := time.Now()
	workerType := workerTypeForExecutionRequest(request)
	workerDef, ok := ae.runtimeConfig.Worker(workerType)
	if !ok {
		return missingWorkerWorkResult(request.Dispatch, workerType, time.Since(start)), nil
	}

	req := inferenceRequestForExecutionRequest(request, workerDef)
	diagnostics := workDiagnosticsForInferenceRequest(req)

	resp, retryCount, err := ae.inferWithRetry(ctx, req)
	if err != nil {
		return inferenceErrorWorkResult(request.Dispatch, err, diagnostics, retryCount, start), nil
	}
	diagnostics = withInferenceResponseDiagnostics(diagnostics, resp, retryCount)

	outcome := ae.evaluateOutcome(resp, workerDef)
	return ae.workResultForInferenceResponse(request, resp, outcome, diagnostics, retryCount, start)
}

func workerTypeForExecutionRequest(request interfaces.WorkstationExecutionRequest) string {
	if request.WorkerType != "" {
		return request.WorkerType
	}
	return request.Dispatch.WorkerType
}

func missingWorkerWorkResult(dispatch interfaces.WorkDispatch, workerType string, duration time.Duration) interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeFailed,
		Error:        "worker config not found: " + workerType,
		Metrics:      interfaces.WorkMetrics{Duration: duration},
	}
}

func inferenceErrorWorkResult(dispatch interfaces.WorkDispatch, err error, diagnostics *interfaces.WorkDiagnostics, retryCount int, start time.Time) interfaces.WorkResult {
	providerErr := NormalizeProviderExecutionError(err)
	failureMetadata := WorkFailureMetadataFromError(providerErr)
	return interfaces.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         interfaces.OutcomeFailed,
		Error:           formatAgentProviderError(err),
		FailureMetadata: interfaces.CloneWorkFailureMetadata(failureMetadata),
		ProviderFailure: interfaces.CloneProviderFailureMetadata(failureMetadata),
		ProviderSession: providerSessionFromError(providerErr),
		Diagnostics:     mergeWorkDiagnostics(withInferenceErrorDiagnostics(diagnostics, err, retryCount), providerDiagnosticsFromError(providerErr)),
		Metrics:         agentWorkMetrics(start, retryCount),
	}
}

func (ae *AgentExecutor) workResultForInferenceResponse(request interfaces.WorkstationExecutionRequest, resp interfaces.InferenceResponse, outcome interfaces.WorkOutcome, diagnostics *interfaces.WorkDiagnostics, retryCount int, start time.Time) (interfaces.WorkResult, error) {
	metrics := agentWorkMetrics(start, retryCount)
	if request.OutputSchema != "" {
		ae.logger.Info("parsing output against schema", "schema", request.OutputSchema)
		parseFailure := ""
		if _, parseErr := parseOutputAgainstSchema(resp.Content, []byte(request.OutputSchema)); parseErr != nil {
			parseFailure = parseErr.Error()
		}
		if parseFailure != "" {
			return interfaces.WorkResult{
				DispatchID:      request.Dispatch.DispatchID,
				TransitionID:    request.Dispatch.TransitionID,
				Outcome:         interfaces.OutcomeFailed,
				Output:          resp.Content,
				Error:           "output parse failed: " + parseFailure,
				ProviderSession: interfaces.CloneProviderSessionMetadata(resp.ProviderSession),
				Diagnostics:     diagnostics,
				Metrics:         metrics,
			}, nil
		}
	}

	return interfaces.WorkResult{
		DispatchID:      request.Dispatch.DispatchID,
		TransitionID:    request.Dispatch.TransitionID,
		Outcome:         outcome,
		Output:          resp.Content,
		ProviderSession: interfaces.CloneProviderSessionMetadata(resp.ProviderSession),
		Diagnostics:     diagnostics,
		Metrics:         metrics,
	}, nil
}

func agentWorkMetrics(start time.Time, retryCount int) interfaces.WorkMetrics {
	return interfaces.WorkMetrics{
		Duration:   time.Since(start),
		RetryCount: retryCount,
	}
}

func inferenceRequestForExecutionRequest(request interfaces.WorkstationExecutionRequest, workerDef *interfaces.WorkerConfig) interfaces.ProviderInferenceRequest {
	req := interfaces.ProviderInferenceRequest{
		Dispatch:                     interfaces.CloneWorkDispatch(request.Dispatch),
		WorkerType:                   request.WorkerType,
		WorkstationType:              inferenceWorkstationType(request),
		RunnerID:                     request.RunnerID,
		ProjectID:                    request.ProjectID,
		InputTokens:                  cloneRawInputTokens(request.InputTokens),
		ModelOperation:               request.ModelOperation,
		ModelBindings:                interfaces.CloneResolvedModelOperationBindings(request.ModelBindings),
		SystemPrompt:                 request.SystemPrompt,
		UserMessage:                  request.UserMessage,
		OutputSchema:                 request.OutputSchema,
		ToolExecutionMode:            interfaces.RunnerToolExecutionModeRequired,
		RequiredOptionalCapabilities: requiredRunnerOptionalCapabilities(request),
		EnvVars:                      cloneEnvVars(request.EnvVars),
		Worktree:                     request.Worktree,
		WorkingDirectory:             request.WorkingDirectory,
	}
	if workerDef != nil {
		req.Model = workerDef.Model
		req.ModelProvider = modelProviderForExecution(workerDef.ModelProvider, interfaces.ResolvedRunnerSelection{
			RunnerID: request.RunnerID,
			Source:   request.RunnerSelectionSource,
		})
		req.ModelLocality = workerDef.ModelLocality
		req.SessionID = workerDef.SessionID
		if workerDef.SessionID != "" {
			req.RequiredOptionalCapabilities = append(req.RequiredOptionalCapabilities, interfaces.RunnerOptionalCapabilitySessionResume)
		}
	}
	return req
}

func modelProviderForExecution(workerModelProvider string, selection interfaces.ResolvedRunnerSelection) string {
	if selection.Source == interfaces.RunnerSelectionSourceWorkstation || selection.Source == interfaces.RunnerSelectionSourceFactory {
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
	switch interfaces.NormalizeRunnerID(runnerID) {
	case interfaces.RunnerIDCodex:
		return string(ModelProviderCodex)
	case interfaces.RunnerIDGemini:
		return string(ModelProviderGemini)
	case interfaces.RunnerIDKiro:
		return string(ModelProviderKiro)
	case interfaces.RunnerIDCursorCLI:
		return string(ModelProviderCursor)
	case interfaces.RunnerIDOpenCode:
		return string(ModelProviderOpenCode)
	default:
		return ""
	}
}

func inferenceWorkstationType(request interfaces.WorkstationExecutionRequest) string {
	if request.WorkstationType != "" {
		return request.WorkstationType
	}
	return request.Dispatch.WorkstationName
}

func providerSessionFromError(providerErr *ProviderError) *interfaces.ProviderSessionMetadata {
	if providerErr == nil {
		return nil
	}
	return interfaces.CloneProviderSessionMetadata(providerErr.ProviderSession)
}

func providerDiagnosticsFromError(providerErr *ProviderError) *interfaces.WorkDiagnostics {
	if providerErr == nil {
		return nil
	}
	return providerErr.Diagnostics
}

func formatAgentProviderError(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message := strings.TrimSpace(providerErr.Message)
		if providerErr.Type == interfaces.ProviderErrorTypeTimeout && message == "execution timeout" {
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

func (ae *AgentExecutor) inferWithRetry(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, int, error) {
	logger := logging.EnsureLogger(ae.logger)
	retryCount := 0

	for {
		resp, err := ae.runner.Execute(ctx, req)
		if err == nil {
			return resp, retryCount, nil
		}

		providerErr := NormalizeProviderExecutionError(err)
		if providerErr == nil {
			return interfaces.InferenceResponse{}, retryCount, err
		}

		decision := ClassifyProviderFailure(providerErr)
		if !decision.Retryable || retryCount >= ae.retryConfig.maxRetries {
			return interfaces.InferenceResponse{}, retryCount, providerErr
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
			return interfaces.InferenceResponse{}, retryCount, err
		}
	}
}

type providerRunnerAdapter struct {
	inner Provider
}

// RunnerFromProvider adapts a legacy provider implementation onto the shared
// runner execution contract.
func RunnerFromProvider(provider Provider) Runner {
	return providerRunnerAdapter{inner: provider}
}

func (a providerRunnerAdapter) Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	if a.inner == nil {
		return interfaces.RunnerExecutionResult{}, NewProviderError(
			interfaces.ProviderErrorTypeMisconfigured,
			"runner requires a provider implementation",
			nil,
		)
	}
	return a.inner.Infer(ctx, request)
}

func requiredRunnerOptionalCapabilities(request interfaces.WorkstationExecutionRequest) []interfaces.RunnerOptionalCapability {
	capabilities := make([]interfaces.RunnerOptionalCapability, 0, 5)
	if request.OutputSchema != "" {
		capabilities = append(capabilities, interfaces.RunnerOptionalCapabilityStructuredOutput)
	}
	if request.WorkingDirectory != "" {
		capabilities = append(capabilities, interfaces.RunnerOptionalCapabilityWorkingDirectory)
	}
	if request.Worktree != "" {
		capabilities = append(capabilities, interfaces.RunnerOptionalCapabilityWorktree)
	}
	for _, token := range cloneInputTokens(request.InputTokens) {
		if tokenHasImageContent(token) {
			capabilities = append(capabilities, interfaces.RunnerOptionalCapabilityImageInput)
			break
		}
	}
	return capabilities
}

func tokenHasImageContent(token interfaces.Token) bool {
	for _, part := range token.Color.Content {
		if part.Type == interfaces.WorkContentPartTypeImage {
			return true
		}
	}
	return false
}

// evaluateOutcome determines the WorkOutcome based on stop token evaluation.
// When no stop token is configured, all successful provider responses are ACCEPTED.
// When a stop token is configured, the output is checked: found → ACCEPTED,
// <CONTINUE> → CONTINUE, otherwise → REJECTED.
func (ae *AgentExecutor) evaluateOutcome(resp interfaces.InferenceResponse, workerDef *interfaces.WorkerConfig) interfaces.WorkOutcome {
	if workerDef.StopToken == "" {
		ae.logger.Info("no stop token configured; defaulting to ACCEPTED outcome")
		return interfaces.OutcomeAccepted
	}
	if ContainsStopToken(resp.Content, workerDef.StopToken) {
		ae.logger.Info("stop token found in output; returning ACCEPTED outcome", "stop_token", workerDef.StopToken)
		return interfaces.OutcomeAccepted
	}
	if strings.Contains(resp.Content, "<CONTINUE>") {
		return interfaces.OutcomeContinue
	}
	return interfaces.OutcomeRejected
}

// parseOutputAgainstSchema parses the response content as JSON and validates
// it can be unmarshalled into TokenColor structs. The schema parameter is
// reserved for future schema validation; for MVP, we just validate JSON.
func parseOutputAgainstSchema(content string, _ []byte) ([]interfaces.TokenColor, error) {
	// Try parsing as array of token colors first.
	var colors []interfaces.TokenColor
	if err := json.Unmarshal([]byte(content), &colors); err == nil {
		return colors, nil
	}

	// Try parsing as a single token color.
	var color interfaces.TokenColor
	if err := json.Unmarshal([]byte(content), &color); err != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", err)
	}

	return []interfaces.TokenColor{color}, nil
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
