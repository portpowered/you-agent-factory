package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	"github.com/portpowered/infinite-you/pkg/workers/worktree"
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
	ParseJSON(response string, schema []byte) ([]factorytoken.Color, error)
}

// WorkstationExecutor wraps a WorkerExecutor with workstation-specific
// prompt rendering. This is what the Dispatcher actually calls.
//
// For MODEL_WORKSTATION: render prompt → call executor → parse output → WorkResult
// For LOGICAL_MOVE:      pass-through input colors → WorkResult (no worker call)
type WorkstationExecutor struct {
	RuntimeConfig   interfaces.RuntimeConfigLookup
	DefaultRunnerID string
	WorkflowContext *factory_context.FactoryContext
	Executor        WorkstationRequestExecutor
	Renderer        workerprompting.PromptRenderer
	Parser          OutputParser
	Logger          logging.Logger        // optional; nil → noop
	GitCommander    worktree.GitCommander // optional; nil → ExecGitCommander
}

const defaultSubprocessExecutionTimeout = 2 * time.Hour
const classifierFailureRawOutputLimit = 160

type resolvedWorkstationExecutionContext struct {
	ProjectID        string
	SessionID        string
	InputTokens      []factorytoken.Token
	EnvVars          map[string]string
	Worktree         string
	WorkingDirectory string
}

// Execute implements WorkerExecutor for WorkstationExecutor.
func (we *WorkstationExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
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
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "workstation not found: " + workstationLookupKey(dispatch),
			Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
		}, nil
	}

	switch workstationDef.Type {
	case workertaxonomy.WorkstationTypeLogical:
		return we.executeLogicalMove(dispatch, start), nil
	default:
		return we.executeModelWorkstation(ctx, dispatch, workstationDef, start)
	}
}

// executeLogicalMove passes input token colors through without calling any worker.
func (we *WorkstationExecutor) executeLogicalMove(dispatch work.WorkDispatch, start time.Time) workerexecution.WorkResult {
	logger := logging.EnsureLogger(we.Logger)

	logger.Info("logical move fired",
		WorkLogFields(dispatch.Execution,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"input_count", len(dispatch.InputTokens))...)

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Metrics: workerexecution.WorkMetrics{
			Duration: time.Since(start),
		},
	}
}

// executeModelWorkstation renders the prompt and calls the configured worker executor.
func (we *WorkstationExecutor) executeModelWorkstation(ctx context.Context, dispatch work.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time) (workerexecution.WorkResult, error) {
	logger := logging.EnsureLogger(we.Logger)
	invocationArgs := invocationArgumentsFromDispatch(dispatch)
	invocationDiagnostics := invocationDiagnosticsForDispatch(we.RuntimeConfig, invocationArgs)
	if invocationArgs != nil {
		interpolatedWorkstation, err := invocations.InterpolateWorkstationConfig(*workstationDef, invocationArgs, os.ReadFile)
		if err != nil {
			return workerexecution.WorkResult{
				DispatchID:   dispatch.DispatchID,
				TransitionID: dispatch.TransitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        err.Error(),
				Diagnostics:  invocationDiagnostics,
				Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
			}, nil
		}
		workstationDef = &interpolatedWorkstation
	}
	workerName := workstationWorkerName(workstationDef, dispatch)
	workerDef, ok := we.RuntimeConfig.Worker(workerName)
	if !ok {
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "worker config not found: " + workerName,
			Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
		}, nil
	}
	if invocationArgs != nil {
		interpolatedWorker, err := invocations.InterpolateWorkerConfig(*workerDef, invocationArgs, os.ReadFile)
		if err != nil {
			return workerexecution.WorkResult{
				DispatchID:   dispatch.DispatchID,
				TransitionID: dispatch.TransitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        err.Error(),
				Diagnostics:  invocationDiagnostics,
				Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
			}, nil
		}
		workerDef = &interpolatedWorker
	}

	resolvedContext, failed := we.resolveWorkstationExecutionContext(dispatch, workstationDef, start, logger)
	if failed != nil {
		return *failed, nil
	}
	if failed := we.applyCodexFactoryWorktreePreparation(ctx, dispatch, workstationDef, workerDef, &resolvedContext, start); failed != nil {
		return *failed, nil
	}

	request, failed := we.buildWorkstationExecutionRequest(dispatch, workerName, workerDef, workstationDef, resolvedContext, start, logger)
	if failed != nil {
		return *failed, nil
	}

	result, err := we.executeInnerWorker(ctx, request, workerDef, workstationDef, start, logger)
	result.Diagnostics = mergeWorkDiagnostics(result.Diagnostics, invocationDiagnostics)
	if err != nil {
		return result, err
	}
	if workstationDef.Type == workertaxonomy.WorkstationTypeClassify {
		return normalizeClassifierWorkResult(result), nil
	}
	return result, nil
}

func invocationDiagnosticsForDispatch(
	runtimeCfg interfaces.RuntimeFactoryConfigLookup,
	args *work.InvocationArguments,
) *workerexecution.WorkDiagnostics {
	if args == nil {
		return nil
	}
	var signature *interfaces.InvocationSignatureConfig
	if runtimeCfg != nil && runtimeCfg.FactoryConfig() != nil {
		signature = runtimeCfg.FactoryConfig().InvocationSignature
	}
	diagnostic := invocations.InvocationDiagnostic(signature, args)
	if diagnostic == nil {
		return nil
	}
	return &workerexecution.WorkDiagnostics{Invocation: diagnostic}
}

func invocationArgumentsFromDispatch(dispatch work.WorkDispatch) *work.InvocationArguments {
	for _, raw := range dispatch.InputTokens {
		token, ok := raw.(factorytoken.Token)
		if !ok {
			continue
		}
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if token.Color.InvocationArguments != nil {
			return work.CloneInvocationArguments(token.Color.InvocationArguments)
		}
	}
	return nil
}

func (we *WorkstationExecutor) resolveWorkstationExecutionContext(dispatch work.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time, logger logging.Logger) (resolvedWorkstationExecutionContext, *workerexecution.WorkResult) {
	requestContext := resolvedWorkstationExecutionContext{
		ProjectID:   dispatch.ProjectID,
		SessionID:   factorySessionIDFromWorkflowContext(we.WorkflowContext),
		InputTokens: workDispatchNonResourceTokensForWorkstation(dispatch, workstationDef),
	}

	if workstationDef.WorkingDirectory != "" || workstationDef.Worktree != "" || len(workstationDef.Env) > 0 {
		templateContext := workstationTemplateContext(we.WorkflowContext, requestContext.ProjectID, requestContext.SessionID, we.RuntimeConfig)
		resolved, err := workerprompting.ResolveTemplateFields(
			workstationDef.WorkingDirectory,
			workstationDef.Env,
			requestContext.InputTokens,
			templateContext,
			workstationDef.Worktree,
		)
		if err != nil {
			logger.Error("parameterized field resolution failed",
				WorkLogFields(dispatch.Execution,
					"transition_id", dispatch.TransitionID,
					"dispatch_id", dispatch.DispatchID,
					"error", err)...)
			failed := workerexecution.WorkResult{
				DispatchID:   dispatch.DispatchID,
				TransitionID: dispatch.TransitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        "parameterized field resolution failed: " + err.Error(),
				Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
			}
			return resolvedWorkstationExecutionContext{}, &failed
		}

		runtimeBaseDir := ""
		if we != nil && we.RuntimeConfig != nil {
			runtimeBaseDir = we.RuntimeConfig.RuntimeBaseDir()
		}
		resolved.WorkingDirectory = resolveRuntimePath(runtimeBaseDir, resolved.WorkingDirectory)
		requestContext.EnvVars = cloneEnvVars(resolved.Env)

		if resolved.Worktree != "" {
			logger.Debug("resolved worktree", "worktree", resolved.Worktree)
			requestContext.Worktree = resolved.Worktree
		}

		if resolved.WorkingDirectory != "" {
			logger.Debug("resolved working directory", "working_directory", resolved.WorkingDirectory)
			requestContext.WorkingDirectory = resolved.WorkingDirectory
		}
	}
	if requestContext.WorkingDirectory == "" && requestContext.Worktree == "" {
		requestContext.WorkingDirectory = defaultRuntimeWorkingDirectory(we.RuntimeConfig)
	}
	return requestContext, nil
}

func workstationTemplateContext(base *factory_context.FactoryContext, projectID, sessionID string, runtimeCfg interfaces.RuntimeConfigLookup) *factory_context.FactoryContext {
	var templateContext factory_context.FactoryContext
	if base != nil {
		templateContext = *base
		templateContext.EnvVars = cloneEnvVars(base.EnvVars)
	}

	if projectID != "" {
		templateContext.ProjectID = projectID
	}
	templateContext.SessionID = factorySessionIDFromWorkflowContext(&templateContext)
	if sessionID != "" {
		templateContext.SessionID = sessionID
	}
	if templateContext.WorkDirectory == "" {
		templateContext.WorkDirectory = defaultRuntimeWorkingDirectory(runtimeCfg)
	}

	return &templateContext
}

func defaultRuntimeWorkingDirectory(runtimeCfg interfaces.RuntimeConfigLookup) string {
	if runtimeCfg == nil {
		return ""
	}
	baseDir := strings.TrimSpace(runtimeCfg.RuntimeBaseDir())
	if baseDir == "" {
		baseDir = strings.TrimSpace(runtimeCfg.FactoryDir())
	}
	if baseDir == "" {
		return ""
	}
	return filepath.Clean(baseDir)
}

func (we *WorkstationExecutor) applyCodexFactoryWorktreePreparation(
	ctx context.Context,
	dispatch work.WorkDispatch,
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	requestContext *resolvedWorkstationExecutionContext,
	start time.Time,
) *workerexecution.WorkResult {
	selection := workerrunner.ResolveRunnerSelection(workstationDef.Runner, we.DefaultRunnerID, workerDef.ModelProvider)
	executionProvider := modelProviderForExecution(workerDef.ModelProvider, selection)
	if !worktree.ShouldPrepareFactoryWorktreeForCodex(executionProvider, workstationDef.WorkingDirectory, requestContext.Worktree) {
		return nil
	}
	if we.RuntimeConfig == nil {
		failed := worktree.FailedWorkResultFromPreparation(
			dispatch.DispatchID,
			dispatch.TransitionID,
			start,
			fmt.Errorf("factory directory unavailable"),
		)
		return &failed
	}
	factoryRoot := strings.TrimSpace(we.RuntimeConfig.FactoryDir())
	if factoryRoot == "" {
		failed := worktree.FailedWorkResultFromPreparation(
			dispatch.DispatchID,
			dispatch.TransitionID,
			start,
			fmt.Errorf("factory directory unavailable"),
		)
		return &failed
	}

	git := we.gitCommander()
	prepared, err := worktree.PrepareFactoryGitWorktree(ctx, factoryRoot, requestContext.Worktree, git)
	if err != nil {
		failed := worktree.FailedWorkResultFromPreparation(dispatch.DispatchID, dispatch.TransitionID, start, err)
		return &failed
	}
	requestContext.WorkingDirectory = prepared.CheckoutPath
	return nil
}

func (we *WorkstationExecutor) gitCommander() worktree.GitCommander {
	if we != nil && we.GitCommander != nil {
		return we.GitCommander
	}
	return worktree.ExecGitCommander{}
}

func resolveRuntimePath(baseDir, value string) string {
	if value == "" {
		return value
	}
	normalized := filepath.FromSlash(value)
	if filepath.IsAbs(normalized) && (!portableRuntimeRootedPath(value) || pathExists(normalized)) {
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

func pathExists(value string) bool {
	_, err := os.Stat(value)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func (we *WorkstationExecutor) buildWorkstationExecutionRequest(dispatch work.WorkDispatch, workerName string, workerDef *workerconfig.Config, workstationDef *interfaces.FactoryWorkstationConfig, requestContext resolvedWorkstationExecutionContext, start time.Time, logger logging.Logger) (workerexecution.WorkstationExecutionRequest, *workerexecution.WorkResult) {
	modelBindings, err := resolveModelOperationBindings(workstationDef, workerDef, requestContext.InputTokens)
	if err != nil {
		logger.Error("model operation binding resolution failed",
			WorkLogFields(dispatch.Execution,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"operation", workstationDef.Operation,
				"error", err)...)
		failed := workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "model operation binding resolution failed: " + err.Error(),
			Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
		}
		return workerexecution.WorkstationExecutionRequest{}, &failed
	}

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
		failed := workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "prompt render failed: " + err.Error(),
			Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
		}
		return workerexecution.WorkstationExecutionRequest{}, &failed
	}

	selection := workerrunner.ResolveRunnerSelection(workstationDef.Runner, we.DefaultRunnerID, workerDef.ModelProvider)
	return workerexecution.WorkstationExecutionRequest{
		Dispatch:                 work.CloneWorkDispatch(dispatch),
		WorkerType:               workerName,
		WorkstationType:          dispatch.WorkstationName,
		RunnerID:                 selection.RunnerID,
		RunnerSelectionSource:    selection.Source,
		ProjectID:                requestContext.ProjectID,
		FactorySessionID:         requestContext.SessionID,
		InputTokens:              InputTokens(requestContext.InputTokens...),
		ModelOperation:           workstationDef.Operation,
		ModelBindings:            modelBindings,
		Model:                    workerDef.Model,
		ModelProvider:            workerDef.ModelProvider,
		SystemPrompt:             workerDef.Body,
		UserMessage:              rendered,
		OutputSchema:             workstationDef.OutputSchema,
		EnvVars:                  cloneEnvVars(requestContext.EnvVars),
		Worktree:                 requestContext.Worktree,
		WorkingDirectory:         requestContext.WorkingDirectory,
		WorkingDirectoryAuthored: workstationDef.WorkingDirectory != "",
	}, nil
}

func (we *WorkstationExecutor) executeInnerWorker(ctx context.Context, request workerexecution.WorkstationExecutionRequest, workerDef *workerconfig.Config, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time, logger logging.Logger) (workerexecution.WorkResult, error) {
	executorCtx := ctx
	executionTimeout, err := resolveExecutionTimeout(workerDef, workstationDef)
	if err != nil {
		return workerexecution.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        err.Error(),
			Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
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
		if errors.Is(executorCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return timeoutWorkResult(request.Dispatch, time.Since(start)), nil
		}
		logger.Error("executor failed",
			WorkLogFields(request.Dispatch.Execution,
				"transition_id", request.Dispatch.TransitionID,
				"dispatch_id", request.Dispatch.DispatchID,
				"error", err)...)
		return workerexecution.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "executor failed: " + err.Error(),
			Metrics:      workerexecution.WorkMetrics{Duration: time.Since(start)},
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

func normalizeClassifierWorkResult(result workerexecution.WorkResult) workerexecution.WorkResult {
	if result.Outcome == workerexecution.OutcomeFailed {
		return result
	}

	label, err := normalizeClassifierLabel(result.Output)
	if err != nil {
		result.Outcome = workerexecution.OutcomeFailed
		result.Error = classifierOutputErrorDetail(result.Output, err)
		return result
	}

	result.Outcome = workerexecution.OutcomeAccepted
	result.Output = label
	result.Feedback = ""
	return result
}

func normalizeClassifierLabel(output string) (string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", fmt.Errorf("empty label")
	}
	if json.Valid([]byte(trimmed)) {
		return "", fmt.Errorf("expected plain string label")
	}

	return trimmed, nil
}

func classifierOutputErrorDetail(rawOutput string, err error) string {
	detail := "classifier output invalid: " + err.Error()
	evidence := classifierRawOutputEvidence(rawOutput)
	if evidence == "" {
		return detail
	}
	return detail + " (raw output: " + evidence + ")"
}

func classifierRawOutputEvidence(rawOutput string) string {
	trimmed := strings.TrimSpace(rawOutput)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > classifierFailureRawOutputLimit {
		trimmed = trimmed[:classifierFailureRawOutputLimit] + "..."
	}
	return strconv.Quote(trimmed)
}

// Compile-time check.
var _ WorkerExecutor = (*WorkstationExecutor)(nil)

func executionRequestInputTokens(request workerexecution.WorkstationExecutionRequest) []factorytoken.Token {
	return cloneInputTokens(request.InputTokens)
}

func executionRequestContext(request workerexecution.WorkstationExecutionRequest) *factory_context.FactoryContext {
	if request.WorkingDirectory == "" && len(request.EnvVars) == 0 && request.ProjectID == "" && request.FactorySessionID == "" {
		return nil
	}

	ctx := &factory_context.FactoryContext{
		ProjectID:     request.ProjectID,
		SessionID:     request.FactorySessionID,
		WorkDirectory: request.WorkingDirectory,
		EnvVars:       cloneEnvVars(request.EnvVars),
	}
	if ctx.WorkDirectory == "" {
		ctx.WorkDirectory = request.Worktree
	}
	return ctx
}

func (ctx resolvedWorkstationExecutionContext) factoryContext() *factory_context.FactoryContext {
	if ctx.WorkingDirectory == "" && ctx.Worktree == "" && len(ctx.EnvVars) == 0 && ctx.ProjectID == "" && ctx.SessionID == "" {
		return nil
	}

	requestContext := &factory_context.FactoryContext{
		ProjectID:     ctx.ProjectID,
		SessionID:     ctx.SessionID,
		WorkDirectory: ctx.WorkingDirectory,
		EnvVars:       cloneEnvVars(ctx.EnvVars),
	}
	if requestContext.WorkDirectory == "" {
		requestContext.WorkDirectory = ctx.Worktree
	}
	return requestContext
}

func factorySessionIDFromWorkflowContext(wfCtx *factory_context.FactoryContext) string {
	if wfCtx == nil || strings.TrimSpace(wfCtx.SessionID) == "" {
		return factory_context.DefaultSessionID
	}
	return strings.TrimSpace(wfCtx.SessionID)
}

func (we *WorkstationExecutor) runtimeWorkstation(dispatch work.WorkDispatch) (*interfaces.FactoryWorkstationConfig, bool) {
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
	if !ok || workerDef.Type != workertaxonomy.WorkerTypeScript {
		return nil, false
	}
	fallback := *workstationDef
	fallback.Type = workertaxonomy.WorkstationTypeModel
	return &fallback, true
}

func workstationWorkerName(workstationDef *interfaces.FactoryWorkstationConfig, dispatch work.WorkDispatch) string {
	if workstationDef != nil && workstationDef.WorkerTypeName != "" {
		return workstationDef.WorkerTypeName
	}
	return dispatch.WorkerType
}

func workstationLookupKey(dispatch work.WorkDispatch) string {
	return dispatch.WorkstationName
}

func resolveExecutionTimeout(workerDef *workerconfig.Config, workstationDef *interfaces.FactoryWorkstationConfig) (time.Duration, error) {
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
			return 0, fmt.Errorf("invalid worker timeout %q: %w", workerDef.Timeout, err)
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

func timeoutWorkResult(dispatch work.WorkDispatch, duration time.Duration) workerexecution.WorkResult {
	failureMetadata := &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyRetryable,
		Type:   workerexecution.WorkFailureTypeTimeout,
	}
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         workerexecution.OutcomeFailed,
		Error:           "execution timeout",
		FailureMetadata: workerexecution.CloneWorkFailureMetadata(failureMetadata),
		Metrics:         workerexecution.WorkMetrics{Duration: duration},
	}
}
