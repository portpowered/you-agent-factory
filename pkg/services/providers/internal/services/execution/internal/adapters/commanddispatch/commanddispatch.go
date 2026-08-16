package commanddispatch

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Request returns a Providers subprocess request with attempt correlation
// preserved for composition-owned effect adapters.
func Request(
	request providers.ExecuteRequest,
	command providerservice.CommandRequest,
) providerservice.CommandRequest {
	command.AttemptID = request.AttemptID
	command.WorkerType = request.WorkerType
	command.WorkstationName = request.WorkstationName
	command.Execution = work.ExecutionMetadata{
		RequestID: request.Correlation.RequestID,
		TraceID:   request.Correlation.TraceID,
		ReplayKey: request.Correlation.ReplayKey,
		WorkIDs:   append([]string(nil), request.Correlation.WorkIDs...),
	}
	command.ExecutionLogger = request.ExecutionLogger
	command.ProcessLifecycleObserver = request.ProcessLifecycleObserver
	return command
}
