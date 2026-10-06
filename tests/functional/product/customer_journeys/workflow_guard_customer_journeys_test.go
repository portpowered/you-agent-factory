package customer_journeys_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWorkflowEligibilityGuardBlocksDispatchUntilSatisfied proves a
// workstation-level VISIT_COUNT eligibility guard keeps the guarded
// completion workstation from dispatching until the watched workstation has
// visited the shared Work enough times, then releases the expected public
// terminal Work outcome through the guarded second-pass-review dispatch.
func runWorkflowGuardJourneys(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	t.Run("EligibilityGuardReleasesAfterRequiredVisits", func(t *testing.T) { runWorkflowEligibilityGuard(t, host) })
	t.Run("MatchingPeerNamesReleaseCorrelatedWork", func(t *testing.T) { runWorkflowParentOrSameNameGuardReleasesExpectedWorkCase0(t, host) })
	t.Run("MismatchedPeerNamesRemainIdle", func(t *testing.T) { runWorkflowParentOrSameNameGuardReleasesExpectedWorkCase1(t, host) })
	t.Run("VisitLimitRoutesWorkToFailed", func(t *testing.T) { runWorkflowVisitOrMatchGuardFailureIsVisibleInPublicWorkStateCase0(t, host) })
	t.Run("MismatchedFieldsRemainIdle", func(t *testing.T) { runWorkflowVisitOrMatchGuardFailureIsVisibleInPublicWorkStateCase1(t, host) })
}

func runWorkflowEligibilityGuard(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, visitGuardedCompletionFactoryConfig())
	support.WriteWorkstationConfig(t, dir, "advance-to-gate", "---\ntype: LOGICAL_MOVE\n---\n")
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

	session, listed, events := host.Run(t, dir, runner, 15*time.Second)

	assertEligibilityDispatchOrder(t, events)
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "complete")); got != 1 {
		t.Fatalf("complete work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("story", "init")); got != 0 {
		t.Fatalf("init work count after completion = %d, want 0", got)
	}
	assertTerminalWorkCorrelatesToTraceID(t, listed, traceID)
	assertGuardSessionQuiescent(t, session, 1, 0)
	if runner.CallCount() != 4 {
		t.Fatalf("provider command calls = %d, want 4 (execute, review reject, execute, guarded second-pass-review)", runner.CallCount())
	}
}

func assertEligibilityDispatchOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	executeIndexes := dispatchResponseIndexesForTransition(t, events, "execute-story")
	secondPassIndexes := dispatchResponseIndexesForTransition(t, events, "second-pass-review")
	reviewIndexes := dispatchResponseIndexesForTransition(t, events, "review-story")
	advanceIndexes := dispatchResponseIndexesForTransition(t, events, "advance-to-gate")
	if len(executeIndexes) < 2 {
		t.Fatalf("execute-story dispatch count = %d, want at least 2 before guarded completion", len(executeIndexes))
	}
	if len(reviewIndexes) != 1 {
		t.Fatalf("review-story dispatch count = %d, want 1 while the guard blocks the gated workstation", len(reviewIndexes))
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
	if len(secondPassIndexes) < 1 {
		t.Fatalf("second-pass-review dispatch count = %d, want at least 1 after the guard is satisfied", len(secondPassIndexes))
	}
	if len(advanceIndexes) != 1 {
		t.Fatalf("advance-to-gate dispatch count = %d, want 1 after execute-story satisfies the visit guard", len(advanceIndexes))
	}
	if advanceIndexes[0] < executeIndexes[1] {
		t.Fatalf(
			"advance-to-gate dispatch index = %d, want after second execute-story dispatch index %d",
			advanceIndexes[0],
			executeIndexes[1],
		)
	}
	if secondPassIndexes[0] <= advanceIndexes[0] {
		t.Fatalf(
			"second-pass-review dispatch index = %d, want after advance-to-gate dispatch index %d",
			secondPassIndexes[0],
			advanceIndexes[0],
		)
	}
}

func visitCountLoopBreakerFactoryConfig() map[string]any {
	return map[string]any{
		"name": "visit-count-guard-failure",
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
				"name":      "execute-story",
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "story", "state": "in-review"}},
				"onFailure": []map[string]string{{"workType": "story", "state": "failed"}},
			},
			{
				"name":        "review-story",
				"worker":      "reviewer",
				"inputs":      []map[string]string{{"workType": "story", "state": "in-review"}},
				"onFailure":   []map[string]string{{"workType": "story", "state": "failed"}},
				"onRejection": []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":     []map[string]string{{"workType": "story", "state": "complete"}},
			},
			{
				"name":   "review-loop-breaker",
				"type":   "LOGICAL_MOVE",
				"inputs": []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{
					{"workType": "story", "state": "failed"},
				},
				"guards": []map[string]any{{
					"type":        "VISIT_COUNT",
					"workstation": "review-story",
					"maxVisits":   float64(2),
				}},
			},
		},
	}
}

func matchesFieldsGuardFactoryConfig() map[string]any {
	return map[string]any{
		"name": "matches-fields-guard-eligibility",
		"workTypes": []map[string]any{
			{
				"name": "plan",
				"states": []map[string]string{
					{"name": "ready", "type": "INITIAL"},
					{"name": "matched", "type": "TERMINAL"},
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
				"name":   "pair-items",
				"worker": "matcher",
				"inputs": []map[string]string{
					{"workType": "plan", "state": "ready"},
					{"workType": "task", "state": "ready"},
				},
				"outputs": []map[string]string{
					{"workType": "plan", "state": "matched"},
					{"workType": "task", "state": "matched"},
				},
				"guards": []map[string]any{{
					"type": "MATCHES_FIELDS",
					"matchConfig": map[string]string{
						"inputKey": ".Name",
					},
				}},
			},
		},
	}
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
				{"name": "gate-ready", "type": "PROCESSING"},
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
				"outputs":     []map[string]string{{"workType": "story", "state": "gate-ready"}},
			},
			{
				"name":   "advance-to-gate",
				"type":   "LOGICAL_MOVE",
				"inputs": []map[string]string{{"workType": "story", "state": "in-review"}},
				"outputs": []map[string]string{
					{"workType": "story", "state": "gate-ready"},
				},
				"guards": []map[string]any{{
					"type":        "VISIT_COUNT",
					"workstation": "execute-story",
					"maxVisits":   float64(2),
				}},
			},
			{
				"name":   "second-pass-review",
				"worker": "gate-reviewer",
				"inputs": []map[string]string{{"workType": "story", "state": "gate-ready"}},
				"outputs": []map[string]string{
					{"workType": "story", "state": "complete"},
				},
				"onFailure": []map[string]string{{"workType": "story", "state": "failed"}},
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

func waitForBlockedGuardObservation(t *testing.T, baseURL, sessionID string, timeout time.Duration) {
	t.Helper()
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
	_, err := support.WaitForObservation(timeout, func() (factoryapi.StatusResponse, error) {
		return support.GetJSON[factoryapi.StatusResponse](t, endpoint), nil
	}, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial == 2 && status.Categories.Processing == 0 &&
			status.Categories.Terminal == 0 && status.Categories.Failed == 0
	})
	if err != nil {
		t.Fatal(err)
	}
}

func waitForMinimumWorkAtCustomerState(t *testing.T, baseURL, sessionID, location string, want int, timeout time.Duration) {
	t.Helper()
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	_, err := support.WaitForObservation(timeout, func() (factoryapi.ListWorkResponse, error) {
		return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint), nil
	}, func(listed factoryapi.ListWorkResponse) bool {
		return support.CountWorkAtCustomerState(listed, location) >= want
	})
	if err != nil {
		t.Fatalf("waiting for %s Work: %v", location, err)
	}
}

func assertGuardSessionQuiescent(t *testing.T, session factoryapi.FactorySession, wantTerminal, wantFailed int) {
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

func runWorkflowParentOrSameNameGuardReleasesExpectedWorkCase0(t *testing.T, host *factorySessionHost) {
	t.Helper()
	t.Parallel()

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
	session, listed, events := host.Run(t, dir, runner, 15*time.Second)

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
	assertGuardSessionQuiescent(t, session, 1, 0)
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want 1 for the correlated match-items dispatch", runner.CallCount())
	}

}

func runWorkflowParentOrSameNameGuardReleasesExpectedWorkCase1(t *testing.T, host *factorySessionHost) {
	t.Helper()
	t.Parallel()

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
	baseURL, sessionID := host.Open(t, dir, runner)
	waitForMinimumWorkAtCustomerState(
		t,
		baseURL,
		sessionID,
		support.WorkCustomerLocation("plan", "ready"),
		1,
		10*time.Second,
	)
	waitForMinimumWorkAtCustomerState(
		t,
		baseURL,
		sessionID,
		support.WorkCustomerLocation("task", "ready"),
		1,
		10*time.Second,
	)
	waitForBlockedGuardObservation(t, baseURL, sessionID, 10*time.Second)

	listed := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+url.PathEscape(sessionID)+"/work")
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "matched")); got != 0 {
		t.Fatalf("matched task count = %d, want 0 for mismatched peer names; listed=%#v", got, listed)
	}
	if !support.HasWorkAtCustomerState(listed, mismatchedTaskWorkID, support.WorkCustomerLocation("task", "ready")) {
		t.Fatalf("task %q missing public ready state; listed=%#v", mismatchedTaskWorkID, listed)
	}

	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	if indexes := dispatchResponseIndexesForTransition(t, events, "match-items"); len(indexes) != 0 {
		t.Fatalf("match-items dispatch count = %d, want 0 while peer names mismatch", len(indexes))
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 while SAME_NAME guard blocks mismatched peers", runner.CallCount())
	}

}

func runWorkflowVisitOrMatchGuardFailureIsVisibleInPublicWorkStateCase0(t *testing.T, host *factorySessionHost) {
	t.Helper()
	t.Parallel()

	dir := support.ScaffoldFactory(t, visitCountLoopBreakerFactoryConfig())
	support.WriteAgentConfig(t, dir, "executor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "reviewer", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteWorkstationConfig(t, dir, "review-loop-breaker", "---\ntype: LOGICAL_MOVE\n---\n")

	const traceID = "trace-visit-count-guard-failure"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"visit-count guard failure proof"}`),
	})

	runner := support.NewShapedProviderCommandRunner(
		codexCommandResult("Done. COMPLETE"),
		codexCommandResult("needs more work"),
		codexCommandResult("Done. COMPLETE"),
		codexCommandResult("needs more work"),
	)
	session, listed, events := host.Run(t, dir, runner, 15*time.Second)

	for placeID, want := range map[string]int{
		support.WorkCustomerLocation("story", "failed"):    1,
		support.WorkCustomerLocation("story", "init"):      0,
		support.WorkCustomerLocation("story", "in-review"): 0,
		support.WorkCustomerLocation("story", "complete"):  0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Fatalf("%s work count = %d, want %d; listed=%#v", placeID, got, want, listed)
		}
	}
	loopBreakerIndexes := dispatchResponseIndexesForTransition(t, events, "review-loop-breaker")
	if len(loopBreakerIndexes) != 1 {
		t.Fatalf("review-loop-breaker dispatch count = %d, want 1 after visit-count guard failure", len(loopBreakerIndexes))
	}
	assertGuardSessionQuiescent(t, session, 0, 1)
	assertTerminalWorkCorrelatesToTraceID(t, listed, traceID)
	if runner.CallCount() != 4 {
		t.Fatalf("provider command calls = %d, want 4 (execute, review reject, execute, review reject)", runner.CallCount())
	}

}

func runWorkflowVisitOrMatchGuardFailureIsVisibleInPublicWorkStateCase1(t *testing.T, host *factorySessionHost) {
	t.Helper()
	t.Parallel()

	dir := support.ScaffoldFactory(t, matchesFieldsGuardFactoryConfig())
	support.WriteAgentConfig(t, dir, "matcher", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	const (
		mismatchedPlanWorkID = "plan-alpha"
		mismatchedTaskWorkID = "task-beta"
		mismatchTraceID      = "trace-matches-fields-mismatch"
	)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "flavor-a",
		WorkID:     mismatchedPlanWorkID,
		WorkTypeID: "plan",
		TraceID:    mismatchTraceID,
		Payload:    []byte(`{"role":"plan"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "flavor-b",
		WorkID:     mismatchedTaskWorkID,
		WorkTypeID: "task",
		TraceID:    mismatchTraceID,
		Payload:    []byte(`{"role":"task"}`),
	})

	runner := support.NewShapedProviderCommandRunner(
		codexCommandResult("Done. COMPLETE"),
	)
	baseURL, sessionID := host.Open(t, dir, runner)
	waitForMinimumWorkAtCustomerState(
		t,
		baseURL,
		sessionID,
		support.WorkCustomerLocation("plan", "ready"),
		1,
		10*time.Second,
	)
	waitForMinimumWorkAtCustomerState(
		t,
		baseURL,
		sessionID,
		support.WorkCustomerLocation("task", "ready"),
		1,
		10*time.Second,
	)
	waitForBlockedGuardObservation(t, baseURL, sessionID, 10*time.Second)

	listed := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+url.PathEscape(sessionID)+"/work")
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("plan", "matched")); got != 0 {
		t.Fatalf("matched plan count = %d, want 0 for mismatched field values; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "matched")); got != 0 {
		t.Fatalf("matched task count = %d, want 0 for mismatched field values; listed=%#v", got, listed)
	}
	for workID, location := range map[string]string{
		mismatchedPlanWorkID: support.WorkCustomerLocation("plan", "ready"),
		mismatchedTaskWorkID: support.WorkCustomerLocation("task", "ready"),
	} {
		if !support.HasWorkAtCustomerState(listed, workID, location) {
			t.Fatalf("work %q missing public %s state; listed=%#v", workID, location, listed)
		}
	}

	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	if indexes := dispatchResponseIndexesForTransition(t, events, "pair-items"); len(indexes) != 0 {
		t.Fatalf("pair-items dispatch count = %d, want 0 while MATCHES_FIELDS guard blocks mismatched names", len(indexes))
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 while MATCHES_FIELDS guard blocks mismatched inputs", runner.CallCount())
	}

}
