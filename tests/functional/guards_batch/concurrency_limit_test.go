package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestConcurrencyLimit_BlocksExcessDispatches(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit blocking sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "concurrency_limit_dir"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-2"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-3"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "item 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "item 2 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "item 3 done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 3).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}

	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("executor-slot:available")
	if len(resourceTokens) != 2 {
		t.Errorf("expected 2 resource tokens in executor-slot:available, got %d", len(resourceTokens))
	}
}

func TestConcurrencyLimit_ResourceTokensConsumedDuringProcessing(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit resource-consumption sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "concurrency_limit_dir"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "B"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "C"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "A done. COMPLETE"},
		interfaces.InferenceResponse{Content: "B done. COMPLETE"},
		interfaces.InferenceResponse{Content: "C done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 100*time.Second)

	snap := h.Marking()
	totalTokens := len(snap.Tokens)
	if totalTokens != 5 {
		t.Errorf("expected 5 total tokens (3 work + 2 resource), got %d", totalTokens)
	}

	h.Assert().
		PlaceTokenCount("task:complete", 3).
		PlaceTokenCount("executor-slot:available", 2)
}

func TestConcurrencyLimit_ResourceReleasedOnFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit failure-release sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "X"}`))

	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{Content: ""}},
		[]error{errors.New("processor failed")},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init")

	if provider.CallCount() != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount())
	}

	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available, got %d", len(resourceTokens))
	}
}

func TestConcurrencyLimit_ResourceReleasedOnExecutorPanic_Inline(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit inline-panic sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "panic-inline"}`))

	h := testutil.NewServiceTestHarness(t, dir)
	h.SetCustomExecutor("processor", &panickingExecutor{})

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init")

	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available after panic, got %d", len(resourceTokens))
	}

	failed := snap.TokensInPlace("task:failed")
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed token, got %d", len(failed))
	}
	if failed[0].History.LastError == "" {
		t.Fatal("expected panic-derived failure message on failed token")
	}
}

func TestConcurrencyLimit_ResourceReleasedOnExecutorPanic_Async(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit async-panic sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "panic-async"}`))

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("processor", &panickingExecutor{})

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init")

	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available after async panic, got %d", len(resourceTokens))
	}
}

func TestConcurrencyLimit_ReducedCapacityStillCompletes(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit reduced-capacity sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "X"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Y"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Z"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "X done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Y done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Z done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 3).
		HasNoTokenInPlace("task:init")

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}

	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available, got %d", len(resourceTokens))
	}
}
