package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/prompting"
)

const (
	scriptRequestEventIDPrefix  = "factory-event/script-request"
	scriptResponseEventIDPrefix = "factory-event/script-response"
)

// ScriptExecutor implements WorkstationRequestExecutor by running shell commands via os/exec.
// It supports template substitution in args using the PromptData model
// (e.g., {{ (index .Inputs 0).Name }}, {{ (index .Inputs 0).WorkID }},
// {{ index (index .Inputs 0).Tags "key" }}, {{ .Context.WorkDir }})
// and merges dispatch env vars into the process environment.
// TODO: consider names for various things.
type ScriptExecutor struct {
	Command       string
	Args          []string
	FactoryDir    string
	CommandRunner CommandRunner
	Logger        logging.Logger
	recorder      ScriptEventRecorder
	Now           func() time.Time
	FactoryDocs   workerexecution.FactoryDocsLoader
}

// ScriptEventRecorder receives worker-owned script-boundary facts.
type ScriptEventRecorder = workerexecution.ScriptEventRecorder

// ScriptFactory is a validated, reusable script-worker construction component.
type ScriptFactory struct {
	commandRunner CommandRunner
	commandClock  workerprocess.Clock
	factoryDocs   workerexecution.FactoryDocsLoader
}

// NewScriptFactory validates the command edge selected by process composition.
func NewScriptFactory(runner CommandRunner, commandClock workerprocess.Clock, factoryDocs workerexecution.FactoryDocsLoader) (*ScriptFactory, error) {
	if runner == nil {
		return nil, errors.New("construct script worker factory: command runner is required")
	}
	if commandClock == nil {
		return nil, errors.New("construct script worker factory: command clock is required")
	}
	if factoryDocs == nil {
		return nil, errors.New("construct script worker factory: Factory docs loader is required")
	}
	return &ScriptFactory{commandRunner: runner, commandClock: commandClock, factoryDocs: factoryDocs}, nil
}

// New constructs one script worker from the factory's validated command edge.
func (f *ScriptFactory) New(
	def *workerconfig.FactoryWorkerConfig,
	logger logging.Logger,
	factoryDir string,
	recorder ScriptEventRecorder,
	now func() time.Time,
) (*ScriptExecutor, error) {
	if f == nil {
		return nil, errors.New("construct script worker: factory is required")
	}
	runner := workerprocess.CommandRunnerWithLogging(f.commandRunner, logger, f.commandClock)
	executor, err := NewScriptExecutorWithDependencies(def, runner, logger, factoryDir, recorder, now, f.factoryDocs)
	if err != nil {
		return nil, err
	}
	return executor, nil
}

// WithCommandRunner returns a validated copy with a per-runtime wrapper edge.
func (f *ScriptFactory) WithCommandRunner(runner CommandRunner) (*ScriptFactory, error) {
	if f == nil {
		return nil, errors.New("construct script worker factory: base factory is required")
	}
	if runner == nil {
		return f, nil
	}
	return NewScriptFactory(runner, f.commandClock, f.factoryDocs)
}

// NewScriptExecutorWithDependencies constructs a script executor from flat dependencies and
// rejects incomplete graphs before execution can start.
func NewScriptExecutorWithDependencies(
	definition *workerconfig.FactoryWorkerConfig,
	commandRunner CommandRunner,
	logger logging.Logger,
	factoryDir string,
	recorder ScriptEventRecorder,
	now func() time.Time,
	factoryDocs workerexecution.FactoryDocsLoader,
) (*ScriptExecutor, error) {
	if definition == nil {
		return nil, errors.New("construct script worker: definition is required")
	}
	if commandRunner == nil {
		return nil, errors.New("construct script worker: command runner is required")
	}
	if factoryDocs == nil {
		return nil, errors.New("construct script worker: Factory docs loader is required")
	}
	executor := NewScriptExecutorWithRunner(
		definition, commandRunner, logger, factoryDir, recorder, now,
	)
	executor.FactoryDocs = factoryDocs
	return executor, nil
}

// commandRunner returns the configured injected CommandRunner.
func (se *ScriptExecutor) commandRunner() CommandRunner {
	if se.CommandRunner != nil {
		return se.CommandRunner
	}
	return workerprocess.CommandRunnerWithLogging(nil, se.Logger, nil)
}

// NewScriptExecutor creates a ScriptExecutor from a WorkerConfig.
func NewScriptExecutor(
	def *workerconfig.FactoryWorkerConfig,
	logger logging.Logger,
	factoryDir string,
	recorder ScriptEventRecorder,
	now func() time.Time,
) *ScriptExecutor {
	args := make([]string, len(def.Args))
	copy(args, def.Args)
	executor := &ScriptExecutor{
		Command:    def.Command,
		Args:       args,
		Logger:     logger,
		FactoryDir: strings.TrimSpace(factoryDir),
		recorder:   recorder,
		Now:        now,
	}
	return executor
}

// NewScriptExecutorWithRunner creates a ScriptExecutor with a custom CommandRunner.
func NewScriptExecutorWithRunner(
	def *workerconfig.FactoryWorkerConfig,
	runner CommandRunner,
	logger logging.Logger,
	factoryDir string,
	recorder ScriptEventRecorder,
	now func() time.Time,
) *ScriptExecutor {
	se := NewScriptExecutor(def, logger, factoryDir, recorder, now)
	se.CommandRunner = runner
	return se
}

// Execute runs the configured command with template-substituted args.
// Exit code 0 produces ACCEPTED with stdout in Output.
// Non-zero exit code produces FAILED with stderr as Error.
func (se *ScriptExecutor) Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	if se == nil || se.Now == nil {
		return workerexecution.WorkResult{}, errors.New("script executor clock is required")
	}
	start := se.clockNow()
	logger := logging.EnsureLogger(se.Logger)

	commandReq, err := se.commandRequest(request)
	if err != nil {
		return argTemplateErrorResult(request.Dispatch, se.clockNow().Sub(start), err), nil
	}
	attempt := 1
	requestID := scriptRequestID(commandReq.DispatchID, attempt)
	se.record(scriptRequestEvent(commandReq, attempt, requestID, start))

	logger.Info("script execution started",
		WorkLogFields(request.Dispatch.Execution,
			"command", se.Command,
			"args", commandReq.Args,
			"transition_id", request.Dispatch.TransitionID,
			"dispatch_id", request.Dispatch.DispatchID)...)

	commandResult, runErr := se.commandRunner().Run(ctx, commandReq)
	finished := se.clockNow()
	duration := finished.Sub(start)
	diagnostics := commandDiagnostics(commandReq, commandResult, duration, false)

	if runErr != nil {
		result := scriptRunErrorResult(ctx, logger, request.Dispatch, commandResult, diagnostics, duration, runErr)
		se.record(scriptResponseEvent(commandReq, result, attempt, requestID, finished))
		return result, nil
	}

	if commandResult.ExitCode != 0 {
		result := scriptExitFailureResult(logger, request.Dispatch, commandResult, diagnostics, duration)
		se.record(scriptResponseEvent(commandReq, result, attempt, requestID, finished))
		return result, nil
	}

	result := scriptAcceptedResult(logger, request.Dispatch, commandResult, diagnostics, duration)
	se.record(scriptResponseEvent(commandReq, result, attempt, requestID, finished))
	return result, nil
}

func (se *ScriptExecutor) clockNow() time.Time {
	return se.Now()
}

func (se *ScriptExecutor) commandRequest(request workerexecution.WorkstationExecutionRequest) (CommandRequest, error) {
	if err := unsupportedImageContentError(request.InputTokens, "script executor"); err != nil {
		return CommandRequest{}, err
	}
	data, err := workerprompting.BuildPromptDataWithFactoryDocs(
		executionRequestInputTokens(request), executionRequestContext(request), se.FactoryDocs,
	)
	if err != nil {
		return CommandRequest{}, err
	}
	resolvedArgs, err := resolveArgs(se.Args, data)
	if err != nil {
		return CommandRequest{}, err
	}
	commandReq := workerprocess.SubprocessRequestBase(request.Dispatch)
	commandReq.Command = resolvePortableFactoryScriptReference(se.FactoryDir, se.Command)
	commandReq.Args = resolvePortableFactoryScriptReferences(se.FactoryDir, resolvedArgs)
	commandReq.Env = buildEnv(request)
	commandReq.WorkDir = executionWorkDir(request)
	commandReq.InputTokens = cloneRawInputTokens(request.InputTokens)
	if request.WorkerType != "" {
		commandReq.WorkerType = request.WorkerType
	}
	if request.WorkstationType != "" {
		commandReq.WorkstationName = request.WorkstationType
	}
	if request.ProjectID != "" {
		commandReq.ProjectID = request.ProjectID
	}
	return commandReq, nil
}

func (se *ScriptExecutor) record(event workerexecution.ScriptEvent) {
	if se.recorder != nil {
		se.recorder(event)
	}
}

func scriptRequestID(dispatchID string, attempt int) string {
	if dispatchID == "" {
		return fmt.Sprintf("script-request/%d", attempt)
	}
	return fmt.Sprintf("%s/script-request/%d", dispatchID, attempt)
}

func scriptRequestEvent(req CommandRequest, attempt int, requestID string, eventTime time.Time) workerexecution.ScriptEvent {
	payload := workerexecution.ScriptRequestEventPayload{
		ScriptRequestID: requestID,
		DispatchID:      req.DispatchID,
		TransitionID:    req.TransitionID,
		Attempt:         attempt,
		Command:         req.Command,
		Args:            append([]string(nil), req.Args...),
	}
	return scriptEvent(req, eventTime, workerexecution.ScriptEventKindRequest, fmt.Sprintf("%s/%s", scriptRequestEventIDPrefix, requestID), &payload, nil)
}

func scriptResponseEvent(req CommandRequest, result workerexecution.WorkResult, attempt int, requestID string, eventTime time.Time) workerexecution.ScriptEvent {
	outcome, failureType := scriptResponseOutcome(result)
	payload := workerexecution.ScriptResponseEventPayload{
		ScriptRequestID: requestID,
		DispatchID:      req.DispatchID,
		TransitionID:    req.TransitionID,
		Attempt:         attempt,
		Outcome:         outcome,
		Stdout:          scriptResponseStdout(result),
		Stderr:          scriptResponseStderr(result),
		DurationMillis:  result.Metrics.Duration.Milliseconds(),
		FailureType:     failureType,
	}
	payload.ExitCode = scriptResponseExitCode(result, outcome)
	return scriptEvent(req, eventTime, workerexecution.ScriptEventKindResponse, scriptResponseEventID(req.DispatchID, attempt), nil, &payload)
}

func scriptEvent(req CommandRequest, eventTime time.Time, kind workerexecution.ScriptEventKind, id string, request *workerexecution.ScriptRequestEventPayload, response *workerexecution.ScriptResponseEventPayload) workerexecution.ScriptEvent {
	return workerexecution.ScriptEvent{
		ID:         id,
		Kind:       kind,
		Tick:       scriptEventTick(req.Execution),
		EventTime:  eventTime.UTC(),
		DispatchID: req.DispatchID,
		RequestID:  req.Execution.RequestID,
		TraceIDs:   scriptEventStrings(req.Execution.TraceID),
		WorkIDs:    scriptEventStrings(req.Execution.WorkIDs...),
		Request:    request,
		Response:   response,
	}
}

func scriptEventStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func scriptEventTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func scriptResponseEventID(dispatchID string, attempt int) string {
	if dispatchID == "" {
		return fmt.Sprintf("%s/%d", scriptResponseEventIDPrefix, attempt)
	}
	return fmt.Sprintf("%s/%s/%d", scriptResponseEventIDPrefix, dispatchID, attempt)
}

func scriptResponseOutcome(result workerexecution.WorkResult) (workerexecution.ScriptExecutionOutcome, *workerexecution.ScriptFailureType) {
	if scriptCommandTimedOut(result) {
		failureType := workerexecution.ScriptFailureTypeTimeout
		return workerexecution.ScriptExecutionOutcomeTimedOut, &failureType
	}
	if result.Outcome == workerexecution.OutcomeFailed {
		if command, ok := scriptCommandDiagnostic(result); ok && command.ExitCode != 0 {
			return workerexecution.ScriptExecutionOutcomeFailedExitCode, nil
		}
		failureType := workerexecution.ScriptFailureTypeProcessError
		return workerexecution.ScriptExecutionOutcomeProcessError, &failureType
	}
	return workerexecution.ScriptExecutionOutcomeSucceeded, nil
}

func scriptResponseExitCode(result workerexecution.WorkResult, outcome workerexecution.ScriptExecutionOutcome) *int {
	command, ok := scriptCommandDiagnostic(result)
	if !ok {
		return nil
	}
	return workerEventExitCode(
		command.ExitCode,
		outcome == workerexecution.ScriptExecutionOutcomeSucceeded || outcome == workerexecution.ScriptExecutionOutcomeFailedExitCode,
		includeZeroWorkerEventExitCode,
	)
}

func scriptResponseStdout(result workerexecution.WorkResult) string {
	command, ok := scriptCommandDiagnostic(result)
	if !ok {
		return ""
	}
	return command.Stdout
}

func scriptResponseStderr(result workerexecution.WorkResult) string {
	command, ok := scriptCommandDiagnostic(result)
	if !ok {
		return ""
	}
	return command.Stderr
}

func scriptCommandTimedOut(result workerexecution.WorkResult) bool {
	failureMetadata := result.FailureMetadata
	if failureMetadata != nil && failureMetadata.Type == workerexecution.WorkFailureTypeTimeout {
		return true
	}
	command, ok := scriptCommandDiagnostic(result)
	return ok && command.TimedOut
}

func scriptCommandDiagnostic(result workerexecution.WorkResult) (*workerexecution.CommandDiagnostic, bool) {
	if result.Diagnostics == nil || result.Diagnostics.Command == nil {
		return nil, false
	}
	return result.Diagnostics.Command, true
}

func argTemplateErrorResult(dispatch work.WorkDispatch, duration time.Duration, err error) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "arg template error: " + err.Error(),
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func scriptRunErrorResult(
	ctx context.Context,
	logger logging.Logger,
	dispatch work.WorkDispatch,
	commandResult CommandResult,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
	runErr error,
) workerexecution.WorkResult {
	if errors.Is(runErr, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
		logger.Warn("script: execution timed out",
			WorkLogFields(dispatch.Execution,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"outcome", string(workerexecution.OutcomeFailed),
				"duration_ms", duration.Milliseconds())...)
		result := timeoutWorkResult(dispatch, duration)
		if diagnostics.Command != nil {
			diagnostics.Command.TimedOut = true
		}
		result.Diagnostics = diagnostics
		return result
	}
	logger.Warn("script: execution failed",
		WorkLogFields(dispatch.Execution,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"outcome", string(workerexecution.OutcomeFailed),
			"stderr_preview", truncate(string(commandResult.Stderr), 200),
			"duration_ms", duration.Milliseconds())...)
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "execution cancelled: " + runErr.Error(),
		Diagnostics:  diagnostics,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func scriptExitFailureResult(
	logger logging.Logger,
	dispatch work.WorkDispatch,
	commandResult CommandResult,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) workerexecution.WorkResult {
	logger.Warn("script: execution failed",
		WorkLogFields(dispatch.Execution,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"outcome", string(workerexecution.OutcomeFailed),
			"stderr_preview", truncate(strings.TrimSpace(string(commandResult.Stderr)), 200),
			"duration_ms", duration.Milliseconds())...)
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        strings.TrimSpace(string(commandResult.Stderr)),
		Diagnostics:  diagnostics,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func scriptAcceptedResult(
	logger logging.Logger,
	dispatch work.WorkDispatch,
	commandResult CommandResult,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) workerexecution.WorkResult {
	output := strings.TrimSpace(string(commandResult.Stdout))
	logger.Info("script execution completed",
		WorkLogFields(dispatch.Execution,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"outcome", string(workerexecution.OutcomeAccepted),
			"output_length", len(output),
			"duration_ms", duration.Milliseconds())...)

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       output,
		Diagnostics:  diagnostics,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

// truncate returns the first n characters of s, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// resolveArgs applies Go text/template substitution to each arg string.
func resolveArgs(args []string, data any) ([]string, error) {
	resolved := make([]string, len(args))
	for i, arg := range args {
		// Only parse as template if it contains {{ — fast path for plain args.
		if !strings.Contains(arg, "{{") {
			resolved[i] = arg
			continue
		}

		tmpl, err := template.New("arg").Option("missingkey=zero").Parse(arg)
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, err
		}
		resolved[i] = buf.String()
	}
	return resolved, nil
}

// buildEnv merges dispatch env vars into the injected process environment.
func buildEnv(request workerexecution.WorkstationExecutionRequest) []string {
	return workerprocess.MergeCommandEnv(request.ProcessEnvironment, workerprocess.CommandEnvEntriesFromMap(request.EnvVars))
}

func executionWorkDir(request workerexecution.WorkstationExecutionRequest) string {
	if request.WorkingDirectory != "" {
		return request.WorkingDirectory
	}
	if request.Worktree != "" {
		return request.Worktree
	}
	return ""
}

func resolvePortableFactoryScriptReferences(factoryDir string, args []string) []string {
	if len(args) == 0 {
		return nil
	}

	resolved := make([]string, len(args))
	for i, arg := range args {
		resolved[i] = resolvePortableFactoryScriptReference(factoryDir, arg)
	}
	return resolved
}

func resolvePortableFactoryScriptReference(factoryDir, raw string) string {
	if strings.TrimSpace(factoryDir) == "" {
		return raw
	}

	trimmed := strings.TrimSpace(raw)
	normalized := filepath.ToSlash(trimmed)
	relativePath, ok := strings.CutPrefix(normalized, "scripts/")
	if !ok {
		relativePath, ok = strings.CutPrefix(normalized, "factory/scripts/")
	}
	if !ok || relativePath == "" {
		return raw
	}
	return filepath.Join(factoryDir, "scripts", filepath.FromSlash(relativePath))
}

// Compile-time check.
var _ WorkstationRequestExecutor = (*ScriptExecutor)(nil)
