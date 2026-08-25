package cli

import factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

func workerSessionConfirmationState(session factoryapi.WorkerSessionObservation) factoryapi.ConfirmationState {
	state := session.ConfirmationState
	if state == factoryapi.CONFIRMED {
		return state
	}
	return factoryapi.UNCONFIRMED
}
