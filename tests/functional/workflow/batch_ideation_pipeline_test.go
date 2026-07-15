package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// seedBatchIdeas writes idea seed files into the fixture directory for multiple
// trace IDs. Returns the trace IDs used.
func seedBatchIdeas(t *testing.T, dir string, count int) []string {
	t.Helper()
	traceIDs := make([]string, count)
	for i := range count {
		traceIDs[i] = fmt.Sprintf("trace-batch-idea-%03d", i+1)
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceIDs[i],
			Payload:    fmt.Appendf(nil, `{"title":"batch idea %d"}`, i+1),
		})
	}
	return traceIDs
}

// TestBatchIdeationPipeline_ConcurrencyLimit2 verifies that three idea Work
// requests seeded via submit files independently progress through the full
// ideation pipeline (idea → prd → story → complete) with resource-limited
// concurrency (agent-slot capacity=2) throttling execution without deadlock.
//
// Each idea pipeline requires: planner(1) + executor(1) + reviewer(1) = 3
// provider calls. Converter is LOGICAL_MOVE — no provider call.
// Total: 3 ideas × 3 calls = 9 provider calls.
func TestBatchIdeationPipeline_ConcurrencyLimit2(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "batch_ideation_pipeline"))

	// Every response contains ALL stop tokens (COMPLETE + ACCEPTED) since
	// execution order is non-deterministic across concurrent pipelines.
	var responses []workerexecution.InferenceResponse
	for range 15 {
		responses = append(responses, workerexecution.InferenceResponse{
			Content: "Done. COMPLETE ACCEPTED",
		})
	}
	provider := testutil.NewMockProvider(responses...)

	// Seed 3 ideas before harness construction.
	traceIDs := seedBatchIdeas(t, dir, 3)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion.
	h.RunUntilComplete(t, 10*time.Second)

	snap := h.Marking()

	// All 3 stories reach story:complete without deadlock.
	storyCompleteTokens := snap.TokensInPlace("story:complete")
	if len(storyCompleteTokens) != 3 {
		t.Fatalf("expected 3 tokens in story:complete, got %d", len(storyCompleteTokens))
	}

	// No tokens remain in intermediate places.
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review")

	// Each completed story Work item preserves one of the seeded trace IDs.
	foundTraces := make(map[string]bool)
	for _, tok := range storyCompleteTokens {
		if tok.Color.DataType != factorytoken.DataTypeWork {
			t.Errorf("story:complete token %s: expected work DataType, got %q", tok.ID, tok.Color.DataType)
			continue
		}
		if tok.Color.TraceID == "" {
			t.Errorf("story:complete token %s: expected TraceID lineage", tok.ID)
			continue
		}
		if !slices.Contains(traceIDs, tok.Color.TraceID) {
			t.Errorf("unexpected TraceID %q in story:complete", tok.Color.TraceID)
			continue
		}
		foundTraces[tok.Color.TraceID] = true
	}
	for _, traceID := range traceIDs {
		if !foundTraces[traceID] {
			t.Errorf("expected TraceID %q in story:complete, not found", traceID)
		}
	}

	// Total provider calls: 3 planner + 3 executor + 3 reviewer = 9.
	if provider.CallCount() != 9 {
		t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
	}
	assertSerialPipelineProviderCallsUseAgentsMD(t, provider.Calls())

	// Resource tokens returned: 2 tokens in agent-slot:available (capacity=2).
	resourceTokens := snap.TokensInPlace("agent-slot:available")
	if len(resourceTokens) != 2 {
		t.Errorf("expected 2 resource tokens in agent-slot:available, got %d", len(resourceTokens))
	}
}

// TestSerialIdeationPipeline_ConcurrencyLimit1 verifies that with agent-slot
// capacity of 1, three idea Work requests are processed serially so only one
// agent runs at a time and all Work completes without deadlock.
//
// Same topology as TestBatchIdeationPipeline_ConcurrencyLimit2 but capacity=1.
// Total: 3 ideas × 3 provider calls = 9.
func TestSerialIdeationPipeline_ConcurrencyLimit1(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "serial_ideation_pipeline"))

	// Every response contains ALL stop tokens since execution order is
	// non-deterministic across serialized pipelines.
	var responses []workerexecution.InferenceResponse
	for range 15 {
		responses = append(responses, workerexecution.InferenceResponse{
			Content: "Done. COMPLETE ACCEPTED",
		})
	}
	provider := testutil.NewMockProvider(responses...)

	// Seed 3 ideas before harness construction.
	traceIDs := seedBatchIdeas(t, dir, 3)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 1000*time.Second)

	snap := h.Marking()

	// All 3 stories reach story:complete without deadlock.
	storyCompleteTokens := snap.TokensInPlace("story:complete")
	if len(storyCompleteTokens) != 3 {
		t.Fatalf("expected 3 tokens in story:complete, got %d", len(storyCompleteTokens))
	}

	// No tokens remain in intermediate places.
	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review")

	// Each idea lineage (TraceID) is independent and traceable through work tokens.
	foundTraces := make(map[string]bool)
	for _, tok := range storyCompleteTokens {
		if tok.Color.DataType == factorytoken.DataTypeWork && tok.Color.TraceID != "" {
			foundTraces[tok.Color.TraceID] = true
		}
	}
	for traceID := range foundTraces {
		if !slices.Contains(traceIDs, traceID) {
			t.Errorf("unexpected TraceID %q in story:complete", traceID)
		}
	}

	// Total provider calls: 3 planner + 3 executor + 3 reviewer = 9.
	if provider.CallCount() != 9 {
		t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
	}
	assertSerialPipelineProviderCallsUseAgentsMD(t, provider.Calls())

	// Resource tokens properly released: exactly 1 token in agent-slot:available (capacity=1).
	resourceTokens := snap.TokensInPlace("agent-slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in agent-slot:available, got %d", len(resourceTokens))
	}
}

func assertSerialPipelineProviderCallsUseAgentsMD(t *testing.T, calls []workerexecution.ProviderInferenceRequest) {
	t.Helper()

	expectedPromptsByWorker := map[string]string{
		"planner":  "You are a planner. Convert ideas into PRDs.",
		"executor": "You are an executor. Implement the story.",
		"reviewer": "You are a reviewer. Review the implementation and accept or reject.",
	}
	seen := make(map[string]bool, len(expectedPromptsByWorker))
	for _, call := range calls {
		expectedPrompt, ok := expectedPromptsByWorker[call.WorkerType]
		if !ok {
			continue
		}
		seen[call.WorkerType] = true
		if call.Model != "test-model" {
			t.Errorf("%s provider call model: want test-model from AGENTS.md, got %q", call.WorkerType, call.Model)
		}
		if !strings.Contains(call.SystemPrompt, expectedPrompt) {
			t.Errorf("%s provider call system prompt does not include AGENTS.md body %q; got %q", call.WorkerType, expectedPrompt, call.SystemPrompt)
		}
	}
	for workerType := range expectedPromptsByWorker {
		if !seen[workerType] {
			t.Errorf("expected at least one provider call for worker %q", workerType)
		}
	}
}
