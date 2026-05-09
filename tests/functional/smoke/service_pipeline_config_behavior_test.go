package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServicePipelineConfigBehavior_SimplePipelineCompletesOneTask(t *testing.T) {
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	writeSharedServicePipelineWorkerConfig(t, dir, "worker-a")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"simple service smoke"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Simple pipeline done. COMPLETE"},
	)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
	assertCompletedFactoryState(t, harness)

	if got := provider.CallCount(); got != 1 {
		t.Fatalf("provider call count = %d, want 1", got)
	}
}

func TestServicePipelineConfigBehavior_TwoStagePipelineCompletesAcrossBothWorkers(t *testing.T) {
	dir := support.ScaffoldFactory(t, twoStageServicePipelineConfig())
	writeSharedServicePipelineWorkerConfig(t, dir, "worker-a")
	writeSharedServicePipelineWorkerConfig(t, dir, "worker-b")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"two-stage service smoke"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed")
	assertCompletedFactoryState(t, harness)

	if got := provider.CallCount(); got != 2 {
		t.Fatalf("provider call count = %d, want 2", got)
	}
}

func writeSharedServicePipelineWorkerConfig(t *testing.T, dir, workerName string) {
	t.Helper()

	support.WriteAgentConfig(t, dir, workerName, support.BuildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))
}

func assertCompletedFactoryState(t *testing.T, harness *testutil.ServiceTestHarness) {
	t.Helper()

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %s, want %s", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
}
