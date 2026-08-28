package acp_test

import (
	"strconv"
	"sync/atomic"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Isolation: isolated-with-reason - process and connection identity; two
// sequential executions must use one retained real OS peer and stdio stream.
func TestProvidersACPRetainsOneOSProcessAndConnectionAcrossExecutions(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "persistent")
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts)
	defer server.Stop(t)

	for attempt := 1; attempt <= 2; attempt++ {
		result, executeErr := invokeACPDaemonWorkflow(t, server, "persistent-attempt-"+strconv.Itoa(attempt), singleACPAgentWorkflow)
		if executeErr != nil || result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("execution %d = %#v, error = %v", attempt, result, executeErr)
		}
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("ACP process starts = %d, want one retained OS process", got)
	}
}

// Isolation: isolated-with-reason - initialization negotiation; the durable
// execution must observe an incompatible version from a fresh stdio peer.
func TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "version")
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts)
	defer server.Stop(t)
	result, executeErr := invokeACPDaemonWorkflow(t, server, "version-attempt", singleACPAgentWorkflow)
	if executeErr != nil || result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("version-incompatible execution = %#v, error = %v; want FAILED", result, executeErr)
	}
}
