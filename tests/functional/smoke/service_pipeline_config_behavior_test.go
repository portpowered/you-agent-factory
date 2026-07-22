package smoke

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServicePipelineConfigBehavior_SimplePipelineCompletesOneTask(t *testing.T) {
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	writeSharedServicePipelineWorkerConfig(t, dir, "worker-a")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"simple service smoke"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Simple pipeline done. COMPLETE"},
	)
	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want one terminal work item", status.Categories)
	}

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
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want one terminal work item", status.Categories)
	}

	if got := provider.CallCount(); got != 2 {
		t.Fatalf("provider call count = %d, want 2", got)
	}
}

func writeSharedServicePipelineWorkerConfig(t *testing.T, dir, workerName string) {
	t.Helper()

	support.WriteAgentConfig(t, dir, workerName, support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
}
