package acp_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// A mid-turn disconnect must fail its own request; the next request gets a
// fresh peer and a complete answer.
// Isolation: isolated-with-reason - scenario-owned disconnect and replacement.
func TestProvidersACPReportsDisconnectThenServesNextRequestFromFreshPeer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	disconnectMarker := filepath.Join(dir, "disconnected")
	fixture := functionalACPFixture("disconnect-once")
	fixture.DisconnectMarkerPath = disconnectMarker
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, fixture)
	defer server.Stop(t)

	first, err := invokeACPDaemonWorkflow(t, server, "disconnect-first", singleACPAgentWorkflow)
	if err != nil || first.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("first execution = %#v, error = %v; want the disconnected peer's request to fail", first, err)
	}
	if first.Result != nil && first.Result.PrimaryResult != nil && len(*first.Result.PrimaryResult) != 0 {
		t.Fatalf("disconnected execution returned a primary result: %#v", first.Result.PrimaryResult)
	}
	assertACPPeerDisconnectFailure(t, server.URL(), first.SessionId)
	// The peer records its own stdout close before it stops answering. That
	// marker is the peer-side evidence that this failure came from a real
	// disconnect rather than from any other fault.
	waitForACPTestFile(t, disconnectMarker)

	second, err := invokeACPDaemonWorkflow(t, server, "disconnect-second", singleACPAgentWorkflow)
	if err != nil || second.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("second execution = %#v, error = %v; want recovery on a fresh peer", second, err)
	}
	if second.Result == nil || second.Result.PrimaryResult == nil || len(*second.Result.PrimaryResult) != 1 {
		t.Fatalf("recovered execution primary result = %#v, want one answer part", second.Result)
	}
	answer, err := (*second.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || answer.Text != "ACP root execution COMPLETE" {
		t.Fatalf("recovered answer = %q, error = %v; want the complete ACP answer", answer.Text, err)
	}
	// One request-owned process per request: the disconnected peer and the peer
	// that served the recovered request.
	if starts.Load() != 2 {
		t.Fatalf("ACP process starts = %d, want one disconnected peer plus one fresh peer", starts.Load())
	}
}

// Pin the actual public failed dispatch; the peer marker identifies its fault.
func assertACPPeerDisconnectFailure(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t, baseURL+"/factory-sessions/"+sessionID+"/dispatches",
	)
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].FailureDetail == nil {
		t.Fatalf("disconnected request dispatches = %#v, want one typed failed child", dispatches.Dispatches)
	}
	detail := dispatches.Dispatches[0].FailureDetail
	// Detached retries currently mask the first disconnect with an unsupported
	// continuation. Pin the actual public outcome rather than inventing a code.
	if detail.Reason != factoryapi.WorkFailureTypePermanentBadRequest || detail.Message != "provider session continuation is unsupported" {
		t.Fatalf("disconnect retry outcome = %#v, want the typed unsupported continuation failure", detail)
	}
}
