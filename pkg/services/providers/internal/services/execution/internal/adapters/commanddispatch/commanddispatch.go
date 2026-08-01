package commanddispatch

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

// Request returns a Providers subprocess request with attempt correlation
// preserved for composition-owned effect adapters.
func Request(
	request providers.ExecuteRequest,
	command effects.CommandRequest,
) effects.CommandRequest {
	command.AttemptID = request.AttemptID
	command.WorkerType = request.WorkerType
	command.WorkstationName = request.WorkstationName
	return command
}
