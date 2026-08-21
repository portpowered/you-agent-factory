package guards

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestLogicalRoundTripFactoryBoundaryProvesProductiveRejectionsSurvive proves
// the customer runtime accepts a productive process/review lane beyond the
// former aggregate raw-visit boundary while keeping the logical budget.
func TestLogicalRoundTripFactoryBoundaryProvesProductiveRejectionsSurvive(t *testing.T) {
	dir := support.ScaffoldFactory(t, logicalRoundTripFactoryConfig(8, 16))
	support.WriteAgentConfig(t, dir, "executor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "reviewer", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteWorkstationConfig(t, dir, "executor-loop-breaker", "---\ntype: LOGICAL_MOVE\n---\n")
	support.WriteWorkstationConfig(t, dir, "review-loop-breaker", "---\ntype: LOGICAL_MOVE\n---\n")

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
	runner := support.NewShapedProviderCommandRunner(responses...)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

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
	assertQuiescentSession(t, session, 1, 0)
	assertTerminalWorkCorrelatesToTraceID(t, listed, traceID)
	if runner.CallCount() != 16 {
		t.Fatalf("provider command calls = %d, want 16", runner.CallCount())
	}
}

// TestLogicalRoundTripFactoryBoundaryStopsUnbalancedRoute proves that a
// route that never completes a pair reaches the configured raw backstop.
func TestLogicalRoundTripFactoryBoundaryStopsUnbalancedRoute(t *testing.T) {
	dir := support.ScaffoldFactory(t, logicalRoundTripFactoryConfig(3, 4))
	support.WriteAgentConfig(t, dir, "executor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "reviewer", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteWorkstationConfig(t, dir, "executor-loop-breaker", "---\ntype: LOGICAL_MOVE\n---\n")
	support.WriteWorkstationConfig(t, dir, "review-loop-breaker", "---\ntype: LOGICAL_MOVE\n---\n")

	const traceID = "trace-logical-round-trip-backstop"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"logical round-trip raw backstop"}`),
	})
	runner := support.NewShapedProviderCommandRunner(
		codexCommandResult("HEAD unchanged\nDone. COMPLETE"),
		codexCommandResult("HEAD unchanged\nneeds more work"),
		codexCommandResult("HEAD unchanged\nDone. COMPLETE"),
		codexCommandResult("HEAD unchanged\nneeds more work"),
	)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

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
	assertQuiescentSession(t, session, 0, 1)
	assertTerminalWorkCorrelatesToTraceID(t, listed, traceID)
	if runner.CallCount() != 4 {
		t.Fatalf("provider command calls = %d, want 4", runner.CallCount())
	}
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
