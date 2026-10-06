package customer_lifecycles_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type mergedRouteCase struct {
	name       string
	rounds     int
	label      string
	exit       int
	rejectLast bool
	success    bool
	breaker    string
}

// Session-scoped Work and Event routing is the API contract under test. The
// shared host enters through Process.Execute; only command effect edges vary.
func testFactoryreviewfailureroutingMergedPRRoute_CompletionAndUnmergedLimits(t *testing.T) {
	cases := []mergedRouteCase{
		{name: "M01_merged_first_visit", rounds: 0, label: "merged", success: true},
		{name: "F01_merged_at_limit", rounds: 12, label: "merged", success: true},
		{name: "F02_unmerged_review_limit", rounds: 12, label: "review", breaker: "review-loop-breaker"},
		{name: "F03_unmerged_executor_limit", rounds: 12, label: "review", rejectLast: true, breaker: "executor-loop-breaker"},
		{name: "F04_ordinary_review", rounds: 0, label: "review", success: true},
		{name: "F05_script_failure", rounds: 0, label: "", exit: 1},
		{name: "F06_unknown_label", rounds: 0, label: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); runMergedRouteCase(t, tc) })
	}
}

func mergedRouteResponder(tc mergedRouteCase) reviewFailureRouteConfig {
	return reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, call int) (platformprocess.CommandResult, error) {
			if call%2 == 0 && call <= tc.rounds*2 {
				if call == tc.rounds*2 && !tc.rejectLast {
					return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(`{"decision":"CONTINUE","feedback":"wait for CI"}`)}, nil
				}
				return reviewFailureRejected("bounded correction"), nil
			}
			return reviewFailureAccepted("delivered"), nil
		},
		script: func(_ context.Context, request platformprocess.CommandRequest, call int) (platformprocess.CommandResult, error) {
			if !strings.Contains(strings.Join(request.Args, " "), "--classification") {
				return platformprocess.CommandResult{Stdout: []byte("script-ok")}, nil
			}
			label, exit := "review", 0
			if call > tc.rounds {
				label, exit = tc.label, tc.exit
			}
			return platformprocess.CommandResult{Stdout: []byte(label + "\n"), Stderr: []byte("controlled ci-wait observation"), ExitCode: exit}, nil
		},
	}
}

func runMergedRouteCase(t *testing.T, tc mergedRouteCase) {
	t.Helper()
	scenario := openReviewFailureScenario(t, mergedRouteResponder(tc))
	stream := scenario.eventStream(t)
	name, trace := scenario.marker+"-lane", scenario.marker+"-trace"
	ideaID, taskID, dependentID := scenario.marker+"-idea", scenario.marker+"-task", scenario.marker+"-dependent"
	// Admit the relation while its target name identifies only the idea. No work
	// can complete before the task is admitted, and the dependent remains gated.
	scenario.submit(t, scenario.marker+"-parents",
		reviewFailureSeed{Name: name, WorkID: ideaID, WorkType: "idea", State: "to-complete", TraceID: trace},
		reviewFailureSeed{Name: dependentID, WorkID: dependentID, WorkType: "validation", State: "init", TraceID: trace,
			DependsOn: name, DependsOnState: "complete"})
	seed := reviewFailureSeed{Name: name, WorkID: taskID, WorkType: "task", State: "init", TraceID: trace,
		Payload: "representative task payload", Tags: map[string]string{"lane": "retirement"}}
	requestID := scenario.marker + "-task-request"
	scenario.submit(t, requestID, seed)
	terminal := "report-idea-failure"
	if tc.success {
		terminal = "report-idea-complete"
	}
	_ = awaitReviewFailureDispatchResponses(t, stream, terminal, 1)
	want := map[string]string{ideaID: "failed", taskID: "escalated"}
	if tc.success {
		_ = awaitReviewFailureDispatchResponses(t, stream, "report-validation-complete", 1)
		want = map[string]string{ideaID: "complete", taskID: "complete", dependentID: "complete"}
	}
	awaitReviewFailureWorkStates(t, scenario, want)
	if tc.success && tc.label == "merged" {
		processes := dispatchesWithTransition(reviewFailureDispatches(t, scenario), "process")
		minted := reviewRetirementOutput(t, processes[len(processes)-1], "review")
		awaitReviewFailureWorkStates(t, scenario, map[string]string{*minted.WorkId: "complete"})
	}
	awaitReviewFailureQuiescence(t, scenario)
	assertMergedRouteDispatches(t, scenario, tc)
	assertMergedReviewOutcome(t, scenario, tc)
	if tc.name == "F01_merged_at_limit" {
		scenario.submit(t, requestID, seed)
		awaitReviewFailureQuiescence(t, scenario)
		assertMergedReviewOutcome(t, scenario, tc)
		assertReviewFailureWorkStates(t, scenario.listWorks(t), want)
		assertMergedRouteDispatches(t, scenario, tc) // F08: same request cannot release again.
	}
}

func assertMergedRouteDispatches(t *testing.T, scenario *reviewFailureScenario, tc mergedRouteCase) {
	t.Helper()
	dispatches := reviewFailureDispatches(t, scenario)
	expected := map[string]int{"process": tc.rounds, "review": tc.rounds, "validate": 0, "ci-wait": tc.rounds,
		"retire-pending-review-after-task-complete": 0, "executor-loop-breaker": 0, "review-loop-breaker": 0, "consume": 0, "escalate-task-failure": 1}
	if tc.rounds == 0 {
		expected["process"]++
	}
	if !tc.rejectLast {
		expected["ci-wait"]++
	}
	if tc.success {
		expected["validate"], expected["consume"], expected["escalate-task-failure"] = 1, 1, 0
		if tc.rounds == 0 && tc.label == "review" {
			expected["review"] = 1
		}
	}
	if tc.success && tc.label == "merged" {
		expected["retire-pending-review-after-task-complete"] = 1
	}
	if tc.breaker != "" {
		expected[tc.breaker] = 1
	}
	for transition, want := range expected {
		if got := len(dispatchesWithTransition(dispatches, transition)); got != want {
			t.Errorf("%s dispatches = %d, want %d", transition, got, want)
		}
	}
	assertNoIncompleteReviewFailureDispatches(t, dispatches)
	assertMergedRouteEvidence(t, scenario, tc, dispatches)
}

func assertMergedRouteEvidence(t *testing.T, scenario *reviewFailureScenario, tc mergedRouteCase, dispatches []support.DispatchEventObservation) {
	t.Helper()
	if tc.success {
		completion := dispatchesWithTransition(dispatches, "report-idea-complete")[0]
		release := dispatchesWithTransition(dispatches, "prepare-validation")[0]
		assertExactReviewFailureInputIDs(t, release, scenario.marker+"-dependent")
		if release.StartedAt.Before(completion.CompletedAt) {
			t.Fatal("dependent dispatched before idea completion")
		}
	}
	if tc.label == "unknown" {
		encoded, err := json.Marshal(support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), "did not match any authored classification route") {
			t.Fatal("unknown classifier label produced no actionable mismatch evidence")
		}
	}
	if tc.label == "merged" {
		observations := dispatchesWithTransition(dispatches, "ci-wait")
		last := observations[len(observations)-1].Response
		if last.SelectedClassificationLabel == nil || *last.SelectedClassificationLabel != "merged" {
			t.Fatalf("confirmed merge selected label = %#v", last.SelectedClassificationLabel)
		}
		assertReviewFailureDispatchOutputStates(t, *last, map[string]string{scenario.marker + "-task": "to-complete"})
	}
}

func testFactoryreviewfailureroutingMergedPRRoute_AuthoredFactoryValidateOnly(t *testing.T) {
	t.Parallel()
	fixture := sharedReviewFailureFixture(t)
	source, err := os.ReadFile(filepath.Join(fixture.sourceFactory, "factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	endpoint := fixture.baseURL + "/factory-sessions/~default/factory"
	before := support.GetJSON[factoryapi.Factory](t, endpoint)
	response, err := http.Post(fixture.baseURL+"/factory-validations", "application/json", bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result factoryapi.FactoryValidationResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(result.Targets) != 0 {
		t.Fatalf("authored Factory validation: status=%d result=%#v", response.StatusCode, result)
	}
	cliDir := t.TempDir()
	if err := os.CopyFS(filepath.Join(cliDir, "factory"), os.DirFS(fixture.sourceFactory)); err != nil {
		t.Fatal(err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "factory", "config", "validate", filepath.Join(cliDir, "factory", "factory.json")})
	inputs.Input.Env = isolatedHomeEnvironment(filepath.Join(cliDir, "home"))
	inputs.Input.WorkingDirectory = cliDir
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("CLI validation: %v; stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout()+inputs.Stderr(), "Factory validation passed.") {
		t.Fatal("CLI did not report successful Factory validation")
	}
	after := support.GetJSON[factoryapi.Factory](t, endpoint)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("validate-only changed the current Factory")
	}
}

// Observe the generated review identity at the public process output, rather
// than assuming how the runtime allocates its Work ID.
func assertMergedReviewOutcome(t *testing.T, scenario *reviewFailureScenario, tc mergedRouteCase) {
	t.Helper()
	if !tc.success {
		return
	}
	dispatches := reviewFailureDispatches(t, scenario)
	processes := dispatchesWithTransition(dispatches, "process")
	minted := reviewRetirementOutput(t, processes[len(processes)-1], "review")
	current := support.GetJSON[factoryapi.ListWorkResponse](t,
		support.SessionWorkURL(scenario.fixture.baseURL, scenario.sessionID, "/work")).Results
	found := false
	for _, work := range current {
		if work.WorkTypeName == nil || *work.WorkTypeName != "review" || work.Name != minted.Name ||
			!reflect.DeepEqual(work.TraceId, minted.TraceId) {
			continue
		}
		if work.State == nil || work.State.Name != "complete" || work.State.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("completed lane retains non-terminal review: %#v", work)
		}
		if reflect.DeepEqual(work.WorkId, minted.WorkId) {
			found = true
		}
	}
	if !found {
		t.Fatalf("process-minted review %v missing from current terminal Work", minted.WorkId)
	}
	if tc.label == "merged" {
		retirement := dispatchesWithTransition(dispatches, "retire-pending-review-after-task-complete")[0]
		consume := dispatchesWithTransition(dispatches, "consume")[0]
		before := reviewRetirementOutput(t, consume, "task")
		after := reviewRetirementOutput(t, retirement, "task")
		assertExactReviewFailureInputIDs(t, retirement, *minted.WorkId, *before.WorkId)
		assertRetirementPreservesWork(t, before, after)
		if retirement.StartedAt.Before(consume.CompletedAt) {
			t.Fatal("review retired before consume completed")
		}
	} else {
		review := dispatchesWithTransition(dispatches, "review")[0]
		assertExactReviewFailureInputIDs(t, review, scenario.marker+"-task", *minted.WorkId)
		assertReviewFailureDispatchOutputStates(t, *review.Response,
			map[string]string{scenario.marker + "-task": "to-complete", *minted.WorkId: "complete"})
		ci := dispatchesWithTransition(dispatches, "ci-wait")[0]
		assertReviewFailureDispatchOutputStates(t, *ci.Response,
			map[string]string{scenario.marker + "-task": "in-review"})
	}
}

func reviewRetirementOutput(t *testing.T, dispatch support.DispatchEventObservation, workType string) factoryapi.Work {
	t.Helper()
	if dispatch.Response != nil && dispatch.Response.OutputWork != nil {
		for _, work := range *dispatch.Response.OutputWork {
			if work.WorkTypeName != nil && *work.WorkTypeName == workType {
				return work
			}
		}
	}
	t.Fatalf("dispatch %s has no %s output", dispatch.DispatchID, workType)
	return factoryapi.Work{}
}

func assertRetirementPreservesWork(t *testing.T, before, after factoryapi.Work) {
	t.Helper()
	// A logical move may update visit/recording metadata; these are the owned
	// identity, content and completion properties visible to the customer.
	if before.Name != after.Name || !reflect.DeepEqual(before.WorkId, after.WorkId) ||
		!reflect.DeepEqual(before.TraceId, after.TraceId) ||
		!reflect.DeepEqual(before.CurrentChainingTraceId, after.CurrentChainingTraceId) ||
		!reflect.DeepEqual(before.Payload, after.Payload) || !reflect.DeepEqual(before.Content, after.Content) ||
		!reflect.DeepEqual(before.Tags, after.Tags) || !reflect.DeepEqual(before.Relations, after.Relations) ||
		!reflect.DeepEqual(before.State, after.State) {
		t.Fatalf("retirement changed Work identity/content/completion: before=%#v after=%#v", before, after)
	}
}

func testReviewRetirement_RequiresMatchingCompletedTask(t *testing.T) {
	for _, mismatch := range []string{"trace", "name", "incomplete"} {
		t.Run(mismatch, func(t *testing.T) {
			t.Parallel()
			scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{})
			stream := scenario.eventStream(t)
			name, trace := scenario.marker+"-lane", scenario.marker+"-trace"
			task := reviewFailureSeed{Name: name, TraceID: trace, WorkID: scenario.marker + "-task",
				WorkType: "task", State: "complete", Payload: "task content", Tags: map[string]string{"owner": "task"}}
			review := reviewFailureSeed{Name: name, TraceID: trace, WorkID: scenario.marker + "-review",
				WorkType: "review", State: "init", Payload: "pending review", Tags: map[string]string{"owner": "review"}}
			switch mismatch {
			case "trace":
				review.TraceID += "-other"
			case "name":
				review.Name += "-other"
			case "incomplete":
				task.State = "to-complete"
			}
			// Batch names must be unique across work types. Neither seed alone
			// can dispatch, so separate admissions cannot race completion.
			scenario.submit(t, scenario.marker+"-task-request", task)
			scenario.submit(t, scenario.marker+"-review-request", review)
			awaitReviewFailureQuiescence(t, scenario)
			before := scenario.listWorks(t)
			assertReviewFailureWorkStates(t, before, map[string]string{task.WorkID: task.State, review.WorkID: "init"})
			if len(reviewFailureDispatches(t, scenario)) != 0 {
				t.Fatal("unmatched or incomplete pair dispatched")
			}
			assertPendingReviewUnchanged(t, before, review)
			if mismatch == "incomplete" {
				scenario.submit(t, scenario.marker+"-idea-request", reviewFailureSeed{Name: name, TraceID: trace,
					WorkID: scenario.marker + "-idea", WorkType: "idea", State: "to-complete"})
				_ = awaitReviewFailureDispatchResponses(t, stream, "retire-pending-review-after-task-complete", 1)
				awaitReviewFailureWorkStates(t, scenario, map[string]string{task.WorkID: "complete", review.WorkID: "complete"})
				awaitReviewFailureQuiescence(t, scenario)
				dispatches := reviewFailureDispatches(t, scenario)
				moves := dispatchesWithTransition(dispatches, "retire-pending-review-after-task-complete")
				if len(moves) != 1 || len(dispatchesWithTransition(dispatches, "consume")) != 1 {
					t.Fatal("matching task completion did not consume and retire exactly once")
				}
				assertExactReviewFailureInputIDs(t, moves[0], task.WorkID, review.WorkID)
			}
		})
	}
}

func assertPendingReviewUnchanged(t *testing.T, works []factoryapi.Work, seed reviewFailureSeed) {
	t.Helper()
	for _, work := range works {
		if work.WorkId == nil || *work.WorkId != seed.WorkID {
			continue
		}
		if work.Name != seed.Name || work.TraceId == nil || *work.TraceId != seed.TraceID ||
			work.Payload != seed.Payload || work.Tags == nil || (*work.Tags)["owner"] != seed.Tags["owner"] {
			t.Fatalf("unrelated pending review changed: %#v", work)
		}
		return
	}
	t.Fatalf("pending review %s disappeared", seed.WorkID)
}
