package executor

import (
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
)

func cloneContinuation(reference *workerexecution.ProviderContinuationRef) *workerexecution.ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	cloned := reference.Clone()
	return &cloned
}

// modelProviderForExecution keeps the retained Workstation prompt adapter's
// provider projection aligned with the canonical selection authority. Agent
// execution itself no longer uses this compatibility package.
func modelProviderForExecution(
	workerModelProvider string,
	selection workerexecution.ResolvedRunnerSelection,
) string {
	if selection.Source == workerexecution.RunnerSelectionSourceWorkstation ||
		selection.Source == workerexecution.RunnerSelectionSourceFactory ||
		selection.Source == workerexecution.RunnerSelectionSourceLegacyProvider {
		if provider := modelProviderForRunnerID(selection.RunnerID); provider != "" {
			return provider
		}
		if strings.TrimSpace(selection.RunnerID) != "" {
			return selection.RunnerID
		}
	}
	if workerModelProvider != "" {
		if provider := modelProviderForRunnerID(workerModelProvider); provider != "" {
			return provider
		}
		return workerModelProvider
	}
	return modelProviderForRunnerID(selection.RunnerID)
}

func modelProviderForRunnerID(runnerID string) string {
	switch workerrunner.NormalizeRunnerID(runnerID) {
	case workerexecution.RunnerIDCodex:
		return string(modelprovider.ProviderCodex)
	case string(modelprovider.ProviderClaude):
		return string(modelprovider.ProviderClaude)
	case workerexecution.RunnerIDAntigravity:
		return string(modelprovider.ProviderAntigravity)
	default:
		return ""
	}
}
