package guards

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPetriAuthoredEligibilityGuardBlocksDispatchUntilSatisfied proves a
// workstation-level VISIT_COUNT eligibility guard keeps the guarded
// completion workstation from dispatching until the watched workstation has
// visited the shared Work enough times, then releases the expected public
// terminal Work outcome after the guard becomes satisfied.
func TestPetriAuthoredEligibilityGuardBlocksDispatchUntilSatisfied(t *testing.T) {
	dir := support.ScaffoldFactory(t, visitGuardedCompletionFactoryConfig())
	support.WriteAgentConfig(t, dir, "executor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "loop-reviewer", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "gate-reviewer", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	traceID := "trace-visit-guard-eligibility"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"visit-guard eligibility proof"}`),
	})

	runner := support.NewShapedProviderCommandRunner(
		codexCommandResult("Done. COMPLETE"),
		codexCommandResult("needs more work"),
		codexCommandResult("Done. COMPLETE"),
		codexCommandResult("Done. COMPLETE"),
	)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	executeIndexes := dispatchResponseIndexesForTransition(t, events, "execute-story")
	secondPassIndexes := dispatchResponseIndexesForTransition(t, events, "second-pass-review")
	reviewIndexes := dispatchResponseIndexesForTransition(t, events, "review-story")
	if len(executeIndexes) < 2 {
		t.Fatalf("execute-story dispatch count = %d, want at least 2 before guarded completion", len(executeIndexes))
	}
	if len(reviewIndexes) < 2 {
		t.Fatalf("review-story dispatch count = %d, want at least 2 while the guard blocks the gated workstation", len(reviewIndexes))
	}
	for _, index := range secondPassIndexes {
		if index < executeIndexes[1] {
			t.Fatalf(
				"second-pass-review dispatch index = %d, want after second execute-story dispatch index %d while the guard is unsatisfied",
				index,
				executeIndexes[1],
			)
		}
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "complete")); got != 1 {
		t.Fatalf("complete work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "init")); got != 0 {
		t.Fatalf("init work count after completion = %d, want 0", got)
	}
	assertTerminalWorkCorrelatesToTraceID(t, listed, traceID)
	assertQuiescentSession(t, session, 1, 0)
	if runner.CallCount() != 4 {
		t.Fatalf("provider command calls = %d, want 4 (execute, review reject, execute, review complete)", runner.CallCount())
	}
}

func visitGuardedCompletionFactoryConfig() map[string]any {
	return map[string]any{
		"name": "visit-guard-eligibility",
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
			{"name": "loop-reviewer"},
			{"name": "gate-reviewer"},
		},
		"workstations": []map[string]any{
			{
				"name":      "execute-story",
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "story", "state": "in-review"}},
				"onFailure": []map[string]string{{"workType": "story", "state": "failed"}},
			},
			{
				"name":        "review-story",
				"worker":      "loop-reviewer",
				"inputs":      []map[string]string{{"workType": "story", "state": "in-review"}},
				"onFailure":   []map[string]string{{"workType": "story", "state": "failed"}},
				"onRejection": []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":     []map[string]string{{"workType": "story", "state": "complete"}},
			},
			{
				"name":   "second-pass-review",
				"worker": "gate-reviewer",
				"inputs": []map[string]string{{"workType": "story", "state": "in-review"}},
				"outputs": []map[string]string{
					{"workType": "story", "state": "complete"},
				},
				"onFailure": []map[string]string{{"workType": "story", "state": "failed"}},
				"guards": []map[string]any{{
					"type":        "VISIT_COUNT",
					"workstation": "execute-story",
					"maxVisits":   float64(2),
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
		if item.State == nil || item.State.Name != "complete" {
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Fatalf("complete work trace ID = %#v, want %q", item.TraceId, traceID)
		}
		return
	}
	t.Fatalf("listed work missing story:complete for trace %q", traceID)
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
