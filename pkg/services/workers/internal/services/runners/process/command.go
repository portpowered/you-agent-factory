// Package process contains the Workers-private command boundary.
//
// The platform process port carries only host-process facts. This package adds
// the request-scoped Worker correlation needed by Workers' private runners
// without publishing that richer shape from the Workers service root.
package process

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type CommandRunner interface {
	Run(context.Context, CommandRequest) (CommandResult, error)
}

type StreamingCommandRunner interface {
	CommandRunner
	RunStreaming(context.Context, CommandRequest, platformprocess.OutputChunkObserver) (CommandResult, error)
}

type CommandRequest struct {
	Command                  string
	Args                     []string
	Stdin                    []byte
	Env                      []string
	WorkDir                  string
	FactorySessionID         string
	DispatchID               string
	TransitionID             string
	WorkerType               string
	WorkstationName          string
	ProjectID                string
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	Execution                work.ExecutionMetadata
	Inputs                   []workers.WorkInput
	ExecutionLogger          logging.Logger
	ProcessLifecycleObserver platformprocess.ProcessLifecycleObserver
}

type CommandResult = platformprocess.CommandResult
type OutputChunkObserver = platformprocess.OutputChunkObserver
type CommandEnvEntry = platformprocess.CommandEnvEntry

func CloneCommandRequest(request CommandRequest) CommandRequest {
	clone := request
	clone.Args = append([]string(nil), request.Args...)
	clone.Stdin = append([]byte(nil), request.Stdin...)
	clone.Env = append([]string(nil), request.Env...)
	clone.PreviousChainingTraceIDs = append([]string(nil), request.PreviousChainingTraceIDs...)
	clone.Execution = work.CloneExecutionMetadata(request.Execution)
	if len(request.Inputs) > 0 {
		clone.Inputs = make([]workers.WorkInput, len(request.Inputs))
		for index, input := range request.Inputs {
			clone.Inputs[index] = input.Clone()
		}
	} else {
		clone.Inputs = nil
	}
	return clone
}

type Clock interface {
	Now() time.Time
}

type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

const (
	OutputStreamStdout = platformprocess.OutputStreamStdout
	OutputStreamStderr = platformprocess.OutputStreamStderr
)

var (
	CommandEnvEntriesFromMap = platformprocess.CommandEnvEntriesFromMap
	MergeCommandEnv          = platformprocess.MergeCommandEnv
)

// AdaptPlatformCommandRunner is the only adapter from the shared platform
// process effect into the richer Workers-private request. The platform edge
// remains the replacement point for Factory Runtime, Sessions, and tests.
func AdaptPlatformCommandRunner(runner platformprocess.CommandRunner) CommandRunner {
	if runner == nil {
		return nil
	}
	if private, ok := runner.(interface {
		privateCommandRunner() CommandRunner
	}); ok {
		return private.privateCommandRunner()
	}
	adapted := ExecCommandRunner{Runner: runner}
	if _, ok := runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	}); ok {
		return StreamingAdaptedCommandRunner{ExecCommandRunner: adapted}
	}
	return adapted
}

// AdaptCommandRunner is kept private to the Workers runner package while the
// command boundary is being assembled from platform effects.
func AdaptCommandRunner(runner platformprocess.CommandRunner) CommandRunner {
	return AdaptPlatformCommandRunner(runner)
}

type ExecCommandRunner struct {
	Runner platformprocess.CommandRunner
	Logger logging.Logger
}

func (runner ExecCommandRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	platformRunner := runner.Runner
	if platformRunner == nil {
		return CommandResult{}, errors.New("workers process command runner is required")
	}
	platformRunner = platformCommandRunnerWithLogger(
		platformRunner,
		commandContextLogger(runner.Logger, request),
	)
	result, err := platformRunner.Run(ctx, platformRequest(request))
	return result, err
}

func platformCommandRunnerWithLogger(
	runner platformprocess.CommandRunner,
	logger logging.Logger,
) platformprocess.CommandRunner {
	switch typed := runner.(type) {
	case platformprocess.ExecCommandRunner:
		typed.Logger = logger
		return typed
	case *platformprocess.ExecCommandRunner:
		if typed == nil {
			return nil
		}
		clone := *typed
		clone.Logger = logger
		return clone
	default:
		return runner
	}
}

type StreamingAdaptedCommandRunner struct {
	ExecCommandRunner
}

func (runner StreamingAdaptedCommandRunner) RunStreaming(
	ctx context.Context,
	request CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (CommandResult, error) {
	platformRunner := runner.Runner
	if platformRunner == nil {
		return CommandResult{}, errors.New("workers process command runner is required")
	}
	platformRunner = platformCommandRunnerWithLogger(
		platformRunner,
		commandContextLogger(runner.Logger, request),
	)
	streaming, ok := platformRunner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if !ok {
		result, err := runner.Run(ctx, request)
		publishCompleteOutput(observer, result)
		return result, err
	}
	return streaming.RunStreaming(ctx, platformRequest(request), observer)
}

type projectedPlatformCommandRunner struct{ runner CommandRunner }

// privateCommandRunner lets Workers recover its richer request boundary when
// a private runner crossed the platform-typed composition seam. External
// platform effects cannot implement this unexported method, so they continue
// through the normal request projection below.
func (runner projectedPlatformCommandRunner) privateCommandRunner() CommandRunner {
	return runner.runner
}

func ProjectPlatformCommandRunner(runner CommandRunner) platformprocess.CommandRunner {
	if runner == nil {
		return nil
	}
	if adapted, ok := runner.(ExecCommandRunner); ok {
		return adapted.Runner
	}
	if adapted, ok := runner.(*ExecCommandRunner); ok && adapted != nil {
		return adapted.Runner
	}
	if adapted, ok := runner.(StreamingAdaptedCommandRunner); ok {
		return adapted.Runner
	}
	if adapted, ok := runner.(*StreamingAdaptedCommandRunner); ok && adapted != nil {
		return adapted.Runner
	}
	return projectedPlatformCommandRunner{runner: runner}
}

func (runner projectedPlatformCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if runner.runner == nil {
		return platformprocess.CommandResult{}, errors.New("workers projected platform command runner is required")
	}
	result, err := runner.runner.Run(ctx, workerRequest(request))
	return result, err
}

func (runner projectedPlatformCommandRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if streaming, ok := runner.runner.(StreamingCommandRunner); ok {
		return streaming.RunStreaming(ctx, workerRequest(request), observer)
	}
	result, err := runner.Run(ctx, request)
	publishCompleteOutput(observer, result)
	return result, err
}

type LoggingCommandRunner struct {
	Runner CommandRunner
	Logger logging.Logger
	Clock  Clock
}

func CommandRunnerWithLogging(runner CommandRunner, logger logging.Logger, clock Clock) CommandRunner {
	if existing, ok := runner.(*LoggingCommandRunner); ok && existing != nil {
		if existing.Logger == nil {
			existing.Logger = logger
		}
		if existing.Clock == nil {
			existing.Clock = clock
		}
		return existing
	}
	return &LoggingCommandRunner{Runner: runner, Logger: logger, Clock: clock}
}

func (runner LoggingCommandRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	if runner.Runner == nil {
		return CommandResult{}, errors.New("workers logging command runner is required")
	}
	if runner.Clock == nil {
		return CommandResult{}, errors.New("workers logging command clock is required")
	}
	logger := commandExecutionLogger(request, runner.Logger)
	logger.Info("command runner: request received", commandRequestLogFields(request)...)
	logger.Verbose("command runner: verbose request details", commandRequestDetailsLogFields(request)...)
	started := runner.Clock.Now()
	result, err := runner.Runner.Run(ctx, request)
	duration := runner.Clock.Now().Sub(started)
	loggedResult := commandResultForLogging(runner.Runner, ctx, request, result)
	status := commandResultStatus(ctx, loggedResult, err)
	completionFields := commandCompletionLogFields(request, loggedResult, duration, status, err)
	if commandStatusIsFailure(status) {
		logger.Error("command runner: request failed", completionFields...)
	} else {
		logger.Info("command runner: request completed", completionFields...)
	}
	logger.Verbose("command runner: verbose output details", commandOutputDetailsLogFields(request, loggedResult, duration)...)
	return result, err
}

func (runner LoggingCommandRunner) RunStreaming(
	ctx context.Context,
	request CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (CommandResult, error) {
	if runner.Runner == nil {
		return CommandResult{}, errors.New("workers logging command runner is required")
	}
	if runner.Clock == nil {
		return CommandResult{}, errors.New("workers logging command clock is required")
	}
	logger := commandExecutionLogger(request, runner.Logger)
	logger.Info("command runner: request received", commandRequestLogFields(request)...)
	logger.Verbose("command runner: verbose request details", commandRequestDetailsLogFields(request)...)
	started := runner.Clock.Now()
	var result CommandResult
	var err error
	if streaming, ok := runner.Runner.(StreamingCommandRunner); ok {
		result, err = streaming.RunStreaming(ctx, request, observer)
	} else {
		result, err = runner.Runner.Run(ctx, request)
		publishCompleteOutput(observer, result)
	}
	duration := runner.Clock.Now().Sub(started)
	loggedResult := commandResultForLogging(runner.Runner, ctx, request, result)
	status := commandResultStatus(ctx, loggedResult, err)
	completionFields := commandCompletionLogFields(request, loggedResult, duration, status, err)
	if commandStatusIsFailure(status) {
		logger.Error("command runner: request failed", completionFields...)
	} else {
		logger.Info("command runner: request completed", completionFields...)
	}
	logger.Verbose("command runner: verbose output details", commandOutputDetailsLogFields(request, loggedResult, duration)...)
	return result, err
}

func SubprocessRequestBase(dispatch work.WorkDispatch) CommandRequest {
	cloned := work.CloneWorkDispatch(dispatch)
	return CommandRequest{
		DispatchID: cloned.DispatchID, TransitionID: cloned.TransitionID,
		WorkerType: cloned.WorkerType, WorkstationName: cloned.WorkstationName,
		ProjectID: cloned.ProjectID, CurrentChainingTraceID: cloned.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloned.PreviousChainingTraceIDs,
		Execution:                cloned.Execution, Inputs: workInputsFromDispatch(cloned),
	}
}

func workInputsFromDispatch(dispatch work.WorkDispatch) []workers.WorkInput {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	if len(tokens) == 0 {
		return nil
	}
	namesByTokenID := make(map[string][]string)
	for name, tokenIDs := range dispatch.InputBindings {
		for _, tokenID := range tokenIDs {
			namesByTokenID[tokenID] = append(namesByTokenID[tokenID], name)
		}
	}
	for tokenID := range namesByTokenID {
		sort.Strings(namesByTokenID[tokenID])
	}
	inputs := make([]workers.WorkInput, 0, len(tokens))
	for _, token := range tokens {
		input := workers.WorkInput{
			Kind: string(token.Color.DataType), State: token.State,
			InputNames: append([]string(nil), namesByTokenID[token.ID]...),
			WorkID:     token.Color.WorkID, Name: token.Color.Name,
			WorkTypeID: token.Color.WorkTypeID, RequestID: token.Color.RequestID,
			Content:   work.CloneWorkContentParts(token.Color.Content),
			Tags:      cloneStringMap(token.Color.Tags),
			Relations: append([]work.Relation(nil), token.Color.Relations...),
			Lineage: workers.WorkLineage{
				ParentWorkID: token.Color.ParentID, TraceID: token.Color.TraceID,
				OriginRef: token.Color.Name,
			},
		}
		if len(input.Content) == 0 && len(token.Color.Payload) > 0 {
			input.Content = []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: string(token.Color.Payload)}}
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func platformRequest(request CommandRequest) platformprocess.CommandRequest {
	return platformprocess.CommandRequest{
		Command: request.Command, Args: request.Args, Stdin: request.Stdin,
		Env: request.Env, WorkDir: request.WorkDir,
		ExecutionScopeID:         request.FactorySessionID,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
}

func workerRequest(request platformprocess.CommandRequest) CommandRequest {
	return CommandRequest{
		Command: request.Command, Args: request.Args, Stdin: request.Stdin,
		Env: request.Env, WorkDir: request.WorkDir,
		FactorySessionID:         request.ExecutionScopeID,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
}

func publishCompleteOutput(observer platformprocess.OutputChunkObserver, result CommandResult) {
	if observer == nil {
		return
	}
	if len(result.Stdout) > 0 {
		observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
	}
	if len(result.Stderr) > 0 {
		observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
	}
}

func commandExecutionLogger(request CommandRequest, fallback logging.Logger) logging.Logger {
	if request.ExecutionLogger != nil {
		return logging.EnsureLogger(request.ExecutionLogger)
	}
	return logging.EnsureLogger(fallback)
}

func workLogFields(metadata work.ExecutionMetadata, values ...any) []any {
	fields := []any{"request_id", metadata.RequestID, "trace_id", metadata.TraceID,
		"work_id", primaryWorkID(metadata.WorkIDs), "work_ids", cloneWorkIDs(metadata.WorkIDs)}
	return append(fields, values...)
}

func commandRequestLogFields(request CommandRequest) []any {
	return commandRequestCorrelationFields(request, "event_name", "command_runner.requested", "status", "requested",
		"command", request.Command, "args_count", len(request.Args), "working_dir", request.WorkDir,
		"stdin_bytes", len(request.Stdin))
}

func commandRequestDetailsLogFields(request CommandRequest) []any {
	return workLogFields(request.Execution, "event_name", "command_runner.request_details", "status", "verbose",
		"command", request.Command, "args_count", len(request.Args), "working_dir", request.WorkDir,
		"stdin_bytes", len(request.Stdin),
		"command_line_chars", platformprocess.ComposedCommandLineLength(request.Command, request.Args),
		"command_line_budget", platformprocess.WindowsCommandLineLimit)
}

func commandOutputDetailsLogFields(
	request CommandRequest,
	result CommandResult,
	duration time.Duration,
) []any {
	return commandRequestCorrelationFields(request, "event_name", "command_runner.output_details", "status", "verbose",
		"command", request.Command, "exit_code", result.ExitCode, "duration_ms", duration.Milliseconds(),
		"stdout_bytes", len(result.Stdout), "stderr_bytes", len(result.Stderr))
}

func commandResultStatus(ctx context.Context, result CommandResult, err error) string {
	reason := result.CancellationReason
	if reason == "" {
		reason = platformprocess.CancellationReasonFromContext(ctx)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return "timed_out"
		}
		if reason == platformprocess.CancellationReasonProcessGone {
			return "error"
		}
		if reason != "" {
			return "canceled"
		}
		if errors.Is(err, context.Canceled) {
			return "canceled"
		}
		return "error"
	}
	if reason == platformprocess.CancellationReasonProcessGone {
		return "error"
	}
	if reason != "" {
		return "canceled"
	}
	if result.ExitCode != 0 {
		return "failed"
	}
	return "succeeded"
}

func commandStatusIsFailure(status string) bool {
	switch status {
	case "failed", "timed_out", "error":
		return true
	default:
		return false
	}
}

func commandFailureReason(status string) string {
	switch status {
	case "failed":
		return "non_zero_exit"
	case "timed_out":
		return "timeout"
	case "error":
		return "execution_error"
	default:
		return ""
	}
}

func commandCompletionLogFields(
	request CommandRequest,
	result CommandResult,
	duration time.Duration,
	status string,
	err error,
) []any {
	fields := commandRequestCorrelationFields(request, "event_name", "command_runner.completed", "status", status, "outcome", status,
		"command", request.Command, "args_count", len(request.Args), "working_dir", request.WorkDir,
		"exit_code", result.ExitCode, "duration_ms", duration.Milliseconds())
	if reason := commandFailureReason(status); reason != "" {
		fields = append(fields, "failure_reason", reason)
	}
	if result.CancellationReason != "" {
		fields = append(fields, "cancellation_reason", string(result.CancellationReason))
	}
	if err != nil && status != "canceled" {
		fields = append(fields, "has_error", true)
	}
	return fields
}

func primaryWorkID(ids []string) string {
	for _, id := range ids {
		if id != "" {
			return id
		}
	}
	return ""
}

func cloneWorkIDs(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return append([]string(nil), ids...)
}

func commandResultForLogging(
	runner CommandRunner,
	ctx context.Context,
	request CommandRequest,
	result CommandResult,
) CommandResult {
	projector, ok := runner.(interface {
		CommandResultForLogging(context.Context, CommandRequest, CommandResult) CommandResult
	})
	if !ok {
		return result
	}
	return projector.CommandResultForLogging(ctx, request, result)
}

func commandContextLogger(logger logging.Logger, request CommandRequest) logging.Logger {
	fields := workLogFields(request.Execution,
		"dispatch_id", request.DispatchID,
		"transition_id", request.TransitionID,
		"work_name", primaryWorkName(request.Inputs),
	)
	if request.WorkerType != "" {
		fields = append(fields, "worker_type", request.WorkerType)
	}
	if request.WorkstationName != "" {
		fields = append(fields, "workstation_name", request.WorkstationName)
	}
	return contextualLogger{logger: logging.EnsureLogger(logger), fields: fields}
}

func commandRequestCorrelationFields(request CommandRequest, keysAndValues ...any) []any {
	fields := workLogFields(request.Execution,
		"dispatch_id", request.DispatchID,
		"transition_id", request.TransitionID,
		"workstation_name", request.WorkstationName,
		"work_name", primaryWorkName(request.Inputs),
	)
	return append(fields, keysAndValues...)
}

func primaryWorkName(inputs []workers.WorkInput) string {
	for _, input := range inputs {
		if strings.TrimSpace(input.WorkID) == "" {
			continue
		}
		if name := strings.TrimSpace(input.Name); name != "" {
			return name
		}
	}
	return ""
}

type contextualLogger struct {
	logger logging.Logger
	fields []any
}

func (logger contextualLogger) append(values []any) []any {
	return append(append([]any(nil), values...), logger.fields...)
}

func (logger contextualLogger) Debug(message string, fields ...any) {
	logger.logger.Debug(message, logger.append(fields)...)
}

func (logger contextualLogger) Info(message string, fields ...any) {
	logger.logger.Info(message, logger.append(fields)...)
}

func (logger contextualLogger) Warn(message string, fields ...any) {
	logger.logger.Warn(message, logger.append(fields)...)
}

func (logger contextualLogger) Error(message string, fields ...any) {
	logger.logger.Error(message, logger.append(fields)...)
}

func (logger contextualLogger) Verbose(message string, fields ...any) {
	logger.logger.Verbose(message, logger.append(fields)...)
}
