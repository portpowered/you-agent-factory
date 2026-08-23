package factorysession_test

import (
	"testing"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestDispatchReadMappingDefaultsLiveFactsToUnconfirmedAndPreservesConfirmedFacts(t *testing.T) {
	t.Parallel()

	live := factorysession.ListDispatchesResponseToAPI(factorysessionexecution.ListDispatchesResult{
		SessionID: "dur-sess-dispatch-confirmation-001",
		Dispatches: []factorysessionexecution.DispatchSummary{{
			ID: "dispatch-live", Status: factorysessionexecution.DispatchStatus("RUNNING"),
			DispatchKind: "JAVASCRIPT_AGENT",
		}},
	})
	if len(live.Dispatches) != 1 || live.Dispatches[0].ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("live dispatch = %#v, want UNCONFIRMED", live.Dispatches)
	}

	confirmed := factorysession.ListDispatchesResponseToAPI(factorysessionexecution.ListDispatchesResult{
		SessionID: "dur-sess-dispatch-confirmation-001",
		Dispatches: []factorysessionexecution.DispatchSummary{{
			ID: "dispatch-recorded", Status: factorysessionexecution.DispatchStatus("COMPLETED"),
			DispatchKind:      "JAVASCRIPT_AGENT",
			ConfirmationState: factorysessionexecution.ConfirmationStateConfirmed,
		}},
	})
	if len(confirmed.Dispatches) != 1 || confirmed.Dispatches[0].ConfirmationState != factoryapi.CONFIRMED {
		t.Fatalf("confirmed dispatch = %#v, want CONFIRMED", confirmed.Dispatches)
	}

	detail := factorysession.DispatchDetailResponseToAPI(factorysessionexecution.DispatchDetail{
		DispatchSummary: factorysessionexecution.DispatchSummary{
			ID: "dispatch-live", Status: factorysessionexecution.DispatchStatus("RUNNING"),
			DispatchKind: "JAVASCRIPT_AGENT",
		},
		SessionID: "dur-sess-dispatch-confirmation-001", OrchestratorKind: "JAVASCRIPT",
	})
	if detail.ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("live detail confirmationState = %q, want UNCONFIRMED", detail.ConfirmationState)
	}
}

func TestHistoricalDispatchReadMappingRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()

	input := factorysession.HistoricalDispatchInput{
		ID: "dispatch-recorded", Status: "INTERRUPTED", DispatchKind: "PETRI_TRANSITION",
	}
	response := factorysession.HistoricalDispatchListToAPI("dur-sess-dispatch-confirmation-002", []factorysession.HistoricalDispatchInput{input})
	if len(response.Dispatches) != 1 || response.Dispatches[0].ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("legacy historical dispatch = %#v, want UNCONFIRMED", response.Dispatches)
	}
	detail := factorysession.HistoricalDispatchDetailToAPI(
		"dur-sess-dispatch-confirmation-002", input, "PETRI",
	)
	if detail.ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("legacy historical dispatch detail = %#v, want UNCONFIRMED", detail)
	}

	input.ConfirmationState = "CONFIRMED"
	response = factorysession.HistoricalDispatchListToAPI("dur-sess-dispatch-confirmation-002", []factorysession.HistoricalDispatchInput{input})
	if len(response.Dispatches) != 1 || response.Dispatches[0].ConfirmationState != factoryapi.CONFIRMED {
		t.Fatalf("explicit historical dispatch = %#v, want CONFIRMED", response.Dispatches)
	}
	detail = factorysession.HistoricalDispatchDetailToAPI(
		"dur-sess-dispatch-confirmation-002", input, "PETRI",
	)
	if detail.ConfirmationState != factoryapi.CONFIRMED {
		t.Fatalf("explicit historical dispatch detail = %#v, want CONFIRMED", detail)
	}
}
