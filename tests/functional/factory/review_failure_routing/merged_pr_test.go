package review_failure_routing

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
func TestMergedPRRoute_CompletionAndUnmergedLimits(t *testing.T) {
	cases := []mergedRouteCase{
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
	seed := reviewFailureSeed{Name: name, WorkID: taskID, WorkType: "task", State: "init", TraceID: trace}
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
	assertMergedRouteDispatches(t, scenario, tc)
	if tc.name == "F01_merged_at_limit" {
		scenario.submit(t, requestID, seed)
		assertReviewFailureWorkStates(t, scenario.listWorks(t), want)
		assertMergedRouteDispatches(t, scenario, tc) // F08: same request cannot release again.
	}
}

func assertMergedRouteDispatches(t *testing.T, scenario *reviewFailureScenario, tc mergedRouteCase) {
	t.Helper()
	dispatches := reviewFailureDispatches(t, scenario)
	expected := map[string]int{"process": tc.rounds, "review": tc.rounds, "validate": 0, "ci-wait": tc.rounds,
		"executor-loop-breaker": 0, "review-loop-breaker": 0, "consume": 0, "escalate-task-failure": 1}
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

func TestMergedPRRoute_AuthoredFactoryValidateOnly(t *testing.T) {
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
	after := support.GetJSON[factoryapi.Factory](t, endpoint)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("validate-only changed the current Factory")
	}
}
