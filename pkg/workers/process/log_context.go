package process

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	workLogEventCommandRunnerRequested      = "command_runner.requested"
	workLogEventCommandRunnerCompleted      = "command_runner.completed"
	workLogEventCommandRunnerRequestDetails = "command_runner.request_details"
	workLogEventCommandRunnerOutputDetails  = "command_runner.output_details"
)

func workLogFields(metadata interfaces.ExecutionMetadata, keysAndValues ...any) []any {
	fields := []any{
		"request_id", metadata.RequestID,
		"trace_id", metadata.TraceID,
		"work_id", primaryWorkID(metadata.WorkIDs),
		"work_ids", cloneWorkIDs(metadata.WorkIDs),
	}
	return append(fields, keysAndValues...)
}

func primaryWorkID(workIDs []string) string {
	for _, workID := range workIDs {
		if workID != "" {
			return workID
		}
	}
	return ""
}

func cloneWorkIDs(workIDs []string) []string {
	if workIDs == nil {
		return []string{}
	}
	return append([]string(nil), workIDs...)
}

func commandRequestLogFields(req CommandRequest) []any {
	return workLogFields(req.Execution,
		"event_name", workLogEventCommandRunnerRequested,
		"status", "requested",
		"command", req.Command,
		"args", append([]string(nil), req.Args...),
		"working_dir", req.WorkDir,
		"stdin_bytes", len(req.Stdin))
}

func commandCompletionLogFields(req CommandRequest, result CommandResult, duration time.Duration, status string, err error) []any {
	fields := workLogFields(req.Execution,
		"event_name", workLogEventCommandRunnerCompleted,
		"status", status,
		"command", req.Command,
		"args", append([]string(nil), req.Args...),
		"working_dir", req.WorkDir,
		"exit_code", result.ExitCode,
		"duration_ms", duration.Milliseconds())
	if status != "succeeded" || err != nil {
		fields = append(fields,
			"stdout", string(result.Stdout),
			"stderr", string(result.Stderr))
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}
	return fields
}

func commandRequestDetailsLogFields(req CommandRequest) []any {
	return workLogFields(req.Execution,
		"event_name", workLogEventCommandRunnerRequestDetails,
		"status", "verbose",
		"command", req.Command,
		"args_count", len(req.Args),
		"working_dir", req.WorkDir,
		"stdin_bytes", len(req.Stdin))
}

func commandOutputDetailsLogFields(req CommandRequest, result CommandResult, duration time.Duration) []any {
	return workLogFields(req.Execution,
		"event_name", workLogEventCommandRunnerOutputDetails,
		"status", "verbose",
		"command", req.Command,
		"exit_code", result.ExitCode,
		"duration_ms", duration.Milliseconds(),
		"stdout_bytes", len(result.Stdout),
		"stderr_bytes", len(result.Stderr))
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
