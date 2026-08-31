package routing

import (
	"errors"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"testing"
	"time"
)

// TestPetriMultiStagePipelineCompletesAtPublicTerminals proves multi-transition
// Petri Factory routing started through root.BuildProcess reaches the documented
// public success terminal location(s) at quiescence. Scenarios cover two-stage
// same-work-type pipelines, cross-work-type dispatcher flows, and three-stage
// ideation pipelines without inspecting internal Petri markings.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestPetriMultiStagePipelineCompletesAtPublicTerminals(t *testing.T) {
	t.Run("two_stage_service_simple_completes_at_terminal", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		traceID := "trace-two-stage-service-simple"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"two-stage routing depth"}`),
		})

		status, listed := runSharedRoutingFactoryToCompletionWithRouteAndWork(
			t, dir, sharedRoutingRouteConfig{}, 10*time.Second,
		)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("task", "init"):       0,
			support.WorkCustomerLocation("task", "processing"): 0,
			support.WorkCustomerLocation("task", "failed"):     0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, status, 1, 0)
		assertSharedRoutingProviderCalls(t, dir, 2)
	})

	t.Run("code_review_multi_stage_completes", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
		traceID := "trace-code-review-routing"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "code-change",
			TraceID:    traceID,
			Payload:    []byte(`{"task":"routing depth review"}`),
		})

		status, listed, events := runSharedRoutingFactoryToCompletionWithRouteAndObservations(
			t, dir, sharedRoutingRouteConfig{}, 15*time.Second,
		)

		terminal := support.WorkCustomerLocation("code-change", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("code-change", "init"):      0,
			support.WorkCustomerLocation("code-change", "in-review"): 0,
			support.WorkCustomerLocation("code-change", "failed"):    0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, status, 1, 0)
		assertDispatchTransitionSequence(t, events, []string{"coding", "review"})
		assertSharedRoutingProviderCalls(t, dir, 2)
	})

	t.Run("three_stage_ideation_reaches_story_complete", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))
		traceID := "trace-ideation-multi-stage"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"multi-transition ideation depth"}`),
		})

		status, listed := runSharedRoutingFactoryToCompletionWithRouteAndWork(
			t,
			dir,
			sharedRoutingRouteConfig{provider: sharedRoutingProviderSequence(
				sharedRoutingProviderOutput("PRD created. COMPLETE"),
				sharedRoutingProviderOutput("Code written. COMPLETE"),
				sharedRoutingProviderOutput("Looks good. ACCEPTED"),
			)},
			15*time.Second,
		)

		terminal := support.WorkCustomerLocation("story", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("idea", "init"):       0,
			support.WorkCustomerLocation("prd", "init"):        0,
			support.WorkCustomerLocation("story", "init"):      0,
			support.WorkCustomerLocation("story", "in-review"): 0,
			support.WorkCustomerLocation("story", "executing"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, status, 1, 0)
		assertSharedRoutingProviderCalls(t, dir, 3)
	})

	t.Run("cross_work_type_dispatcher_reaches_prd_complete", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		traceID := "trace-dispatcher-multi-transition"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"dispatcher routing depth"}`),
		})

		status, listed := runSharedRoutingFactoryToCompletionWithRouteAndWork(
			t, dir, sharedRoutingRouteConfig{}, 10*time.Second,
		)

		terminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("idea", "init"): 0,
			support.WorkCustomerLocation("prd", "init"):  0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, status, 1, 0)
		assertSharedRoutingProviderCalls(t, dir, 3)
	})
}

// TestPetriFailureRoutesToDocumentedFailedPlace proves worker or provider
// failures in multi-transition Petri Factory routing project Work to the
// Factory-documented failed place on public Work / session surfaces. Failed
// dispatch outcomes are asserted on Factory Events when that is the natural
// observation surface, without routing the same Work to success terminals or
// inspecting internal Petri markings.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestPetriFailureRoutesToDocumentedFailedPlace(t *testing.T) {
	t.Run("two_stage_service_simple_second_stage_exit_routes_to_failed", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		traceID := "trace-two-stage-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"two-stage failure routing"}`),
		})

		status, listed, events := runSharedRoutingFactoryToCompletionWithRouteAndObservations(
			t,
			dir,
			sharedRoutingRouteConfig{provider: sharedRoutingProviderSequence(
				sharedRoutingProviderOutput("Done. COMPLETE"),
				sharedRoutingCommandResult(platformprocess.CommandResult{
					Stderr:   []byte("stage-two provider unavailable"),
					ExitCode: 1,
				}),
			)},
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
		assertQuiescentSession(t, status, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		dispatches := support.ObserveDispatchEvents(t, events)
		assertFailedDispatchForWork(t, dispatches, failedWorkID)
		assertDispatchTransitionSequence(t, events, []string{"step-one", "step-two"})
		assertSharedRoutingProviderCalls(t, dir, 2)
	})

	t.Run("cross_work_type_dispatcher_executor_exit_routes_prd_to_failed", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		traceID := "trace-dispatcher-executor-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"dispatcher failure routing"}`),
		})

		status, listed, events := runSharedRoutingFactoryToCompletionWithRouteAndObservations(
			t,
			dir,
			sharedRoutingRouteConfig{provider: sharedRoutingProviderSequence(
				sharedRoutingProviderOutput("Done. COMPLETE"),
				sharedRoutingCommandResult(platformprocess.CommandResult{
					Stderr:   []byte("executor subprocess failed"),
					ExitCode: 1,
				}),
			)},
			15*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("prd", "failed")
		successTerminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal:  1,
			successTerminal: 0,
			support.WorkCustomerLocation("idea", "init"):     0,
			support.WorkCustomerLocation("prd", "init"):      0,
			support.WorkCustomerLocation("prd", "in-review"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertTraceAbsentAtCustomerState(t, listed, successTerminal, traceID)
		assertQuiescentSession(t, status, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		assertDispatchTransitionSequence(t, events, []string{"plan", "execute"})
		assertSharedRoutingProviderCalls(t, dir, 2)
	})

	t.Run("code_review_reviewer_error_routes_to_failed", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
		traceID := "trace-code-review-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "code-change",
			TraceID:    traceID,
			Payload:    []byte(`{"task":"routing failure review"}`),
		})

		status, listed, events := runSharedRoutingFactoryToCompletionWithRouteAndObservations(
			t,
			dir,
			sharedRoutingRouteConfig{provider: sharedRoutingProviderSequence(
				sharedRoutingProviderOutput("Done. COMPLETE"),
				sharedRoutingCommandError(errors.New("reviewer inference failed")),
			)},
			15*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("code-change", "failed")
		successTerminal := support.WorkCustomerLocation("code-change", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal:  1,
			successTerminal: 0,
			support.WorkCustomerLocation("code-change", "init"):      0,
			support.WorkCustomerLocation("code-change", "in-review"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertTraceAbsentAtCustomerState(t, listed, successTerminal, traceID)
		assertQuiescentSession(t, status, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		assertDispatchTransitionSequence(t, events, []string{"coding", "review"})
		assertSharedRoutingProviderCalls(t, dir, 2)
	})
}

// TestPetriMultiTransitionPreservesWorkCorrelation proves a known public Work
// identity submitted through multi-transition Petri Factory routing remains
// attributable on public Work listings and Factory Event projections after
// routing completes or lands at the documented failed place, without
// inspecting internal Petri markings.
func TestPetriMultiTransitionPreservesWorkCorrelation(t *testing.T) {
	t.Run("cross_work_type_dispatcher_preserves_origin_trace_through_stages", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		originTraceID := "trace-correlation-dispatcher"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"correlation across dispatcher stages"}`),
		})

		_, listed, events := runSharedRoutingFactoryToCompletionWithRouteAndObservations(
			t, dir, sharedRoutingRouteConfig{}, 10*time.Second,
		)

		terminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("idea", "init"): 0,
			support.WorkCustomerLocation("prd", "init"):  0,
		})
		assertListedWorkStateTrace(t, listed, "prd", "complete", originTraceID)
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{originTraceID})
		assertDispatchEventsReferenceTerminalWork(t, events, listed, terminal, []string{originTraceID})
	})

	t.Run("three_stage_ideation_correlates_trace_on_terminal_and_events", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))
		originTraceID := "trace-correlation-ideation"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"correlation across ideation stages"}`),
		})

		_, listed, events := runSharedRoutingFactoryToCompletionWithRouteAndObservations(
			t,
			dir,
			sharedRoutingRouteConfig{provider: sharedRoutingProviderSequence(
				sharedRoutingProviderOutput("PRD created. COMPLETE"),
				sharedRoutingProviderOutput("Code written. COMPLETE"),
				sharedRoutingProviderOutput("Looks good. ACCEPTED"),
			)},
			15*time.Second,
		)

		terminal := support.WorkCustomerLocation("story", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("idea", "init"):       0,
			support.WorkCustomerLocation("prd", "init"):        0,
			support.WorkCustomerLocation("story", "init"):      0,
			support.WorkCustomerLocation("story", "in-review"): 0,
			support.WorkCustomerLocation("story", "executing"): 0,
		})
		assertListedWorkStateTrace(t, listed, "story", "complete", originTraceID)
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{originTraceID})
		assertDispatchEventsReferenceTerminalWork(t, events, listed, terminal, []string{originTraceID})
	})

	t.Run("failure_routing_preserves_origin_trace_at_failed_place", func(t *testing.T) {
		enterSharedRoutingScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		originTraceID := "trace-correlation-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"correlation on failure routing"}`),
		})

		_, listed, events := runSharedRoutingFactoryToCompletionWithRouteAndObservations(
			t,
			dir,
			sharedRoutingRouteConfig{provider: sharedRoutingProviderSequence(
				sharedRoutingProviderOutput("Done. COMPLETE"),
				sharedRoutingCommandResult(platformprocess.CommandResult{
					Stderr:   []byte("stage-two provider unavailable"),
					ExitCode: 1,
				}),
			)},
			10*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("task", "failed")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			support.WorkCustomerLocation("task", "complete"): 0,
		})
		assertListedWorkStateTrace(t, listed, "task", "failed", originTraceID)
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{originTraceID})
		assertDispatchEventsReferenceTerminalWork(t, events, listed, failedTerminal, []string{originTraceID})
	})
}

func workIDAtCustomerState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	location string,
	traceID string,
) (string, bool) {
	t.Helper()
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != location {
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			continue
		}
		if item.WorkId == nil || *item.WorkId == "" {
			t.Fatalf("work at %s for trace %q missing workId", location, traceID)
		}
		return *item.WorkId, true
	}
	return "", false
}

func assertTraceAbsentAtCustomerState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	location string,
	traceID string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != location {
			continue
		}
		if item.TraceId != nil && *item.TraceId == traceID {
			t.Errorf("trace %q should not be at %s", traceID, location)
		}
	}
}

func assertFailedDispatchForWork(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workID string,
) {
	t.Helper()

	for _, dispatch := range dispatches {
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		if dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
			continue
		}
		return
	}
	t.Fatalf("no failed dispatch observation for work %q", workID)
}

func assertDispatchTransitionSequence(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want []string,
) {
	t.Helper()
	observed := support.ObserveDispatchEvents(t, events)
	if len(observed) != len(want) {
		t.Fatalf("dispatch transition count = %d, want %d", len(observed), len(want))
	}
	for index, transition := range want {
		if got := observed[index].Request.TransitionId; got != transition {
			t.Errorf("dispatch transition[%d] = %q, want %q", index, got, transition)
		}
	}
}

func assertWorkAtCustomerStates(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Errorf("%s Work count = %d, want %d", location, got, want)
		}
	}
}

func assertQuiescentSession(t *testing.T, status factoryapi.StatusResponse, wantTerminal, wantFailed int) {
	t.Helper()
	categories := status.Categories
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

func assertListedWorkStateTrace(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	workType, state, traceID string,
) {
	t.Helper()
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != state {
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Errorf("%s:%s trace ID = %#v, want %q", workType, state, item.TraceId, traceID)
		}
		return
	}
	t.Errorf("listed Work missing %s:%s", workType, state)
}

func assertDispatchEventsReferenceTerminalWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	traceIDs []string,
) {
	t.Helper()
	terminalWorkIDs := map[string]string{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.TraceId == nil || item.WorkId == nil {
			continue
		}
		terminalWorkIDs[*item.TraceId] = *item.WorkId
	}
	for _, traceID := range traceIDs {
		workID, ok := terminalWorkIDs[traceID]
		if !ok {
			t.Errorf("missing terminal Work ID for trace %q", traceID)
			continue
		}
		referenced := false
		for _, dispatch := range support.ObserveDispatchEvents(t, events) {
			if support.DispatchObservationIncludesWork(dispatch, workID) {
				referenced = true
				break
			}
		}
		if !referenced {
			t.Errorf("dispatch events missing public Work ID %q for trace %q", workID, traceID)
		}
	}
}

func assertTerminalWorkCorrelatesToTraceIDs(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	traceIDs []string,
) {
	t.Helper()
	wants := make(map[string]bool, len(traceIDs))
	for _, traceID := range traceIDs {
		wants[traceID] = true
	}
	found := map[string]bool{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.TraceId == nil || !wants[*item.TraceId] {
			t.Errorf("unexpected %s trace ID %#v", terminalLocation, item.TraceId)
			continue
		}
		if item.WorkId == nil || *item.WorkId == "" {
			t.Errorf("terminal Work at %s missing workId for trace %q", terminalLocation, *item.TraceId)
			continue
		}
		if found[*item.TraceId] {
			t.Errorf("duplicate terminal Work for trace %q at %s", *item.TraceId, terminalLocation)
			continue
		}
		found[*item.TraceId] = true
	}
	for traceID := range wants {
		if !found[traceID] {
			t.Errorf("listed Work missing %s trace %q", terminalLocation, traceID)
		}
	}
}
