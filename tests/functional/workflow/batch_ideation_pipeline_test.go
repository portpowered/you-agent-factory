package workflow

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{
		"story:complete": 3, "idea:init": 0, "prd:init": 0, "story:init": 0, "story:in-review": 0,
	})
	assertCompletedStoryTraces(t, listedWork, traceIDs)

	// Total provider calls: 3 planner + 3 executor + 3 reviewer = 9.
	if provider.CallCount() != 9 {
		t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
	}
	assertSerialPipelineProviderCallsUseAgentsMD(t, provider.Calls())

	// Resource tokens returned: 2 tokens in agent-slot:available (capacity=2).
	assertResourceAvailability(t, session, "agent-slot", 2)
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

	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 30*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{
		"story:complete": 3, "idea:init": 0, "prd:init": 0, "story:init": 0, "story:in-review": 0,
	})
	assertCompletedStoryTraces(t, listedWork, traceIDs)

	// Total provider calls: 3 planner + 3 executor + 3 reviewer = 9.
	if provider.CallCount() != 9 {
		t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
	}
	assertSerialPipelineProviderCallsUseAgentsMD(t, provider.Calls())

	// Resource tokens properly released: exactly 1 token in agent-slot:available (capacity=1).
	assertResourceAvailability(t, session, "agent-slot", 1)
}

func assertCompletedStoryTraces(t *testing.T, response factoryapi.ListWorkResponse, traceIDs []string) {
	t.Helper()
	wants := make(map[string]bool, len(traceIDs))
	for _, traceID := range traceIDs {
		wants[traceID] = true
	}
	found := map[string]bool{}
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != "story" || item.State == nil || item.State.Name != "complete" {
			continue
		}
		if item.TraceId == nil || !wants[*item.TraceId] {
			t.Errorf("unexpected story:complete trace ID %#v", item.TraceId)
			continue
		}
		found[*item.TraceId] = true
	}
	for traceID := range wants {
		if !found[traceID] {
			t.Errorf("listed Work missing story:complete trace %q", traceID)
		}
	}
}

func assertResourceAvailability(t *testing.T, session factoryapi.FactorySession, name string, want int) {
	t.Helper()
	for _, resource := range session.Runtime.Usage.Resources {
		if resource.Name == name {
			if resource.Available != want || resource.Total != want {
				t.Errorf("resource %s usage = %#v, want %d available and total", name, resource, want)
			}
			return
		}
	}
	t.Errorf("session usage missing resource %q", name)
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
