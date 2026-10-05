package customer_lifecycles_test

import (
	"errors"
	"fmt"
	"strings"
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

func runFactoryDispatchSuccessCase1(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
	traceID := "trace-simple-pipeline"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"single-worker smoke"}`),
	})

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("task", "init"):   0,
		support.WorkCustomerLocation("task", "failed"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	if calls := sharedPetriProcess(t).router.callsFor(dir); calls != 1 {
		t.Errorf("provider command call count = %d, want 1", calls)
	}

}
func runFactoryDispatchSuccessCase2(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "auth"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "logging"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "metrics"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 15*time.Second)

	terminal := support.WorkCustomerLocation("code-change", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 3,
		support.WorkCustomerLocation("code-change", "init"):      0,
		support.WorkCustomerLocation("code-change", "in-review"): 0,
		support.WorkCustomerLocation("code-change", "failed"):    0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 3, 0)
	assertSharedPetriCommandCalls(t, dir, 6)

}
func runFactoryDispatchSuccessCase3(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "pre-existing"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "new-arrival"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 15*time.Second)

	terminal := support.WorkCustomerLocation("code-change", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 2,
		support.WorkCustomerLocation("code-change", "init"):      0,
		support.WorkCustomerLocation("code-change", "in-review"): 0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 2, 0)
	assertSharedPetriProviderCalls(t, dir, 4)

}
func runFactoryDispatchSuccessCase4(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "settings page"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("code-change", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("code-change", "init"):      0,
		support.WorkCustomerLocation("code-change", "in-review"): 0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriCommandCalls(t, dir, 2)

}
func runFactoryDispatchSuccessCase5(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"two-stage pipeline"}`))

	session, listed, events := runSharedPetriFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "failed"):     0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertDispatchTransitionSequence(
		t,
		assertPublicDispatchEvents(t, events, 2),
		[]string{"step-one", "step-two"},
	)
	assertSharedPetriCommandCalls(t, dir, 2)

}
func runFactoryDispatchSuccessCase6(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := support.ScaffoldFactory(t, simpleSingleWorkerPipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"scaffolded simple pipeline"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("task", "init"):   0,
		support.WorkCustomerLocation("task", "failed"): 0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runFactoryDispatchSuccessCase7(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))
	originTraceID := "trace-ideation-happy-path"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"search bar on docs"}`),
	})

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 15*time.Second)

	terminal := support.WorkCustomerLocation("story", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("idea", "init"):       0,
		support.WorkCustomerLocation("prd", "init"):        0,
		support.WorkCustomerLocation("story", "init"):      0,
		support.WorkCustomerLocation("story", "in-review"): 0,
		support.WorkCustomerLocation("story", "executing"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{originTraceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriCommandCalls(t, dir, 3)

}
func runFactoryDispatchSuccessCase8(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
	traceID := "trace-dispatcher-single"
	seedIdeas(t, dir, []seedIdea{{traceID: traceID, title: "add login page"}})

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("prd", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("idea", "init"): 0,
		support.WorkCustomerLocation("prd", "init"):  0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriCommandCalls(t, dir, 3)

}
func runFactoryDispatchSuccessCase9(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))
	originTraceID := "trace-idea-lifecycle-test"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"improve onboarding flow"}`),
	})

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 30*time.Second)

	terminal := support.WorkCustomerLocation("code-change", "archived")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("idea", "init"):            0,
		support.WorkCustomerLocation("idea", "failed"):          0,
		support.WorkCustomerLocation("prd", "init"):             0,
		support.WorkCustomerLocation("prd", "failed"):           0,
		support.WorkCustomerLocation("code-change", "init"):     0,
		support.WorkCustomerLocation("code-change", "approved"): 0,
		support.WorkCustomerLocation("code-change", "failed"):   0,
	})
	assertListedWorkStateTrace(t, listed, "code-change", "archived", originTraceID)
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriCommandCalls(t, dir, 4)

}
func runFactoryDispatchSuccessCase10(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))
	originTraceID := "trace-rejection-loop-001"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"rejection loop test"}`),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("PRD created. COMPLETE"),
			sharedPetriProviderOutput("Code written. COMPLETE"),
			sharedPetriProviderOutput("Needs more work. REJECTED"),
			sharedPetriProviderOutput("Code revised. COMPLETE"),
			sharedPetriProviderOutput("Still not right. REJECTED"),
			sharedPetriProviderOutput("Code revised again. COMPLETE"),
			sharedPetriProviderOutput("Looks good now. ACCEPTED"),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		30*time.Second,
	)

	terminal := support.WorkCustomerLocation("story", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("idea", "init"):       0,
		support.WorkCustomerLocation("prd", "init"):        0,
		support.WorkCustomerLocation("story", "init"):      0,
		support.WorkCustomerLocation("story", "in-review"): 0,
	})
	assertListedWorkStateTrace(t, listed, "story", "complete", originTraceID)
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	dispatches := assertPublicDispatchEvents(t, events, 7)
	assertDispatchTransitionSubsequence(t, dispatches, []string{
		"plan-idea",
		"execute-story",
		"review-story",
		"execute-story",
		"review-story",
		"execute-story",
		"review-story",
	})
	assertDispatchTransitionOutcomes(t, dispatches, "review-story", []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeRejected,
		factoryapi.WorkOutcomeRejected,
		factoryapi.WorkOutcomeAccepted,
	})
	assertSharedPetriProviderCalls(t, dir, 7)

}
func runFactoryDispatchSuccessCase11(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))
	testutil.WriteSeedMarkdownFile(t, dir, "idea", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 15*time.Second)

	assertWorkAtCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("idea", "complete"): 1,
		support.WorkCustomerLocation("plan", "complete"): 1,
		support.WorkCustomerLocation("task", "complete"): 1,
		support.WorkCustomerLocation("idea", "init"):     0,
		support.WorkCustomerLocation("plan", "init"):     0,
		support.WorkCustomerLocation("task", "init"):     0,
		support.WorkCustomerLocation("task", "failed"):   0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 3, 0)
	assertSharedPetriProviderCalls(t, dir, 3)

}
func runFactoryDispatchSuccessCase12(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))
	trace1 := "trace-idea-multi-1"
	trace2 := "trace-idea-multi-2"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    trace1,
		Payload:    []byte(`{"title":"idea one"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    trace2,
		Payload:    []byte(`{"title":"idea two"}`),
	})

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("prd", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 2,
		support.WorkCustomerLocation("idea", "init"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{trace1, trace2})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 2, 0)
	assertSharedPetriProviderCalls(t, dir, 2)

}
func runFactoryDispatchSuccessCase13(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Config-driven happy path"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriProviderCalls(t, dir, 2)

}
func runFactoryDispatchSuccessCase14(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "noop_pipeline"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "noop fallback test"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{},
		10*time.Second,
	)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriProviderCalls(t, dir, 0)

}
func runFactoryDispatchSuccessCase15(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-2"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 2})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 2, 0)
	assertSharedPetriProviderCalls(t, dir, 4)

}
func runFactoryDispatchSuccessCase16(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := support.ScaffoldFactory(t, simpleSingleWorkerPipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	for i := 0; i < 3; i++ {
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			Name:       fmt.Sprintf("batch-item-%d", i),
			WorkTypeID: "task",
			TraceID:    fmt.Sprintf("trace-e2e-batch-%d", i),
			Payload:    []byte(`{"title":"batch item"}`),
		})
	}

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 3})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 3, 0)
	assertSharedPetriProviderCalls(t, dir, 3)

}
func runFactoryDispatchSuccessCase17(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := support.ScaffoldFactory(t, twoStageServicePipelineConfig())
	writeServicePipelineWorkerConfig(t, dir, "worker-a")
	writeServicePipelineWorkerConfig(t, dir, "worker-b")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"two-stage service pipeline"}`))

	session, listed := runSharedPetriFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriProviderCalls(t, dir, 2)

}
func runFactoryDispatchFailureCase1(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
	traceID := "trace-provider-error"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"will fail at provider"}`),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriCommandError(fmt.Errorf("provider inference failed")),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("task", "failed")
	successTerminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal:  1,
		successTerminal: 0,
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	assertTraceAbsentAtCustomerState(t, listed, successTerminal, traceID)
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runFactoryDispatchFailureCase2(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
	traceID := "trace-command-exit-failure"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`work payload`),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriCommandResult(platformprocess.CommandResult{
				Stderr:   []byte("provider unavailable"),
				ExitCode: 1,
			}),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("task", "failed")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal: 1,
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runFactoryDispatchFailureCase3(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))
	traceID := "trace-rejected-outcome"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`work payload`),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("not good enough"),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("task", "failed")
	successTerminal := support.WorkCustomerLocation("task", "done")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal:  1,
		successTerminal: 0,
		support.WorkCustomerLocation("task", "init"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	assertTraceAbsentAtCustomerState(t, listed, successTerminal, traceID)
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertRejectedDispatchForWork(
		t,
		support.ObserveDispatchEvents(t, events),
		failedWorkID,
		"not good enough",
	)

}
func runFactoryDispatchFailureCase4(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))
	traceID := "trace-planner-failure"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"broken idea"}`),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("failed"),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("idea", "failed")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal: 1,
		support.WorkCustomerLocation("idea", "init"):            0,
		support.WorkCustomerLocation("prd", "init"):             0,
		support.WorkCustomerLocation("code-change", "init"):     0,
		support.WorkCustomerLocation("code-change", "archived"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertRejectedDispatchForWork(
		t,
		support.ObserveDispatchEvents(t, events),
		failedWorkID,
		"failed",
	)

}
func runFactoryDispatchFailureCase5(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))
	traceID := "trace-executor-failure"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"failing executor"}`),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("success<COMPLETE>"),
			sharedPetriCommandError(errors.New("failed executors")),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		15*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("prd", "failed")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal: 1,
		support.WorkCustomerLocation("idea", "init"):            0,
		support.WorkCustomerLocation("prd", "init"):             0,
		support.WorkCustomerLocation("code-change", "init"):     0,
		support.WorkCustomerLocation("code-change", "archived"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)

}
func runFactoryDispatchFailureCase6(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))
	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title":"broken idea"}`))

	route := sharedPetriRouteConfig{
		provider: sharedPetriFixedCommandResult(platformprocess.CommandResult{
			Stderr:   []byte("LLM timeout"),
			ExitCode: 1,
		}),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	assertWorkAtCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("idea", "failed"):  1,
		support.WorkCustomerLocation("prd", "init"):     0,
		support.WorkCustomerLocation("prd", "complete"): 0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)
	assertPublicDispatchEvents(t, events, 1)

}
func runFactoryDispatchFailureCase7(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))
	testutil.WriteSeedMarkdownFile(t, dir, "idea", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("Task processed successfully.\n<COMPLETE>\n"),
		),
		script: sharedPetriScriptSequence(
			sharedPetriCommandResult(platformprocess.CommandResult{
				Stderr:   []byte("script execution failed"),
				ExitCode: 1,
			}),
		),
	}
	_, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	assertWorkAtCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("idea", "complete"): 1,
		support.WorkCustomerLocation("plan", "failed"):   1,
		support.WorkCustomerLocation("plan", "complete"): 0,
		support.WorkCustomerLocation("task", "init"):     0,
		support.WorkCustomerLocation("task", "failed"):   0,
		support.WorkCustomerLocation("task", "complete"): 0,
	})
	assertSharedPetriProviderCalls(t, dir, 1)
	assertSharedPetriCommandCalls(t, dir, 2)
	assertPublicDispatchEvents(t, events, 1)

}
func runFactoryDispatchFailureCase8(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))
	testutil.WriteSeedMarkdownFile(t, dir, "idea", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("Task processed unsuccessfully.<FAILED>"),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	assertWorkAtCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("idea", "failed"):   1,
		support.WorkCustomerLocation("plan", "init"):     0,
		support.WorkCustomerLocation("plan", "complete"): 0,
		support.WorkCustomerLocation("task", "init"):     0,
	})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)
	assertPublicDispatchEvents(t, events, 1)

}
func runFactoryDispatchFailureCase9(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))
	originTraceID := "trace-processor-exhaustion"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"processor exhaustion"}`),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("Task processed successfully.\n<COMPLETE>\n"),
			sharedPetriProviderOutput("Task execution failed.<FAILED>"),
			sharedPetriProviderOutput("Task execution failed.<FAILED>"),
			sharedPetriProviderOutput("Task execution failed.<FAILED>"),
			sharedPetriProviderOutput("Task execution failed.<FAILED>"),
			sharedPetriProviderOutput("Task execution failed.<FAILED>"),
		),
	}
	_, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		15*time.Second,
	)

	assertWorkAtCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("idea", "complete"): 1,
		support.WorkCustomerLocation("plan", "complete"): 1,
		support.WorkCustomerLocation("task", "failed"):   1,
		support.WorkCustomerLocation("task", "complete"): 0,
	})
	assertListedWorkStateTrace(t, listed, "idea", "complete", originTraceID)
	assertListedWorkStateTrace(t, listed, "plan", "complete", originTraceID)
	assertListedWorkStateTrace(t, listed, "task", "failed", originTraceID)
	assertSharedPetriProviderCalls(t, dir, 6)
	assertPublicDispatchEvents(t, events, 6)

}
func runFactoryProviderSelectionCase1(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
	traceID := "trace-executor-process-failure"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("work payload"),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriCommandError(errors.New("executor crashed")),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("task", "failed")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal: 1,
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	assertFailedDispatchResponseErrorForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runFactoryProviderSelectionCase2(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
	traceID := "trace-executor-exit-failure"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("work"),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriCommandResult(platformprocess.CommandResult{
				Stderr:   []byte("provider unavailable"),
				ExitCode: 1,
			}),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("task", "failed")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal: 1,
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runFactoryProviderSelectionCase3(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_with_arcs"))
	traceID := "trace-executor-failure-arcs"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("work"),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriCommandResult(platformprocess.CommandResult{
				Stderr:   []byte("intentional failure"),
				ExitCode: 1,
			}),
		),
	}
	session, listed, events := runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		route,
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("task", "failed")
	doneTerminal := support.WorkCustomerLocation("task", "done")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal: 1,
		doneTerminal:   0,
		support.WorkCustomerLocation("task", "init"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	assertTraceAbsentAtCustomerState(t, listed, doneTerminal, traceID)
	orchestrationpetridispatchAssertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	assertNoAcceptedDispatchMovesWorkToCustomerState(t, events, failedWorkID, doneTerminal)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runFactoryProviderSelectionCase4(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	traceID := "trace-executor-success"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("work"),
	})

	route := sharedPetriRouteConfig{
		provider: sharedPetriProviderSequence(
			sharedPetriProviderOutput("COMPLETE"),
		),
	}
	session, listed := runSharedPetriFactoryToCompletionWithRouteAndWork(
		t,
		dir,
		route,
		10*time.Second,
	)

	doneTerminal := support.WorkCustomerLocation("task", "done")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		doneTerminal: 1,
		support.WorkCustomerLocation("task", "init"):   0,
		support.WorkCustomerLocation("task", "failed"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, doneTerminal, []string{traceID})
	orchestrationpetridispatchAssertQuiescentSession(t, session, 1, 0)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runProviderInvocationMappingCase1(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"model mapping probe"}`))

	listed := runSharedPetriFactoryToCompletionWithEdgesAndListedWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
	assertSharedPetriProviderCalls(t, dir, 1)
	request := sharedPetriProviderRequest(t, dir)
	support.AssertArgsContainSequence(t, request.Args, []string{"--model", "test-model"})
	if len(request.Stdin) == 0 {
		t.Error("provider command stdin is empty, want rendered Factory worker prompt content")
	}

}
func runProviderInvocationMappingCase2(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
	markdownNeedle := "# Architecture Review"
	testutil.WriteSeedMarkdownFile(t, dir, "task", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	listed := runSharedPetriFactoryToCompletionWithEdgesAndListedWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
	assertCompletedWorkName(t, listed, "task", "architecture-review")
	assertSharedPetriProviderCalls(t, dir, 1)
	userMessage := string(sharedPetriProviderRequest(t, dir).Stdin)
	if !strings.Contains(userMessage, markdownNeedle) {
		t.Errorf(
			"provider user message = %q, want markdown payload needle %q",
			userMessage,
			markdownNeedle,
		)
	}
	if !strings.Contains(userMessage, "Task Name: architecture-review") {
		t.Errorf(
			"provider user message = %q, want seeded Work name architecture-review",
			userMessage,
		)
	}

}
func runProviderInvocationMappingCase3(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
	workName := "design-doc-review"
	traceID := "trace-prompt-mapping"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       workName,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`review the design document`),
	})

	listed := runSharedPetriFactoryToCompletionWithEdgesAndListedWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
	assertSharedPetriProviderCalls(t, dir, 1)
	userMessage := string(sharedPetriProviderRequest(t, dir).Stdin)
	if !strings.Contains(userMessage, "Task Name: "+workName) {
		t.Errorf(
			"provider user message = %q, want rendered prompt to contain Task Name: %s",
			userMessage,
			workName,
		)
	}

}
func runProviderInvocationMappingCase4(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))
	originTraceID := "trace-cross-work-type-mapping"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title":"search bar on docs"}`),
	})

	listed := runSharedPetriFactoryToCompletionWithEdgesAndListedWork(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("prd", "complete")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("idea", "init"): 0,
	})
	assertListedWorkStateTrace(t, listed, "prd", "complete", originTraceID)
	assertSharedPetriProviderCalls(t, dir, 1)

}
func runProviderInvocationMappingCase5(t *testing.T) {
	t.Helper()

	enterSharedPetriScenario(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
	traceID := "trace-dispatch-event-mapping"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"dispatch event mapping"}`),
	})

	_, listed, events := runSharedPetriFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{}, 10*time.Second)

	terminal := support.WorkCustomerLocation("task", "complete")
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
	assertDispatchEventsReferenceTerminalWork(t, events, listed, terminal, []string{traceID})
	assertSharedPetriProviderCalls(t, dir, 1)

}
