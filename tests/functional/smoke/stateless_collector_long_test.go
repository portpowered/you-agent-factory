//go:build functionallong

package smoke

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestStatelessCollector_TwoStagePipeline validates end-to-end processing
// through the two-stage pipeline: tokens injected at init flow through
// stage1 -> done, proving results flow through the full service layer
// with MockProvider driving stop-token evaluation.
func TestStatelessCollector_TwoStagePipeline(t *testing.T) {
	support.SkipLongFunctional(t, "slow stateless-collector pipeline sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w1"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
	)
	runFactoryThroughCustomerProcess(t, dir, provider)

	if provider.CallCount() != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.CallCount())
	}
}

// TestStatelessCollector_MultipleWorkItems validates that multiple work items
// all flow through the pipeline independently.
func TestStatelessCollector_MultipleWorkItems(t *testing.T) {
	support.SkipLongFunctional(t, "slow stateless-collector multi-item sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w2"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w3"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
	)
	runFactoryThroughCustomerProcess(t, dir, provider)
	if provider.CallCount() != 6 {
		t.Fatalf("provider call count = %d, want 6 for three two-stage items", provider.CallCount())
	}
}
