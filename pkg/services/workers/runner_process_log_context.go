package workers

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	workLogEventCommandRunnerRequested      = "command_runner.requested"
	workLogEventCommandRunnerCompleted      = "command_runner.completed"
	workLogEventCommandRunnerRequestDetails = "command_runner.request_details"
	workLogEventCommandRunnerOutputDetails  = "command_runner.output_details"
)

func workLogFields(metadata work.ExecutionMetadata, keysAndValues ...any) []any {
	fields := []any{"request_id", metadata.RequestID, "trace_id", metadata.TraceID,
		"work_id", primaryWorkID(metadata.WorkIDs), "work_ids", cloneWorkIDs(metadata.WorkIDs)}
	return append(fields, keysAndValues...)
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

func commandRequestLogFields(req CommandRequest) []any {
	return workLogFields(req.Execution, "event_name", workLogEventCommandRunnerRequested, "status", "requested",
		"command", req.Command, "args_count", len(req.Args), "working_dir", req.WorkDir, "stdin_bytes", len(req.Stdin))
}
func commandCompletionLogFields(req CommandRequest, result CommandResult, duration time.Duration, status string, err error) []any {
	fields := workLogFields(req.Execution, "event_name", workLogEventCommandRunnerCompleted, "status", status,
		"command", req.Command, "args_count", len(req.Args), "working_dir", req.WorkDir,
		"exit_code", result.ExitCode, "duration_ms", duration.Milliseconds())
	if err != nil {
		fields = append(fields, "has_error", true)
	}
	return fields
}

// commandRequestDetailsLogFields records the composed command-line size next to
// the argument count. The count alone cannot distinguish a dispatch that fits
// the host command-line limit from one that will be rejected by the process
// loader, so an operator had no way to see a prompt growing toward that bound.
func commandRequestDetailsLogFields(req CommandRequest) []any {
	return workLogFields(req.Execution, "event_name", workLogEventCommandRunnerRequestDetails, "status", "verbose",
		"command", req.Command, "args_count", len(req.Args), "working_dir", req.WorkDir, "stdin_bytes", len(req.Stdin),
		"command_line_chars", platformprocess.ComposedCommandLineLength(req.Command, req.Args),
		"command_line_limit", platformprocess.HostCommandLineLimit())
}
func commandOutputDetailsLogFields(req CommandRequest, result CommandResult, duration time.Duration) []any {
	return workLogFields(req.Execution, "event_name", workLogEventCommandRunnerOutputDetails, "status", "verbose",
		"command", req.Command, "exit_code", result.ExitCode, "duration_ms", duration.Milliseconds(),
		"stdout_bytes", len(result.Stdout), "stderr_bytes", len(result.Stderr))
}
func commandResultStatus(ctx context.Context, result CommandResult, err error) string {
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return "timed_out"
		}
		return "error"
	}
	if result.ExitCode != 0 {
		return "failed"
	}
	return "succeeded"
}

type contextualLogger struct {
	logger logging.Logger
	fields []any
}

// commandExecutionLogger resolves the sink one command execution must write
// to. The Runtime-selected sink travels with the request, so a process-scoped
// runner constructed with a placeholder logger still reaches the opened
// Runtime's log without consulting the execution context or retaining any
// Factory Session state between requests.
func commandExecutionLogger(req CommandRequest, logger logging.Logger) logging.Logger {
	if req.ExecutionLogger != nil {
		return logging.EnsureLogger(req.ExecutionLogger)
	}
	return logging.EnsureLogger(logger)
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

func commandContextLogger(logger logging.Logger, req CommandRequest) logging.Logger {
	fields := workLogFields(req.Execution, "dispatch_id", req.DispatchID)
	if req.WorkerType != "" {
		fields = append(fields, "worker_type", req.WorkerType)
	}
	if req.WorkstationName != "" {
		fields = append(fields, "workstation_name", req.WorkstationName)
	}
	return contextualLogger{logger: logging.EnsureLogger(logger), fields: fields}
}
func (l contextualLogger) append(values []any) []any {
	return append(append([]any(nil), values...), l.fields...)
}
func (l contextualLogger) Debug(msg string, fields ...any) { l.logger.Debug(msg, l.append(fields)...) }
func (l contextualLogger) Info(msg string, fields ...any)  { l.logger.Info(msg, l.append(fields)...) }
func (l contextualLogger) Warn(msg string, fields ...any)  { l.logger.Warn(msg, l.append(fields)...) }
func (l contextualLogger) Error(msg string, fields ...any) { l.logger.Error(msg, l.append(fields)...) }
func (l contextualLogger) Verbose(msg string, fields ...any) {
	l.logger.Verbose(msg, l.append(fields)...)
}
