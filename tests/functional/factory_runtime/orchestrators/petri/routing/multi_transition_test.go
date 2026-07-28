package routing

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
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
