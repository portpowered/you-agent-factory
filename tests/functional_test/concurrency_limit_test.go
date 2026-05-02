package functional_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestConcurrencyLimit_BlocksExcessDispatches verifies that with resource
// capacity=2, submitting 3 work items results in throttled processing. The
// resource mechanism correctly constrains the total in-flight work: resource
// tokens are consumed on dispatch and freed on completion.
//
// Concretely, this test verifies:
//  1. All 3 items eventually complete (resources throttle but don't deadlock)
//  2. The marking contains exactly 2 resource tokens in executor-slot:available
//     after all work completes (proving resource tokens are properly managed)
//  3. The processor is called exactly 3 times (once per item)
func TestConcurrencyLimit_BlocksExcessDispatches(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "concurrency_limit_dir"))

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

	// RunUntilComplete injects queued tokens and processes them to completion.
	h.RunUntilComplete(t, 10*time.Second)

	// All 3 items complete -- resource throttling didn't cause deadlock.
	h.Assert().
		PlaceTokenCount("task:complete", 3).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	// Processor called exactly 3 times (one per work item).
	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}

	// Both resource tokens returned to available (capacity=2 -> 2 tokens).
	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("executor-slot:available")
	if len(resourceTokens) != 2 {
		t.Errorf("expected 2 resource tokens in executor-slot:available, got %d", len(resourceTokens))
	}
}

// TestConcurrencyLimit_ResourceTokensConsumedDuringProcessing verifies that
// the resource mechanism actually constrains dispatch by checking token
// conservation. With 3 work tokens and 2 resource tokens, the total should
// be exactly 5 after all work completes.
func TestConcurrencyLimit_ResourceTokensConsumedDuringProcessing(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "concurrency_limit_dir"))

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

	// Run to completion.
	h.RunUntilComplete(t, 100*time.Second)

	// Final state: all work complete, all resources available.
	// Token conservation: 3 work + 2 resource = 5 total tokens.
	snap := h.Marking()
	totalTokens := len(snap.Tokens)
	if totalTokens != 5 {
		t.Errorf("expected 5 total tokens (3 work + 2 resource), got %d", totalTokens)
	}

	h.Assert().
		PlaceTokenCount("task:complete", 3).
		PlaceTokenCount("executor-slot:available", 2)
}

// TestConcurrencyLimit_ResourceReleasedOnFailure validates that if we have
// a failing worker, the resource is released.
func TestConcurrencyLimit_ResourceReleasedOnFailure(t *testing.T) {
	// Use the existing resource_contention directory fixture which has capacity=1.
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "resource_contention"))
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

	// Only 1 resource token should be available (capacity=1).
	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available, got %d", len(resourceTokens))
	}
}

// This remains on the inline custom-executor seam to preserve legacy panic
// recovery coverage for tick-time dispatch. The async companion below covers
// the same panic-derived failed-token behavior through the worker pool.
func TestConcurrencyLimit_ResourceReleasedOnExecutorPanic_Inline(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "resource_contention"))
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

// This remains on the async custom-executor seam because it verifies panic
// recovery around WorkerExecutor.Execute itself. Provider or command-runner
// edge mocks can return errors, but they cannot exercise executor panic
// recovery while preserving the panic-derived failed-token assertion.
func TestConcurrencyLimit_ResourceReleasedOnExecutorPanic_Async(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "resource_contention"))
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

// TestConcurrencyLimit_ReducedCapacityStillCompletes verifies that even with
// capacity=1 (modified at config level via a separate fixture), all items
// still complete. This confirms the resource mechanism serializes without
// blocking indefinitely.
func TestConcurrencyLimit_ReducedCapacityStillCompletes(t *testing.T) {
	// Use the existing resource_contention directory fixture which has capacity=1.
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "resource_contention"))

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

	// Only 1 resource token should be available (capacity=1).
	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available, got %d", len(resourceTokens))
	}
}
