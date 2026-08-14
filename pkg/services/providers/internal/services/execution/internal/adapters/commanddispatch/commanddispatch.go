package commanddispatch

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkersCommand returns a Workers subprocess request with dispatch
// correlation preserved for mock-worker interception and provider logging.
// The attempt's command log sink travels with the request so a process-scoped
// command runner logs into the caller's log for this attempt only.
func WorkersCommand(
	request providers.ExecuteRequest,
	command workers.CommandRequest,
) workers.CommandRequest {
	command.DispatchID = request.AttemptID
	command.WorkerType = request.WorkerType
	command.WorkstationName = request.WorkstationName
	command.ExecutionLogger = request.ExecutionLogger
	return command
}
