package factorycontracts

import "testing"

// TestHostedWorkerAuthoredShapesRemainAutomationsIngressCompatibility documents
// the WSE-08 compatibility outcome: legacy HOSTED_WORKER and POLLER_WORKER stay
// valid authored shapes for Automations-owned poller ingress without becoming
// Workers executor types.
func TestHostedWorkerAuthoredShapesRemainAutomationsIngressCompatibility(t *testing.T) {
	t.Parallel()

	for _, workerType := range []string{
		WorkerTypeHosted,
		WorkerTypePoller,
	} {
		if !IsPollerWorkerType(workerType) {
			t.Fatalf("IsPollerWorkerType(%q) = false, want true for Automations ingress compatibility", workerType)
		}
		if got := ProjectWorkerBehaviorClass(workerType); got != WorkerTypePoller {
			t.Fatalf("ProjectWorkerBehaviorClass(%q) = %q, want %q", workerType, got, WorkerTypePoller)
		}
		if !CompatibleWorkerWorkstationBehavior(
			workerType,
			WorkstationTypePoller,
			WorkstationKindPoller,
		) {
			t.Fatalf("hosted/poller worker type %q must remain compatible with poller workstations", workerType)
		}
		if CompatibleWorkerWorkstationBehavior(
			workerType,
			WorkstationTypeAgent,
			WorkstationKindStandard,
		) {
			t.Fatalf("hosted/poller worker type %q must not be compatible with executable agent workstations", workerType)
		}
	}
}
