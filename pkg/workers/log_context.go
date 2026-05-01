package workers

import "github.com/portpowered/infinite-you/pkg/interfaces"

const (
	WorkLogEventWorkerPoolSubmitted         = "worker_pool.submitted"
	WorkLogEventWorkerPoolExecutorEntered   = "worker_pool.executor_entered"
	WorkLogEventWorkerPoolResponseSubmitted = "worker_pool.response_submitted"
	WorkLogEventCommandRunnerRequested      = "command_runner.requested"
	WorkLogEventCommandRunnerCompleted      = "command_runner.completed"
	WorkLogEventCommandRunnerRequestDetails = "command_runner.request_details"
	WorkLogEventCommandRunnerOutputDetails  = "command_runner.output_details"
)

// WorkLogFields returns stable structured log fields for work-scoped runtime
// records. Empty strings are intentional so unavailable IDs remain explicit.
func WorkLogFields(metadata interfaces.ExecutionMetadata, keysAndValues ...any) []any {
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
