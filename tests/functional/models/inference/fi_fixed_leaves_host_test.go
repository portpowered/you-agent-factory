package inference_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertFixedLeafOwnedPeerRecovery(t *testing.T, baseURL, peer, peerScope string, peerRoute *fixedLeafRoute, routes *fixedLeafRoutes) {
	t.Helper()
	select {
	case <-peerRoute.canceled:
		t.Fatal("selected cancellation canceled the accepted peer effect")
	default:
	}
	peerRoute.release <- struct{}{}
	status := support.WaitForSessionTerminalStatus(t, baseURL, peer, 5*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("peer terminal state = %#v", status)
	}
	works := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+peer+"/work")
	if len(works.Results) != 1 {
		t.Fatalf("peer Work = %#v, want one completed result", works)
	}
	assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted, PrimaryResult: works.Results[0].Content}, peerRoute.output)
	retry := routes.register("owned-host-peer-retry", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, peer, "owned-host-peer-retry", "owned-host-peer-retry"), retry.output)
	if scope := waitFixedLeafAccepted(t, retry); scope != peerScope {
		t.Fatalf("peer retry scope = %s, want %s", scope, peerScope)
	}
}
