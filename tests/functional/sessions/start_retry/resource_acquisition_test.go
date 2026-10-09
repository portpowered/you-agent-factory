package start_retry_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The public close must cancel only the selected session's running worker.
// Both exact command edges enter before close; the peer stays gated until its
// live route and event generation have been observed after candidate cleanup.
func testSelectedProviderCancellation(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, gate *selectedProviderGate, baseURL string) {
	t.Helper()
	defer gate.unblock()
	startInitialOpeningSession(t, sessions, scenario.request())
	peer := scenario.request()
	peer.SessionID, peer.FolderPath = scenario.peerID, scenario.peerDir
	peer.RuntimeSelection.DefinitionSourcePath = scenario.peerDir + "/factory.json"
	peer.RuntimeSelection.ExecutionBaseDir, peer.RuntimeSelection.RuntimeInstanceID = scenario.peerDir, uuid.NewString()
	startInitialOpeningSession(t, sessions, peer)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	candidateDone, peerDone := make(chan error, 1), make(chan error, 1)
	go func() { candidateDone <- selectedProviderInvoke(ctx, sessions, scenario.candidateID) }()
	go func() { peerDone <- selectedProviderInvoke(ctx, sessions, scenario.peerID) }()
	awaitSelectedProviderPair(t, ctx, gate)
	peerHistory := initialOpeningHistory(t, sessions, scenario.peerID)
	closeInitialOpeningSession(t, sessions, scenario.candidateID)
	select {
	case err := <-candidateDone:
		if err == nil {
			t.Fatal("closed candidate returned successful worker output")
		}
	case <-ctx.Done():
		t.Fatal("closing the candidate did not cancel its invocation")
	}
	assertInitialOpeningNotPublished(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertGatedPeerWork(t, scenario, baseURL, peerDone)
	gate.unblock()
	select {
	case err := <-peerDone:
		if err != nil {
			t.Fatalf("peer did not retain its selected result after candidate cleanup: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("peer invocation did not finish after release")
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	if err := selectedProviderInvoke(ctx, sessions, scenario.peerID); err != nil {
		t.Fatal(err)
	}
}

func awaitSelectedProviderPair(t *testing.T, ctx context.Context, gate *selectedProviderGate) {
	t.Helper()
	seen := make(map[string]bool)
	for range 2 {
		select {
		case request := <-gate.entered:
			id := gate.paths[request.WorkDir]
			if id == "" || seen[id] {
				t.Fatalf("overlapping command identity = %q, seen=%v", id, seen)
			}
			seen[id] = true
		case <-ctx.Done():
			t.Fatal("both workers did not reach their owned command gate")
		}
	}
}

func assertGatedPeerWork(t *testing.T, scenario initialOpeningScenario, baseURL string, peerDone <-chan error) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, scenario.peerID, "/work"))
	if len(listed.Results) != 1 || fmt.Sprint(listed.Results[0].Payload) != scenario.peerID+" selected Work" {
		t.Fatalf("peer Work changed during candidate cleanup: %#v", listed)
	}
	select {
	case err := <-peerDone:
		t.Fatalf("peer invocation ended before its gate was released: %v", err)
	default:
	}
}
