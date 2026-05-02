package smoke

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

type executorFunc func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error)

func (f executorFunc) Execute(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return f(ctx, d)
}

func TestServiceHarness_MockWorker(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness mock-worker sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "mock worker test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	mockA := h.MockWorker("worker-a", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})
	mockB := h.MockWorker("worker-b", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	h.RunUntilComplete(t, 10*time.Second)

	if mockA.CallCount() != 1 {
		t.Errorf("expected mockA called 1 time, got %d", mockA.CallCount())
	}
	if mockB.CallCount() != 1 {
		t.Errorf("expected mockB called 1 time, got %d", mockB.CallCount())
	}

	callA := mockA.LastCall()
	if callA.TransitionID == "" {
		t.Error("expected non-empty TransitionID in mock dispatch")
	}

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)
}

func TestServiceHarness_MockWorker_Idempotent(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness mock-worker idempotency sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))

	h := testutil.NewServiceTestHarness(t, dir)

	mock1 := h.MockWorker("worker-a")
	mock2 := h.MockWorker("worker-a")

	if mock1 != mock2 {
		t.Error("expected MockWorker to return same executor for same worker type")
	}
}

func TestServiceHarness_SetCustomExecutor(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness custom-executor sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "custom executor test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	mockB := h.MockWorker("worker-b", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	var customCalled bool
	h.SetCustomExecutor("worker-a", executorFunc(func(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
		customCalled = true
		return interfaces.WorkResult{
			DispatchID:   d.DispatchID,
			TransitionID: d.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
		}, nil
	}))

	h.RunUntilComplete(t, 10*time.Second)

	if !customCalled {
		t.Error("expected custom executor to be called for worker-a")
	}
	if mockB.CallCount() != 1 {
		t.Errorf("expected mockB called 1 time, got %d", mockB.CallCount())
	}

	h.Assert().
		HasTokenInPlace("task:complete").
		TokenCount(1)
}

func TestServiceHarness_CustomExecutor_Precedence(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness custom-executor precedence sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "precedence test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	mockA := h.MockWorker("worker-a", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})
	h.MockWorker("worker-b", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	var customCalled bool
	h.SetCustomExecutor("worker-a", executorFunc(func(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
		customCalled = true
		return interfaces.WorkResult{
			DispatchID:   d.DispatchID,
			TransitionID: d.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
		}, nil
	}))

	h.RunUntilComplete(t, 10*time.Second)

	if !customCalled {
		t.Error("expected custom executor to be called, not mock")
	}
	if mockA.CallCount() != 0 {
		t.Errorf("expected mockA not called (custom should take precedence), got %d calls", mockA.CallCount())
	}
}
