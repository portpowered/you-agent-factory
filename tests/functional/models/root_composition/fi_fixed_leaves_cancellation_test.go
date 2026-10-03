package root_composition_test

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
	case <-time.After(5 * time.Second):
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

func waitFixedLeafAccepted(t *testing.T, route *fixedLeafRoute) string {
	t.Helper()
	select {
	case request := <-route.observed:
		return request.Scope.String()
	case <-time.After(5 * time.Second):
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
