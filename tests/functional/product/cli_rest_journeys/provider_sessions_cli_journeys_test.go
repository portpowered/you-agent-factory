package cli_rest_journeys_test

import (
	"testing"

	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestProviderSessionCLIJourneys owns its immutable fixture until all customer scenarios finish.
func TestProviderSessionCLIJourneys(t *testing.T) {
	t.Parallel()
	resetprovidersessionscli3State()
	resetprovidersessionscli5State()
	initializeProvidersessionscliFixture(t)
	t.Run("TestFactoryTargetReadinessCustomerBehavior", testProvidersessionscliFactoryTargetReadinessCustomerBehavior)
	t.Run("TestTerminalProviderSessionReadsPreserveTranscriptOutcomes", testProvidersessionscliTerminalProviderSessionReadsPreserveTranscriptOutcomes)
	t.Run("TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess", testProvidersessionscliWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess)
	t.Run("TestWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON", testProvidersessionscliWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON)
	t.Run("TestWSRFT001OpeningRecordPrecedesProviderOutput", testProvidersessionscliWSRFT001OpeningRecordPrecedesProviderOutput)
	t.Run("TestWSRFT002LiveAndReplayCorrelationRemainStable", testProvidersessionscliWSRFT002LiveAndReplayCorrelationRemainStable)
	t.Run("TestWorkerSessionsCLI", testProvidersessionscliWorkerSessionsCLI)
	t.Run("TestWorkerSessionsFleetActiveListDeadlineRepair", testProvidersessionscliWorkerSessionsFleetActiveListDeadlineRepair)
	t.Run("TestWorkerSessionsFleetListBoundedRootPages", testProvidersessionscliWorkerSessionsFleetListBoundedRootPages)
	t.Run("TestWorkerSessionsFleetListCLIConcurrent", testProvidersessionscliWorkerSessionsFleetListCLIConcurrent)
	functionalevidence.Covers(t, "cli/you.worker-sessions.list", "cli/you.worker-sessions.read", "cli/you.worker-sessions.show", "cli/you.worker-sessions.stream")
}
