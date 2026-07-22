//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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

	providerV1 := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})

	sessionV1 := support.RunFactoryToCompletion(t, v1Dir, providerV1, 10*time.Second)
	assertWorkflowSessionPlaces(t, sessionV1, map[string]int{
		"task:complete": 1, "task:init": 0, "task:processing": 0,
	})

	if providerV1.CallCount("processor") != 1 {
		t.Errorf("v1: expected processor called 1 time, got %d", providerV1.CallCount("processor"))
	}
	if providerV1.CallCount("finalizer") != 1 {
		t.Errorf("v1: expected finalizer called 1 time, got %d", providerV1.CallCount("finalizer"))
	}

	v2Dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v2_dir"))
	testutil.WriteSeedFile(t, v2Dir, "task", []byte("v2 work item"))

	providerV2 := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"reviewer":  {{Content: "Reviewed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})

	sessionV2 := support.RunFactoryToCompletion(t, v2Dir, providerV2, 10*time.Second)
	assertWorkflowSessionPlaces(t, sessionV2, map[string]int{
		"task:complete": 1, "task:init": 0, "task:processing": 0, "task:in-review": 0,
	})

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
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"drafter":  {{Content: "draft COMPLETE"}, {Content: "revised draft COMPLETE"}},
		"approver": {{Content: "needs revision"}, {Content: "approved COMPLETE"}},
	})
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"doc:complete": 1, "doc:init": 0, "doc:processing": 0,
	})

	if got := provider.CallCount("drafter"); got != 2 {
		t.Errorf("expected drafter called 2 times, got %d", got)
	}
	if got := provider.CallCount("approver"); got != 2 {
		t.Errorf("expected approver called 2 times, got %d", got)
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

	providerA := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Done. COMPLETE"}},
	})

	sessionA := support.RunFactoryToCompletion(t, dirA, providerA, 10*time.Second)
	assertWorkflowSessionPlaces(t, sessionA, map[string]int{"task:complete": 1})

	dirB := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v1_dir"))
	testutil.WriteSeedFile(t, dirB, "task", []byte("task for B"))

	providerB := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})

	sessionB := support.RunFactoryToCompletion(t, dirB, providerB, 10*time.Second)
	assertWorkflowSessionPlaces(t, sessionB, map[string]int{"task:complete": 1})
	assertWorkflowSessionPlaces(t, sessionA, map[string]int{"task:complete": 1, "task:init": 0})
}
