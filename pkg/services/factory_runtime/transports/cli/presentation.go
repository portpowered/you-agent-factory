package cli

import (
	"strings"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboard"
)

const (
	completedPlaceIDSuffix = "completed"
	failedPlaceIDSuffix    = "failed"
)

func countTokenStates(snap *factoryruntime.PetriMarkingSnapshot) (wip, completed, failed int) {
	if snap == nil {
		return 0, 0, 0
	}
	for _, token := range snap.Tokens {
		placeID := token.PlaceID
		state := placeID
		if idx := strings.LastIndexByte(placeID, ':'); idx >= 0 {
			state = placeID[idx+1:]
		}

		switch {
		case isFailedState(state):
			failed++
		case isTerminalState(state):
			completed++
		default:
			wip++
		}
	}
	return wip, completed, failed
}

func isTerminalState(state string) bool {
	return state == completedPlaceIDSuffix
}

func isFailedState(state string) bool {
	return state == failedPlaceIDSuffix
}

func formatDuration(d time.Duration) string {
	return dashboard.FormatDuration(d)
}
