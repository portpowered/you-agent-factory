package local_inference_test

import (
	"encoding/json"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func openFixedLeafSession(t *testing.T, baseURL string) string {
	t.Helper()
	dir := functionalScaffoldFactory(t, fixedLeafFactoryConfig())
	support.WriteWorkstationConfig(t, dir, "embed", "---\ntype: MODEL_INVOKE\n---\nEmbed the selected text.\n")
	session := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, session) })
	return session
}

func runFixedLeafSessionCancellation(t *testing.T, baseURL string, routes *fixedLeafRoutes) {
	t.Helper()
	selected, peer := openFixedLeafSession(t, baseURL), openFixedLeafSession(t, baseURL)
	selectedRoute, peerRoute := routes.registerBlocked("cancel-selected"), routes.registerBlocked("cancel-peer")
	// Each owned route is unblocked on failure as well as the normal path.
	t.Cleanup(func() { close(selectedRoute.release); close(peerRoute.release) })
	// Work admission returns before model completion, so session cancellation
	// is observed independently of canceling an HTTP invocation waiter.
	submitFixedLeafWork(t, baseURL, selected, "cancel-selected")
	submitFixedLeafWork(t, baseURL, peer, "cancel-peer")
	selectedRequest := waitFixedLeafAccepted(t, selectedRoute)
	peerRequest := waitFixedLeafAccepted(t, peerRoute)
	if selectedRequest == peerRequest {
		t.Fatal("accepted peer and selected model invocations shared a scope")
	}
	ack := postFunctionalJSON[factoryapi.FactorySessionLifecycleControlResponse](t,
		baseURL+"/factory-sessions/"+selected+"/cancel", factoryapi.FactorySessionLifecycleControlRequest{},
		"cancel selected Factory Session")
	if ack.SessionId != selected || ack.Operation != factoryapi.FactorySessionLifecycleControlKindCancel ||
		ack.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("selected cancellation acknowledgement = %#v", ack)
	}
	select {
	case <-selectedRoute.canceled:
	case <-time.After(30 * time.Second):
		t.Fatal("accepted selected model effect did not observe cancellation")
	}
	support.WaitForSessionStopped(t, baseURL, selected, 5*time.Second)
	assertFixedLeafCanceledWork(t, baseURL, selected)
	select {
	case <-peerRoute.canceled:
		t.Fatal("selected cancellation canceled the accepted peer effect")
	default:
	}
	// A sent value opens only this peer gate; cleanup later closes each channel.
	peerRoute.release <- struct{}{}
	status := support.WaitForSessionTerminalStatus(t, baseURL, peer, 5*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("peer terminal state = %#v", status)
	}
	works := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+peer+"/work")
	if len(works.Results) != 1 {
		t.Fatalf("peer work = %#v, want one completed result", works)
	}
	assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: works.Results[0].Content}, peerRoute.output)
	retry := routes.register("cancel-retry", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, peer, "fixed-leaf-cancel-retry", "cancel-retry"), retry.output)
}

func assertFixedLeafCanceledWork(t *testing.T, baseURL, session string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+session+"/work")
	if len(listed.Results) != 1 || listed.Results[0].Content == nil || len(*listed.Results[0].Content) != 1 {
		t.Fatalf("canceled Work = %#v, want only the original text input", listed)
	}
	part, err := (*listed.Results[0].Content)[0].AsWorkTextContentPart()
	if err != nil || part.Text != "cancel-selected" || part.Type != factoryapi.WorkContentPartTypeText {
		t.Fatalf("canceled Work content = %#v, %v, want preserved input without model output", part, err)
	}
}

// Hold the peer at its accepted external effect until the selected failure and
// retry complete. This ordering proves survival across the fault rather than
// relying on the test runner to overlap two otherwise independent invocations.
func runFixedLeafSessionFaultWithHeldPeer(t *testing.T, baseURL string, routes *fixedLeafRoutes) {
	t.Helper()
	selected, peer := openFixedLeafSession(t, baseURL), openFixedLeafSession(t, baseURL)
	peerRoute := routes.registerBlocked("fault-held-peer")
	t.Cleanup(func() { close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, peer, "fault-held-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	routes.recordScope(t, "fault-held-peer", peerScope)
	fault := routes.register("fault-selected", true)
	failed := invokeFixedLeafSession(t, baseURL, selected, "fixed-leaf-held-peer-fault", "fault-selected")
	if failed.Status != factoryapi.InvocationTerminalStatusFailed || failed.PrimaryResult != nil {
		t.Fatalf("selected fault = %#v, want FAILED without success output", failed)
	}
	routes.recordScope(t, "fault-selected", waitFixedLeafAccepted(t, fault))
	assertFixedLeafModelEvent(t, baseURL, selected, failed.RequestId, true)
	retry := routes.register("fault-selected-retry", false)
	retried := invokeFixedLeafSession(t, baseURL, selected, "fixed-leaf-held-peer-retry", "fault-selected-retry")
	assertFixedLeafSuccess(t, retried, retry.output)
	routes.recordScope(t, "fault-selected", waitFixedLeafAccepted(t, retry))
	assertFixedLeafModelEvent(t, baseURL, selected, retried.RequestId, false)
	select {
	case <-peerRoute.canceled:
		t.Fatal("selected fault or retry canceled the accepted peer effect")
	default:
	}
	peerRoute.release <- struct{}{}
	status := support.WaitForSessionTerminalStatus(t, baseURL, peer, 5*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("peer terminal state after selected fault/retry = %#v", status)
	}
	works := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+peer+"/work")
	if len(works.Results) != 1 {
		t.Fatalf("peer work = %#v, want one completed result", works)
	}
	assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: works.Results[0].Content}, peerRoute.output)
}

func (routes *fixedLeafRoutes) registerBlocked(text string) *fixedLeafRoute {
	routes.mu.Lock()
	defer routes.mu.Unlock()
	route := &fixedLeafRoute{output: "201", observed: make(chan models.InvokeModelRequest, 1),
		release: make(chan struct{}, 1), canceled: make(chan struct{})}
	routes.routes[text] = route
	return route
}

// Separate authored reservations allow two workstations to dispatch the same
// model while its host slot still has capacity one. Removing resource bindings
// would fail graph validation before reaching the public capacity boundary.
func openFixedLeafCapacitySession(t *testing.T, baseURL, endpoint string) string {
	t.Helper()
	config := fixedLeafFactoryConfig()
	resources := config["resources"].([]map[string]any)
	resources = append(resources, map[string]any{"name": "embed-competing-cache", "type": "MODEL",
		"capacity": 1, "model": "embed", "backend": "localai-llamacpp", "loadPolicy": "ON_DEMAND"})
	config["resources"] = resources
	workers := config["workers"].([]map[string]any)
	workers[0]["command"] = "capacity-embed"
	workers[0]["args"] = []string{"--grpc-endpoint", endpoint}
	competing := make(map[string]any)
	for key, value := range workers[0] {
		competing[key] = value
	}
	competing["name"] = "embed-competing-worker"
	competing["resources"] = []map[string]any{{"name": "embed-competing-cache", "capacity": 1}}
	config["workers"] = append(workers, competing)
	stations := config["workstations"].([]map[string]any)
	station := make(map[string]any)
	for key, value := range stations[0] {
		station[key] = value
	}
	station["name"], station["worker"] = "embed-competing", "embed-competing-worker"
	config["workstations"] = append(stations, station)
	dir := functionalScaffoldFactory(t, config)
	for _, name := range []string{"embed", "embed-competing"} {
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_INVOKE\n---\nEmbed the selected text.\n")
	}
	session := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, session) })
	return session
}

func runFixedLeafSessionCapacityWithHeldPeer(t *testing.T, baseURL string, routes *fixedLeafRoutes, launcher *recordingModelHostLauncher) {
	t.Helper()
	selected, peer := openFixedLeafCapacitySession(t, baseURL, launcher.endpoint), openFixedLeafSession(t, baseURL)
	holder, peerRoute := routes.registerBlocked("capacity-holder"), routes.registerBlocked("capacity-peer")
	t.Cleanup(func() { close(holder.release); close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, selected, "capacity-holder")
	selectedScope := waitFixedLeafAccepted(t, holder)
	submitFixedLeafWork(t, baseURL, peer, "capacity-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	if selectedScope == peerScope {
		t.Fatal("capacity holder and peer shared a model scope")
	}
	rejected := routes.register("capacity-rejected", false)
	response := invokeFixedLeafSession(t, baseURL, selected, "capacity-rejected", "capacity-rejected")
	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
		t.Fatalf("competing capacity response = %#v, want FAILED without output", response)
	}
	select {
	case request := <-rejected.observed:
		t.Fatalf("capacity refusal allocated a backend invocation: %#v", request)
	default:
	}
	assertFixedLeafCapacityFailure(t, baseURL, selected)
	if calls := launcher.CallsForCommand("capacity-embed"); calls != 1 {
		t.Fatalf("capacity refusal host starts = %d, want only holder host", calls)
	}
	holder.release <- struct{}{}
	status := support.WaitForSessionTerminalStatus(t, baseURL, selected, 30*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 1 {
		t.Fatalf("capacity holder/refusal state = %#v", status)
	}
	assertFixedLeafCapacityWork(t, baseURL, selected, holder.output)
	retry := routes.register("capacity-retry", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, selected, "capacity-retry", "capacity-retry"), retry.output)
	if scope := waitFixedLeafAccepted(t, retry); scope != selectedScope {
		t.Fatalf("capacity retry scope = %s, want %s", scope, selectedScope)
	}
	assertFixedLeafModelEvent(t, baseURL, selected, "capacity-retry", false)
	if calls := launcher.CallsForCommand("capacity-embed"); calls != 1 {
		t.Fatalf("capacity recovery host starts = %d, want reused holder host", calls)
	}
	assertFixedLeafOwnedPeerRecovery(t, baseURL, peer, peerScope, peerRoute, routes)
}

func assertFixedLeafCapacityWork(t *testing.T, baseURL, session, output string) {
	t.Helper()
	completed := 0
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, session) {
		if event.Type != "MODEL_RESPONSE" {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil || event.Context.SessionId == nil || *event.Context.SessionId != session {
			t.Fatalf("capacity Model response = %#v, %v", event, err)
		}
		if payload.Outcome == factoryapi.InferenceOutcomeSucceeded && payload.OutputContent != nil {
			completed++
			content := []factoryapi.WorkContentPart(*payload.OutputContent)
			assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted,
				PrimaryResult: &content}, output)
		}
	}
	if completed != 1 {
		t.Fatalf("completed capacity holder results = %d, want one", completed)
	}
}

func assertFixedLeafCapacityFailure(t *testing.T, baseURL, session string) {
	t.Helper()
	assertFixedLeafModelEvent(t, baseURL, session, "capacity-rejected", true)
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, session) {
		if event.Type != "MODEL_RESPONSE" || event.Context.RequestId == nil || *event.Context.RequestId != "capacity-rejected" {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		want := "inference failed for worker \"embed-competing-worker\" model \"embed\" operation \"EMBED\": model host capacity exhausted"
		if err != nil || payload.FailureDetail == nil || payload.FailureDetail.Message != want {
			t.Fatalf("capacity failure detail = %#v, %v, want %s", payload.FailureDetail, err, want)
		}
	}
}

func waitFixedLeafAccepted(t *testing.T, route *fixedLeafRoute) string {
	t.Helper()
	select {
	case request := <-route.observed:
		return request.Scope.String()
	case <-time.After(30 * time.Second):
		t.Fatal("model effect did not reach accepted readiness")
		return ""
	}
}

func assertFixedLeafSuccess(t *testing.T, response factoryapi.InvocationResponse, output string) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted || response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("selected model response = %#v, want COMPLETED with one embedding", response)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil || part.Type != factoryapi.WorkContentPartTypeJSON || part.Slot == nil || *part.Slot != "embedding" {
		t.Fatalf("embedding part = %#v, %v", part, err)
	}
	encoded, err := json.Marshal(part.Json)
	if err != nil || string(encoded) != "["+output+"]" {
		t.Fatalf("selected embedding = %s, %v, want [%s]", encoded, err, output)
	}
}

func submitFixedLeafWork(t *testing.T, baseURL, session, text string) {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{Type: factoryapi.WorkContentPartTypeText, Text: text}); err != nil {
		t.Fatal(err)
	}
	content := factoryapi.WorkContent{part}
	response := support.SubmitSessionWorkAt(t, baseURL, session,
		factoryapi.SubmitWorkRequest{WorkTypeName: "task", Content: &content})
	if !response.Accepted {
		t.Fatalf("session Work admission = %#v, want accepted", response)
	}
}

func assertFixedLeafModelEvent(t *testing.T, baseURL, session, requestID string, failure bool) {
	t.Helper()
	matched := 0
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, session) {
		if event.Type != "MODEL_RESPONSE" || event.Context.RequestId == nil || *event.Context.RequestId != requestID {
			continue
		}
		matched++
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil || event.Context.SessionId == nil || *event.Context.SessionId != session || payload.Operation != "EMBED" || payload.Model != "embed" {
			t.Fatalf("session Model response = %#v, %v", event, err)
		}
		if failure {
			if payload.Outcome != factoryapi.InferenceOutcomeFailed || payload.OutputContent != nil ||
				payload.FailureDetail == nil || payload.FailureDetail.Message == "" {
				t.Fatalf("failed Model response = %#v, want classified failure without output", payload)
			}
		} else if payload.Outcome != factoryapi.InferenceOutcomeSucceeded || payload.OutputContent == nil {
			t.Fatalf("successful Model response = %#v, want selected output", payload)
		}
	}
	if matched != 1 {
		t.Fatalf("session %s request %s Model responses = %d, want one", session, requestID, matched)
	}
}
