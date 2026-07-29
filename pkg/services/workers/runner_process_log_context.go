package workers

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
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
func commandRequestDetailsLogFields(req CommandRequest) []any {
	return workLogFields(req.Execution, "event_name", workLogEventCommandRunnerRequestDetails, "status", "verbose",
		"command", req.Command, "args_count", len(req.Args), "working_dir", req.WorkDir, "stdin_bytes", len(req.Stdin))
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
