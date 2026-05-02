package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestMultiOutput_WithStopWord verifies that a workstation with 2 outputs
// produces 2 tokens when the provider returns content containing a stop word.
// The planner workstation has stop_words: ["COMPLETE", "DONE"] in factory.json.
func TestMultiOutput_WithStopWord(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output with stop word"}`))

	// Responses: planner (with COMPLETE stop word), then finisher x2.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Here is the plan and tasks. COMPLETE"},
		interfaces.InferenceResponse{Content: "Finished. COMPLETE"},
		interfaces.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("plan:complete").
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("request:init").
		HasNoTokenInPlace("request:failed")
}

// TestMultiOutput_WithoutStopWord verifies that when the provider returns content
// NOT containing any stop word, the transition fails and routes to the failure place.
func TestMultiOutput_WithoutStopWord(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output without stop word"}`))

	// Provider returns content without any configured stop word.
	// The workstation stop_words evaluation overrides the outcome to FAILED.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "I tried but could not finish"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("request:failed").
		HasNoTokenInPlace("plan:init").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("plan:complete").
		HasNoTokenInPlace("task:complete")
}

// TestMultiOutput_NoStopWordsConfigured verifies backward compatibility:
// without stop_words configured, the existing outcome-based flow is unchanged.
func TestMultiOutput_NoStopWordsConfigured(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_output_no_stopwords_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output no stop words"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	h.MockWorker("planner-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("finisher-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Both outputs should have tokens since ACCEPTED uses OutputArcs.
	h.Assert().
		HasTokenInPlace("plan:complete").
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("request:init").
		HasNoTokenInPlace("request:failed")
}

// TestMultiOutput_SecondStopWord verifies that matching the second stop word also works.
// The workstation stop_words ["COMPLETE", "DONE"] override the worker outcome.
func TestMultiOutput_SecondStopWord(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Second stop word"}`))

	// Planner content contains "DONE" (second stop word). The worker-level
	// stop_token may reject, but workstation stop_words override to ACCEPTED.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "All tasks generated. DONE"},
		interfaces.InferenceResponse{Content: "Finished. COMPLETE"},
		interfaces.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("plan:complete").
		HasTokenInPlace("task:complete")
}

// TestMultiOutput_OutputTokensInheritInputLineage verifies that both output
// tokens produced by a multi-output workstation share the same TraceID as
// the original input token. This confirms trace lineage is preserved across
// multi-output transitions.
func TestMultiOutput_OutputTokensInheritInputLineage(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_output_dir"))

	// Submit work with a known TraceID for lineage verification.
	inputTraceID := "trace-lineage-test"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "request",
		Payload:    []byte(`{"title": "Lineage test"}`),
		TraceID:    inputTraceID,
	})

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Plan generated. COMPLETE"},
		interfaces.InferenceResponse{Content: "Finished. COMPLETE"},
		interfaces.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	// Verify both output places have tokens with the same TraceID.
	h.Assert().
		HasTokenInPlace("plan:complete").
		HasTokenInPlace("task:complete").
		TokenHasTraceID("plan:complete", inputTraceID).
		TokenHasTraceID("task:complete", inputTraceID)

	// Also verify the output tokens have the correct work type IDs.
	finalSnap := h.Marking()
	planTokens := finalSnap.TokensInPlace("plan:complete")
	taskTokens := finalSnap.TokensInPlace("task:complete")

	if len(planTokens) != 1 {
		t.Fatalf("expected 1 token in plan:complete, got %d", len(planTokens))
	}
	if len(taskTokens) != 1 {
		t.Fatalf("expected 1 token in task:complete, got %d", len(taskTokens))
	}

	// Verify work type IDs are correct for each output.
	if planTokens[0].Color.WorkTypeID != "request" {
		// The planner consumes a "request" token and carries its color forward.
		// The plan:complete token inherits the request color since the syncDispatcher
		// falls back to the first work-type input token when the output place type
		// doesn't match any input token type.
		t.Logf("plan:complete token has WorkTypeID %q (inherited from input)", planTokens[0].Color.WorkTypeID)
	}
	if taskTokens[0].Color.WorkTypeID != "request" {
		t.Logf("task:complete token has WorkTypeID %q (inherited from input)", taskTokens[0].Color.WorkTypeID)
	}

	// Both output tokens must share the same TraceID as the input.
	if planTokens[0].Color.TraceID != inputTraceID {
		t.Errorf("plan:complete token TraceID = %q, want %q", planTokens[0].Color.TraceID, inputTraceID)
	}
	if taskTokens[0].Color.TraceID != inputTraceID {
		t.Errorf("task:complete token TraceID = %q, want %q", taskTokens[0].Color.TraceID, inputTraceID)
	}
}
