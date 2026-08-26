package acp_test

import (
	"strconv"
	"sync/atomic"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestProvidersACPRetainsOneOSProcessAndConnectionAcrossExecutions(t *testing.T) {
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, functionalACPFixture("persistent"))
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

func TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary(t *testing.T) {
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, functionalACPFixture("version"))
	defer server.Stop(t)
	result, executeErr := invokeACPDaemonWorkflow(t, server, "version-attempt", singleACPAgentWorkflow)
	if executeErr != nil || result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("version-incompatible execution = %#v, error = %v; want FAILED", result, executeErr)
	}
}
