package functional_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

// executorFunc adapts a function to the WorkerExecutor interface for tests.
type executorFunc func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error)

func (f executorFunc) Execute(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return f(ctx, d)
}

var workingDirectoryMu sync.Mutex

// fixtureDir returns the absolute path to a directory-based test fixture.
func fixtureDir(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", name)
}

func resolvedRuntimePath(factoryDir, configuredPath string) string {
	return filepath.Clean(filepath.Join(factoryDir, filepath.FromSlash(configuredPath)))
}

func setWorkingDirectory(t *testing.T, dir string) {
	t.Helper()

	workingDirectoryMu.Lock()
	originalDir, err := os.Getwd()
	if err != nil {
		workingDirectoryMu.Unlock()
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		workingDirectoryMu.Unlock()
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
		workingDirectoryMu.Unlock()
	})
}

// TestServiceHarness_MockWorker verifies that MockWorker registers a mock
// executor that intercepts dispatches for the given worker type.
// Work enters via a seed file.
func TestServiceHarness_MockWorker(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "mock worker test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Register mock executors for both worker types.
	mockA := h.MockWorker("worker-a", interfaces.WorkResult{
		Outcome: interfaces.OutcomeAccepted,
	})
	mockB := h.MockWorker("worker-b", interfaces.WorkResult{
		Outcome: interfaces.OutcomeAccepted,
	})

	h.RunUntilComplete(t, 10*time.Second)

	// Verify mock A was called exactly once (step-one).
	if mockA.CallCount() != 1 {
		t.Errorf("expected mockA called 1 time, got %d", mockA.CallCount())
	}

	// Verify mock B was called exactly once (step-two).
	if mockB.CallCount() != 1 {
		t.Errorf("expected mockB called 1 time, got %d", mockB.CallCount())
	}

	// Verify the dispatch received correct fields.
	callA := mockA.LastCall()
	if callA.TransitionID == "" {
		t.Error("expected non-empty TransitionID in mock dispatch")
	}

	// Token should be in terminal state.
	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)
}

// TestServiceHarness_MockWorker_Idempotent verifies that calling MockWorker
// twice for the same worker type returns the same mock executor.
func TestServiceHarness_MockWorker_Idempotent(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))

	h := testutil.NewServiceTestHarness(t, dir)

	mock1 := h.MockWorker("worker-a")
	mock2 := h.MockWorker("worker-a")

	if mock1 != mock2 {
		t.Error("expected MockWorker to return same executor for same worker type")
	}
}

// TestServiceHarness_SetCustomExecutor verifies that SetCustomExecutor
// registers a custom executor that takes precedence over mock executors.
// Work enters via a seed file.
func TestServiceHarness_SetCustomExecutor(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "custom executor test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Register a mock for worker-b (step-two).
	mockB := h.MockWorker("worker-b", interfaces.WorkResult{
		Outcome: interfaces.OutcomeAccepted,
	})

	// Register a custom executor for worker-a (step-one) that records calls.
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

// TestServiceHarness_CustomExecutor_Precedence verifies that custom executors
// take precedence over mock executors for the same worker type.
// Work enters via a seed file.
func TestServiceHarness_CustomExecutor_Precedence(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "precedence test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Register both a mock and a custom executor for worker-a.
	mockA := h.MockWorker("worker-a", interfaces.WorkResult{
		Outcome: interfaces.OutcomeAccepted,
	})
	h.MockWorker("worker-b", interfaces.WorkResult{
		Outcome: interfaces.OutcomeAccepted,
	})

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
