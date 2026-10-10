package inference_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func runResidualHostCleanup(t *testing.T, baseURL string, hosts *residualHosts, routes *fixedLeafRoutes, control string) {
	t.Helper()
	name := "residual-" + control
	host := hosts.register(name, "gated")
	peerHost := hosts.register(name+"-peer", "ready")
	selected, closeSelected := openResidualHostSessionWithClose(t, baseURL, name)
	peer := openResidualHostSession(t, baseURL, name+"-peer")
	selectedRoute := routes.register(name+"-selected", false)
	peerRoute := routes.registerBlocked(name + "-peer")
	t.Cleanup(func() { close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, selected, name+"-selected")
	waitResidualHostSignal(t, host.probed, "selected host did not reach pending readiness")
	submitFixedLeafWork(t, baseURL, peer, name+"-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	assertResidualNoInference(t, selectedRoute)
	if control == "close" {
		closeSelected()
	} else {
		ack := postFunctionalJSON[factoryapi.FactorySessionLifecycleControlResponse](t,
			baseURL+"/factory-sessions/"+selected+"/cancel", factoryapi.FactorySessionLifecycleControlRequest{},
			"cancel selected host loading")
		if ack.SessionId != selected || ack.Operation != factoryapi.FactorySessionLifecycleControlKindCancel ||
			ack.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
			t.Fatalf("selected host cancellation acknowledgement = %#v", ack)
		}
		support.WaitForSessionStopped(t, baseURL, selected, 30*time.Second)
	}
	waitResidualHostSignal(t, host.stopped, "selected Session did not stop its loading host")
	assertResidualNoInference(t, selectedRoute)
	assertResidualPeerAfterCleanup(t, baseURL, peer, peerScope, peerRoute, peerHost, routes, name)
	// A cancelled/closed Session is terminal. A new explicit Session is the
	// customer recovery boundary; it still uses the same constructed process.
	recoveredHost := hosts.register(name+"-recovered", "ready")
	recovered := openResidualHostSession(t, baseURL, name+"-recovered")
	route := routes.register(name+"-recovered", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, recovered, name+"-recovered", name+"-recovered"), route.output)
	if waitFixedLeafAccepted(t, route) == peerScope || recoveredHost.starts.Load() != 1 {
		t.Fatal("replacement Session failed to own its recovered host")
	}
}

func assertResidualPeerAfterCleanup(t *testing.T, baseURL, peer, scope string, route *fixedLeafRoute, host *residualHostRoute, routes *fixedLeafRoutes, name string) {
	t.Helper()
	select {
	case <-route.canceled:
		t.Fatal("selected Session cleanup canceled peer inference")
	case <-host.stopped:
		t.Fatal("selected Session cleanup stopped peer host")
	default:
	}
	route.release <- struct{}{}
	status := support.WaitForSessionTerminalStatus(t, baseURL, peer, 30*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("peer Work after Session cleanup = %#v", status)
	}
	works := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+peer+"/work")
	if len(works.Results) != 1 {
		t.Fatalf("peer Work after Session cleanup = %#v", works)
	}
	assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: works.Results[0].Content}, route.output)
	assertResidualHostReuse(t, baseURL, peer, name+"-peer-reuse", scope, routes)
	if host.starts.Load() != 1 {
		t.Fatal("peer cleanup recovery lost its reusable host")
	}
}
