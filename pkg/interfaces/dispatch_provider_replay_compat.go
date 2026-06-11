package interfaces

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// PublicModelProviderFromDispatchRequestMetadata returns the canonical public
// modelProvider value recorded on dispatch-request metadata when present.
func PublicModelProviderFromDispatchRequestMetadata(metadata *factoryapi.DispatchRequestEventMetadata) string {
	if metadata == nil || metadata.ModelProvider == nil {
		return ""
	}
	return string(*metadata.ModelProvider)
}

// RunnerMetadataFromDispatchRequestMetadata maps dispatch-request event metadata
// into the legacy runner projection fields retained for world-state replay.
func RunnerMetadataFromDispatchRequestMetadata(metadata *factoryapi.DispatchRequestEventMetadata) (runnerID string, source RunnerSelectionSource) {
	if metadata == nil {
		return "", ""
	}
	if metadata.ModelProvider != nil {
		if id, ok := InternalRunnerIDFromPublicWorkerModelProvider(*metadata.ModelProvider); ok {
			runnerID = id
		}
	}
	if metadata.ModelProviderSelectionSource != nil {
		source = modelProviderSelectionSourceToRunnerSelectionSource(
			ModelProviderSelectionSource(*metadata.ModelProviderSelectionSource),
		)
	}
	return runnerID, source
}

// PublicModelProviderFromLegacyRunnerID maps one replay artifact runnerId value to
// the public WorkerModelProvider enum used by new dispatch metadata.
func PublicModelProviderFromLegacyRunnerID(runnerID string) (factoryapi.WorkerModelProvider, error) {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" {
		return "", nil
	}
	if public, ok := GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProvider(runnerID); ok {
		return public, nil
	}
	return "", fmt.Errorf("unknown legacy runnerId %q", runnerID)
}

// PublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource maps one replay
// artifact runnerSelectionSource value to the public modelProviderSelectionSource enum.
func PublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource(source string) factoryapi.ModelProviderSelectionSource {
	return GeneratedPublicFactoryModelProviderSelectionSource(
		PermissivePublicFactoryModelProviderSelectionSource(source),
	)
}
