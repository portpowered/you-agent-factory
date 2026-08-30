//go:build functionallong

package repeater

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater proves that a
// guarded loop breaker terminates a repeatedly rejected repeater, leaving Work
// in a failed public state with the expected loop-breaker dispatch route.
func TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater guarded loop-breaker sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "exhaustion test"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker": {
			{Content: "retry"}, {Content: "retry"}, {Content: "retry"}, {Content: "retry"},
			{Content: "retry"}, {Content: "retry"}, {Content: "retry"},
		},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second, 1)
	assertRepeaterWorkStates(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:complete": 0})
	assertPublicDispatchRoute(t, events, "executor-loop-breaker", "task:failed")
}

// TestRepeater_RefiresOnRejectedStopsOnAccepted proves that a repeater
// workstation refires Work while worker outputs omit the accept stop signal
// and stops with Work in a completed public state once an accepting output
// arrives, leaving no remaining init Work.
func TestRepeater_RefiresOnRejectedStopsOnAccepted(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater rejection-to-acceptance sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "repeater test"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker":   {{Content: "retry"}, {Content: "retry"}, {Content: "done COMPLETE"}},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second, 1)

	if provider.CallCount("exec-worker") != 3 {
		t.Errorf("exec-worker call count = %d, want 3 reject-then-accept iterations", provider.CallCount("exec-worker"))
	}
	assertRepeaterWorkStates(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})
}

// TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness proves that a
// repeater releases held resources between non-accepting iterations through the
// public functional service harness so a later accepting output completes Work.
func TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater service-harness resource-release sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_resource"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "service resource repeater test"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Still working"},
		workerexecution.InferenceResponse{Content: "Almost there"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finalized. COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second, 1)
	assertRepeaterWorkStates(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})

	if provider.CallCount() != 4 {
		t.Errorf("provider call count = %d, want 4 reject-then-accept iterations", provider.CallCount())
	}
}

// TestWorkstationStopWords_ThroughCustomerProcess proves that each configured
// stop word accepts or rejects Work through the customer process boundary,
// including factory-json, frontmatter, and workstation override policies.
func TestWorkstationStopWords_ThroughCustomerProcess(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word customer-boundary sweep")

	tests := []struct {
		name        string
		fixture     string
		title       string
		response    string
		wantPlace   string
		emptyPlaces []string
	}{
		{
			name: "FactoryJSON_Success", fixture: "workstation_stopwords_factory_dir",
			title: "factory stop word success", response: "Work completed successfully. COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "FactoryJSON_SecondWord", fixture: "workstation_stopwords_factory_dir",
			title: "factory stop word second", response: "All tasks finished. DONE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "FactoryJSON_Failure", fixture: "workstation_stopwords_factory_dir",
			title: "factory stop word failure", response: "I tried but could not finish the work",
			wantPlace: "task:failed", emptyPlaces: []string{"task:init", "task:complete"},
		},
		{
			name: "Frontmatter_Success", fixture: "workstation_stopwords_frontmatter_dir",
			title: "frontmatter stop word success", response: "Work completed successfully. COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "Frontmatter_SecondWord", fixture: "workstation_stopwords_frontmatter_dir",
			title: "frontmatter stop word second", response: "All tasks finished. DONE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "Frontmatter_Failure", fixture: "workstation_stopwords_frontmatter_dir",
			title: "frontmatter stop word failure", response: "I tried but could not finish the work",
			wantPlace: "task:failed", emptyPlaces: []string{"task:init", "task:complete"},
		},
		{
			name: "Override_StationAcceptsWorkerRejects", fixture: "workstation_stopwords_override_dir",
			title: "station overrides worker", response: "The work is finished. STATION_COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "Override_StationRejectsWorkerAccepts", fixture: "workstation_stopwords_override_dir",
			title: "station rejects worker accepts", response: "The work is done. WORKER_COMPLETE",
			wantPlace: "task:failed", emptyPlaces: []string{"task:init", "task:complete"},
		},
		{
			name: "Override_BothMatch", fixture: "workstation_stopwords_override_dir",
			title: "both match", response: "WORKER_COMPLETE and STATION_COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:failed"},
		},
		{
			name: "Override_NeitherMatch", fixture: "workstation_stopwords_override_dir",
			title: "neither match", response: "I tried but could not finish the work",
			wantPlace: "task:failed", emptyPlaces: []string{"task:complete"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, test.fixture))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"`+test.title+`"}`))
			provider := testutil.NewMockProvider(
				workerexecution.InferenceResponse{Content: test.response},
			)

			_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second, 1)
			if got := support.CountWorkAtCustomerState(listed, test.wantPlace); got != 1 {
				t.Errorf("%s token count = %d, want 1", test.wantPlace, got)
			}
			for _, placeID := range test.emptyPlaces {
				if got := support.CountWorkAtCustomerState(listed, placeID); got != 0 {
					t.Errorf("%s token count = %d, want 0", placeID, got)
				}
			}
		})
	}
}

// TestMultiOutput_WithStopWord proves that multi-output fan-out completes plan
// and task Work when the planner output includes a configured stop word.
func TestMultiOutput_WithStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output stop-word workflow sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output with stop word"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Here is the plan and tasks. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second, 3)
	assertRepeaterWorkStates(t, listed, map[string]int{
		"plan:complete": 1, "task:complete": 1, "request:init": 0, "request:failed": 0,
	})
}

// TestMultiOutput_WithoutStopWord proves that missing a required stop word
// fails the request Work and leaves downstream plan and task Work uninitiated.
func TestMultiOutput_WithoutStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output failure routing sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output without stop word"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "I tried but could not finish"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second, 1)
	assertRepeaterWorkStates(t, listed, map[string]int{
		"request:failed": 1, "plan:init": 0, "task:init": 0, "plan:complete": 0, "task:complete": 0,
	})
}

// TestMultiOutput_NoStopWordsConfigured proves that multi-output fan-out still
// completes plan and task Work when no stop words are configured on the factory.
func TestMultiOutput_NoStopWordsConfigured(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output no-stopword harness sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_no_stopwords_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output no stop words"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "planner output COMPLETE"},
		workerexecution.InferenceResponse{Content: "finisher output COMPLETE"},
		workerexecution.InferenceResponse{Content: "finisher output COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second, 3)
	assertRepeaterWorkStates(t, listed, map[string]int{
		"plan:complete": 1, "task:complete": 1, "request:init": 0, "request:failed": 0,
	})
}

// TestMultiOutput_SecondStopWord proves that an alternate configured stop word
// also accepts multi-output fan-out and completes plan and task Work.
func TestMultiOutput_SecondStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output alternate stop-word sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Second stop word"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "All tasks generated. DONE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second, 3)
	assertRepeaterWorkStates(t, listed, map[string]int{"plan:complete": 1, "task:complete": 1})
}

// TestMultiOutput_OutputTokensInheritInputLineage proves that successful
// multi-output fan-out preserves input trace identity on completed plan and
// task Work observed through the public list surface.
func TestMultiOutput_OutputTokensInheritInputLineage(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output lineage propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))

	inputTraceID := "trace-lineage-test"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "request",
		Payload:    []byte(`{"title": "Lineage test"}`),
		TraceID:    inputTraceID,
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Plan generated. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second, 3)
	assertRepeaterWorkStates(t, listed, map[string]int{"plan:complete": 1, "task:complete": 1})
	assertListedLineage(t, listed, map[string]string{"plan": inputTraceID, "task": inputTraceID})
}

// TestRalphLoop_TemplateFieldsResolvePerIteration proves that per-iteration
// template fields resolve on each executor dispatch while reject-then-accept
// still reaches story Work completion.
func TestRalphLoop_TemplateFieldsResolvePerIteration(t *testing.T) {
	support.SkipLongFunctional(t, "slow Ralph-loop template-field resolution sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "ralph_loop"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		Payload:    []byte(`{"title": "template test"}`),
		Tags: map[string]string{
			"project":      "inventory-service",
			"branch":       "ralph/ralph-loop",
			"iteration_id": "iter-001",
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"executor-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
		},
		"reviewer-worker": {
			{Content: "missing error handling"},
			{Content: "looks good<COMPLETE>"},
		},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second, 1)
	assertRepeaterWorkStates(t, listed, map[string]int{"story:complete": 1})

	if provider.CallCount("executor-worker") != 2 {
		t.Fatalf("executor-worker call count = %d, want 2 reject-then-accept iterations", provider.CallCount("executor-worker"))
	}

	expectedDir := support.ResolvedRuntimePath(dir, "/workspaces/ralph-loop-fixture/ralph/ralph-loop")
	for i, dispatch := range provider.Calls("executor-worker") {
		if dispatch.WorkingDirectory == "" {
			t.Errorf("dispatch %d: expected WorkingDirectory to be set, got empty", i)
		} else if dispatch.WorkingDirectory != expectedDir {
			t.Errorf("dispatch %d: expected WorkingDirectory %q, got %q", i, expectedDir, dispatch.WorkingDirectory)
		}
		if dispatch.EnvVars["PROJECT"] != "ralph-loop-fixture" {
			t.Errorf("dispatch %d: expected env PROJECT=ralph-loop-fixture, got %s", i, dispatch.EnvVars["PROJECT"])
		}
		if dispatch.EnvVars["ITERATION_ID"] != "iter-001" {
			t.Errorf("dispatch %d: expected env ITERATION_ID=iter-001, got %s", i, dispatch.EnvVars["ITERATION_ID"])
		}
	}
}

func assertPublicDispatchRoute(t *testing.T, events []factoryapi.FactoryEvent, transitionID, toPlaceID string) {
	t.Helper()
	var sawDispatch bool
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		sawDispatch = sawDispatch || payload.TransitionId == transitionID
	}
	if !sawDispatch {
		t.Fatalf("public events missing transition %s before terminal place %s", transitionID, toPlaceID)
	}
}

func assertListedLineage(t *testing.T, response factoryapi.ListWorkResponse, wants map[string]string) {
	t.Helper()
	seen := map[string]bool{}
	for _, item := range response.Results {
		if item.State == nil || item.State.Name != "complete" || item.WorkTypeName == nil {
			continue
		}
		want, ok := wants[*item.WorkTypeName]
		if !ok {
			continue
		}
		if item.TraceId == nil || *item.TraceId != want {
			t.Errorf("%s complete trace ID = %#v, want %q", *item.WorkTypeName, item.TraceId, want)
		}
		seen[*item.WorkTypeName] = true
	}
	for workType := range wants {
		if !seen[workType] {
			t.Errorf("listed Work missing %s:complete", workType)
		}
	}
}
