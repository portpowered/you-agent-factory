package routing

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPetriMultiStagePipelineCompletesAtPublicTerminals proves multi-transition
// Petri Factory routing started through root.BuildProcess reaches the documented
// public success terminal location(s) at quiescence. Scenarios cover two-stage
// same-work-type pipelines, cross-work-type dispatcher flows, and three-stage
// ideation pipelines without inspecting internal Petri markings.
func TestPetriMultiStagePipelineCompletesAtPublicTerminals(t *testing.T) {
	t.Run("two_stage_service_simple_completes_at_terminal", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		traceID := "trace-two-stage-service-simple"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"two-stage routing depth"}`),
		})

		runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(2)...)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("task", "init"):        0,
			support.WorkCustomerLocation("task", "processing"): 0,
			support.WorkCustomerLocation("task", "failed"):    0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, session, 1, 0)
		if runner.CallCount() != 2 {
			t.Errorf("provider command call count = %d, want 2", runner.CallCount())
		}
	})

	t.Run("code_review_multi_stage_completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
		traceID := "trace-code-review-routing"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "code-change",
			TraceID:    traceID,
			Payload:    []byte(`{"task":"routing depth review"}`),
		})

		provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
			"swe":      {{Content: "Done. COMPLETE"}},
			"reviewer": {{Content: "Done. COMPLETE"}},
		})
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 15*time.Second)

		terminal := support.WorkCustomerLocation("code-change", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("code-change", "init"):      0,
			support.WorkCustomerLocation("code-change", "in-review"): 0,
			support.WorkCustomerLocation("code-change", "failed"):    0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount("swe") != 1 || provider.CallCount("reviewer") != 1 {
			t.Errorf(
				"provider calls = swe:%d reviewer:%d, want swe:1 reviewer:1",
				provider.CallCount("swe"),
				provider.CallCount("reviewer"),
			)
		}
	})

	t.Run("three_stage_ideation_reaches_story_complete", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))
		traceID := "trace-ideation-multi-stage"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"multi-transition ideation depth"}`),
		})

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "PRD created. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Code written. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Looks good. ACCEPTED"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 15*time.Second)

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
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount() != 3 {
			t.Errorf("provider call count = %d, want 3", provider.CallCount())
		}
	})

	t.Run("cross_work_type_dispatcher_reaches_prd_complete", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		traceID := "trace-dispatcher-multi-transition"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"dispatcher routing depth"}`),
		})

		runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(3)...)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("idea", "init"): 0,
			support.WorkCustomerLocation("prd", "init"):  0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, session, 1, 0)
		if runner.CallCount() != 3 {
			t.Errorf("provider command call count = %d, want 3", runner.CallCount())
		}
	})
}

// TestPetriFailureRoutesToDocumentedFailedPlace proves worker or provider
// failures in multi-transition Petri Factory routing project Work to the
// Factory-documented failed place on public Work / session surfaces. Failed
// dispatch outcomes are asserted on Factory Events when that is the natural
// observation surface, without routing the same Work to success terminals or
// inspecting internal Petri markings.
func TestPetriFailureRoutesToDocumentedFailedPlace(t *testing.T) {
	t.Run("two_stage_service_simple_second_stage_exit_routes_to_failed", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		traceID := "trace-two-stage-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"two-stage failure routing"}`),
		})

		runner := testutil.NewProviderCommandRunner(
			platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Done. COMPLETE")},
			platformprocess.CommandResult{
				Stderr:   []byte("stage-two provider unavailable"),
				ExitCode: 1,
			},
		)
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			10*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("task", "failed")
		successTerminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			successTerminal: 0,
			support.WorkCustomerLocation("task", "init"):        0,
			support.WorkCustomerLocation("task", "processing"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertTraceAbsentAtCustomerState(t, listed, successTerminal, traceID)
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		if runner.CallCount() != 2 {
			t.Errorf("provider command call count = %d, want 2", runner.CallCount())
		}
	})

	t.Run("cross_work_type_dispatcher_executor_exit_routes_prd_to_failed", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		traceID := "trace-dispatcher-executor-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"dispatcher failure routing"}`),
		})

		runner := testutil.NewProviderCommandRunner(
			platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Done. COMPLETE")},
			platformprocess.CommandResult{
				Stderr:   []byte("executor subprocess failed"),
				ExitCode: 1,
			},
		)
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			15*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("prd", "failed")
		successTerminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			successTerminal: 0,
			support.WorkCustomerLocation("idea", "init"):        0,
			support.WorkCustomerLocation("prd", "init"):         0,
			support.WorkCustomerLocation("prd", "in-review"):      0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertTraceAbsentAtCustomerState(t, listed, successTerminal, traceID)
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		if runner.CallCount() != 2 {
			t.Errorf("provider command call count = %d, want 2", runner.CallCount())
		}
	})

	t.Run("code_review_reviewer_error_routes_to_failed", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
		traceID := "trace-code-review-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "code-change",
			TraceID:    traceID,
			Payload:    []byte(`{"task":"routing failure review"}`),
		})

		provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
			"swe":      {{Content: "Done. COMPLETE"}},
			"reviewer": {{Content: "", Error: errors.New("reviewer inference failed")}},
		})
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
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
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		if provider.CallCount("swe") != 1 || provider.CallCount("reviewer") != 1 {
			t.Errorf(
				"provider calls = swe:%d reviewer:%d, want swe:1 reviewer:1",
				provider.CallCount("swe"),
				provider.CallCount("reviewer"),
			)
		}
	})
}

// TestPetriMultiTransitionPreservesWorkCorrelation proves a known public Work
// identity submitted through multi-transition Petri Factory routing remains
// attributable on public Work listings and Factory Event projections after
// routing completes or lands at the documented failed place, without
// inspecting internal Petri markings.
func TestPetriMultiTransitionPreservesWorkCorrelation(t *testing.T) {
	t.Run("cross_work_type_dispatcher_preserves_origin_trace_through_stages", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		originTraceID := "trace-correlation-dispatcher"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"correlation across dispatcher stages"}`),
		})

		runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(3)...)
		_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			10*time.Second,
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
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))
		originTraceID := "trace-correlation-ideation"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"correlation across ideation stages"}`),
		})

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "PRD created. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Code written. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Looks good. ACCEPTED"},
		)
		_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
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
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		originTraceID := "trace-correlation-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"correlation on failure routing"}`),
		})

		runner := testutil.NewProviderCommandRunner(
			platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Done. COMPLETE")},
			platformprocess.CommandResult{
				Stderr:   []byte("stage-two provider unavailable"),
				ExitCode: 1,
			},
		)
		_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
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
