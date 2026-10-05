// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
package customer_lifecycles_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPetriSharedDispatchSuccess proves a simple Petri Factory
// started through the customer process reaches quiescence with submitted Work
// at the expected success terminal locations. Subtests absorb cold-start,
// preseeded and late-submit admission, archive-terminal completion, config-driven
// and scaffolded service-pipeline happy paths, noop fallback, multi-item
// completion, single- and two-stage pipelines, and ideation happy-path coverage
// without inspecting internal Petri markings.
func testOrchestrationpetridispatchPetriSharedDispatchSuccess(t *testing.T) {
	t.Run("simple_single_worker_pipeline_completes", runFactoryDispatchSuccessCase1)

	t.Run("preseeded_work_reaches_success_terminal", runFactoryDispatchSuccessCase2)

	t.Run("mixed_preseeded_and_late_submit_completes", runFactoryDispatchSuccessCase3)

	t.Run("archive_terminal_work_completes_without_refire", runFactoryDispatchSuccessCase4)

	t.Run("two_stage_pipeline_reaches_terminal", runFactoryDispatchSuccessCase5)

	t.Run("scaffolded_simple_pipeline_completes_one_task", runFactoryDispatchSuccessCase6)

	t.Run("ideation_happy_path_reaches_story_complete", runFactoryDispatchSuccessCase7)

	t.Run("dispatcher_workflow_single_idea_reaches_prd_complete", runFactoryDispatchSuccessCase8)

	t.Run("dispatcher_lifecycle_idea_reaches_archived_terminal", runFactoryDispatchSuccessCase9)

	t.Run("ideation_rejection_loop_reaches_story_complete", runFactoryDispatchSuccessCase10)

	t.Run("idea_plan_execute_review_reaches_task_complete", runFactoryDispatchSuccessCase11)

	t.Run("idea_to_prd_multiple_ideas_each_reach_terminal", runFactoryDispatchSuccessCase12)

	t.Run("config_driven_happy_path_two_stage_completes", runFactoryDispatchSuccessCase13)

	t.Run("noop_pipeline_completes_without_provider", runFactoryDispatchSuccessCase14)

	t.Run("service_simple_multiple_work_items_complete", runFactoryDispatchSuccessCase15)

	t.Run("scaffolded_multiple_work_items_complete_independently", runFactoryDispatchSuccessCase16)

	t.Run("scaffolded_two_stage_service_pipeline_completes", runFactoryDispatchSuccessCase17)

	runSharedPetriInvocationMapping(t)
}

// TestPetriWorkerErrorReturnsFailedTerminalOutcome proves worker or provider
// failures at the external-effect edge project Work to the Factory-configured
// public failed location with a failed dispatch outcome on Factory Events,
// without routing the same Work to success terminals.
func testOrchestrationpetridispatchPetriWorkerErrorReturnsFailedTerminalOutcome(t *testing.T) {
	t.Run("mock_provider_error_routes_to_failed_terminal", runFactoryDispatchFailureCase1)

	t.Run("provider_command_exit_routes_to_failed_terminal", runFactoryDispatchFailureCase2)

	t.Run("rejected_worker_outcome_routes_to_failed_terminal", runFactoryDispatchFailureCase3)

	t.Run("planner_failure_routes_idea_to_failed", runFactoryDispatchFailureCase4)

	t.Run("executor_failure_routes_prd_to_failed", runFactoryDispatchFailureCase5)

	t.Run("idea_to_prd_planner_command_failure_routes_idea_to_failed", runFactoryDispatchFailureCase6)

	t.Run("idea_plan_execute_review_script_failure_routes_plan_to_failed", runFactoryDispatchFailureCase7)

	t.Run("idea_plan_execute_review_planner_failure_routes_idea_to_failed", runFactoryDispatchFailureCase8)

	t.Run("idea_plan_execute_review_processor_exhaustion_routes_task_to_failed", runFactoryDispatchFailureCase9)
}

// TestPetriExecutorDispatchTerminalRouting proves executor failure and success
// routing for Petri workstations with and without authored failure arcs using
// root.BuildProcess and the shared ProviderCommandRunner edge.
func testOrchestrationpetridispatchPetriExecutorDispatchTerminalRouting(t *testing.T) {
	t.Run("provider_process_failure_without_failure_arcs_routes_to_failed", runFactoryProviderSelectionCase1)

	t.Run("provider_nonzero_exit_without_failure_arcs_routes_to_failed", runFactoryProviderSelectionCase2)

	t.Run("provider_failure_with_failure_arcs_routes_to_failed_not_done", runFactoryProviderSelectionCase3)

	t.Run("provider_success_leaves_work_at_authored_done_place", runFactoryProviderSelectionCase4)
}

// runSharedPetriInvocationMapping proves submitted Work payload and
// Trace identity map into worker invocation inputs at the external-effect edge
// and that public Work projections and Factory Events keep outputs and lineage
// attributable to the originating Work identity without inspecting internal
// Petri structures.
func runSharedPetriInvocationMapping(t *testing.T) {
	t.Run("factory_model_maps_to_provider_invocation", runProviderInvocationMappingCase1)

	t.Run("work_payload_maps_into_provider_user_message", runProviderInvocationMappingCase2)

	t.Run("work_name_maps_into_invocation_prompt", runProviderInvocationMappingCase3)

	t.Run("cross_work_type_terminal_preserves_origin_trace", runProviderInvocationMappingCase4)

	t.Run("dispatch_events_reference_terminal_work_identity", runProviderInvocationMappingCase5)
}

// TestPetriInvocationInputAndOutputMapping retains the failed-lineage witness
// while the shared command edge supplies deterministic provider outcomes.
func testOrchestrationpetridispatchPetriInvocationInputAndOutputMapping(t *testing.T) {
	t.Run("failed_terminal_preserves_origin_trace_lineage", func(t *testing.T) {
		enterSharedPetriScenario(t)
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))
		originTraceID := "trace-failed-lineage-mapping"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"failed lineage mapping"}`),
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
		listed := runSharedPetriFactoryToCompletionWithRouteAndListedWork(
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
			support.WorkCustomerLocation("idea", "init"):     0,
			support.WorkCustomerLocation("plan", "init"):     0,
			support.WorkCustomerLocation("task", "init"):     0,
		})
		assertListedWorkStateTrace(t, listed, "idea", "complete", originTraceID)
		assertListedWorkStateTrace(t, listed, "plan", "complete", originTraceID)
		assertListedWorkStateTrace(t, listed, "task", "failed", originTraceID)
	})
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

func assertCompletedWorkName(t *testing.T, response factoryapi.ListWorkResponse, workType, wantName string) {
	t.Helper()
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != "complete" {
			continue
		}
		if item.Name != wantName {
			t.Errorf("%s:complete name = %q, want %q", workType, item.Name, wantName)
		}
		return
	}
	t.Errorf("listed Work missing %s:complete", workType)
}

func assertFailedDispatchResponseErrorForWork(
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
		if dispatch.Response.Error != nil && *dispatch.Response.Error != "" {
			return
		}
	}
	t.Fatalf("no failed dispatch response with public error for work %q", workID)
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

func assertRejectedDispatchForWork(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workID string,
	wantFeedback string,
) {
	t.Helper()

	for _, dispatch := range dispatches {
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		if dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeRejected {
			continue
		}
		if dispatch.Response.Output != nil && *dispatch.Response.Output == wantFeedback {
			return
		}
	}
	t.Fatalf(
		"no rejected dispatch observation with feedback %q for work %q",
		wantFeedback,
		workID,
	)
}

func simpleSingleWorkerPipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func twoStageServicePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "processing", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-b"},
		},
		"workstations": []map[string]any{
			{
				"name":      "step-one",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "step-two",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeServicePipelineWorkerConfig(t *testing.T, dir, workerName string) {
	t.Helper()
	support.WriteAgentConfig(
		t,
		dir,
		workerName,
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
}

func orchestrationpetridispatchAssertQuiescentSession(t *testing.T, status factoryapi.StatusResponse, wantTerminal, wantFailed int) {
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

func assertQuiescentFactorySession(
	t *testing.T,
	session factoryapi.FactorySession,
	wantTerminal,
	wantFailed int,
) {
	t.Helper()
	orchestrationpetridispatchAssertQuiescentSession(t, factoryapi.StatusResponse{
		Categories: session.Runtime.Progress.Categories,
	}, wantTerminal, wantFailed)
}
