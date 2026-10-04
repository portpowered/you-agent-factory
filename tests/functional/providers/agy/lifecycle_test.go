package agy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAgyQuietConcurrentSuccessAndFailureKeepSeparateStreams exercises two
// explicit Factory Sessions on the reusable public process. Both commands
// reach a scenario-owned gate before either returns; captured CLI streams and
// retained Factory Events are the customer observers, not the runner ledger.
func TestAgyQuietConcurrentSuccessAndFailureKeepSeparateStreams(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	host := fixture.startRoleHost(t)
	trace := newAgySharedLifecycleTrace()
	t.Cleanup(func() { trace.log(t) })
	ctx, cancel := context.WithTimeout(t.Context(), agySharedInvocationTimeout)
	defer cancel()
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	success := startAgyQuietInvocation(t, ctx, fixture, host, "quiet-success", trace, release)
	failure := startAgyQuietInvocation(t, ctx, fixture, host, "quiet-failure", trace, release)
	if err := trace.waitForBothEntered(ctx); err != nil {
		for _, invocation := range []*agyQuietInvocation{success, failure} {
			select {
			case <-invocation.done:
				t.Logf("early quiet exit: err=%v stdout=%q stderr=%q", invocation.err, invocation.inputs.Stdout(), invocation.inputs.Stderr())
			default:
			}
		}
		t.Fatalf("quiet invocations did not overlap at their command gates: %v", err)
	}
	unblock()
	success.wait(t, ctx)
	failure.wait(t, ctx)
	assertAgyQuietSuccess(t, success)
	assertAgyQuietFailure(t, failure)
	successEvents := support.GetFactoryEventsForSessionAt(t, host.baseURL, success.sessionID)
	failureEvents := support.GetFactoryEventsForSessionAt(t, host.baseURL, failure.sessionID)
	assertAgySingleDispatch(t, successEvents, factoryapi.WorkOutcomeAccepted)
	assertAgySingleDispatch(t, failureEvents, factoryapi.WorkOutcomeFailed)
	assertAgyFactoryEventOrderForSession(t, success.sessionID, successEvents)
	assertAgyFactoryEventOrderForSession(t, failure.sessionID, failureEvents)
}

type agyQuietInvocation struct {
	inputs    *support.CapturedInputs
	sessionID string
	done      chan struct{}
	err       error
}

// Quiet presentation is invocation-scoped even when a normal or verbose peer
// is active. Each subtest owns its routes, sessions and captured streams.
func TestAgyQuietOutputIsolatedFromNormalAndVerbosePeers(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"normal", "verbose"} {
		for _, outcome := range []string{"success", "failure"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				t.Parallel()
				runAgyQuietPeerScenario(t, mode, outcome)
			})
		}
	}
}

func runAgyQuietPeerScenario(t *testing.T, mode, outcome string) {
	t.Helper()
	fixture := agySharedProcess(t)
	host := fixture.startRoleHost(t)
	trace := newAgySharedLifecycleTrace()
	t.Cleanup(func() { trace.log(t) })
	ctx, cancel := context.WithTimeout(t.Context(), agySharedInvocationTimeout)
	defer cancel()
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	prefix := "quiet-" + mode + "-" + outcome
	quiet := startAgyQuietInvocation(t, ctx, fixture, host, prefix, trace, release)
	presentation := []string{"--output", "primary"}
	if mode == "verbose" {
		presentation = append(presentation, "--verbose")
	}
	peer := startAgyQuietInvocation(t, ctx, fixture, host, prefix+"-peer", trace, release, presentation...)
	if err := trace.waitForBothEntered(ctx); err != nil {
		t.Fatalf("quiet and %s peer did not reach both command gates: %v", mode, err)
	}
	unblock()
	quiet.wait(t, ctx)
	peer.wait(t, ctx)
	assertAgyPrimaryOutput(t, peer, agyMarkedColdWatchTrace(t, prefix+"-peer"))
	assertAgyQuietPeerOutcome(t, host, quiet, prefix, outcome)
	assertAgyStreamsExclude(t, quiet, "["+prefix+"-peer]", prefix+"-private-command-diagnostic", "peer-secret-token")
	assertAgyStreamsExclude(t, peer, "quiet-secret-peer-token", "quiet-secret-peer-diagnostic", "["+prefix+"]",
		prefix+"-private-command-diagnostic", "peer-secret-token")
	// AGY print mode consumes stdout only. Even nonempty native stderr must
	// stay absent from both invocation streams, including the verbose peer.
	if got := peer.inputs.Stderr(); got != "" {
		t.Fatalf("%s primary-output peer stderr = %q, want empty", mode, got)
	}
	assertAgyQuietInvocationEvents(t, host, peer, factoryapi.WorkOutcomeAccepted)

	// A fresh request on the same process must retain quiet presentation after
	// the overlapping invocations, including a failed provider attempt.
	reuse := startAgyQuietInvocation(t, ctx, fixture, host, prefix+"-reuse", newAgySharedLifecycleTrace(), nil)
	reuse.wait(t, ctx)
	assertAgyPrimaryOutput(t, reuse, agyMarkedColdWatchTrace(t, prefix+"-reuse"))
	if got := reuse.inputs.Stderr(); got != "" {
		t.Fatalf("subsequent quiet stderr = %q, want empty", got)
	}
	assertAgyStreamsExclude(t, reuse, prefix+"-peer", "quiet-secret-peer-token", "quiet-secret-peer-diagnostic",
		prefix+"-private-command-diagnostic", "peer-secret-token")
	assertAgyQuietInvocationEvents(t, host, reuse, factoryapi.WorkOutcomeAccepted)
}

func assertAgyQuietPeerOutcome(t *testing.T, host *agySharedRoleHost, quiet *agyQuietInvocation, prefix, outcome string) {
	t.Helper()
	if outcome == "failure" {
		assertAgyQuietFailure(t, quiet)
		assertAgyQuietInvocationEvents(t, host, quiet, factoryapi.WorkOutcomeFailed)
		return
	}
	assertAgyPrimaryOutput(t, quiet, agyMarkedColdWatchTrace(t, prefix))
	if got := quiet.inputs.Stderr(); got != "" {
		t.Fatalf("quiet success stderr = %q, want empty", got)
	}
	assertAgyQuietInvocationEvents(t, host, quiet, factoryapi.WorkOutcomeAccepted)
}

func assertAgyQuietInvocationEvents(t *testing.T, host *agySharedRoleHost, invocation *agyQuietInvocation, outcome factoryapi.WorkOutcome) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, host.baseURL, invocation.sessionID)
	assertAgySingleDispatch(t, events, outcome)
	assertAgyFactoryEventOrderForSession(t, invocation.sessionID, events)
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"quiet-secret-peer-token", "quiet-secret-peer-diagnostic", "private-command-diagnostic", "peer-secret-token"} {
		if strings.Contains(string(encoded), marker) {
			t.Fatalf("session %s Factory Events leaked native diagnostic %q", invocation.sessionID, marker)
		}
	}
}

func assertAgyStreamsExclude(t *testing.T, invocation *agyQuietInvocation, markers ...string) {
	t.Helper()
	for _, marker := range markers {
		if strings.Contains(invocation.inputs.Stdout(), marker) || strings.Contains(invocation.inputs.Stderr(), marker) {
			t.Fatalf("session %s streams leaked marker %q", invocation.sessionID, marker)
		}
	}
}

func startAgyQuietInvocation(t *testing.T, ctx context.Context, fixture *agySharedProcessFixture,
	host *agySharedRoleHost, selector string, trace *agySharedLifecycleTrace, release <-chan struct{},
	presentation ...string,
) *agyQuietInvocation {
	t.Helper()
	route := fixture.routes[selector]
	route.setRelease(release)
	if err := trace.expectRoute(selector); err != nil {
		t.Fatal(err)
	}
	if err := route.bindLifecycleTrace("", trace); err != nil {
		t.Fatal(err)
	}
	opened := support.OpenFactorySessionAt(t, host.baseURL, host.factories[route.factoryName])
	id := opened.Session.Id
	if err := fixture.runner.registerScope(id, route); err != nil {
		t.Fatal(err)
	}
	if err := route.setLifecycleTraceScope(id, trace); err != nil {
		t.Fatal(err)
	}
	invocationContext, cancel := context.WithCancel(ctx)
	args := []string{
		"you", "--remote", "--server", host.baseURL, "run", "--session", id,
		"--named", route.factoryName, "--cut-path", route.assetPath,
	}
	if len(presentation) == 0 {
		presentation = []string{"--quiet"}
	}
	inputs := support.FakeInputs(invocationContext, append(args, presentation...))
	inputs.Input.Env = agySharedEnvironment(host.homeDir)
	inputs.Input.WorkingDirectory = route.workDir
	invocation := &agyQuietInvocation{inputs: inputs, sessionID: id, done: make(chan struct{})}
	t.Cleanup(func() {
		cancel()
		// Failure ceiling only: cancellation should promptly join the invocation.
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), agySharedInvocationTimeout)
		defer cleanupCancel()
		invocation.wait(t, cleanupContext)
		fixture.runner.unregisterScope(id, route)
		support.CloseFactorySessionAt(t, host.baseURL, id)
		route.unbindLifecycleTrace(trace)
		route.setRelease(nil)
	})
	go func() {
		defer close(invocation.done)
		invocation.err = fixture.process.Execute(inputs.Input)
	}()
	return invocation
}

func (invocation *agyQuietInvocation) wait(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-invocation.done:
	case <-ctx.Done():
		t.Fatalf("quiet invocation did not join: %v", ctx.Err())
	}
}

func assertAgyQuietSuccess(t *testing.T, invocation *agyQuietInvocation) {
	t.Helper()
	assertAgyPrimaryOutput(t, invocation, agyColdWatchCompleteReportTrace(t))
	if got := invocation.inputs.Stderr(); got != "" {
		t.Fatalf("quiet success stderr = %q, want empty", got)
	}
}

func assertAgyPrimaryOutput(t *testing.T, invocation *agyQuietInvocation, expectedTrace []byte) {
	t.Helper()
	if invocation.err != nil {
		t.Fatalf("primary-output invocation failed: %v", invocation.err)
	}
	var trace struct {
		Result struct {
			Response string `json:"response"`
		} `json:"result"`
	}
	if err := json.Unmarshal(expectedTrace, &trace); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(invocation.inputs.Stdout()); got != trace.Result.Response {
		t.Fatalf("quiet stdout = %q, want raw primary result %q", got, trace.Result.Response)
	}
}

func assertAgyQuietFailure(t *testing.T, invocation *agyQuietInvocation) {
	t.Helper()
	if invocation.err == nil {
		t.Fatal("quiet native failure succeeded")
	}
	if got := invocation.inputs.Stdout(); got != "" {
		t.Fatalf("quiet failure stdout = %q, want empty", got)
	}
	var response factoryapi.ErrorResponse
	// Unmarshal rejects trailing output, including a second response or logs.
	if err := json.Unmarshal([]byte(invocation.inputs.Stderr()), &response); err != nil {
		t.Fatalf("quiet failure stderr is not one ErrorResponse: %v; stderr=%q", err, invocation.inputs.Stderr())
	}
	if string(response.Code) != "INVOCATION_RUNTIME_FAILURE" || string(response.Family) != "INTERNAL_SERVER_ERROR" || response.Message == "" {
		t.Fatalf("quiet failure ErrorResponse = %#v", response)
	}
	for _, secret := range []string{"quiet-secret-peer-token", "quiet-secret-peer-diagnostic", "Recommendation: pass"} {
		if strings.Contains(invocation.inputs.Stderr(), secret) {
			t.Fatalf("quiet failure leaked %q", secret)
		}
	}
}

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
	trace := newAgySharedLifecycleTrace()
	t.Cleanup(func() { trace.log(t) })
	scenarioContext, cancel := context.WithTimeout(context.Background(), agySharedInvocationTimeout)
	t.Cleanup(cancel)
	fixture := agySharedProcess(t)
	trace.record("host-start-request", "", "", "shared role host")
	host := fixture.startRoleHost(t)
	trace.record("host-ready", "", "", fmt.Sprintf("baseURL=%q homeDir=%q", host.baseURL, host.homeDir))
	firstRoute := fixture.routes["concurrency-a"]
	secondRoute := fixture.routes["concurrency-b"]
	release := make(chan struct{})
	firstRoute.setRelease(release)
	secondRoute.setRelease(release)
	var releaseOnce sync.Once
	releaseGate := func(stage, detail string) {
		releaseOnce.Do(func() {
			close(release)
			trace.record(stage, "", "", detail)
		})
	}
	// Register release cleanup before either session setup can enter the held
	// provider edge. A partial setup failure must still unblock a peer route.
	t.Cleanup(func() {
		releaseGate("release-forced", "test cleanup closed the shared release gate")
		firstRoute.setRelease(nil)
		secondRoute.setRelease(nil)
	})
	firstCallStart := firstRoute.callCount()
	secondCallStart := secondRoute.callCount()
	firstSessionID, firstStream := fixture.openConcurrentRoute(t, host, firstRoute, trace)
	secondSessionID, secondStream := fixture.openConcurrentRoute(t, host, secondRoute, trace)
	trace.record(
		"rendezvous-wait",
		"",
		fmt.Sprintf("%s,%s", firstSessionID, secondSessionID),
		"scenario-owned provider-entry gates for both expected routes",
	)
	if err := trace.waitForBothEntered(scenarioContext); err != nil {
		t.Fatalf("wait for concurrent AGY route entries: %v", err)
	}
	traceActive, traceMaxActive := trace.activeCounts()
	traceEntries := trace.entryCounts()
	trace.record(
		"rendezvous-observed",
		"",
		fmt.Sprintf("%s,%s", firstSessionID, secondSessionID),
		fmt.Sprintf(
			"aggregateRequests=%d aggregateActive=%d aggregateMaxActive=%d scenarioActive=%d scenarioMaxActive=%d scenarioEntries=%d,%d routeCalls=%d,%d",
			fixture.runner.callCount(),
			fixture.runner.activeCallCount(),
			fixture.runner.maxActiveCallCount(),
			traceActive,
			traceMaxActive,
			traceEntries[firstRoute.selector],
			traceEntries[secondRoute.selector],
			firstRoute.callCount()-firstCallStart,
			secondRoute.callCount()-secondCallStart,
		),
	)
	if traceActive != 2 {
		t.Fatalf("scenario active AGY calls while routes are held = %d, want 2", traceActive)
	}
	if traceMaxActive < 2 {
		t.Fatalf("scenario maximum active AGY calls = %d, want overlap of both routes", traceMaxActive)
	}
	if traceEntries[firstRoute.selector] != 1 || traceEntries[secondRoute.selector] != 1 {
		t.Fatalf(
			"scenario provider entries = %d,%d, want exactly one per route",
			traceEntries[firstRoute.selector], traceEntries[secondRoute.selector],
		)
	}
	if got := firstRoute.callCount() - firstCallStart; got != 1 {
		t.Fatalf("route %q provider calls = %d, want exactly one", firstRoute.selector, got)
	}
	if got := secondRoute.callCount() - secondCallStart; got != 1 {
		t.Fatalf("route %q provider calls = %d, want exactly one", secondRoute.selector, got)
	}
	releaseGate("released", fmt.Sprintf("shared release gate closed for %s,%s", firstSessionID, secondSessionID))
	firstSession, firstListed, firstEvents, firstResponseEvents := fixture.observeHostedSession(
		t, host.baseURL, firstSessionID, firstStream,
	)
	trace.record("terminal-observed", firstRoute.selector, firstSessionID, "session/work/events/response events collected")
	secondSession, secondListed, secondEvents, secondResponseEvents := fixture.observeHostedSession(
		t, host.baseURL, secondSessionID, secondStream,
	)
	trace.record("terminal-observed", secondRoute.selector, secondSessionID, "session/work/events/response events collected")
	if firstSession.Id == secondSession.Id {
		t.Fatalf("concurrent Factory Session IDs are identical: %q", firstSession.Id)
	}
	assertAgyInvocationIdentitiesDistinct(
		t,
		readAgyFactoryEventIdentity(t, firstEvents),
		readAgyFactoryEventIdentity(t, secondEvents),
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
