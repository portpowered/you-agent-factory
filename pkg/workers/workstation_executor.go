package workers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/agent-factory/pkg/config"
	factory_context "github.com/portpowered/agent-factory/pkg/factory/context"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/logging"
)

// AgentContext is the merged execution context assembled from
// worker AGENTS.md + workstation AGENTS.md at execution time.
type AgentContext struct {
	SystemPrompt string `json:"system_prompt"` // worker AGENTS.md body
	UserMessage  string `json:"user_message"`  // rendered workstation prompt
	Tools        []Tool `json:"tools,omitempty"`
	OutputSchema []byte `json:"output_schema,omitempty"`
}

// Tool describes a tool available to the agent during execution.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// OutputParser parses structured output from a worker execution response.
type OutputParser interface {
	ParseJSON(response string, schema []byte) ([]interfaces.TokenColor, error)
}

// WorkstationExecutor wraps a WorkerExecutor with workstation-specific
// prompt rendering. This is what the Dispatcher actually calls.
//
// For MODEL_WORKSTATION: render prompt → call executor → parse output → WorkResult
// For LOGICAL_MOVE:      pass-through input colors → WorkResult (no worker call)
type WorkstationExecutor struct {
	RuntimeConfig interfaces.RuntimeConfigLookup
	Executor      WorkstationRequestExecutor
	Renderer      PromptRenderer
	Parser        OutputParser
	Logger        logging.Logger // optional; nil → noop
}

const defaultSubprocessExecutionTimeout = 2 * time.Hour

type resolvedWorkstationExecutionContext struct {
	ProjectID        string
	InputTokens      []interfaces.Token
	EnvVars          map[string]string
	Worktree         string
	WorkingDirectory string
}

// Execute implements WorkerExecutor for WorkstationExecutor.
func (we *WorkstationExecutor) Execute(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	start := time.Now()
	logger := logging.EnsureLogger(we.Logger)
	logger.Info("workstation: execution entered",
		WorkLogFields(dispatch.Execution,
			"worker_type", dispatch.WorkerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"workstation", dispatch.WorkstationName)...)
	workstationDef, ok := we.runtimeWorkstation(dispatch)
	if !ok {
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        "workstation not found: " + workstationLookupKey(dispatch),
			Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
		}, nil
	}

	switch workstationDef.Type {
	case interfaces.WorkstationTypeLogical:
		return we.executeLogicalMove(dispatch, start), nil
	default:
		return we.executeModelWorkstation(ctx, dispatch, workstationDef, start)
	}
}

// executeLogicalMove passes input token colors through without calling any worker.
func (we *WorkstationExecutor) executeLogicalMove(dispatch interfaces.WorkDispatch, start time.Time) interfaces.WorkResult {
	logger := logging.EnsureLogger(we.Logger)

	logger.Info("logical move fired",
		WorkLogFields(dispatch.Execution,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"input_count", len(dispatch.InputTokens))...)

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Metrics: interfaces.WorkMetrics{
			Duration: time.Since(start),
		},
	}
}

// executeModelWorkstation renders the prompt and calls the configured worker executor.
func (we *WorkstationExecutor) executeModelWorkstation(ctx context.Context, dispatch interfaces.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time) (interfaces.WorkResult, error) {
	logger := logging.EnsureLogger(we.Logger)
	workerName := workstationWorkerName(workstationDef, dispatch)
	workerDef, ok := we.RuntimeConfig.Worker(workerName)
	if !ok {
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        "worker config not found: " + workerName,
			Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
		}, nil
	}

	resolvedContext, failed := we.resolveWorkstationExecutionContext(dispatch, workstationDef, start, logger)
	if failed != nil {
		return *failed, nil
	}

	request, failed := we.buildWorkstationExecutionRequest(dispatch, workerName, workerDef, workstationDef, resolvedContext, start, logger)
	if failed != nil {
		return *failed, nil
	}

	return we.executeInnerWorker(ctx, request, workerDef, workstationDef, start, logger)
}

func (we *WorkstationExecutor) resolveWorkstationExecutionContext(dispatch interfaces.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time, logger logging.Logger) (resolvedWorkstationExecutionContext, *interfaces.WorkResult) {
	requestContext := resolvedWorkstationExecutionContext{
		ProjectID:   dispatch.ProjectID,
		InputTokens: workDispatchNonResourceTokensForWorkstation(dispatch, workstationDef),
	}

	if workstationDef.WorkingDirectory != "" || workstationDef.Worktree != "" || len(workstationDef.Env) > 0 {
		resolved, err := ResolveTemplateFields(
			workstationDef.WorkingDirectory,
			workstationDef.Env,
			requestContext.InputTokens,
			requestContext.factoryContext(),
			workstationDef.Worktree,
		)
		if err != nil {
			logger.Error("parameterized field resolution failed",
				WorkLogFields(dispatch.Execution,
					"transition_id", dispatch.TransitionID,
					"dispatch_id", dispatch.DispatchID,
					"error", err)...)
			failed := interfaces.WorkResult{
				DispatchID:   dispatch.DispatchID,
				TransitionID: dispatch.TransitionID,
				Outcome:      interfaces.OutcomeFailed,
				Error:        "parameterized field resolution failed: " + err.Error(),
				Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
			}
			return resolvedWorkstationExecutionContext{}, &failed
		}

		runtimeBaseDir := ""
		if we != nil && we.RuntimeConfig != nil {
			runtimeBaseDir = we.RuntimeConfig.RuntimeBaseDir()
		}
		resolved.WorkingDirectory = resolveRuntimePath(runtimeBaseDir, resolved.WorkingDirectory)

		appliedContext := applyResolvedFields(requestContext.factoryContext(), resolved)
		if appliedContext != nil {
			requestContext.ProjectID = appliedContext.ProjectID
			requestContext.WorkingDirectory = appliedContext.WorkDirectory
			requestContext.EnvVars = cloneEnvVars(appliedContext.EnvVars)
		}

		if resolved.Worktree != "" {
			logger.Debug("resolved worktree", "worktree", resolved.Worktree)
			requestContext.Worktree = resolved.Worktree
		}

		if resolved.WorkingDirectory != "" {
			logger.Debug("resolved working directory", "working_directory", resolved.WorkingDirectory)
			requestContext.WorkingDirectory = resolved.WorkingDirectory
		}
	}
	return requestContext, nil
}

func resolveRuntimePath(baseDir, value string) string {
	if value == "" {
		return value
	}
	normalized := filepath.FromSlash(value)
	if !portableRuntimeRootedPath(value) && filepath.IsAbs(normalized) {
		return filepath.Clean(normalized)
	}
	if baseDir != "" {
		return filepath.Clean(filepath.Join(baseDir, normalized))
	}
	workingDirectory, err := os.Getwd()
	if err != nil || workingDirectory == "" {
		return filepath.Clean(normalized)
	}
	return filepath.Clean(filepath.Join(workingDirectory, normalized))
}

func portableRuntimeRootedPath(value string) bool {
	return filepath.VolumeName(value) == "" && strings.HasPrefix(value, "/")
}

func (we *WorkstationExecutor) buildWorkstationExecutionRequest(dispatch interfaces.WorkDispatch, workerName string, workerDef *interfaces.WorkerConfig, workstationDef *interfaces.FactoryWorkstationConfig, requestContext resolvedWorkstationExecutionContext, start time.Time, logger logging.Logger) (interfaces.WorkstationExecutionRequest, *interfaces.WorkResult) {
	rendered, err := we.Renderer.Render(
		workstationDef.PromptTemplate,
		requestContext.InputTokens,
		requestContext.factoryContext(),
	)
	if err != nil {
		logger.Error("prompt render failed",
			WorkLogFields(dispatch.Execution,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"prompt_template", workstationDef.PromptTemplate,
				"error", err)...)
		failed := interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        "prompt render failed: " + err.Error(),
			Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
		}
		return interfaces.WorkstationExecutionRequest{}, &failed
	}

	return interfaces.WorkstationExecutionRequest{
		Dispatch:         interfaces.CloneWorkDispatch(dispatch),
		WorkerType:       workerName,
		WorkstationType:  dispatch.WorkstationName,
		ProjectID:        requestContext.ProjectID,
		InputTokens:      InputTokens(requestContext.InputTokens...),
		SystemPrompt:     workerDef.Body,
		UserMessage:      rendered,
		OutputSchema:     workstationDef.OutputSchema,
		EnvVars:          cloneEnvVars(requestContext.EnvVars),
		Worktree:         requestContext.Worktree,
		WorkingDirectory: requestContext.WorkingDirectory,
	}, nil
}

func (we *WorkstationExecutor) executeInnerWorker(ctx context.Context, request interfaces.WorkstationExecutionRequest, workerDef *interfaces.WorkerConfig, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time, logger logging.Logger) (interfaces.WorkResult, error) {
	executorCtx := ctx
	executionTimeout, err := resolveExecutionTimeout(workerDef, workstationDef)
	if err != nil {
		return interfaces.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        err.Error(),
			Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
		}, nil
	}
	if executionTimeout > 0 {
		var cancel context.CancelFunc
		executorCtx, cancel = context.WithTimeout(ctx, executionTimeout)
		defer cancel()
	}

	// Call the underlying worker executor.
	result, err := we.Executor.Execute(executorCtx, request)
	if err != nil {
		if executorCtx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			return timeoutWorkResult(request.Dispatch, time.Since(start)), nil
		}
		logger.Error("executor failed",
			WorkLogFields(request.Dispatch.Execution,
				"transition_id", request.Dispatch.TransitionID,
				"dispatch_id", request.Dispatch.DispatchID,
				"error", err)...)
		return interfaces.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        "executor failed: " + err.Error(),
			Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
		}, nil
	}

	logger.Info("workstation: executor result",
		WorkLogFields(request.Dispatch.Execution,
			"transition_id", request.Dispatch.TransitionID,
			"dispatch_id", request.Dispatch.DispatchID,
			"outcome", result.Outcome)...)
	result.Metrics.Duration = time.Since(start)
	return result, nil
}

// Compile-time check.
var _ WorkerExecutor = (*WorkstationExecutor)(nil)

func executionRequestInputTokens(request interfaces.WorkstationExecutionRequest) []interfaces.Token {
	return cloneInputTokens(request.InputTokens)
}

func executionRequestContext(request interfaces.WorkstationExecutionRequest) *factory_context.FactoryContext {
	if request.WorkingDirectory == "" && len(request.EnvVars) == 0 && request.ProjectID == "" {
		return nil
	}

	ctx := &factory_context.FactoryContext{
		ProjectID:     request.ProjectID,
		WorkDirectory: request.WorkingDirectory,
		EnvVars:       cloneEnvVars(request.EnvVars),
	}
	if ctx.WorkDirectory == "" {
		ctx.WorkDirectory = request.Worktree
	}
	return ctx
}

func (ctx resolvedWorkstationExecutionContext) factoryContext() *factory_context.FactoryContext {
	if ctx.WorkingDirectory == "" && ctx.Worktree == "" && len(ctx.EnvVars) == 0 && ctx.ProjectID == "" {
		return nil
	}

	requestContext := &factory_context.FactoryContext{
		ProjectID:     ctx.ProjectID,
		WorkDirectory: ctx.WorkingDirectory,
		EnvVars:       cloneEnvVars(ctx.EnvVars),
	}
	if requestContext.WorkDirectory == "" {
		requestContext.WorkDirectory = ctx.Worktree
	}
	return requestContext
}

func (we *WorkstationExecutor) runtimeWorkstation(dispatch interfaces.WorkDispatch) (*interfaces.FactoryWorkstationConfig, bool) {
	if we.RuntimeConfig == nil {
		return nil, false
	}
	workstationDef, ok := we.RuntimeConfig.Workstation(workstationLookupKey(dispatch))
	if !ok || workstationDef == nil {
		return nil, false
	}
	if workstationDef.Type != "" {
		return workstationDef, true
	}
	workerName := workstationWorkerName(workstationDef, dispatch)
	workerDef, ok := we.RuntimeConfig.Worker(workerName)
	if !ok || workerDef.Type != interfaces.WorkerTypeScript {
		return nil, false
	}
	fallback := *workstationDef
	fallback.Type = interfaces.WorkstationTypeModel
	return &fallback, true
}

func workstationWorkerName(workstationDef *interfaces.FactoryWorkstationConfig, dispatch interfaces.WorkDispatch) string {
	if workstationDef != nil && workstationDef.WorkerTypeName != "" {
		return workstationDef.WorkerTypeName
	}
	return dispatch.WorkerType
}

func workstationLookupKey(dispatch interfaces.WorkDispatch) string {
	return dispatch.WorkstationName
}

func resolveExecutionTimeout(workerDef *interfaces.WorkerConfig, workstationDef *interfaces.FactoryWorkstationConfig) (time.Duration, error) {
	if workstationDef != nil {
		timeout, err := config.WorkstationExecutionTimeout(workstationDef)
		if err != nil {
			return 0, err
		}
		if timeout > 0 {
			return timeout, nil
		}
	}

	if workerDef != nil && workerDef.Timeout != "" {
		timeout, err := time.ParseDuration(workerDef.Timeout)
		if err != nil {
			return 0, fmt.Errorf("invalid worker timeout %q: %v", workerDef.Timeout, err)
		}
		if timeout > 0 {
			return timeout, nil
		}
	}

	if workerDef != nil && workerDef.Type != "" {
		return defaultSubprocessExecutionTimeout, nil
	}

	return 0, nil
}

func timeoutWorkResult(dispatch interfaces.WorkDispatch, duration time.Duration) interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeFailed,
		Error:        "execution timeout",
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeTimeout,
		},
		Metrics: interfaces.WorkMetrics{Duration: duration},
	}
}
