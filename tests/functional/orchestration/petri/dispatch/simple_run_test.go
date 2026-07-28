package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPetriSingleWorkerRunCompletesAtQuiescence proves a simple Petri Factory
// started through the customer process reaches quiescence with submitted Work
// at the expected success terminal locations. Subtests absorb cold-start,
// preseeded and late-submit admission, archive-terminal completion, config-driven
// and scaffolded service-pipeline happy paths, noop fallback, multi-item
// completion, single- and two-stage pipelines, and ideation happy-path coverage
// without inspecting internal Petri markings.
func TestPetriSingleWorkerRunCompletesAtQuiescence(t *testing.T) {
	t.Run("simple_single_worker_pipeline_completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
		traceID := "trace-simple-pipeline"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"single-worker smoke"}`),
		})

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal:                                    1,
			support.WorkCustomerLocation("task", "init"): 0,
			support.WorkCustomerLocation("task", "failed"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount() != 1 {
			t.Errorf("provider call count = %d, want 1", provider.CallCount())
		}
	})

	t.Run("preseeded_work_reaches_success_terminal", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
		testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "auth"}`))
		testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "logging"}`))
		testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "metrics"}`))

		provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
			"swe": {
				{Content: "Done. COMPLETE"},
				{Content: "Done. COMPLETE"},
				{Content: "Done. COMPLETE"},
			},
			"reviewer": {
				{Content: "Done. COMPLETE"},
				{Content: "Done. COMPLETE"},
				{Content: "Done. COMPLETE"},
			},
		})
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 15*time.Second)

		terminal := support.WorkCustomerLocation("code-change", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 3,
			support.WorkCustomerLocation("code-change", "init"):       0,
			support.WorkCustomerLocation("code-change", "in-review"):  0,
			support.WorkCustomerLocation("code-change", "failed"):     0,
		})
		assertQuiescentSession(t, session, 3, 0)
		if provider.CallCount("swe") != 3 {
			t.Errorf("swe call count = %d, want 3", provider.CallCount("swe"))
		}
	})

	t.Run("mixed_preseeded_and_late_submit_completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
		testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "pre-existing"}`))
		testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "new-arrival"}`))

		provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
			"swe":      {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
			"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		})
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 15*time.Second)

		terminal := support.WorkCustomerLocation("code-change", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 2,
			support.WorkCustomerLocation("code-change", "init"):      0,
			support.WorkCustomerLocation("code-change", "in-review"): 0,
		})
		assertQuiescentSession(t, session, 2, 0)
	})

	t.Run("archive_terminal_work_completes_without_refire", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
		testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "settings page"}`))

		provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
			"swe":      {{Content: "Done. COMPLETE"}},
			"reviewer": {{Content: "Approved. COMPLETE"}},
		})
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("code-change", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("code-change", "init"):      0,
			support.WorkCustomerLocation("code-change", "in-review"): 0,
		})
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount("swe") != 1 || provider.CallCount("reviewer") != 1 {
			t.Errorf(
				"provider calls = swe:%d reviewer:%d, want swe:1 reviewer:1",
				provider.CallCount("swe"),
				provider.CallCount("reviewer"),
			)
		}
	})

	t.Run("two_stage_pipeline_reaches_terminal", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"two-stage pipeline"}`))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("task", "init"):        0,
			support.WorkCustomerLocation("task", "processing"): 0,
			support.WorkCustomerLocation("task", "failed"):    0,
		})
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount() != 2 {
			t.Errorf("provider call count = %d, want 2", provider.CallCount())
		}
	})

	t.Run("scaffolded_simple_pipeline_completes_one_task", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, simpleSingleWorkerPipelineConfig())
		support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"scaffolded simple pipeline"}`))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Simple pipeline done. COMPLETE"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("task", "init"):   0,
			support.WorkCustomerLocation("task", "failed"): 0,
		})
		assertQuiescentSession(t, session, 1, 0)
	})

	t.Run("ideation_happy_path_reaches_story_complete", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "full_ideation_pipeline"))
		originTraceID := "trace-ideation-happy-path"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"search bar on docs"}`),
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
			support.WorkCustomerLocation("idea", "init"):         0,
			support.WorkCustomerLocation("prd", "init"):          0,
			support.WorkCustomerLocation("story", "init"):        0,
			support.WorkCustomerLocation("story", "in-review"):   0,
			support.WorkCustomerLocation("story", "executing"):   0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{originTraceID})
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount() != 3 {
			t.Errorf("provider call count = %d, want 3", provider.CallCount())
		}
	})

	t.Run("dispatcher_workflow_single_idea_reaches_prd_complete", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		traceID := "trace-dispatcher-single"
		seedIdeas(t, dir, []seedIdea{{traceID: traceID, title: "add login page"}})

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
	})

	t.Run("config_driven_happy_path_two_stage_completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Config-driven happy path"}`))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount() != 2 {
			t.Errorf("provider call count = %d, want 2", provider.CallCount())
		}
	})

	t.Run("noop_pipeline_completes_without_provider", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "noop_pipeline"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "noop fallback test"}`))

		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
			t,
			dir,
			serviceedges.Edges{},
			10*time.Second,
		)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
		assertQuiescentSession(t, session, 1, 0)
	})

	t.Run("service_simple_multiple_work_items_complete", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-1"}`))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-2"}`))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 2})
		assertQuiescentSession(t, session, 2, 0)
		if provider.CallCount() != 4 {
			t.Errorf("provider call count = %d, want 4", provider.CallCount())
		}
	})

	t.Run("scaffolded_multiple_work_items_complete_independently", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, simpleSingleWorkerPipelineConfig())
		support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
		for i := 0; i < 3; i++ {
			testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
				WorkTypeID: "task",
				TraceID:    fmt.Sprintf("trace-e2e-batch-%d", i),
				Payload:    []byte(`{"title":"batch item"}`),
			})
		}

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 3})
		assertQuiescentSession(t, session, 3, 0)
	})

	t.Run("scaffolded_two_stage_service_pipeline_completes", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, twoStageServicePipelineConfig())
		writeServicePipelineWorkerConfig(t, dir, "worker-a")
		writeServicePipelineWorkerConfig(t, dir, "worker-b")
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"two-stage service pipeline"}`))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
			workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
		)
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
		assertQuiescentSession(t, session, 1, 0)
		if provider.CallCount() != 2 {
			t.Errorf("provider call count = %d, want 2", provider.CallCount())
		}
	})
}

// TestPetriWorkerErrorReturnsFailedTerminalOutcome proves worker or provider
// failures at the external-effect edge project Work to the Factory-configured
// public failed location with a failed dispatch outcome on Factory Events,
// without routing the same Work to success terminals.
func TestPetriWorkerErrorReturnsFailedTerminalOutcome(t *testing.T) {
	t.Run("mock_provider_error_routes_to_failed_terminal", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
		traceID := "trace-provider-error"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"will fail at provider"}`),
		})

		provider := testutil.NewMockProviderWithErrors(
			[]workerexecution.InferenceResponse{{Content: ""}},
			[]error{fmt.Errorf("provider inference failed")},
		)
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
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
		if provider.CallCount() != 1 {
			t.Errorf("provider call count = %d, want 1", provider.CallCount())
		}
	})

	t.Run("provider_command_exit_routes_to_failed_terminal", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
		traceID := "trace-command-exit-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`work payload`),
		})

		runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stderr:   []byte("provider unavailable"),
			ExitCode: 1,
		})
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			10*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("task", "failed")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			support.WorkCustomerLocation("task", "init"):        0,
			support.WorkCustomerLocation("task", "processing"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	})

	t.Run("rejected_worker_outcome_routes_to_failed_terminal", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))
		traceID := "trace-rejected-outcome"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`work payload`),
		})

		provider := testutil.NewMockProvider(support.RejectedProviderResponse("not good enough"))
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
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
		assertQuiescentSession(t, session, 0, 1)

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
	})

	t.Run("planner_failure_routes_idea_to_failed", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))
		traceID := "trace-planner-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"broken idea"}`),
		})

		provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
			"planner": {{Content: "failed"}},
		})
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
			10*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("idea", "failed")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			support.WorkCustomerLocation("idea", "init"):              0,
			support.WorkCustomerLocation("prd", "init"):               0,
			support.WorkCustomerLocation("code-change", "init"):       0,
			support.WorkCustomerLocation("code-change", "archived"):     0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertQuiescentSession(t, session, 0, 1)

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
	})

	t.Run("executor_failure_routes_prd_to_failed", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))
		traceID := "trace-executor-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"failing executor"}`),
		})

		provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
			"planner":  {{Content: "success<COMPLETE>"}},
			"executor": {{Content: "failed", Error: errors.New("failed executors")}},
		})
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
			15*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("prd", "failed")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			support.WorkCustomerLocation("idea", "init"):          0,
			support.WorkCustomerLocation("prd", "init"):           0,
			support.WorkCustomerLocation("code-change", "init"):   0,
			support.WorkCustomerLocation("code-change", "archived"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
	})
}

// TestPetriExecutorDispatchTerminalRouting proves executor failure and success
// routing for Petri workstations with and without authored failure arcs using
// root.BuildProcess + ProviderCommandRunner captures (retired guards_batch
// executor_failure scenarios).
func TestPetriExecutorDispatchTerminalRouting(t *testing.T) {
	t.Run("provider_process_failure_without_failure_arcs_routes_to_failed", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
		traceID := "trace-executor-process-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte("work payload"),
		})

		runner := &recordingProviderCommandRunner{runErr: errors.New("executor crashed")}
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			10*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("task", "failed")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			support.WorkCustomerLocation("task", "init"):        0,
			support.WorkCustomerLocation("task", "processing"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		assertFailedDispatchResponseErrorForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		if runner.callCount() != 1 {
			t.Errorf("provider command call count = %d, want 1", runner.callCount())
		}
	})

	t.Run("provider_nonzero_exit_without_failure_arcs_routes_to_failed", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
		traceID := "trace-executor-exit-failure"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte("work"),
		})

		runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stderr:   []byte("provider unavailable"),
			ExitCode: 1,
		})
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			10*time.Second,
		)

		failedTerminal := support.WorkCustomerLocation("task", "failed")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			failedTerminal: 1,
			support.WorkCustomerLocation("task", "init"):        0,
			support.WorkCustomerLocation("task", "processing"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		if runner.CallCount() != 1 {
			t.Errorf("provider command call count = %d, want 1", runner.CallCount())
		}
	})

	t.Run("provider_failure_with_failure_arcs_routes_to_failed_not_done", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_with_arcs"))
		traceID := "trace-executor-failure-arcs"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte("work"),
		})

		runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stderr:   []byte("intentional failure"),
			ExitCode: 1,
		})
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
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
		assertQuiescentSession(t, session, 0, 1)

		failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
		if !ok {
			t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
		}
		assertFailedDispatchForWork(t, support.ObserveDispatchEvents(t, events), failedWorkID)
		assertNoAcceptedDispatchMovesWorkToCustomerState(t, events, failedWorkID, doneTerminal)
		if runner.CallCount() != 1 {
			t.Errorf("provider command call count = %d, want 1", runner.CallCount())
		}
	})

	t.Run("provider_success_leaves_work_at_authored_done_place", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		traceID := "trace-executor-success"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte("work"),
		})

		runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout("COMPLETE"),
		})
		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			10*time.Second,
		)

		doneTerminal := support.WorkCustomerLocation("task", "done")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			doneTerminal: 1,
			support.WorkCustomerLocation("task", "init"):   0,
			support.WorkCustomerLocation("task", "failed"): 0,
		})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, doneTerminal, []string{traceID})
		assertQuiescentSession(t, session, 1, 0)
		if runner.CallCount() != 1 {
			t.Errorf("provider command call count = %d, want 1", runner.CallCount())
		}
	})
}

type recordingProviderCommandRunner struct {
	requests []platformprocess.CommandRequest
	runErr   error
}

func (r *recordingProviderCommandRunner) Run(
	_ context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.requests = append(r.requests, req)
	if r.runErr != nil {
		return platformprocess.CommandResult{}, r.runErr
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")}, nil
}

func (r *recordingProviderCommandRunner) callCount() int {
	return len(r.requests)
}

// TestPetriInvocationInputAndOutputMapping proves submitted Work payload and
// Trace identity map into worker invocation inputs at the external-effect edge
// and that public Work projections and Factory Events keep outputs and lineage
// attributable to the originating Work identity without inspecting internal
// Petri structures.
func TestPetriInvocationInputAndOutputMapping(t *testing.T) {
	t.Run("factory_model_maps_to_provider_invocation", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"model mapping probe"}`))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		)
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
		if provider.CallCount() != 1 {
			t.Fatalf("provider call count = %d, want 1", provider.CallCount())
		}

		call := provider.LastCall()
		if call.Model != "test-model" {
			t.Errorf("provider model = %q, want test-model", call.Model)
		}
		if call.SystemPrompt == "" {
			t.Error("provider system prompt is empty, want Factory worker prompt content")
		}
	})

	t.Run("work_payload_maps_into_provider_user_message", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
		markdownNeedle := "# Architecture Review"
		testutil.WriteSeedMarkdownFile(t, dir, "task", "architecture-review",
			[]byte("# Architecture Review\n\nPlease review the system architecture."))

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Reviewed. COMPLETE"},
		)
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
		assertCompletedWorkName(t, listed, "task", "architecture-review")
		userMessage := provider.LastCall().UserMessage
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
	})

	t.Run("work_name_maps_into_invocation_prompt", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
		workName := "design-doc-review"
		traceID := "trace-prompt-mapping"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			Name:       workName,
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`review the design document`),
		})

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Reviewed. COMPLETE"},
		)
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		userMessage := provider.LastCall().UserMessage
		if !strings.Contains(userMessage, "Task Name: "+workName) {
			t.Errorf(
				"provider user message = %q, want rendered prompt to contain Task Name: %s",
				userMessage,
				workName,
			)
		}
	})

	t.Run("cross_work_type_terminal_preserves_origin_trace", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))
		originTraceID := "trace-cross-work-type-mapping"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"search bar on docs"}`),
		})

		provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
			"planner":       {{Content: "Done. COMPLETE"}},
			"prd-processor": {{Content: "Done. COMPLETE"}},
		})
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{
			terminal: 1,
			support.WorkCustomerLocation("idea", "init"): 0,
		})
		assertListedWorkStateTrace(t, listed, "prd", "complete", originTraceID)
		if provider.CallCount("planner") != 1 {
			t.Errorf("planner call count = %d, want 1", provider.CallCount("planner"))
		}
	})

	t.Run("failed_terminal_preserves_origin_trace_lineage", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))
		originTraceID := "trace-failed-lineage-mapping"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    originTraceID,
			Payload:    []byte(`{"title":"failed lineage mapping"}`),
		})

		provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
			"planner": {
				{Content: "Task processed successfully.<COMPLETE>"},
			},
			"processor": {
				{Content: "Task execution failed.<FAILED>"},
			},
		})
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride:    provider,
			ScriptCommandRunner: support.NewStaticSuccessCommandRunner("script-output-ok"),
		}, 15*time.Second)

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

	t.Run("dispatch_events_reference_terminal_work_identity", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
		traceID := "trace-dispatch-event-mapping"
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    traceID,
			Payload:    []byte(`{"title":"dispatch event mapping"}`),
		})

		provider := testutil.NewMockProvider(
			workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		)
		_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
			10*time.Second,
		)

		terminal := support.WorkCustomerLocation("task", "complete")
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
		assertDispatchEventsReferenceTerminalWork(t, events, listed, terminal, []string{traceID})
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
