package acp_test

import (
	"sync/atomic"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Isolation: isolated-with-reason - initialization negotiation; the durable
// execution must observe an incompatible version from a fresh stdio peer.
func TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary(t *testing.T) {
	t.Parallel()
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, functionalACPFixture("version"))
	defer server.Stop(t)
	result, executeErr := invokeACPDaemonWorkflow(t, server, "version-attempt", singleACPAgentWorkflow)
	if executeErr != nil || result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("version-incompatible execution = %#v, error = %v; want FAILED", result, executeErr)
	}
}
