package functional_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// executorFunc adapts a function to the WorkerExecutor interface for tests.
type executorFunc func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error)

func (f executorFunc) Execute(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return f(ctx, d)
}

type fixtureLoadSmokeCase struct {
	name      string
	workType  string
	responses []interfaces.InferenceResponse
	minCalls  int
}

var fixtureLoadSmokeCases = []fixtureLoadSmokeCase{
	{
		name:     "happy_path",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
			{Content: "Finalized. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "retry_exhaustion",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Processed. COMPLETE"},
			{Content: "Looks good. ACCEPTED"},
		},
		minCalls: 2,
	},
	{
		name:     "resource_contention",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "multi_work_type",
		workType: "request",
		responses: []interfaces.InferenceResponse{
			{Content: "Handled. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "simple_pipeline",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "factory_request_batch",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Processed. COMPLETE"},
			{Content: "Finished. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "code_review",
		workType: "code-change",
		responses: []interfaces.InferenceResponse{
			{Content: "Code written. COMPLETE"},
			{Content: "Looks good. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "rejection_no_arcs",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "dependency_terminal",
		workType: "prd",
		responses: []interfaces.InferenceResponse{
			{Content: "Executed. COMPLETE"},
			{Content: "Reviewed. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "executor_failure_with_arcs",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "executor_success",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "cascading_failure",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Started. COMPLETE"},
			{Content: "Finished. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "conflict_resolution_dir",
		workType: "code-change",
		responses: []interfaces.InferenceResponse{
			{Content: "Code written. COMPLETE"},
			{Content: "Approved. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "dispatcher_lifecycle_dir",
		workType: "idea",
		responses: []interfaces.InferenceResponse{
			{Content: "Planned. COMPLETE"},
			{Content: "Executed. COMPLETE"},
			{Content: "Reviewed. COMPLETE"},
			{Content: "Archived. COMPLETE"},
		},
		minCalls: 4,
	},
	{
		name:     "workflow_v1_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Processed. COMPLETE"},
			{Content: "Finalized. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "workflow_v2_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Processed. COMPLETE"},
			{Content: "Reviewed. COMPLETE"},
			{Content: "Finalized. COMPLETE"},
		},
		minCalls: 3,
	},
	{
		name:     "workflow_v2_rejection_dir",
		workType: "doc",
		responses: []interfaces.InferenceResponse{
			{Content: "Drafted. COMPLETE"},
			{Content: "Approved. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "concurrency_limit_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "multi_output_dir",
		workType: "request",
		responses: []interfaces.InferenceResponse{
			{Content: "Planned. COMPLETE"},
			{Content: "Finished. COMPLETE"},
			{Content: "Finished. COMPLETE"},
		},
		minCalls: 3,
	},
	{
		name:     "multi_output_no_stopwords_dir",
		workType: "request",
		responses: []interfaces.InferenceResponse{
			{Content: "Planned. COMPLETE"},
			{Content: "Finished. COMPLETE"},
			{Content: "Finished. COMPLETE"},
		},
		minCalls: 3,
	},
	{
		name:     "logical_move_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "logical_move_pipeline_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Routed. COMPLETE"},
			{Content: "Processed. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "dependency_tracking_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Started. COMPLETE"},
			{Content: "Finished. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "dependency_tracking_simple_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "ralph_loop",
		workType: "story",
		responses: []interfaces.InferenceResponse{
			{Content: "Executed. COMPLETE"},
			{Content: "Approved. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "review_retry_exhaustion",
		workType: "code-change",
		responses: []interfaces.InferenceResponse{
			{Content: "Code written. COMPLETE"},
			{Content: "Looks good. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "filewatcher_flow",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Processed. COMPLETE"},
		},
		minCalls: 1,
	},
	// script_executor_dir is excluded because it uses SCRIPT_WORKER rather than
	// the MODEL_WORKER flow that this smoke exercises.
	{
		name:     "repeater_workstation",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
			{Content: "Finalized. COMPLETE"},
		},
		minCalls: 2,
	},
	{
		name:     "dispatcher_workflow",
		workType: "idea",
		responses: []interfaces.InferenceResponse{
			{Content: "Planned. COMPLETE"},
			{Content: "Executed. COMPLETE"},
			{Content: "Reviewed. COMPLETE"},
		},
		minCalls: 3,
	},
	{
		name:     "workstation_stopwords_factory_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "workstation_stopwords_frontmatter_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. COMPLETE"},
		},
		minCalls: 1,
	},
	{
		name:     "workstation_stopwords_override_dir",
		workType: "task",
		responses: []interfaces.InferenceResponse{
			{Content: "Done. STATION_COMPLETE"},
		},
		minCalls: 1,
	},
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

func withWorkingDirectory(t *testing.T, dir string, fn func()) {
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
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
		workingDirectoryMu.Unlock()
	}()

	fn()
}

// TestServiceHarness_HappyPath validates that the ServiceTestHarness can
// build a factory from a directory fixture, inject a mock Provider, and
// drive a two-stage pipeline to completion through the full service layer:
// BuildFactoryService → WorkstationExecutor → AgentExecutor → MockProvider.
// Work enters via a seed file picked up by the file watcher preseed.
func TestServiceHarness_HappyPath(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))

	// Write seed file before harness construction so preseed picks it up.
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Service harness happy path"}`))

	// The mock provider returns responses containing the stop token "COMPLETE"
	// so that both MODEL_WORKER agents accept.
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

	// Verify the mock provider was called twice (once per worker).
	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}

	// Verify the provider received proper inference requests with system prompts
	// from the AGENTS.md files.
	calls := provider.Calls()
	if calls[0].Model != "test-model" {
		t.Errorf("expected model test-model for call 0, got %q", calls[0].Model)
	}
	if calls[0].SystemPrompt == "" {
		t.Error("expected non-empty system prompt for call 0 (from AGENTS.md body)")
	}
}

// TestServiceHarness_NoopFallback demonstrates a minimal ServiceTestHarness
// setup with no AGENTS.md files. When workers have no configuration on disk,
// the harness registers a NoopExecutor that returns OutcomeAccepted so that
// the petri-net topology can be exercised without any real worker setup.
// Work enters via a seed file.
func TestServiceHarness_NoopFallback(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "noop_pipeline"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "noop fallback test"}`))

	// No WithProvider needed — no real workers will be called.
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

// TestFixtureDirectories_Load verifies that BuildFactoryService() can load
// each directory-based test fixture without error. This is a smoke test to
// ensure the factory.json + workers/{name}/AGENTS.md structure is valid.
// Each fixture's mock provider returns responses whose Content includes the
// appropriate stop token(s) so all workers accept.
// Work enters via seed files written to each fixture's inputs directory.
func TestFixtureDirectories_Load(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow fixture loading smoke")
	for _, tc := range fixtureLoadSmokeCases {
		t.Run(tc.name, func(t *testing.T) {
			runFixtureDirectoryLoadSmoke(t, tc)
		})
	}
}

func runFixtureDirectoryLoadSmoke(t *testing.T, tc fixtureLoadSmokeCase) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, fixtureDir(t, tc.name))
	testutil.WriteSeedFile(t, dir, tc.workType, []byte(`{"title": "fixture load test"}`))

	provider := testutil.NewMockProvider(tc.responses...)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 15*time.Second)

	if provider.CallCount() < tc.minCalls {
		t.Errorf("expected at least %d provider calls, got %d", tc.minCalls, provider.CallCount())
	}
}

// TestServiceHarness_MultipleWorkItems verifies that multiple seed files
// are all picked up and processed to completion.
func TestServiceHarness_MultipleWorkItems(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))

	// Write two seed files before harness construction.
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

	snap := h.Marking()
	taskTokens := 0
	for _, tok := range snap.Tokens {
		if tok.Color.WorkTypeID == "task" {
			taskTokens++
		}
	}
	if taskTokens < 2 {
		t.Errorf("expected at least 2 task tokens after seed file submission, got %d", taskTokens)
	}
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
