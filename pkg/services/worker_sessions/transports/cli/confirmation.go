package cli

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func workerSessionConfirmationState(session factoryapi.WorkerSessionObservation) factoryapi.ConfirmationState {
	state := session.ConfirmationState
	if strings.TrimSpace(string(state)) == "" {
		return factoryapi.UNCONFIRMED
	}
	return state
}
