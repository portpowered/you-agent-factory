package inference_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One immutable graph serves independent explicit Sessions. A pinned LocalAI
// host uses protocol negotiation as its readiness boundary (not HTTP health).
func TestModelsControlledHostResidual(t *testing.T) {
	t.Parallel()
	hosts := &residualHosts{routes: map[string]*residualHostRoute{}}
	routes := newFixedLeafRoutes()
	t.Cleanup(func() {
		hosts.mu.Lock()
		defer hosts.mu.Unlock()
		for name, host := range hosts.routes {
			if host.mode == "ready" && (host.starts.Load() != host.stops.Load() || host.starts.Load() != host.exits.Load()) {
				t.Errorf("process close did not drain %s: starts=%d stops=%d joins=%d", name, host.starts.Load(), host.stops.Load(), host.exits.Load())
			}
		}
	})
	server := startResidualHostServer(t, hosts, routes)
	t.Run("distinct configuration survives repeat while peer is accepted", func(t *testing.T) {
		t.Parallel()
		runResidualScopedConfigurations(t, server.URL(), hosts, routes)
	})
	t.Run("readiness withholds inference then reuses host", func(t *testing.T) {
		t.Parallel()
		runResidualHostReadiness(t, server.URL(), hosts, routes)
	})
	for _, mode := range []string{"launch", "crash", "protocol"} {
		t.Run(mode+" failure preserves peer and repairs", func(t *testing.T) {
			t.Parallel()
			runResidualHostFailure(t, server.URL(), hosts, routes, mode)
		})
	}
	for _, control := range []string{"cancel", "close"} {
		t.Run(control+" owns host cleanup and preserves peer", func(t *testing.T) {
			t.Parallel()
			runResidualHostCleanup(t, server.URL(), hosts, routes, control)
		})
	}
	t.Run("ready Session close stops only its reused host", func(t *testing.T) {
		t.Parallel()
		runResidualReadyHostClose(t, server.URL(), hosts, routes)
	})
	t.Run("HTTP-only readiness configuration fails without launching", func(t *testing.T) {
		t.Parallel()
		runResidualHealthConfigurationFailure(t, server.URL(), hosts, routes)
	})
}

func runResidualHostReadiness(t *testing.T, baseURL string, hosts *residualHosts, routes *fixedLeafRoutes) {
	t.Helper()
	host := hosts.register("residual-readiness", "gated")
	t.Cleanup(func() { close(host.ready) })
	session := openResidualHostSession(t, baseURL, "residual-readiness")
	route := routes.register("residual-readiness-first", false)
	submitFixedLeafWork(t, baseURL, session, "residual-readiness-first")
	waitResidualHostSignal(t, host.probed, "selected host did not enter protocol readiness")
	assertResidualNoInference(t, route)
	works := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+session+"/work")
	if len(works.Results) != 1 || works.Results[0].State == nil || works.Results[0].State.Name != "init" {
		t.Fatalf("Work before protocol readiness = %#v, want pending input", works)
	}
	host.fault.Store(false)
	host.ready <- struct{}{}
	scope := waitFixedLeafAccepted(t, route)
	status := support.WaitForSessionTerminalStatus(t, baseURL, session, 30*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("ready host Work = %#v", status)
	}
	works = support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+session+"/work")
	assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: works.Results[0].Content}, route.output)
	assertResidualHostReuse(t, baseURL, session, "residual-readiness-reuse", scope, routes)
	if host.starts.Load() != 1 {
		t.Fatal("successful Session reuse launched another host")
	}
}

func runResidualHostFailure(t *testing.T, baseURL string, hosts *residualHosts, routes *fixedLeafRoutes, mode string) {
	t.Helper()
	name := "residual-" + mode
	host := hosts.register(name, mode)
	peerHost := hosts.register(name+"-peer", "ready")
	selected := openResidualHostSession(t, baseURL, name)
	peer := openResidualHostSession(t, baseURL, name+"-peer")
	peerRoute := routes.registerBlocked(name + "-peer")
	t.Cleanup(func() { close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, peer, name+"-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	failedRoute := routes.register(name+"-failed", false)
	failed := invokeFixedLeafSession(t, baseURL, selected, name+"-failed", name+"-failed")
	if failed.Status != factoryapi.InvocationTerminalStatusFailed || failed.PrimaryResult != nil {
		t.Fatalf("host %s failure = %#v, want FAILED without inference output", mode, failed)
	}
	assertResidualNoInference(t, failedRoute)
	assertFixedLeafModelEvent(t, baseURL, selected, failed.RequestId, true)
	assertResidualHostFailureEvent(t, baseURL, selected, failed.RequestId, mode)
	host.fault.Store(false)
	retry := routes.register(name+"-retry", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, selected, name+"-retry", name+"-retry"), retry.output)
	scope := waitFixedLeafAccepted(t, retry)
	if scope == peerScope || host.starts.Load() != 2 {
		t.Fatalf("repair scope/starts = %s/%d, peer %s", scope, host.starts.Load(), peerScope)
	}
	assertResidualHostReuse(t, baseURL, selected, name+"-reuse", scope, routes)
	select {
	case <-peerRoute.canceled:
		t.Fatal("selected host failure canceled healthy peer inference")
	case <-peerHost.stopped:
		t.Fatal("selected host failure stopped healthy peer host")
	default:
	}
	peerRoute.release <- struct{}{}
	status := support.WaitForSessionTerminalStatus(t, baseURL, peer, 30*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("healthy peer Work after host failure = %#v", status)
	}
	works := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+peer+"/work")
	assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: works.Results[0].Content}, peerRoute.output)
	assertResidualHostReuse(t, baseURL, peer, name+"-peer-reuse", peerScope, routes)
}

func assertResidualNoInference(t *testing.T, route *fixedLeafRoute) {
	t.Helper()
	select {
	case request := <-route.observed:
		t.Fatalf("unready host admitted inference: %#v", request)
	default:
	}
}

func assertResidualHostReuse(t *testing.T, baseURL, session, text, scope string, routes *fixedLeafRoutes) {
	t.Helper()
	route := routes.register(text, false)
	response := invokeFixedLeafSession(t, baseURL, session, text, text)
	assertFixedLeafSuccess(t, response, route.output)
	assertFixedLeafModelEvent(t, baseURL, session, text, false)
	if actual := waitFixedLeafAccepted(t, route); actual != scope {
		t.Fatalf("reused host scope = %s, want %s", actual, scope)
	}
}

func assertResidualHostFailureEvent(t *testing.T, baseURL, session, requestID, mode string) {
	t.Helper()
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, session) {
		if event.Type != "MODEL_RESPONSE" || event.Context.RequestId == nil || *event.Context.RequestId != requestID {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		// The public Work diagnostic is bounded by the existing sanitizer.
		want := "inference failed for worker \"embed-worker\" model \"embed\" operation \"EMBED\": model host \"embed\" readiness is FAILED (lifecycle LOADED): resolve failure class pro..."
		if mode == "timeout" {
			want = "inference timed out for model \"embed\" operation \"EMBED\": wait and retry the request"
		}
		if err != nil || payload.FailureDetail == nil || payload.FailureDetail.Message != want || payload.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown {
			t.Fatalf("host %s diagnostic = %#v, %v, want %s", mode, payload.FailureDetail, err, want)
		}
		return
	}
	t.Fatal("missing public host crash event")
}

func runResidualScopedConfigurations(t *testing.T, baseURL string, hosts *residualHosts, routes *fixedLeafRoutes) {
	t.Helper()
	selectedHost := hosts.register("config-selected", "ready")
	peerHost := hosts.register("config-peer", "ready")
	selected := openResidualHostSession(t, baseURL, "config-selected")
	peer := openResidualHostSession(t, baseURL, "config-peer")
	peerRoute := routes.registerBlocked("config-peer-held")
	t.Cleanup(func() { close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, peer, "config-peer-held")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	var selectedScope string
	for _, name := range []string{"config-selected-first", "config-selected-repeat"} {
		route := routes.register(name, false)
		result := invokeFixedLeafSession(t, baseURL, selected, name, name)
		assertFixedLeafSuccess(t, result, route.output)
		scope := waitFixedLeafAccepted(t, route)
		if scope == peerScope || (selectedScope != "" && scope != selectedScope) {
			t.Fatalf("configuration scopes: selected=%s previous=%s peer=%s", scope, selectedScope, peerScope)
		}
		selectedScope = scope
		assertFixedLeafModelEvent(t, baseURL, selected, name, false)
	}
	for endpoint, host := range map[string]*residualHostRoute{"config-selected": selectedHost, "config-peer": peerHost} {
		select {
		case spec := <-host.specs:
			if spec.HealthEndpoint != endpoint || spec.Command != "controlled-embed" || spec.ModelPath == "" {
				t.Fatalf("scoped host selection: %#v, want %s", spec, endpoint)
			}
		default:
			t.Fatalf("no host selection for %s", endpoint)
		}
		if host.starts.Load() != 1 {
			t.Fatalf("host %s starts=%d, want reuse", endpoint, host.starts.Load())
		}
	}
	assertResidualPeerAfterCleanup(t, baseURL, peer, peerScope, peerRoute, peerHost, routes, "config-selected")
}
