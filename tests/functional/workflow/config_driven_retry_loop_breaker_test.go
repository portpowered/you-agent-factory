package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDrivenRetryLoopBreaker_TerminatesAfterMaxRetries(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will exhaust retries"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},
		interfaces.InferenceResponse{Content: "Needs work"},
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},
		interfaces.InferenceResponse{Content: "Still needs work"},
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},
		interfaces.InferenceResponse{Content: "Not good enough"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:complete")

	if provider.CallCount() != 6 {
		t.Errorf("expected provider called 6 times, got %d", provider.CallCount())
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "review-exhaustion", "task:failed")
}

func TestConfigDrivenRetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will succeed on second try"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},
		interfaces.InferenceResponse{Content: "Needs work"},
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},
		interfaces.InferenceResponse{Content: "Looks good. ACCEPTED"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}
