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

// TestPetriParentOrSameNameGuardReleasesExpectedWork proves a SAME_NAME input
// guard dispatches only when peer Work names correlate, releasing the expected
// matched Work to its public terminal state while mismatched peers remain idle
// without the guarded workstation's success outcome.
func TestPetriParentOrSameNameGuardReleasesExpectedWork(t *testing.T) {
	t.Run("matching peer names release correlated work", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, sameNameGuardFactoryConfig())
		support.WriteAgentConfig(t, dir, "matcher", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

		const (
			matchedPlanWorkID = "plan-alpha"
			matchedTaskWorkID = "task-alpha"
			matchedName       = "alpha"
			matchTraceID      = "trace-same-name-match"
		)
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			Name:       matchedName,
			WorkID:     matchedPlanWorkID,
			WorkTypeID: "plan",
			TraceID:    matchTraceID,
			Payload:    []byte(`{"role":"plan"}`),
		})
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			Name:       matchedName,
			WorkID:     matchedTaskWorkID,
			WorkTypeID: "task",
			TraceID:    matchTraceID,
			Payload:    []byte(`{"role":"task"}`),
		})

		runner := support.NewShapedProviderCommandRunner(
			codexCommandResult("Done. COMPLETE"),
		)
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			15*time.Second,
		)

		matchIndexes := dispatchResponseIndexesForTransition(t, events, "match-items")
		if len(matchIndexes) != 1 {
			t.Fatalf("match-items dispatch count = %d, want 1 for correlated peer names", len(matchIndexes))
		}
		if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "matched")); got != 1 {
			t.Fatalf("matched task count = %d, want 1; listed=%#v", got, listed)
		}
		if !support.HasWorkAtCustomerState(listed, matchedTaskWorkID, support.WorkCustomerLocation("task", "matched")) {
			t.Fatalf("task %q missing public matched outcome; listed=%#v", matchedTaskWorkID, listed)
		}
		if support.HasWorkAtCustomerState(listed, matchedTaskWorkID, support.WorkCustomerLocation("task", "ready")) {
			t.Fatalf("task %q still at ready after SAME_NAME guard released correlated work", matchedTaskWorkID)
		}
		assertQuiescentSession(t, session, 1, 0)
		if runner.CallCount() != 1 {
			t.Fatalf("provider command calls = %d, want 1 for the correlated match-items dispatch", runner.CallCount())
		}
	})

	t.Run("mismatched peer names keep guarded workstation idle", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, sameNameGuardFactoryConfig())
		support.WriteAgentConfig(t, dir, "matcher", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

		const (
			mismatchedPlanWorkID = "plan-beta"
			mismatchedTaskWorkID = "task-gamma"
			mismatchTraceID      = "trace-same-name-mismatch"
		)
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			Name:       "beta",
			WorkID:     mismatchedPlanWorkID,
			WorkTypeID: "plan",
			TraceID:    mismatchTraceID,
			Payload:    []byte(`{"role":"plan"}`),
		})
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			Name:       "gamma",
			WorkID:     mismatchedTaskWorkID,
			WorkTypeID: "task",
			TraceID:    mismatchTraceID,
			Payload:    []byte(`{"role":"task"}`),
		})

		runner := support.NewShapedProviderCommandRunner(
			codexCommandResult("Done. COMPLETE"),
		)
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir: dir,
			Edges: serviceedges.Edges{
				ProviderCommandRunner: runner,
			},
		})
		defer server.Stop(t)

		baseURL := server.URL()
		waitForMinimumWorkAtCustomerState(
			t,
			baseURL,
			support.WorkCustomerLocation("plan", "ready"),
			1,
			10*time.Second,
		)
		waitForMinimumWorkAtCustomerState(
			t,
			baseURL,
			support.WorkCustomerLocation("task", "ready"),
			1,
			10*time.Second,
		)
		waitForBlockedSameNameGuardObservation(t, baseURL, 10*time.Second)

		listed := support.ListDefaultSessionWork(t, baseURL)
		if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "matched")); got != 0 {
			t.Fatalf("matched task count = %d, want 0 for mismatched peer names; listed=%#v", got, listed)
		}
		if !support.HasWorkAtCustomerState(listed, mismatchedTaskWorkID, support.WorkCustomerLocation("task", "ready")) {
			t.Fatalf("task %q missing public ready state; listed=%#v", mismatchedTaskWorkID, listed)
		}

		events := server.GetFactoryEvents(t)
		if indexes := dispatchResponseIndexesForTransition(t, events, "match-items"); len(indexes) != 0 {
			t.Fatalf("match-items dispatch count = %d, want 0 while peer names mismatch", len(indexes))
		}
		if runner.CallCount() != 0 {
			t.Fatalf("provider command calls = %d, want 0 while SAME_NAME guard blocks mismatched peers", runner.CallCount())
		}
	})
}

func sameNameGuardFactoryConfig() map[string]any {
	return map[string]any{
		"name": "same-name-guard-eligibility",
		"workTypes": []map[string]any{
			{
				"name": "plan",
				"states": []map[string]string{
					{"name": "ready", "type": "INITIAL"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "ready", "type": "INITIAL"},
					{"name": "matched", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "matcher"},
		},
		"workstations": []map[string]any{
			{
				"name":   "match-items",
				"worker": "matcher",
				"inputs": []map[string]any{
					{"workType": "plan", "state": "ready"},
					{
						"workType": "task",
						"state":    "ready",
						"guards": []map[string]string{
							{"type": "SAME_NAME", "matchInput": "plan"},
						},
					},
				},
				"outputs": []map[string]string{
					{"workType": "task", "state": "matched"},
				},
			},
		},
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

func waitForBlockedSameNameGuardObservation(t *testing.T, baseURL string, timeout time.Duration) {
	t.Helper()

	support.WaitForStatus(t, baseURL, timeout, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial == 2 &&
			status.Categories.Processing == 0 &&
			status.Categories.Terminal == 0
	})
}

func waitForMinimumWorkAtCustomerState(
	t *testing.T,
	baseURL string,
	location string,
	want int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		if support.CountWorkAtCustomerState(listed, location) >= want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf(
		"%s work count = %d, want at least %d within %s; listed=%#v",
		location,
		support.CountWorkAtCustomerState(listed, location),
		want,
		timeout,
		listed,
	)
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
