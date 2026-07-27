package dispatch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
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
// preseeded and late-submit admission, archive-terminal completion, single- and
// two-stage pipelines, and ideation happy-path coverage without inspecting
// internal Petri markings.
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
