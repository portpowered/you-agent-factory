package acp_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestProvidersACPRestartsAfterCrashWithoutReplayingUncertainPrompt(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "crash-once")
	marker := filepath.Join(t.TempDir(), "crashed")
	t.Setenv("YOU_TEST_ACP_CRASH_MARKER", marker)
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts)
	defer server.Stop(t)
	first, err := invokeACPDaemonWorkflow(t, server, "crash", singleACPAgentWorkflow)
	if err != nil || first.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("first execution = %#v, error = %v; want failed peer crash", first, err)
	}
	result, err := invokeACPDaemonWorkflow(t, server, "after-crash", singleACPAgentWorkflow)
	if err != nil || result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("second execution = %#v, error = %v; want recovered success", result, err)
	}
	if starts.Load() != 2 {
		t.Fatalf("ACP process starts = %d, want one crash plus one replacement", starts.Load())
	}
}
