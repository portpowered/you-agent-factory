package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
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
	RuntimeConfig   interfaces.RuntimeConfigLookup
	DefaultRunnerID string
	Executor        WorkstationRequestExecutor
	CommandRunner   CommandRunner
	Renderer        PromptRenderer
	Parser          OutputParser
	Logger          logging.Logger // optional; nil → noop
}

const defaultSubprocessExecutionTimeout = 2 * time.Hour
const codexWorktreeValidationTimeout = 5 * time.Second
const codexWorktreeCreationTimeout = 30 * time.Second

type resolvedWorkstationExecutionContext struct {
	ProjectID        string
	InputTokens      []interfaces.Token
	EnvVars          map[string]string
	WorktreeHandling string
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

	request, failed := we.buildWorkstationExecutionRequest(ctx, dispatch, workerName, workerDef, workstationDef, resolvedContext, start, logger)
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

const (
	codexWorktreeHandlingReuseWorkingDirectory = "reuse_working_directory_overlap"
	codexWorktreeHandlingReuseExistingWorktree = "reuse_existing_worktree"
	codexWorktreeHandlingCreateMissingWorktree = "create_missing_worktree"
)

func (we *WorkstationExecutor) prepareCodexWorktreeRequest(ctx context.Context, request interfaces.WorkstationExecutionRequest, workerDef *interfaces.WorkerConfig) (interfaces.WorkstationExecutionRequest, error) {
	if !shouldPrepareCodexWorktree(request, workerDef) {
		return request, nil
	}

	resolvedWorktreePath := request.Worktree
	if resolvedWorktreePath != "" {
		resolvedWorktreePath = resolveCodexWorktreePath(we.runtimeBaseDir(), resolvedWorktreePath)
	}

	if pathsOverlapForCodexReuse(request.WorkingDirectory, resolvedWorktreePath) {
		request.WorktreeHandling = codexWorktreeHandlingReuseWorkingDirectory
		if request.WorkingDirectory == "" {
			request.WorkingDirectory = resolvedWorktreePath
		}
		request.Worktree = ""
		return request, nil
	}

	worktreeState, err := we.inspectCodexWorktree(ctx, resolvedWorktreePath)
	if err != nil {
		return interfaces.WorkstationExecutionRequest{}, err
	}
	if worktreeState.reusable {
		request.WorktreeHandling = codexWorktreeHandlingReuseExistingWorktree
		request.WorkingDirectory = resolvedWorktreePath
		request.Worktree = ""
		return request, nil
	}
	if worktreeState.exists {
		return request, nil
	}

	return we.codexCreateMissingWorktree(ctx, request, resolvedWorktreePath)
}

func pathsOverlapForCodexReuse(workingDirectory, worktree string) bool {
	if workingDirectory == "" || worktree == "" {
		return false
	}
	workingDirectory = filepath.Clean(filepath.FromSlash(workingDirectory))
	worktree = filepath.Clean(filepath.FromSlash(worktree))
	return sameOrNestedPath(workingDirectory, worktree) || sameOrNestedPath(worktree, workingDirectory)
}

func sameOrNestedPath(root, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (we *WorkstationExecutor) buildWorkstationExecutionRequest(ctx context.Context, dispatch interfaces.WorkDispatch, workerName string, workerDef *interfaces.WorkerConfig, workstationDef *interfaces.FactoryWorkstationConfig, requestContext resolvedWorkstationExecutionContext, start time.Time, logger logging.Logger) (interfaces.WorkstationExecutionRequest, *interfaces.WorkResult) {
	selection := interfaces.ResolveRunnerSelection(workstationDef.Runner, we.DefaultRunnerID, workerDef.ModelProvider)
	var err error
	request := interfaces.WorkstationExecutionRequest{
		Dispatch:              interfaces.CloneWorkDispatch(dispatch),
		WorkerType:            workerName,
		WorkstationType:       dispatch.WorkstationName,
		RunnerID:              selection.RunnerID,
		RunnerSelectionSource: selection.Source,
		ProjectID:             requestContext.ProjectID,
		InputTokens:           InputTokens(requestContext.InputTokens...),
		SystemPrompt:          workerDef.Body,
		OutputSchema:          workstationDef.OutputSchema,
		EnvVars:               cloneEnvVars(requestContext.EnvVars),
		WorktreeHandling:      requestContext.WorktreeHandling,
		Worktree:              requestContext.Worktree,
		WorkingDirectory:      requestContext.WorkingDirectory,
	}
	request, err = we.prepareCodexWorktreeRequest(ctx, request, workerDef)
	if err != nil {
		logger.Error("codex worktree preparation failed",
			WorkLogFields(dispatch.Execution,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"worktree", requestContext.Worktree,
				"error", err)...)
		failed := interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        "codex worktree preparation failed: " + err.Error(),
			Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
		}
		return interfaces.WorkstationExecutionRequest{}, &failed
	}

	rendered, err := we.Renderer.Render(
		workstationDef.PromptTemplate,
		requestContext.InputTokens,
		executionRequestContext(request),
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
	request.UserMessage = rendered
	if request.WorktreeHandling != "" {
		logger.Info("workstation: prepared codex worktree handling",
			WorkLogFields(dispatch.Execution,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"worktree_handling", request.WorktreeHandling,
				"working_directory", request.WorkingDirectory)...)
	}
	return request, nil
}

func shouldPrepareCodexWorktree(request interfaces.WorkstationExecutionRequest, workerDef *interfaces.WorkerConfig) bool {
	if interfaces.NormalizeRunnerID(request.RunnerID) != interfaces.RunnerIDCodex {
		return false
	}
	if workerDef == nil || workerDef.Type != interfaces.WorkerTypeModel {
		return false
	}
	if strings.EqualFold(workerDef.ModelProvider, string(ModelProviderCodex)) {
		return true
	}
	return request.RunnerSelectionSource == interfaces.RunnerSelectionSourceWorkstation ||
		request.RunnerSelectionSource == interfaces.RunnerSelectionSourceFactory
}

type codexWorktreeInspection struct {
	exists   bool
	reusable bool
}

func (we *WorkstationExecutor) inspectCodexWorktree(ctx context.Context, worktreePath string) (codexWorktreeInspection, error) {
	if worktreePath == "" {
		return codexWorktreeInspection{}, nil
	}
	info, err := os.Stat(worktreePath)
	if errors.Is(err, os.ErrNotExist) {
		return codexWorktreeInspection{}, nil
	}
	if err != nil {
		return codexWorktreeInspection{}, fmt.Errorf("stat %q: %w", worktreePath, err)
	}
	if !info.IsDir() {
		return codexWorktreeInspection{exists: true}, nil
	}

	validateCtx, cancel := context.WithTimeout(ctx, codexWorktreeValidationTimeout)
	defer cancel()

	topLevel, err := we.gitRevParse(validateCtx, worktreePath, "--show-toplevel")
	if err != nil {
		return codexWorktreeInspection{exists: true}, nil
	}
	return codexWorktreeInspection{
		exists:   true,
		reusable: pathsEquivalentForCodex(topLevel, worktreePath),
	}, nil
}

func (we *WorkstationExecutor) codexCreateMissingWorktree(ctx context.Context, request interfaces.WorkstationExecutionRequest, worktreePath string) (interfaces.WorkstationExecutionRequest, error) {
	if worktreePath == "" {
		return request, nil
	}
	if request.WorkingDirectory == "" {
		return request, nil
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return interfaces.WorkstationExecutionRequest{}, fmt.Errorf("create worktree parent for %q: %w", worktreePath, err)
	}

	createCtx, cancel := context.WithTimeout(ctx, codexWorktreeCreationTimeout)
	defer cancel()

	repoRoot, err := we.gitRevParse(createCtx, request.WorkingDirectory, "--show-toplevel")
	if err != nil {
		return interfaces.WorkstationExecutionRequest{}, fmt.Errorf("resolve source repository for %q: %w", request.WorkingDirectory, err)
	}
	headCommit, err := we.gitRevParse(createCtx, repoRoot, "HEAD")
	if err != nil {
		return interfaces.WorkstationExecutionRequest{}, fmt.Errorf("resolve source commit for %q: %w", repoRoot, err)
	}
	if err := we.gitWorktreeAddDetached(createCtx, repoRoot, worktreePath, headCommit); err != nil {
		return interfaces.WorkstationExecutionRequest{}, fmt.Errorf("create worktree %q from %q: %w", worktreePath, repoRoot, err)
	}

	request.WorktreeHandling = codexWorktreeHandlingCreateMissingWorktree
	request.WorkingDirectory = worktreePath
	request.Worktree = ""
	return request, nil
}

func (we *WorkstationExecutor) gitRevParse(ctx context.Context, workDir string, args ...string) (string, error) {
	result, err := we.runGitCommand(ctx, workDir, append([]string{"rev-parse"}, args...)...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("git rev-parse %s exit code %d: %s", strings.Join(args, " "), result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

func (we *WorkstationExecutor) gitWorktreeAddDetached(ctx context.Context, workDir, worktreePath, headCommit string) error {
	result, err := we.runGitCommand(ctx, workDir, "worktree", "add", "--detach", worktreePath, headCommit)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("git worktree add --detach %s %s exit code %d: %s", worktreePath, headCommit, result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	return nil
}

func (we *WorkstationExecutor) runGitCommand(ctx context.Context, workDir string, args ...string) (CommandResult, error) {
	return we.commandRunner().Run(ctx, CommandRequest{
		Command: "git",
		Args:    args,
		Env:     isolatedGitEnv(os.Environ()),
		WorkDir: workDir,
	})
}

func (we *WorkstationExecutor) commandRunner() CommandRunner {
	if we != nil && we.CommandRunner != nil {
		return we.CommandRunner
	}
	return ExecCommandRunner{}
}

func (we *WorkstationExecutor) runtimeBaseDir() string {
	if we == nil || we.RuntimeConfig == nil {
		return ""
	}
	return we.RuntimeConfig.RuntimeBaseDir()
}

func resolveCodexWorktreePath(baseDir, worktree string) string {
	if worktree == "" {
		return ""
	}
	normalized := filepath.Clean(filepath.FromSlash(worktree))
	if filepath.IsAbs(normalized) {
		return normalized
	}
	if baseDir != "" {
		return filepath.Clean(filepath.Join(baseDir, normalized))
	}
	return resolveRuntimePath("", normalized)
}

func pathsEquivalentForCodex(left, right string) bool {
	leftPath, leftErr := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(left)))
	if leftErr != nil {
		leftPath = filepath.Clean(filepath.FromSlash(left))
	}
	rightPath, rightErr := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(right)))
	if rightErr != nil {
		rightPath = filepath.Clean(filepath.FromSlash(right))
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftPath, rightPath)
	}
	return leftPath == rightPath
}

func isolatedGitEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || inheritedGitRepoEnv[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

var inheritedGitRepoEnv = map[string]bool{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"GIT_COMMON_DIR":                   true,
	"GIT_DIR":                          true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_PREFIX":                       true,
	"GIT_QUARANTINE_PATH":              true,
	"GIT_WORK_TREE":                    true,
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
		if errors.Is(executorCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
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
