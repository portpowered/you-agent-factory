package internal

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func providerRunnerID(id providers.ID) string {
	return strings.ToLower(strings.TrimSpace(id.String()))
}

func workerSelectionSource(source providers.SelectionSource) workers.RunnerSelectionSource {
	switch source {
	case providers.SelectionSourceWorkstation:
		return workers.RunnerSelectionSourceWorkstation
	case providers.SelectionSourceFactory:
		return workers.RunnerSelectionSourceFactory
	case providers.SelectionSourceLegacyProvider:
		return workers.RunnerSelectionSourceLegacyProvider
	default:
		return workers.RunnerSelectionSourceDefault
	}
}
