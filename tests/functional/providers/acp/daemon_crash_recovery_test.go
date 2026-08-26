package acp_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestProvidersACPRestartsAfterCrashWithoutReplayingUncertainPrompt(t *testing.T) {
	t.Parallel()
	fixture := functionalACPFixture("crash-once")
	fixture.CrashMarkerPath = filepath.Join(t.TempDir(), "crashed")
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, fixture)
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

func TestProvidersACPRetiresDisconnectedConnectionBeforeReuse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	disconnectMarker := filepath.Join(dir, "disconnected")
	readyMarker := filepath.Join(dir, "response-ready")
	releaseMarker := filepath.Join(dir, "release")
	fixture := functionalACPFixture("disconnect-once")
	fixture.DisconnectMarkerPath = disconnectMarker
	fixture.DisconnectReadyPath = readyMarker
	fixture.DisconnectReleasePath = releaseMarker
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, fixture)
	defer server.Stop(t)

	first, err := invokeACPDaemonWorkflow(t, server, "disconnect-first", singleACPAgentWorkflow)
	if err != nil || first.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("first execution = %#v, error = %v; want successful response before peer disconnect", first, err)
	}
	// The helper first signals that the prompt response was flushed and waits for
	// this test to release it. That keeps the first public execution terminal
	// before the peer disconnects, while the later marker proves the connection
	// has disconnected and the child remains alive awaiting stdin closure.
	if _, err := support.WaitForObservation(
		5*time.Second,
		func() (bool, error) {
			_, statErr := os.Stat(readyMarker)
			if statErr != nil && !os.IsNotExist(statErr) {
				return false, statErr
			}
			return statErr == nil, nil
		},
		func(observed bool) bool { return observed },
	); err != nil {
		t.Fatalf("wait for ACP peer response-ready checkpoint: %v", err)
	}
	if err := os.WriteFile(releaseMarker, []byte("release"), 0o600); err != nil {
		t.Fatalf("release ACP peer disconnect: %v", err)
	}
	// The helper exposes no parent-process event for a peer-side stdout close;
	// polling this isolated durable checkpoint keeps the second public
	// invocation deterministic without synchronizing on a fixed delay.
	if _, err := support.WaitForObservation(
		5*time.Second,
		func() (bool, error) {
			_, statErr := os.Stat(disconnectMarker)
			if statErr != nil && !os.IsNotExist(statErr) {
				return false, statErr
			}
			return statErr == nil, nil
		},
		func(observed bool) bool { return observed },
	); err != nil {
		t.Fatalf("wait for ACP peer disconnect: %v", err)
	}

	second, err := invokeACPDaemonWorkflow(t, server, "disconnect-second", singleACPAgentWorkflow)
	if err != nil || second.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("second execution = %#v, error = %v; want replacement peer success", second, err)
	}
	if starts.Load() != 2 {
		t.Fatalf("ACP process starts = %d, want one disconnected peer plus one replacement", starts.Load())
	}
}
