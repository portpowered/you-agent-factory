package agy

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAgySharedProcessFailureThenSuccessRecovers proves that an empty
// provider result cannot poison a later invocation on the reusable process.
// The two named-Factory executions retain separate Work, dispatch, event and
// recording identities while using the same frozen external route.
func TestAgySharedProcessFailureThenSuccessRecovers(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	fixture.startRoleHost(t)
	selector := "role-recovery-empty-then-valid"
	route := fixture.routes[selector]
	route.resetOutcomeSequence()

	firstResponse, firstEvents, _, _, firstCallStart := fixture.runRoleFailure(t, selector, []string{
		"you", "--json", "run",
		"--named", agyColdWatchFactoryName,
		"--cut-path", route.assetPath,
	})
	assertAgyFailedInvocation(t, firstResponse, firstEvents)
	assertAgyFactoryEventOrder(t, firstEvents)
	if got := route.callCount() - firstCallStart; got != 1 {
		t.Fatalf("empty-result provider calls = %d, want exactly one", got)
	}
	firstIdentity := readAgyFactoryEventIdentity(t, firstEvents)

	secondResponse, secondEvents, _, _, secondCallStart := fixture.runRole(t, selector, []string{
		"you", "--json", "run",
		"--named", agyColdWatchFactoryName,
		"--cut-path", route.assetPath,
	})
	if secondResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("recovery invocation status = %q, want COMPLETED; response=%#v", secondResponse.Status, secondResponse)
	}
	if secondResponse.PrimaryResult == nil {
		t.Fatal("recovery invocation primaryResult is nil, want valid report")
	}
	if !strings.Contains(invocationPrimaryText(t, *secondResponse.PrimaryResult), "Recommendation: pass") {
		t.Fatalf("recovery primary result = %q, want accepted cold-watch report", invocationPrimaryText(t, *secondResponse.PrimaryResult))
	}
	assertAgyFactoryEventOrder(t, secondEvents)
	if got := route.callCount() - secondCallStart; got != 1 {
		t.Fatalf("recovery provider calls = %d, want exactly one", got)
	}
	secondIdentity := readAgyFactoryEventIdentity(t, secondEvents)
	assertAgyInvocationIdentitiesDistinct(t, firstIdentity, secondIdentity)
	assertAgySingleDispatch(t, firstEvents, factoryapi.WorkOutcomeFailed)
	assertAgySingleDispatch(t, secondEvents, factoryapi.WorkOutcomeAccepted)

}

// TestAgySharedProcessConcurrentRoutesRemainIsolated proves that two hosted
// invocations can overlap on the one immutable process without sharing route,
// Work, Factory Event, Response Event or HTTP-server state.
func TestAgySharedProcessConcurrentRoutesRemainIsolated(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	host := fixture.startRoleHost(t)
	firstRoute := fixture.routes["concurrency-a"]
	secondRoute := fixture.routes["concurrency-b"]
	release := make(chan struct{})
	released := false
	firstRoute.setRelease(release)
	secondRoute.setRelease(release)
	t.Cleanup(func() {
		if !released {
			close(release)
		}
		firstRoute.setRelease(nil)
		secondRoute.setRelease(nil)
	})
	runnerCallStart := fixture.runner.callCount()
	firstCallStart := firstRoute.callCount()
	secondCallStart := secondRoute.callCount()
	openRoute := func(route *agySharedCommandRoute) (string, *support.FactoryResponseEventStream) {
		opened := support.OpenFactorySessionAt(t, host.baseURL, route.workDir)
		sessionID := opened.Session.Id
		if err := fixture.runner.registerScope(sessionID, route); err != nil {
			t.Fatalf("register concurrent AGY route: %v", err)
		}
		stream := support.OpenFactoryResponseEventStreamAt(
			t, support.SessionResponseEventsURL(host.baseURL, sessionID),
		)
		t.Cleanup(func() {
			stream.Close()
			fixture.runner.unregisterScope(sessionID, route)
			support.CloseFactorySessionAt(t, host.baseURL, sessionID)
		})
		return sessionID, stream
	}
	firstSessionID, firstStream := openRoute(firstRoute)
	secondSessionID, secondStream := openRoute(secondRoute)
	fixture.runner.waitForCallCount(t, runnerCallStart+2)
	if got := fixture.runner.activeCallCount(); got != 2 {
		t.Fatalf("active AGY calls while routes are held = %d, want 2", got)
	}
	if got := fixture.runner.maxActiveCallCount(); got < 2 {
		t.Fatalf("maximum active AGY calls = %d, want overlap of both routes", got)
	}
	close(release)
	released = true
	firstSession, firstListed, firstEvents, firstResponseEvents := fixture.observeHostedSession(
		t, host.baseURL, firstSessionID, firstStream,
	)
	secondSession, secondListed, secondEvents, secondResponseEvents := fixture.observeHostedSession(
		t, host.baseURL, secondSessionID, secondStream,
	)

	assertAgyConcurrentInvocation(t, firstSession, firstListed, firstEvents, firstResponseEvents, firstRoute, firstCallStart, firstSessionID, "shared concurrency A COMPLETE", "shared concurrency B COMPLETE")
	assertAgyConcurrentInvocation(t, secondSession, secondListed, secondEvents, secondResponseEvents, secondRoute, secondCallStart, secondSessionID, "shared concurrency B COMPLETE", "shared concurrency A COMPLETE")
}

func assertAgyFailedInvocation(
	t *testing.T,
	response factoryapi.InvocationResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response=%#v", response.Status, response)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("failed invocation primaryResult = %#v, want nil", response.PrimaryResult)
	}
	if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
		t.Fatalf("failed invocation message = %#v, want actionable diagnostic", response.Message)
	}
	if len(events) == 0 {
		t.Fatal("failed invocation Factory Events are empty")
	}
}

func assertAgyConcurrentInvocation(
	t *testing.T,
	session factoryapi.FactorySession,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
	route *agySharedCommandRoute,
	callStart int,
	expectedEventSessionID string,
	wantOutput, foreignOutput string,
) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("route %q completed Work = %d, want 1", route.selector, got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("route %q failed Work = %d, want 0", route.selector, got)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("route %q provider calls = %d, want exactly one", route.selector, got)
	}
	if request := route.lastRequest(); request.WorkDir != route.workDir {
		t.Fatalf("route %q request WorkDir = %q, want %q", route.selector, request.WorkDir, route.workDir)
	}
	assertAgyFactoryEventOrderForSession(t, expectedEventSessionID, events)
	assertAgySingleDispatchOutput(t, events, factoryapi.WorkOutcomeAccepted, wantOutput)
	assertAgyResponseEventIsolation(t, session.Id, responseEvents, wantOutput, foreignOutput)
}

func assertAgyResponseEventIsolation(
	t *testing.T,
	sessionID string,
	events []factoryapi.FactoryResponseEvent,
	wantText, foreignText string,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("hosted invocation response events are empty")
	}
	seenIDs := make(map[string]struct{}, len(events))
	foundText := false
	for index, event := range events {
		if event.EventId == "" {
			t.Fatalf("response event %d has empty identity", index)
		}
		if _, exists := seenIDs[event.EventId]; exists {
			t.Fatalf("response event %q is duplicated", event.EventId)
		}
		seenIDs[event.EventId] = struct{}{}
		if event.FactorySessionId != sessionID {
			t.Fatalf("response event %q session = %q, want %q", event.EventId, event.FactorySessionId, sessionID)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal response event %q: %v", event.EventId, err)
		}
		payload := string(encoded)
		if strings.Contains(payload, foreignText) {
			t.Fatalf("response event %q crossed sibling output %q: %s", event.EventId, foreignText, payload)
		}
		if strings.Contains(payload, wantText) {
			foundText = true
		}
		if index > 0 && events[index-1].Sequence >= event.Sequence {
			t.Fatalf("response event sequence %d at index %d follows %d; want strict order", event.Sequence, index, events[index-1].Sequence)
		}
	}
	if !foundText {
		t.Fatalf("response events contain no final output %q", wantText)
	}
}

func assertAgyFactoryEventOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	assertAgyFactoryEventOrderForSession(t, "", events)
}

func assertAgyFactoryEventOrderForSession(
	t *testing.T,
	expectedSessionID string,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	indexes := map[factoryapi.FactoryEventType]int{
		factoryapi.FactoryEventTypeWorkRequest:      -1,
		factoryapi.FactoryEventTypeDispatchRequest:  -1,
		factoryapi.FactoryEventTypeDispatchResponse: -1,
	}
	for index, event := range events {
		if event.Context.SessionId != nil {
			if strings.TrimSpace(*event.Context.SessionId) == "" {
				t.Fatalf("Factory Event %q has empty session identity", event.Id)
			}
			if expectedSessionID != "" && *event.Context.SessionId != expectedSessionID {
				t.Fatalf("Factory Event %q session = %q, want %q", event.Id, *event.Context.SessionId, expectedSessionID)
			}
		}
		if _, wanted := indexes[event.Type]; wanted && event.Context.SessionId == nil {
			t.Fatalf("Factory Event %q type %q has no session identity", event.Id, event.Type)
		}
		if _, wanted := indexes[event.Type]; wanted && indexes[event.Type] == -1 {
			indexes[event.Type] = index
		}
	}
	if indexes[factoryapi.FactoryEventTypeWorkRequest] == -1 ||
		indexes[factoryapi.FactoryEventTypeDispatchRequest] == -1 ||
		indexes[factoryapi.FactoryEventTypeDispatchResponse] == -1 {
		t.Fatalf("Factory Event order missing Work Request or dispatch lifecycle: %#v", indexes)
	}
	if !(indexes[factoryapi.FactoryEventTypeWorkRequest] < indexes[factoryapi.FactoryEventTypeDispatchRequest] &&
		indexes[factoryapi.FactoryEventTypeDispatchRequest] < indexes[factoryapi.FactoryEventTypeDispatchResponse]) {
		t.Fatalf("Factory Event order = %#v, want Work Request < dispatch request < dispatch response", indexes)
	}
}

func assertAgySingleDispatch(t *testing.T, events []factoryapi.FactoryEvent, want factoryapi.WorkOutcome) {
	t.Helper()
	observations := support.ObserveDispatchEvents(t, events)
	if len(observations) != 1 || observations[0].Response == nil {
		t.Fatalf("dispatch observations = %#v, want one response", observations)
	}
	if observations[0].Response.Outcome != want {
		t.Fatalf("dispatch outcome = %q, want %q", observations[0].Response.Outcome, want)
	}
}

func assertAgySingleDispatchOutput(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantOutcome factoryapi.WorkOutcome,
	wantOutput string,
) {
	t.Helper()
	observations := support.ObserveDispatchEvents(t, events)
	if len(observations) != 1 || observations[0].Response == nil {
		t.Fatalf("dispatch observations = %#v, want one response", observations)
	}
	response := observations[0].Response
	if response.Outcome != wantOutcome || response.Output == nil || *response.Output != wantOutput {
		t.Fatalf("dispatch response = %#v, want outcome %q and output %q", response, wantOutcome, wantOutput)
	}
}

type agyFactoryEventIDs struct {
	requestID  string
	workID     string
	dispatchID string
}

func readAgyFactoryEventIdentity(t *testing.T, events []factoryapi.FactoryEvent) agyFactoryEventIDs {
	t.Helper()
	var identity agyFactoryEventIDs
	for _, event := range events {
		if identity.requestID == "" && event.Context.RequestId != nil {
			identity.requestID = *event.Context.RequestId
		}
		if identity.workID == "" && event.Context.WorkIds != nil && len(*event.Context.WorkIds) > 0 {
			identity.workID = (*event.Context.WorkIds)[0]
		}
		if identity.dispatchID == "" && event.Context.DispatchId != nil {
			identity.dispatchID = *event.Context.DispatchId
		}
	}
	if identity.requestID == "" || identity.workID == "" || identity.dispatchID == "" {
		t.Fatalf("Factory Event identity = %#v, want request, Work and dispatch identities", identity)
	}
	return identity
}

func assertAgyInvocationIdentitiesDistinct(
	t *testing.T,
	first, second agyFactoryEventIDs,
) {
	t.Helper()
	if first.requestID == second.requestID || first.workID == second.workID || first.dispatchID == second.dispatchID {
		t.Fatalf("recovery identities crossed: first=%#v second=%#v", first, second)
	}
}
