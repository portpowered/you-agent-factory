package functional_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestDispatcherLifecycle_IdeaToArchive exercises the full dispatcher lifecycle:
//
//	idea → plan (produces prd) → execute (produces code-change) → review → archive-gate → archived
//
// This verifies cross-work-type token production at the plan and execute stages,
// and confirms that the archived code-change token traces back to the original idea
// via a shared TraceID.
func TestDispatcherLifecycle_IdeaToArchive(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dispatcher_lifecycle_dir"))

	// Write seed file with a known TraceID for lineage verification.
	originTraceID := "trace-idea-lifecycle-test"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "improve onboarding flow"}`),
		TraceID:    originTraceID,
	})

	// Interleaved responses: processor (COMPLETE→accept), reviewer (no ACCEPTED→reject), ...
	work := make(map[string][]interfaces.InferenceResponse)

	work["planner"] = []interfaces.InferenceResponse{
		{Content: "success<COMPLETE>"},
	}

	work["executor"] = []interfaces.InferenceResponse{
		{Content: "success<COMPLETE>"},
	}

	work["reviewer"] = []interfaces.InferenceResponse{
		{Content: "success<COMPLETE>"},
	}

	work["archiver"] = []interfaces.InferenceResponse{
		{Content: "success<COMPLETE>"},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 1000*time.Second)

	// Token ends in archived terminal state.
	h.Assert().
		HasTokenInPlace("code-change:archived").
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:failed").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:approved").
		HasNoTokenInPlace("code-change:failed")

	types := []string{
		"reviewer",
		"planner",
		"executor",
		"archiver",
	}
	for _, d := range types {
		if len(provider.Calls(d)) != 1 {
			t.Errorf("expected %s called 1 time, got %d", d, len(provider.Calls(d)))
		}
	}

	// Verify token lineage: the archived code-change shares the original idea's TraceID.
	h.Assert().TokenHasTraceID("code-change:archived", originTraceID)
}

// TestDispatcherLifecycle_PlannerFailure verifies that when the planner fails,
// the idea token moves to the failed state and no downstream tokens are created.
func TestDispatcherLifecycle_PlannerFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dispatcher_lifecycle_dir"))

	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title": "broken idea"}`))

	work := make(map[string][]interfaces.InferenceResponse)

	work["planner"] = []interfaces.InferenceResponse{
		{Content: "failed"},
	}

	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:archived")
}

// NOTE: rejection paths.... we are currently not propagating the right type of content.
// The input towards the executor is an idea... not a prd. This is a bug.
// TODO: we need to test the rejection channel si failing as expected.

// TestDispatcherLifecycle_ExecutorFailure verifies that when the executor fails,
// the prd token moves to failed state and no code-change tokens are created.
func TestDispatcherLifecycle_ExecutorFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dispatcher_lifecycle_dir"))

	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title": "failing executor"}`))

	// Interleaved responses: processor (COMPLETE→accept), reviewer (no ACCEPTED→reject), ...
	work := make(map[string][]testutil.WorkResponse)

	work["planner"] = []testutil.WorkResponse{
		{Content: "success<COMPLETE>"},
	}

	work["executor"] = []testutil.WorkResponse{
		{Content: "failed", Error: errors.New("failed executors")},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 1000*time.Second)

	// Planner succeeded (idea consumed), but executor failed (prd → failed).
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasTokenInPlace("prd:failed").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:archived")
}
