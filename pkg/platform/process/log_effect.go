package process

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

const (
	workLogEventCommandRunnerRequested      = "command_runner.requested"
	workLogEventCommandRunnerCompleted      = "command_runner.completed"
	workLogEventCommandRunnerRequestDetails = "command_runner.request_details"
	workLogEventCommandRunnerOutputDetails  = "command_runner.output_details"
)

// LoggingCommandRunner emits policy-free subprocess effect diagnostics.
type LoggingCommandRunner struct {
	Runner CommandRunner
	Logger logging.Logger
	Clock  Clock
}

func (r LoggingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	logger := logging.EnsureLogger(r.Logger)
	logger.Info("command runner: request received", commandRequestLogFields(req)...)
	logger.Verbose("command runner: verbose request details", commandRequestDetailsLogFields(req)...)
	started := r.Clock.Now()
	result, err := r.Runner.Run(ctx, req)
	duration := r.Clock.Now().Sub(started)
	status := commandResultStatus(ctx, result, err)
	completionFields := commandCompletionLogFields(req, result, duration, status, err)
	if commandStatusIsFailure(status) {
		logger.Error("command runner: request failed", completionFields...)
	} else {
		logger.Info("command runner: request completed", completionFields...)
	}
	logger.Verbose("command runner: verbose output details", commandOutputDetailsLogFields(req, result, duration)...)
	return result, err
}

func CommandRunnerWithLogging(runner CommandRunner, logger logging.Logger, clock Clock) CommandRunner {
	return &LoggingCommandRunner{Runner: execCommandRunnerWithLogger(runner, logger), Logger: logger, Clock: clock}
}

func execCommandRunnerWithLogger(runner CommandRunner, logger logging.Logger) CommandRunner {
	switch typed := runner.(type) {
	case ExecCommandRunner:
		if typed.Logger == nil {
			typed.Logger = logger
		}
		return typed
	case *ExecCommandRunner:
		if typed != nil && typed.Logger == nil {
			typed.Logger = logger
		}
		return typed
	default:
		return runner
	}
}

func commandRequestLogFields(req CommandRequest) []any {
	return []any{"event_name", workLogEventCommandRunnerRequested, "status", "requested", "command", req.Command, "args_count", len(req.Args), "working_dir", req.WorkDir, "stdin_bytes", len(req.Stdin)}
}
func commandRequestDetailsLogFields(req CommandRequest) []any {
	return []any{"event_name", workLogEventCommandRunnerRequestDetails, "status", "verbose", "command", req.Command, "args_count", len(req.Args), "working_dir", req.WorkDir, "stdin_bytes", len(req.Stdin)}
}
func commandCompletionLogFields(req CommandRequest, result CommandResult, duration time.Duration, status string, err error) []any {
	fields := []any{"event_name", workLogEventCommandRunnerCompleted, "status", status, "outcome", status, "command", req.Command, "args_count", len(req.Args), "working_dir", req.WorkDir, "exit_code", result.ExitCode, "duration_ms", duration.Milliseconds()}
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
func commandOutputDetailsLogFields(req CommandRequest, result CommandResult, duration time.Duration) []any {
	return []any{"event_name", workLogEventCommandRunnerOutputDetails, "status", "verbose", "command", req.Command, "exit_code", result.ExitCode, "duration_ms", duration.Milliseconds(), "stdout_bytes", len(result.Stdout), "stderr_bytes", len(result.Stderr)}
}
func commandResultStatus(ctx context.Context, result CommandResult, err error) string {
	reason := firstCancellationReason(result.CancellationReason, CancellationReasonFromContext(ctx))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return "timed_out"
		}
		if reason == CancellationReasonProcessGone {
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
	if reason == CancellationReasonProcessGone {
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

var _ CommandRunner = (*LoggingCommandRunner)(nil)
