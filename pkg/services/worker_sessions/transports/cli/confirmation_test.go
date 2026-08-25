package cli

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWorkerSessionConfirmationStateDefaultsUnknownValues(t *testing.T) {
	session := factoryapi.WorkerSessionObservation{ConfirmationState: factoryapi.ConfirmationState("LEGACY")}
	if got := workerSessionConfirmationState(session); got != factoryapi.UNCONFIRMED {
		t.Fatalf("confirmationState = %q, want UNCONFIRMED", got)
	}
}

func TestWorkerSessionConfirmationStatePreservesConfirmed(t *testing.T) {
	session := factoryapi.WorkerSessionObservation{ConfirmationState: factoryapi.CONFIRMED}
	if got := workerSessionConfirmationState(session); got != factoryapi.CONFIRMED {
		t.Fatalf("confirmationState = %q, want CONFIRMED", got)
	}
}
