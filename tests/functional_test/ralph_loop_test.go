package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestRalphLoop_ConvergesOnReviewerAccept verifies the happy path: executor
// produces work, reviewer accepts on first review, token reaches complete.
func TestRalphLoop_ConvergesOnReviewerAccept(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "ralph_loop"))

	testutil.WriteSeedFile(t, dir, "story", []byte(`{"title": "implement feature"}`))

	work := make(map[string][]interfaces.InferenceResponse)

	work["executor-worker"] = []interfaces.InferenceResponse{
		{Content: "code with missing error handling <COMPLETE>"},
	}

	work["reviewer-worker"] = []interfaces.InferenceResponse{
		{Content: "code with missing error handling <COMPLETE>"},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
	)

	h.RunUntilComplete(t, 10*time.Second)

	if provider.CallCount("executor-worker") != 1 {
		t.Errorf("expected executor called 1 time, got %d", provider.CallCount("executor-worker"))
	}
	if provider.CallCount("reviewer-worker") != 1 {
		t.Errorf("expected reviewer called 1 time, got %d", provider.CallCount("reviewer-worker"))
	}

	h.Assert().
		PlaceTokenCount("story:complete", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:failed")
}

// TestRalphLoop_IteratesOnRejectionThenConverges verifies that when the
// reviewer rejects, the token loops back to the executor for another attempt,
// and eventually converges when the reviewer accepts.
func TestRalphLoop_IteratesOnRejectionThenConverges(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "ralph_loop"))

	testutil.WriteSeedFile(t, dir, "story", []byte(`{"title": "iterate and converge"}`))

	work := make(map[string][]interfaces.InferenceResponse)

	work["executor-worker"] = []interfaces.InferenceResponse{
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
	}

	work["reviewer-worker"] = []interfaces.InferenceResponse{
		{Content: "missing error handling"},
		{Content: "missing error handling"},
		{Content: "code with missing error handling <COMPLETE>"},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
	)

	h.RunUntilComplete(t, 10*time.Second)

	if provider.CallCount("executor-worker") != 3 {
		t.Errorf("expected executor called 3 times, got %d", provider.CallCount("executor-worker"))
	}
	if provider.CallCount("reviewer-worker") != 3 {
		t.Errorf("expected reviewer called 3 times, got %d", provider.CallCount("reviewer-worker"))
	}

	h.Assert().
		PlaceTokenCount("story:complete", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:failed")
}

// TestRalphLoop_GuardedReviewLoopBreakerTerminatesInfiniteLoop verifies that
// when the reviewer always rejects, the guarded review loop breaker terminates
// the loop at max_visits and routes the token to failed.
func TestRalphLoop_GuardedReviewLoopBreakerTerminatesInfiniteLoop(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "ralph_loop"))

	testutil.WriteSeedFile(t, dir, "story", []byte(`{"title": "infinite loop test"}`))

	work := make(map[string][]interfaces.InferenceResponse)

	work["executor-worker"] = []interfaces.InferenceResponse{
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
	}

	work["reviewer-worker"] = []interfaces.InferenceResponse{
		{Content: "missing error handling"},
		{Content: "missing error handling"},
		{Content: "missing error handling"},
		{Content: "missing error handling"},
		{Content: "missing error handling"},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// The guarded review loop breaker max_visits=3 should terminate after 3 reviewer calls.
	if provider.CallCount("reviewer-worker") != 3 {
		t.Errorf("expected reviewer called exactly 3 times (max_visits), got %d", provider.CallCount("reviewer-worker"))
	}

	h.Assert().
		PlaceTokenCount("story:failed", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "reviewer-loop-breaker", "story:failed")
}

// TestRalphLoop_TemplateFieldsResolvePerIteration verifies that the executor
// workstation's parameterized working_directory and env fields are resolved
// from token tags and passed to the dispatch on each iteration.
func TestRalphLoop_TemplateFieldsResolvePerIteration(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "ralph_loop"))
	setWorkingDirectory(t, dir)

	// Write seed file with tags that feed into working_directory and env templates.
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "story",
		Payload:    []byte(`{"title": "template test"}`),
		Tags: map[string]string{
			"project":      "inventory-service",
			"branch":       "ralph/ralph-loop",
			"iteration_id": "iter-001",
		},
	})

	work := make(map[string][]interfaces.InferenceResponse)

	work["executor-worker"] = []interfaces.InferenceResponse{
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with missing error handling <COMPLETE>"},
	}

	work["reviewer-worker"] = []interfaces.InferenceResponse{
		{Content: "missing error handling"},
		{Content: "looks good<COMPLETE>"},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Must have at least 2 executor dispatches (proving templates work across iterations).
	if provider.CallCount("executor-worker") != 2 {
		t.Fatalf("expected at least 2 executor dispatches, got %d", provider.CallCount("executor-worker"))
	}

	// Verify each dispatch has WorkingDirTemplate set and tags present.
	for i, dispatch := range provider.Calls("executor-worker") {
		if dispatch.WorkingDirectory == "" {
			t.Errorf("dispatch %d: expected WorkingDirectory to be set, got empty", i)
		} else {
			expectedDir := resolvedRuntimePath(dir, "/workspaces/ralph-loop-fixture/ralph/ralph-loop")
			if dispatch.WorkingDirectory != expectedDir {
				t.Errorf("dispatch %d: expected WorkingDirectory '%s', got '%s'", i, expectedDir, dispatch.WorkingDirectory)
			}
		}
		if dispatch.EnvVars["PROJECT"] != "ralph-loop-fixture" {
			t.Errorf("dispatch %d: expected env PROJECT=ralph-loop-fixture, got %s", i, dispatch.EnvVars["PROJECT"])
		}
		if dispatch.EnvVars["ITERATION_ID"] != "iter-001" {
			t.Errorf("dispatch %d: expected env ITERATION_ID=iter-001, got %s", i, dispatch.EnvVars["ITERATION_ID"])
		}
	}
}
