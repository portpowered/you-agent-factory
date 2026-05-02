//go:build functionallong

package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceHarness_HappyPath(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness happy-path sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "service harness happy path"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}

	calls := provider.Calls()
	if calls[0].Model != "test-model" {
		t.Errorf("expected model test-model for call 0, got %q", calls[0].Model)
	}
	if calls[0].SystemPrompt == "" {
		t.Error("expected non-empty system prompt for call 0")
	}
}

func TestServiceHarness_NoopFallback(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness noop sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "noop_pipeline"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "noop fallback test"}`))

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)
}

func TestServiceHarness_MultipleWorkItems(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness multi-item sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-2"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 2).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing")
}
