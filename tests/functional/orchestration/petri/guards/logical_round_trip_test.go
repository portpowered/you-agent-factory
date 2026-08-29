package guards

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestLogicalRoundTripFactoryBoundaryProvesProductiveRejectionsSurvive proves
// the customer runtime accepts a productive process/review lane beyond the
// former aggregate raw-visit boundary while keeping the logical budget.
// CASE-G-001 and CASE-G-005 cover productive rejection cycles and the
// configured maximum visit boundary.
func TestLogicalRoundTripFactoryBoundaryProvesProductiveRejectionsSurvive(t *testing.T) {
	dir := newSharedGuardScenario(t, logicalRoundTripFactoryConfig(8, 16))

	const traceID = "trace-logical-round-trip-productive"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"logical round-trip productive lane"}`),
	})

	responses := make([]platformprocess.CommandResult, 0, 16)
	for cycle := 1; cycle <= 7; cycle++ {
		responses = append(responses,
			codexCommandResult("HEAD head-"+string(rune('a'+cycle-1))+"\nDone. COMPLETE"),
			codexCommandResult("HEAD head-"+string(rune('a'+cycle-1))+"\nneeds more work"),
		)
	}
	responses = append(responses,
		codexCommandResult("HEAD head-final\nDone. COMPLETE"),
		codexCommandResult("HEAD head-final\nDone. COMPLETE"),
	)
	route := sharedGuardProviderSequence(sharedGuardCommandResponsesFromResults(responses)...)
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: route})
	supportWaitForGuardTerminal(t, session)
	publicSession, listed, events := readSharedGuardSession(t, session)

	processIndexes := dispatchResponseIndexesForTransition(t, events, "process")
	reviewIndexes := dispatchResponseIndexesForTransition(t, events, "review")
	if len(processIndexes) != 8 || len(reviewIndexes) != 8 {
		t.Fatalf("process/review dispatch counts = %d/%d, want 8/8; events=%#v", len(processIndexes), len(reviewIndexes), events)
	}
	if rawVisits := len(processIndexes) + len(reviewIndexes); rawVisits <= 12 {
		t.Fatalf("raw process/review visits = %d, want more than former 12-visit boundary", rawVisits)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "complete")); got != 1 {
		t.Fatalf("complete work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "failed")); got != 0 {
		t.Fatalf("failed work count = %d, want 0; listed=%#v", got, listed)
	}
	if got := dispatchResponseIndexesForTransition(t, events, "executor-loop-breaker"); len(got) != 0 {
		t.Fatalf("executor-loop-breaker dispatch count = %d, want 0", len(got))
	}
	if got := dispatchResponseIndexesForTransition(t, events, "review-loop-breaker"); len(got) != 0 {
		t.Fatalf("review-loop-breaker dispatch count = %d, want 0", len(got))
	}
	assertQuiescentSession(t, publicSession, 1, 0)
	assertTerminalWorkCorrelatesToTraceID(t, listed, traceID)
	if got := session.fixture.router.providerCallsFor(dir); got != 16 {
		t.Fatalf("provider command calls = %d, want 16", got)
	}
}

// TestLogicalRoundTripFactoryBoundaryStopsUnbalancedRoute proves that a
// route that never completes a pair reaches the configured raw backstop.
// CASE-G-002 and CASE-G-005 cover the false guard and minimum/backstop
// boundary without an off-by-one failure route.
func TestLogicalRoundTripFactoryBoundaryStopsUnbalancedRoute(t *testing.T) {
	dir := newSharedGuardScenario(t, logicalRoundTripFactoryConfig(3, 4))

	const traceID = "trace-logical-round-trip-backstop"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"logical round-trip raw backstop"}`),
	})
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardProviderSequence(
		sharedGuardProviderOutput("HEAD unchanged\nDone. COMPLETE"),
		sharedGuardProviderOutput("HEAD unchanged\nneeds more work"),
		sharedGuardProviderOutput("HEAD unchanged\nDone. COMPLETE"),
		sharedGuardProviderOutput("HEAD unchanged\nneeds more work"),
	)})
	supportWaitForGuardTerminal(t, session)
	publicSession, listed, events := readSharedGuardSession(t, session)

	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "failed")); got != 1 {
		t.Fatalf("failed work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "complete")); got != 0 {
		t.Fatalf("complete work count = %d, want 0; listed=%#v", got, listed)
	}
	if got := dispatchResponseIndexesForTransition(t, events, "executor-loop-breaker"); len(got) != 1 {
		t.Fatalf("executor-loop-breaker dispatch count = %d, want 1", len(got))
	}
	if got := dispatchResponseIndexesForTransition(t, events, "review-loop-breaker"); len(got) != 0 {
		t.Fatalf("review-loop-breaker dispatch count = %d, want 0", len(got))
	}
	assertQuiescentSession(t, publicSession, 0, 1)
	assertTerminalWorkCorrelatesToTraceID(t, listed, traceID)
	if got := session.fixture.router.providerCallsFor(dir); got != 4 {
		t.Fatalf("provider command calls = %d, want 4", got)
	}
}

// TestLogicalRoundTripFactoryBoundaryRecordReplayPreservesTerminalProjection proves
// a recorded logical round trip across the Factory boundary preserves its
// terminal projection through replay.
// CASE-G-016 covers retained canonical history recovery without another Worker call.
func TestLogicalRoundTripFactoryBoundaryRecordReplayPreservesTerminalProjection(t *testing.T) {
	dir := newSharedGuardScenario(t, logicalRoundTripFactoryConfig(3, 6))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    "trace-logical-round-trip-replay",
		Payload:    []byte(`{"title":"logical round-trip replay"}`),
	})

	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardProviderSequence(
		sharedGuardProviderOutput("HEAD stable\nDone. COMPLETE"),
		sharedGuardProviderOutput("HEAD stable\nneeds more work"),
		sharedGuardProviderOutput("HEAD stable\nDone. COMPLETE"),
		sharedGuardProviderOutput("HEAD stable\nneeds more work"),
		sharedGuardProviderOutput("HEAD stable\nDone. COMPLETE"),
		sharedGuardProviderOutput("HEAD stable\nneeds more work"),
	)})
	supportWaitForGuardTerminal(t, session)
	_, liveWork, liveEvents := readSharedGuardSession(t, session)

	if got := support.CountWorkAtCustomerState(liveWork, support.WorkCustomerLocation("story", "failed")); got != 1 {
		t.Fatalf("live failed work count = %d, want 1; listed=%#v", got, liveWork)
	}
	if got := session.fixture.router.providerCallsFor(dir); got != 6 {
		t.Fatalf("live provider command calls = %d, want 6", got)
	}

	if len(liveEvents) < 2 {
		t.Fatalf("live logical round-trip event history = %d, want replayable history", len(liveEvents))
	}
	replayedTail := support.GetFactoryEventsAfterForSessionAt(
		t,
		session.fixture.baseURL,
		session.sessionID,
		support.FactoryEventReadCursor{AfterEventID: liveEvents[0].Id},
	)
	if len(replayedTail) != len(liveEvents)-1 {
		t.Fatalf("replayed event tail length = %d, want %d", len(replayedTail), len(liveEvents)-1)
	}
	for index, event := range replayedTail {
		if event.Id != liveEvents[index+1].Id || event.Context.Sequence != liveEvents[index+1].Context.Sequence {
			t.Fatalf("replayed event[%d] = %#v, want live event %#v", index+1, event, liveEvents[index+1])
		}
	}
	_, replayedWork, replayedEvents := readSharedGuardSession(t, session)

	if got := support.CountWorkAtCustomerState(replayedWork, support.WorkCustomerLocation("story", "failed")); got != 1 {
		t.Fatalf("replayed failed work count = %d, want 1; listed=%#v", got, replayedWork)
	}
	if got := support.CountWorkAtCustomerState(replayedWork, support.WorkCustomerLocation("story", "complete")); got != 0 {
		t.Fatalf("replayed complete work count = %d, want 0; listed=%#v", got, replayedWork)
	}
	for _, transitionID := range []string{"process", "review"} {
		liveVisits := len(dispatchResponseIndexesForTransition(t, liveEvents, transitionID))
		replayedVisits := len(dispatchResponseIndexesForTransition(t, replayedEvents, transitionID))
		if replayedVisits != liveVisits || liveVisits != 3 {
			t.Fatalf("%s replay visits = %d, live visits = %d; want three each", transitionID, replayedVisits, liveVisits)
		}
	}
	if got := session.fixture.router.providerCallsFor(dir); got != 6 {
		t.Fatalf("provider command calls after replay = %d, want unchanged 6", got)
	}
}

func sharedGuardCommandResponsesFromResults(results []platformprocess.CommandResult) []sharedGuardCommandResponse {
	responses := make([]sharedGuardCommandResponse, 0, len(results))
	for _, result := range results {
		responses = append(responses, sharedGuardCommandResponse{result: result})
	}
	return responses
}

func logicalRoundTripFactoryConfig(maxVisits, maxRawVisits int) map[string]any {
	logicalRoundTrip := map[string]any{
		"maxRawVisits": float64(maxRawVisits),
		"workstations": []string{"process", "review"},
	}
	return map[string]any{
		"name": "logical-round-trip-boundary",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "in-review", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{
			{"name": "executor"},
			{"name": "reviewer"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "story", "state": "in-review"}},
				"onFailure": []map[string]string{{"workType": "story", "state": "failed"}},
			},
			{
				"name":        "review",
				"worker":      "reviewer",
				"inputs":      []map[string]string{{"workType": "story", "state": "in-review"}},
				"outputs":     []map[string]string{{"workType": "story", "state": "complete"}},
				"onRejection": []map[string]string{{"workType": "story", "state": "init"}},
				"onFailure":   []map[string]string{{"workType": "story", "state": "failed"}},
			},
			{
				"name":   "executor-loop-breaker",
				"type":   "LOGICAL_MOVE",
				"inputs": []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{
					{"workType": "story", "state": "failed"},
				},
				"guards": []map[string]any{{
					"type":             "VISIT_COUNT",
					"workstation":      "process",
					"maxVisits":        float64(maxVisits),
					"logicalRoundTrip": logicalRoundTrip,
				}},
			},
			{
				"name":   "review-loop-breaker",
				"type":   "LOGICAL_MOVE",
				"inputs": []map[string]string{{"workType": "story", "state": "in-review"}},
				"outputs": []map[string]string{
					{"workType": "story", "state": "failed"},
				},
				"guards": []map[string]any{{
					"type":             "VISIT_COUNT",
					"workstation":      "review",
					"maxVisits":        float64(maxVisits),
					"logicalRoundTrip": logicalRoundTrip,
				}},
			},
		},
	}
}

func codexCommandResult(stdout string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(stdout)}
}

func dispatchResponseIndexesForTransition(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	transitionID string,
) []int {
	t.Helper()

	var indexes []int
	for index, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.TransitionId == transitionID {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func assertTerminalWorkCorrelatesToTraceID(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	traceID string,
) {
	t.Helper()

	for _, item := range listed.Results {
		if item.State == nil {
			continue
		}
		switch item.State.Name {
		case "complete", "failed":
		default:
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Fatalf("%s work trace ID = %#v, want %q", item.State.Name, item.TraceId, traceID)
		}
		return
	}
	t.Fatalf("listed work missing terminal story outcome for trace %q", traceID)
}

func assertQuiescentSession(t *testing.T, session factoryapi.FactorySession, wantTerminal, wantFailed int) {
	t.Helper()
	categories := session.Runtime.Progress.Categories
	if categories.Initial != 0 || categories.Processing != 0 {
		t.Errorf(
			"session still has in-progress Work: initial=%d processing=%d",
			categories.Initial,
			categories.Processing,
		)
	}
	if categories.Terminal != wantTerminal {
		t.Errorf("session terminal count = %d, want %d", categories.Terminal, wantTerminal)
	}
	if categories.Failed != wantFailed {
		t.Errorf("session failed count = %d, want %d", categories.Failed, wantFailed)
	}
}
