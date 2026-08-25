package cli

import factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

func normalizeWorkConfirmationState(work *factoryapi.Work) {
	if work == nil {
		return
	}
	normalizeStopSummaryConfirmationState(work.StopSummary)
	if work.ConfirmationState != nil {
		switch *work.ConfirmationState {
		case factoryapi.CONFIRMED, factoryapi.UNCONFIRMED:
			return
		}
	}
	state := factoryapi.UNCONFIRMED
	work.ConfirmationState = &state
}

func normalizeStopSummaryConfirmationState(summary *factoryapi.FactoryStopSummary) {
	if summary == nil || summary.LatestDispatch == nil {
		return
	}
	if summary.LatestDispatch.ConfirmationState == factoryapi.CONFIRMED {
		return
	}
	summary.LatestDispatch.ConfirmationState = factoryapi.UNCONFIRMED
}

func normalizeWorkConfirmationStates(result *factoryapi.ListWorkResponse) {
	if result == nil {
		return
	}
	for index := range result.Results {
		normalizeWorkConfirmationState(&result.Results[index])
	}
}

func workConfirmationState(work factoryapi.Work) string {
	normalizeWorkConfirmationState(&work)
	return string(*work.ConfirmationState)
}
