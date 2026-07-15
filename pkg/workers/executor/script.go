package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
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
	now           func() time.Time
}

// ScriptEventRecorder receives generated script-boundary events.
type ScriptEventRecorder func(factoryapi.FactoryEvent)

// ScriptExecutorOption configures a ScriptExecutor.
type ScriptExecutorOption func(*ScriptExecutor)

// WithScriptEventRecorder records script-boundary events on the canonical event
// history owned by the runtime.
func WithScriptEventRecorder(recorder ScriptEventRecorder) ScriptExecutorOption {
	return func(se *ScriptExecutor) {
		if recorder != nil {
			se.recorder = recorder
		}
	}
}

// WithScriptFactoryDir resolves portable factory/scripts/... references against
// the active factory directory before subprocess execution.
func WithScriptFactoryDir(factoryDir string) ScriptExecutorOption {
	return func(se *ScriptExecutor) {
		se.FactoryDir = strings.TrimSpace(factoryDir)
	}
}

// WithScriptClock sets the clock used for script event occurrence times and
// subprocess duration measurement.
func WithScriptClock(now func() time.Time) ScriptExecutorOption {
	return func(se *ScriptExecutor) {
		if now != nil {
			se.now = now
		}
	}
}

// commandRunner returns the configured CommandRunner, falling back to
// ExecCommandRunner when none was provided.
func (se *ScriptExecutor) commandRunner() CommandRunner {
	if se.CommandRunner != nil {
		return workerprocess.CommandRunnerWithLogging(se.CommandRunner, se.Logger)
	}
	return workerprocess.CommandRunnerWithLogging(workerprocess.ExecCommandRunner{}, se.Logger)
}

// NewScriptExecutor creates a ScriptExecutor from a WorkerConfig.
func NewScriptExecutor(def *workerconfig.Config, logger logging.Logger, opts ...ScriptExecutorOption) *ScriptExecutor {
	args := make([]string, len(def.Args))
	copy(args, def.Args)
	executor := &ScriptExecutor{
		Command: def.Command,
		Args:    args,
		Logger:  logger,
	}
	for _, opt := range opts {
		opt(executor)
	}
	return executor
}

// NewScriptExecutorWithRunner creates a ScriptExecutor with a custom CommandRunner.
func NewScriptExecutorWithRunner(def *workerconfig.Config, runner CommandRunner, logger logging.Logger, opts ...ScriptExecutorOption) *ScriptExecutor {
	se := NewScriptExecutor(def, logger, opts...)
	se.CommandRunner = runner
	return se
}

// Execute runs the configured command with template-substituted args.
// Exit code 0 produces ACCEPTED with stdout in Output.
// Non-zero exit code produces FAILED with stderr as Error.
func (se *ScriptExecutor) Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
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
	if se != nil && se.now != nil {
		return se.now()
	}
	return time.Now()
}

func (se *ScriptExecutor) commandRequest(request workerexecution.WorkstationExecutionRequest) (CommandRequest, error) {
	if err := unsupportedImageContentError(request.InputTokens, "script executor"); err != nil {
		return CommandRequest{}, err
	}
	data := workerprompting.BuildPromptData(executionRequestInputTokens(request), executionRequestContext(request))
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

func (se *ScriptExecutor) record(event factoryapi.FactoryEvent) {
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

func scriptRequestEvent(req CommandRequest, attempt int, requestID string, eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.ScriptRequestEventPayload{
		ScriptRequestId: requestID,
		DispatchId:      req.DispatchID,
		TransitionId:    req.TransitionID,
		Attempt:         attempt,
		Command:         req.Command,
		Args:            append([]string(nil), req.Args...),
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeScriptRequest,
		Id:            fmt.Sprintf("%s/%s", scriptRequestEventIDPrefix, requestID),
		Context:       scriptEventContext(req, eventTime),
		Payload:       scriptRequestFactoryEventPayload(payload),
	}
}

func scriptResponseEvent(req CommandRequest, result workerexecution.WorkResult, attempt int, requestID string, eventTime time.Time) factoryapi.FactoryEvent {
	outcome, failureType := scriptResponseOutcome(result)
	payload := factoryapi.ScriptResponseEventPayload{
		ScriptRequestId: requestID,
		DispatchId:      req.DispatchID,
		TransitionId:    req.TransitionID,
		Attempt:         attempt,
		Outcome:         outcome,
		Stdout:          scriptResponseStdout(result),
		Stderr:          scriptResponseStderr(result),
		DurationMillis:  result.Metrics.Duration.Milliseconds(),
	}
	if failureType != nil {
		payload.FailureType = failureType
	}
	payload.ExitCode = scriptResponseExitCode(result, outcome)
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeScriptResponse,
		Id:            scriptResponseEventID(req.DispatchID, attempt),
		Context:       scriptEventContext(req, eventTime),
		Payload:       scriptResponseFactoryEventPayload(payload),
	}
}

func scriptEventContext(req CommandRequest, eventTime time.Time) factoryapi.FactoryEventContext {
	return factoryapi.FactoryEventContext{
		Tick:       scriptEventTick(req.Execution),
		EventTime:  interfaces.CanonicalEventTime(eventTime),
		DispatchId: stringPtrIfNotEmpty(req.DispatchID),
		RequestId:  stringPtrIfNotEmpty(req.Execution.RequestID),
		TraceIds:   stringSlicePtr(req.Execution.TraceID),
		WorkIds:    stringSlicePtr(req.Execution.WorkIDs...),
	}
}

func scriptEventTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func scriptRequestFactoryEventPayload(payload factoryapi.ScriptRequestEventPayload) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	if err := out.FromScriptRequestEventPayload(payload); err != nil {
		panic(fmt.Sprintf("script request event payload: %v", err))
	}
	return out
}

func scriptResponseFactoryEventPayload(payload factoryapi.ScriptResponseEventPayload) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	if err := out.FromScriptResponseEventPayload(payload); err != nil {
		panic(fmt.Sprintf("script response event payload: %v", err))
	}
	return out
}

func scriptResponseEventID(dispatchID string, attempt int) string {
	if dispatchID == "" {
		return fmt.Sprintf("%s/%d", scriptResponseEventIDPrefix, attempt)
	}
	return fmt.Sprintf("%s/%s/%d", scriptResponseEventIDPrefix, dispatchID, attempt)
}

func scriptResponseOutcome(result workerexecution.WorkResult) (factoryapi.ScriptExecutionOutcome, *factoryapi.ScriptFailureType) {
	if scriptCommandTimedOut(result) {
		failureType := factoryapi.ScriptFailureTypeTimeout
		return factoryapi.ScriptExecutionOutcomeTimedOut, &failureType
	}
	if result.Outcome == workerexecution.OutcomeFailed {
		if command, ok := scriptCommandDiagnostic(result); ok && command.ExitCode != 0 {
			return factoryapi.ScriptExecutionOutcomeFailedExitCode, nil
		}
		failureType := factoryapi.ScriptFailureTypeProcessError
		return factoryapi.ScriptExecutionOutcomeProcessError, &failureType
	}
	return factoryapi.ScriptExecutionOutcomeSucceeded, nil
}

func scriptResponseExitCode(result workerexecution.WorkResult, outcome factoryapi.ScriptExecutionOutcome) *int {
	command, ok := scriptCommandDiagnostic(result)
	if !ok {
		return nil
	}
	return workerEventExitCode(
		command.ExitCode,
		outcome == factoryapi.ScriptExecutionOutcomeSucceeded || outcome == factoryapi.ScriptExecutionOutcomeFailedExitCode,
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

// buildEnv merges dispatch env vars into the current process environment.
func buildEnv(request workerexecution.WorkstationExecutionRequest) []string {
	return workerprocess.MergeCommandEnv(os.Environ(), workerprocess.CommandEnvEntriesFromMap(request.EnvVars))
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
	if !strings.HasPrefix(normalized, "factory/scripts/") {
		return raw
	}

	relativePath := strings.TrimPrefix(normalized, "factory/scripts/")
	if relativePath == "" {
		return raw
	}
	return filepath.Join(factoryDir, "scripts", filepath.FromSlash(relativePath))
}

// Compile-time check.
var _ WorkstationRequestExecutor = (*ScriptExecutor)(nil)
