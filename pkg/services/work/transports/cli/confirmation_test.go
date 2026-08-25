package cli

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNormalizeWorkConfirmationStateDefaultsUnknownValues(t *testing.T) {
	unknown := factoryapi.ConfirmationState("LEGACY")
	work := &factoryapi.Work{
		ConfirmationState: &unknown,
		StopSummary: &factoryapi.FactoryStopSummary{
			LatestDispatch: &factoryapi.FactoryStopDispatchSummary{ConfirmationState: unknown},
		},
	}

	normalizeWorkConfirmationState(work)

	if work.ConfirmationState == nil || *work.ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("work confirmationState = %v, want UNCONFIRMED", work.ConfirmationState)
	}
	if work.StopSummary.LatestDispatch.ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("dispatch confirmationState = %q, want UNCONFIRMED", work.StopSummary.LatestDispatch.ConfirmationState)
	}
}
