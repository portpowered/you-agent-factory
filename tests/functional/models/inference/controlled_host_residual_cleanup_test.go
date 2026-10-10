package inference_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func runResidualHealthConfigurationFailure(t *testing.T, baseURL string, hosts *residualHosts, routes *fixedLeafRoutes) {
	t.Helper()
	name := "residual-health-configuration"
	host := hosts.register("http://residual-health.invalid", "ready")
	selected := openResidualHealthSession(t, baseURL, "http://residual-health.invalid")
	peerHost := hosts.register(name+"-peer", "ready")
	peer := openResidualHostSession(t, baseURL, name+"-peer")
	peerRoute := routes.registerBlocked(name + "-peer")
	t.Cleanup(func() { close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, peer, name+"-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	route := routes.register(name+"-failed", false)
	failed := invokeFixedLeafSession(t, baseURL, selected, name+"-failed", name+"-failed")
	if failed.Status != factoryapi.InvocationTerminalStatusFailed || failed.PrimaryResult != nil || host.starts.Load() != 0 {
		t.Fatalf("HTTP-only pinned configuration = %#v, host starts %d, want failure before launch", failed, host.starts.Load())
	}
	assertResidualNoInference(t, route)
	assertFixedLeafModelEvent(t, baseURL, selected, failed.RequestId, true)
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, selected) {
		if event.Type == "MODEL_RESPONSE" && event.Context.RequestId != nil && *event.Context.RequestId == failed.RequestId {
			payload, err := event.Payload.AsModelResponseEventPayload()
			want := "inference failed for worker \"embed-worker\" model \"embed\" operation \"EMBED\": managed LocalAI backend requires one of \"--grpc-endpoint\", \"--grpc-address\", or \"--e..."
			if err != nil || payload.FailureDetail == nil || payload.FailureDetail.Message != want {
				t.Fatalf("HTTP-only configuration diagnostic = %#v/%v", payload.FailureDetail, err)
			}
		}
	}
	recoveredHost := hosts.register(name+"-recovered", "ready")
	recovered := openResidualHostSession(t, baseURL, name+"-recovered")
	retry := routes.register(name+"-retry", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, recovered, name+"-retry", name+"-retry"), retry.output)
	if waitFixedLeafAccepted(t, retry) == peerScope || recoveredHost.starts.Load() != 1 {
		t.Fatal("corrected protocol configuration failed to own its recovered host")
	}
	assertResidualPeerAfterCleanup(t, baseURL, peer, peerScope, peerRoute, peerHost, routes, name)
}

func runResidualReadyHostClose(t *testing.T, baseURL string, hosts *residualHosts, routes *fixedLeafRoutes) {
	t.Helper()
	name := "residual-ready-close"
	host := hosts.register(name, "ready")
	peerHost := hosts.register(name+"-peer", "ready")
	selected, closeSelected := openResidualHostSessionWithClose(t, baseURL, name)
	peer := openResidualHostSession(t, baseURL, name+"-peer")
	first := routes.register(name+"-first", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, selected, name+"-first", name+"-first"), first.output)
	scope := waitFixedLeafAccepted(t, first)
	assertResidualHostReuse(t, baseURL, selected, name+"-reuse", scope, routes)
	peerRoute := routes.registerBlocked(name + "-peer")
	t.Cleanup(func() { close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, peer, name+"-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	closeSelected()
	assertResidualDeletedSessionClose(t, baseURL, selected)
	waitResidualHostSignal(t, host.stopped, "closing ready Session did not stop its host")
	if host.starts.Load() != 1 || host.stops.Load() != 1 || host.exits.Load() != 1 {
		t.Fatalf("ready close launch/stop/join = %d/%d/%d, want 1/1/1", host.starts.Load(), host.stops.Load(), host.exits.Load())
	}
	assertResidualPeerAfterCleanup(t, baseURL, peer, peerScope, peerRoute, peerHost, routes, name)
}

// The immutable manual clock is an incompatible process dependency shape.
// This graph is reused across the failure, healthy peer, repair and reuse;
// parallel real-clock scenarios must not inherit its five-minute advance.
func TestModelsControlledHostResidualTimeoutRecovery(t *testing.T) {
	t.Parallel()
	hosts := &residualHosts{routes: map[string]*residualHostRoute{}}
	routes := newFixedLeafRoutes()
	clock := newControlledGenericCLIHostClock()
	server := startResidualHostServer(t, hosts, routes, clock)
	name := "residual-timeout"
	host := hosts.register(name, "timeout")
	peerHost := hosts.register(name+"-peer", "ready")
	selected := openResidualHostSession(t, server.URL(), name)
	peer := openResidualHostSession(t, server.URL(), name+"-peer")
	peerRoute := routes.registerBlocked(name + "-peer")
	t.Cleanup(func() { close(peerRoute.release) })
	submitFixedLeafWork(t, server.URL(), peer, name+"-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	failedRoute := routes.register(name+"-failed", false)
	submitFixedLeafWork(t, server.URL(), selected, name+"-failed")
	waitResidualHostSignal(t, host.probed, "selected host did not check readiness")
	assertResidualNoInference(t, failedRoute)
	clock.expireNextReadinessInterval(t)
	status := support.WaitForSessionTerminalStatus(t, server.URL(), selected, 30*time.Second)
	if status.Categories.Failed != 1 {
		t.Fatalf("timed-out Work = %#v, want one failed result", status)
	}
	assertResidualNoInference(t, failedRoute)
	waitResidualHostSignal(t, host.stopped, "readiness timeout did not stop its owned host")
	works := support.GetJSON[factoryapi.ListWorkResponse](t, server.URL()+"/factory-sessions/"+selected+"/work")
	if len(works.Results) != 1 || works.Results[0].State == nil || works.Results[0].State.Name != "failed" {
		t.Fatalf("timed-out public Work = %#v", works)
	}
	assertResidualTimeoutEvent(t, server.URL(), selected)
	host.fault.Store(false)
	retry := routes.register(name+"-retry", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, server.URL(), selected, name+"-retry", name+"-retry"), retry.output)
	scope := waitFixedLeafAccepted(t, retry)
	if host.starts.Load() != 2 || host.stops.Load() != 1 || host.exits.Load() != 1 || scope == peerScope {
		t.Fatalf("timeout repair launch/stop/join/scope = %d/%d/%d/%s", host.starts.Load(), host.stops.Load(), host.exits.Load(), scope)
	}
	assertResidualHostReuse(t, server.URL(), selected, name+"-reuse", scope, routes)
	assertResidualPeerAfterCleanup(t, server.URL(), peer, peerScope, peerRoute, peerHost, routes, name)
}

func assertResidualTimeoutEvent(t *testing.T, baseURL, session string) {
	t.Helper()
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, session) {
		if event.Type == "MODEL_RESPONSE" && event.Context.RequestId != nil {
			assertResidualHostFailureEvent(t, baseURL, session, *event.Context.RequestId, "timeout")
			return
		}
	}
	t.Fatal("missing public readiness timeout event")
}

func assertResidualDeletedSessionClose(t *testing.T, baseURL, session string) {
	t.Helper()
	// Close terminates then deletes. The existing public contract reports
	// NOT_FOUND for another close of the deleted identity, without new effects.
	request, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+session, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil || response.StatusCode != http.StatusNotFound ||
		failure.Code != "NOT_FOUND" || failure.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("repeat close = %d/%#v/%v, want NOT_FOUND", response.StatusCode, failure, err)
	}
}

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
