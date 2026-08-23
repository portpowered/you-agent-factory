package cli

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func normalizeWorkConfirmationState(work *factoryapi.Work) {
	if work == nil {
		return
	}
	if work.ConfirmationState != nil && strings.TrimSpace(string(*work.ConfirmationState)) != "" {
		return
	}
	state := factoryapi.UNCONFIRMED
	work.ConfirmationState = &state
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
