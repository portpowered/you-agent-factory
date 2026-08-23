package projections

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func runnerIDFromDispatchMetadata(metadata *interfaces.DispatchRequestEventMetadata) string {
	if metadata == nil {
		return ""
	}
	return stringValue(metadata.RunnerID)
}

func runnerSelectionSource(metadata *interfaces.DispatchRequestEventMetadata) workerexecution.RunnerSelectionSource {
	if metadata == nil || metadata.RunnerSelectionSource == nil {
		return ""
	}
	return *metadata.RunnerSelectionSource
}
