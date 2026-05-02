//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestMultiOutput_WithStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output stop-word workflow sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output with stop word"}`))

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

func TestMultiOutput_WithoutStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output failure routing sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output without stop word"}`))

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

func TestMultiOutput_NoStopWordsConfigured(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output no-stopword harness sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_no_stopwords_dir"))
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

	h.Assert().
		HasTokenInPlace("plan:complete").
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("request:init").
		HasNoTokenInPlace("request:failed")
}

func TestMultiOutput_SecondStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output alternate stop-word sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Second stop word"}`))

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

func TestMultiOutput_OutputTokensInheritInputLineage(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output lineage propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))

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

	h.Assert().
		HasTokenInPlace("plan:complete").
		HasTokenInPlace("task:complete").
		TokenHasTraceID("plan:complete", inputTraceID).
		TokenHasTraceID("task:complete", inputTraceID)

	finalSnap := h.Marking()
	planTokens := finalSnap.TokensInPlace("plan:complete")
	taskTokens := finalSnap.TokensInPlace("task:complete")

	if len(planTokens) != 1 {
		t.Fatalf("expected 1 token in plan:complete, got %d", len(planTokens))
	}
	if len(taskTokens) != 1 {
		t.Fatalf("expected 1 token in task:complete, got %d", len(taskTokens))
	}
	if planTokens[0].Color.WorkTypeID != "request" {
		t.Logf("plan:complete token has WorkTypeID %q (inherited from input)", planTokens[0].Color.WorkTypeID)
	}
	if taskTokens[0].Color.WorkTypeID != "request" {
		t.Logf("task:complete token has WorkTypeID %q (inherited from input)", taskTokens[0].Color.WorkTypeID)
	}
	if planTokens[0].Color.TraceID != inputTraceID {
		t.Errorf("plan:complete token TraceID = %q, want %q", planTokens[0].Color.TraceID, inputTraceID)
	}
	if taskTokens[0].Color.TraceID != inputTraceID {
		t.Errorf("task:complete token TraceID = %q, want %q", taskTokens[0].Color.TraceID, inputTraceID)
	}
}
