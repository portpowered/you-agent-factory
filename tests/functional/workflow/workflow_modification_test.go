package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

// TestWorkflowModificationAndReload validates that different workflow versions
// produce correct results when loaded from config:
//
//	Given: V1 config (2-transition pipeline) and V2 config (3-transition pipeline with review)
//	When:  work is submitted to each version independently
//	Then:  V1 completes via 2 transitions, V2 completes via 3 transitions
func TestWorkflowModificationAndReload(t *testing.T) {
	if testing.Short() {
		t.Skip("slow workflow reload smoke")
	}

	v1Dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v1_dir"))
	testutil.WriteSeedFile(t, v1Dir, "task", []byte("v1 work item"))

	providerV1 := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})

	h1 := testutil.NewServiceTestHarness(t, v1Dir,
		testutil.WithProvider(providerV1),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h1.RunUntilComplete(t, 10*time.Second)

	h1.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		TokenCount(1)

	if providerV1.CallCount("processor") != 1 {
		t.Errorf("v1: expected processor called 1 time, got %d", providerV1.CallCount("processor"))
	}
	if providerV1.CallCount("finalizer") != 1 {
		t.Errorf("v1: expected finalizer called 1 time, got %d", providerV1.CallCount("finalizer"))
	}

	v2Dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v2_dir"))
	testutil.WriteSeedFile(t, v2Dir, "task", []byte("v2 work item"))

	providerV2 := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"reviewer":  {{Content: "Reviewed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})

	h2 := testutil.NewServiceTestHarness(t, v2Dir,
		testutil.WithProvider(providerV2),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h2.RunUntilComplete(t, 10*time.Second)

	h2.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:in-review").
		TokenCount(1)

	if providerV2.CallCount("processor") != 1 {
		t.Errorf("v2: expected processor called 1 time, got %d", providerV2.CallCount("processor"))
	}
	if providerV2.CallCount("reviewer") != 1 {
		t.Errorf("v2: expected reviewer called 1 time, got %d", providerV2.CallCount("reviewer"))
	}
	if providerV2.CallCount("finalizer") != 1 {
		t.Errorf("v2: expected finalizer called 1 time, got %d", providerV2.CallCount("finalizer"))
	}
}

// TestWorkflowModificationRejectionLoop validates that a v2 workflow
// with a rejection loop works correctly when loaded from config:
//
//	Given: V2 config with rejection routing from approve back to init
//	When:  approver rejects once, then accepts
//	Then:  token completes after one rejection loop
func TestWorkflowModificationRejectionLoop(t *testing.T) {
	support.SkipLongFunctional(t, "slow workflow-modification rejection-loop sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v2_rejection_dir"))
	testutil.WriteSeedFile(t, dir, "doc", []byte("needs-revision draft"))
	h := testutil.NewServiceTestHarness(t, dir)

	drafterMock := h.MockWorker("drafter",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	approverMock := h.MockWorker("approver",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Feedback: "needs revision"},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("doc:complete").
		HasNoTokenInPlace("doc:init").
		HasNoTokenInPlace("doc:processing").
		TokenCount(1)

	if drafterMock.CallCount() != 2 {
		t.Errorf("expected drafter called 2 times, got %d", drafterMock.CallCount())
	}
	if approverMock.CallCount() != 2 {
		t.Errorf("expected approver called 2 times, got %d", approverMock.CallCount())
	}
}

// TestWorkflowModificationPreservesIndependentWorkflows verifies that
// running two different configs independently produces isolated results:
//
//	Given: Two independent workflow configs
//	When:  each runs work items to completion
//	Then:  neither workflow's results are affected by the other
func TestWorkflowModificationPreservesIndependentWorkflows(t *testing.T) {
	if testing.Short() {
		t.Skip("slow independent-workflow reload smoke")
	}

	dirA := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	testutil.WriteSeedFile(t, dirA, "task", []byte("item for A"))

	providerA := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "Done. COMPLETE"}},
	})

	hA := testutil.NewServiceTestHarness(t, dirA,
		testutil.WithProvider(providerA),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	hA.RunUntilComplete(t, 10*time.Second)

	hA.Assert().HasTokenInPlace("task:complete").TokenCount(1)

	dirB := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v1_dir"))
	testutil.WriteSeedFile(t, dirB, "task", []byte("task for B"))

	providerB := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})

	hB := testutil.NewServiceTestHarness(t, dirB,
		testutil.WithProvider(providerB),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	hB.RunUntilComplete(t, 10*time.Second)

	hB.Assert().HasTokenInPlace("task:complete").TokenCount(1)

	hA.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)
}
