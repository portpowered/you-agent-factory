package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/worktree"
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
	ParseJSON(response string, schema []byte) ([]workerexecution.Color, error)
}

// WorkstationExecutor wraps a WorkerExecutor with workstation-specific
// prompt rendering. This is what the Dispatcher actually calls.
//
// For MODEL_WORKSTATION: render prompt → call executor → parse output → WorkResult
// For LOGICAL_MOVE:      pass-through input colors → WorkResult (no worker call)
type WorkstationExecutor struct {
	ProcessEnvironment      func() []string
	CurrentWorkingDirectory func() (string, error)
	RuntimeConfig           interfaces.RuntimeConfigLookup
	DefaultRunnerID         string
	ResolveRunnerSelection  workerexecution.RunnerSelectionResolver
	ResolveProviderIdentity workerexecution.ProviderIdentityResolver
	WorkflowContext         *workerexecution.Context
	Executor                WorkstationRequestExecutor
	Interpolation           factorydefinitions.InvocationInterpolationService
	ExecutionPolicy         factorydefinitions.WorkstationExecutionPolicyService
	Renderer                workerprompting.PromptRenderer
	Parser                  OutputParser
	Logger                  logging.Logger // optional; nil → noop
	WorktreePreparer        workerexecution.FactoryWorktreePreparer
	FileSystem              platformfilesystem.ReadFileInspector
	Now                     func() time.Time
}

const defaultSubprocessExecutionTimeout = 2 * time.Hour
const classifierFailureRawOutputLimit = 160

type resolvedWorkstationExecutionContext struct {
	ProjectID        string
	SessionID        string
	InputTokens      []workerexecution.Token
	EnvVars          map[string]string
	Worktree         string
	WorkingDirectory string
}

// Execute implements WorkerExecutor for WorkstationExecutor.
func (we *WorkstationExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	if we == nil || we.Now == nil {
		return workerexecution.WorkResult{}, fmt.Errorf("workstation executor clock is required")
	}
	start := we.Now()
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
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}, nil
	}

	switch workstationDef.Type {
	case factorydefinitions.WorkstationTypeLogical:
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
			Duration: we.Now().Sub(start),
		},
	}
}

// executeModelWorkstation renders the prompt and calls the configured worker executor.
func (we *WorkstationExecutor) executeModelWorkstation(ctx context.Context, dispatch work.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time) (workerexecution.WorkResult, error) {
	logger := logging.EnsureLogger(we.Logger)
	invocationArgs := invocationArgumentsFromDispatch(dispatch)
	invocationDiagnostics := invocationDiagnosticsForDispatch(we.RuntimeConfig, invocationArgs)
	if we.Interpolation == nil && invocationArgs != nil {
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "Factory Definition invocation interpolation service is unavailable",
			Diagnostics:  invocationDiagnostics,
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}, nil
	}
	var readFile factorydefinitions.FileReader
	if we.FileSystem != nil {
		readFile = we.FileSystem.ReadFile
	}
	if we.Interpolation != nil {
		interpolatedWorkstation, err := we.Interpolation.InterpolateWorkstationConfig(*workstationDef, invocationArgs, readFile)
		if err != nil {
			return workerexecution.WorkResult{
				DispatchID:   dispatch.DispatchID,
				TransitionID: dispatch.TransitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        err.Error(),
				Diagnostics:  invocationDiagnostics,
				Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
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
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}, nil
	}
	if we.Interpolation != nil {
		interpolatedWorker, err := we.Interpolation.InterpolateWorkerConfig(*workerDef, invocationArgs, readFile)
		if err != nil {
			return workerexecution.WorkResult{
				DispatchID:   dispatch.DispatchID,
				TransitionID: dispatch.TransitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        err.Error(),
				Diagnostics:  invocationDiagnostics,
				Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
			}, nil
		}
		workerDef = &interpolatedWorker
		if strings.TrimSpace(workerDef.ModelProvider) == "" {
			workerDef.ModelProvider = workerDef.RuntimeDefaultModelProvider
		}
		if strings.TrimSpace(workerDef.Model) == "" {
			workerDef.Model = workerDef.RuntimeDefaultModel
		}
		if failed := we.resolveInvocationProvider(
			dispatch,
			workerDef,
			invocationDiagnostics,
			start,
		); failed != nil {
			return *failed, nil
		}
	}

	resolvedContext, failed := we.resolveWorkstationExecutionContext(dispatch, workstationDef, start, logger)
	if failed != nil {
		return *failed, nil
	}
	// TODO: we should make workers agnostic.
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
	if workstationDef.Type == factorydefinitions.WorkstationTypeClassify {
		return normalizeClassifierWorkResult(result), nil
	}
	return result, nil
}

func (we *WorkstationExecutor) resolveInvocationProvider(
	dispatch work.WorkDispatch,
	worker *factorydefinitions.FactoryWorkerConfig,
	diagnostics *workerexecution.WorkDiagnostics,
	start time.Time,
) *workerexecution.WorkResult {
	if we.ResolveProviderIdentity == nil || worker == nil ||
		strings.TrimSpace(worker.ModelProvider) == "" {
		return nil
	}
	canonical, err := we.ResolveProviderIdentity(worker.ModelProvider)
	if err == nil {
		worker.ModelProvider = canonical
		return nil
	}
	return &workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "invocation modelProvider selection failed: " + err.Error(),
		Diagnostics:  diagnostics,
		Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
	}
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
	diagnostic := invocationDiagnostic(signature, args)
	if diagnostic == nil {
		return nil
	}
	return &workerexecution.WorkDiagnostics{Invocation: diagnostic}
}

func invocationDiagnostic(
	signature *work.InvocationSignatureConfig,
	args *work.InvocationArguments,
) *workerexecution.InvocationDiagnostic {
	if signature == nil && (args == nil || len(args.Arguments) == 0) {
		return nil
	}
	diagnostic := &workerexecution.InvocationDiagnostic{SignatureHash: work.InvocationSignatureHash(signature)}
	if args == nil || len(args.Arguments) == 0 {
		if diagnostic.SignatureHash == "" {
			return nil
		}
		return diagnostic
	}
	names := make([]string, 0, len(args.Arguments))
	for name := range args.Arguments {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		argument := args.Arguments[name]
		entry := workerexecution.InvocationParameterDiagnostic{
			Name: name, ValueCount: len(argument.Values), Redacted: argument.Sensitive,
		}
		for _, source := range argument.Sources {
			if kind := strings.TrimSpace(source.Kind); kind != "" {
				entry.SourceKinds = append(entry.SourceKinds, kind)
			}
			entry.Redacted = entry.Redacted || source.Redact
		}
		diagnostic.Parameters = append(diagnostic.Parameters, entry)
	}
	return diagnostic
}

func invocationArgumentsFromDispatch(dispatch work.WorkDispatch) *work.InvocationArguments {
	for _, raw := range dispatch.InputTokens {
		token := factoryTokenFromDispatchInput(raw)
		if token == nil {
			continue
		}
		if token.Color.DataType == workerexecution.DataTypeResource {
			continue
		}
		if token.Color.InvocationArguments != nil {
			return work.CloneInvocationArguments(token.Color.InvocationArguments)
		}
	}
	return nil
}

func factoryTokenFromDispatchInput(raw any) *workerexecution.Token {
	switch token := raw.(type) {
	case workerexecution.Token:
		return &token
	case *workerexecution.Token:
		return token
	default:
		return nil
	}
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
				Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
			}
			return resolvedWorkstationExecutionContext{}, &failed
		}

		runtimeBaseDir := ""
		if we != nil && we.RuntimeConfig != nil {
			runtimeBaseDir = we.RuntimeConfig.RuntimeBaseDir()
		}
		resolved.WorkingDirectory = resolveRuntimePath(runtimeBaseDir, resolved.WorkingDirectory, we.CurrentWorkingDirectory, we.FileSystem)
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

func workstationTemplateContext(base *workerexecution.Context, projectID, sessionID string, runtimeCfg interfaces.RuntimeConfigLookup) *workerexecution.Context {
	var templateContext workerexecution.Context
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
	workerDef *factorydefinitions.FactoryWorkerConfig,
	requestContext *resolvedWorkstationExecutionContext,
	start time.Time,
) *workerexecution.WorkResult {
	selectionIdentity := workstationDef.Runner
	if identity, identityErr := workerexecution.RunnerIdentityForWorker(workerDef.ExecutorProvider, workerDef.ModelProvider); identityErr != nil {
		failed := worktree.FailedWorkResultFromPreparation(dispatch.DispatchID, dispatch.TransitionID, we.Now().Sub(start), identityErr)
		return &failed
	} else if identity != "" {
		selectionIdentity = identity
	}
	selection, err := we.resolveRunnerSelection(selectionIdentity, workerDef.ModelProvider)
	if err != nil {
		failed := worktree.FailedWorkResultFromPreparation(
			dispatch.DispatchID,
			dispatch.TransitionID,
			we.Now().Sub(start),
			err,
		)
		return &failed
	}
	executionProvider := modelProviderForExecution(workerDef.ModelProvider, selection)
	if !worktree.ShouldPrepareFactoryWorktreeForCodex(executionProvider, workstationDef.WorkingDirectory, requestContext.Worktree) {
		return nil
	}
	if we.RuntimeConfig == nil {
		failed := worktree.FailedWorkResultFromPreparation(
			dispatch.DispatchID,
			dispatch.TransitionID,
			we.Now().Sub(start),
			fmt.Errorf("factory directory unavailable"),
		)
		return &failed
	}
	factoryRoot := strings.TrimSpace(we.RuntimeConfig.FactoryDir())
	if factoryRoot == "" {
		failed := worktree.FailedWorkResultFromPreparation(
			dispatch.DispatchID,
			dispatch.TransitionID,
			we.Now().Sub(start),
			fmt.Errorf("factory directory unavailable"),
		)
		return &failed
	}

	if we.WorktreePreparer == nil {
		failed := worktree.FailedWorkResultFromPreparation(
			dispatch.DispatchID,
			dispatch.TransitionID,
			we.Now().Sub(start),
			fmt.Errorf("worktree preparer unavailable"),
		)
		return &failed
	}
	prepared, err := we.WorktreePreparer.Prepare(ctx, factoryRoot, requestContext.Worktree)
	if err != nil {
		failed := worktree.FailedWorkResultFromPreparation(dispatch.DispatchID, dispatch.TransitionID, we.Now().Sub(start), err)
		return &failed
	}
	requestContext.WorkingDirectory = prepared.CheckoutPath
	return nil
}

func resolveRuntimePath(baseDir, value string, currentWorkingDirectory func() (string, error), fileSystem platformfilesystem.ReadFileInspector) string {
	if value == "" {
		return value
	}
	normalized := filepath.FromSlash(value)
	if filepath.IsAbs(normalized) && (!portableRuntimeRootedPath(value) || pathExists(fileSystem, normalized)) {
		return filepath.Clean(normalized)
	}
	if baseDir != "" {
		return filepath.Clean(filepath.Join(baseDir, normalized))
	}
	if currentWorkingDirectory == nil {
		return filepath.Clean(normalized)
	}
	workingDirectory, err := currentWorkingDirectory()
	if err != nil || workingDirectory == "" {
		return filepath.Clean(normalized)
	}
	return filepath.Clean(filepath.Join(workingDirectory, normalized))
}

func portableRuntimeRootedPath(value string) bool {
	return filepath.VolumeName(value) == "" && strings.HasPrefix(value, "/")
}

func pathExists(fileSystem platformfilesystem.ReadFileInspector, value string) bool {
	if fileSystem == nil {
		return false
	}
	_, err := fileSystem.Stat(value)
	return err == nil || !errors.Is(err, fs.ErrNotExist)
}

func (we *WorkstationExecutor) buildWorkstationExecutionRequest(dispatch work.WorkDispatch, workerName string, workerDef *factorydefinitions.FactoryWorkerConfig, workstationDef *interfaces.FactoryWorkstationConfig, requestContext resolvedWorkstationExecutionContext, start time.Time, logger logging.Logger) (workerexecution.WorkstationExecutionRequest, *workerexecution.WorkResult) {
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
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
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
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}
		return workerexecution.WorkstationExecutionRequest{}, &failed
	}

	selection, err := we.resolveRunnerSelection(workstationDef.Runner, workerDef.ModelProvider)
	if err != nil {
		failed := workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "provider selection failed: " + err.Error(),
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}
		return workerexecution.WorkstationExecutionRequest{}, &failed
	}
	return workerexecution.WorkstationExecutionRequest{
		Dispatch:                 work.CloneWorkDispatch(dispatch),
		WorkerType:               workerName,
		WorkstationType:          dispatch.WorkstationName,
		RunnerID:                 selection.RunnerID,
		RunnerSelectionSource:    selection.Source,
		ExecutorProvider:         workerDef.ExecutorProvider,
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
		ProcessEnvironment:       processEnvironment(we.ProcessEnvironment),
		Worktree:                 requestContext.Worktree,
		WorkingDirectory:         requestContext.WorkingDirectory,
		WorkingDirectoryAuthored: workstationDef.WorkingDirectory != "",
	}, nil
}

func (we *WorkstationExecutor) resolveRunnerSelection(
	workstationRunner string,
	workerModelProvider string,
) (workerexecution.ResolvedRunnerSelection, error) {
	if we.ResolveRunnerSelection != nil {
		return we.ResolveRunnerSelection(
			workstationRunner,
			we.DefaultRunnerID,
			workerModelProvider,
		)
	}
	return workerrunner.ResolveRunnerSelection(
		workstationRunner,
		we.DefaultRunnerID,
		workerModelProvider,
	), nil
}

func processEnvironment(read func() []string) []string {
	if read == nil {
		return nil
	}
	return append([]string(nil), read()...)
}

func (we *WorkstationExecutor) executeInnerWorker(ctx context.Context, request workerexecution.WorkstationExecutionRequest, workerDef *factorydefinitions.FactoryWorkerConfig, workstationDef *interfaces.FactoryWorkstationConfig, start time.Time, logger logging.Logger) (workerexecution.WorkResult, error) {
	executorCtx := ctx
	executionTimeout, err := resolveExecutionTimeout(
		we.ExecutionPolicy,
		workerDef,
		workstationDef,
	)
	if err != nil {
		return workerexecution.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        err.Error(),
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}, nil
	}
	if executionTimeout > 0 {
		var cancel context.CancelFunc
		executorCtx, cancel = context.WithTimeout(ctx, executionTimeout)
		defer cancel()
	}

	// Script Workers carry interpolatable command arguments, so execute from
	// the per-dispatch definition rather than the invocation-neutral runner
	// captured when the Factory Runtime opened.
	var result workerexecution.WorkResult
	if interpolated, ok := we.Executor.(interface {
		ExecuteWithWorker(context.Context, workerexecution.WorkstationExecutionRequest, *factorydefinitions.FactoryWorkerConfig) (workerexecution.WorkResult, error)
	}); ok {
		result, err = interpolated.ExecuteWithWorker(executorCtx, request, workerDef)
	} else {
		result, err = we.Executor.Execute(executorCtx, request)
	}
	if err != nil {
		if errors.Is(executorCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return timeoutWorkResult(request.Dispatch, we.Now().Sub(start)), nil
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
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}, nil
	}

	logger.Info("workstation: executor result",
		WorkLogFields(request.Dispatch.Execution,
			"transition_id", request.Dispatch.TransitionID,
			"dispatch_id", request.Dispatch.DispatchID,
			"outcome", result.Outcome)...)
	result.Metrics.Duration = we.Now().Sub(start)
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

func executionRequestInputTokens(request workerexecution.WorkstationExecutionRequest) []workerexecution.Token {
	return cloneInputTokens(request.InputTokens)
}

func (ctx resolvedWorkstationExecutionContext) factoryContext() *workerexecution.Context {
	if ctx.WorkingDirectory == "" && ctx.Worktree == "" && len(ctx.EnvVars) == 0 && ctx.ProjectID == "" && ctx.SessionID == "" {
		return nil
	}

	requestContext := &workerexecution.Context{
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

func factorySessionIDFromWorkflowContext(wfCtx *workerexecution.Context) string {
	if wfCtx == nil || strings.TrimSpace(wfCtx.SessionID) == "" {
		return workerexecution.DefaultSessionID
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
	if !ok || workerDef.Type != factorydefinitions.WorkerTypeScript {
		return nil, false
	}
	fallback := *workstationDef
	fallback.Type = factorydefinitions.WorkstationTypeModel
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

func resolveExecutionTimeout(
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	workstationDef *interfaces.FactoryWorkstationConfig,
) (time.Duration, error) {
	if executionPolicy != nil && workstationDef != nil {
		timeout, err := executionPolicy.ExecutionTimeout(workstationDef)
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
