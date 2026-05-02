package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestDispatcherWorkflow_SingleSeedFile(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	originTraceID := "trace-single-seed"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "add login page"}`),
		TraceID:    originTraceID,
	})

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(3)...)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("prd:complete").
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:in-review").
		HasNoTokenInPlace("prd:failed")

	if got := len(support.ProviderCommandRequestsForWorker(runner, "planner")); got != 1 {
		t.Errorf("expected planner called 1 time, got %d", got)
	}
	if got := len(support.ProviderCommandRequestsForWorker(runner, "executor")); got != 1 {
		t.Errorf("expected executor called 1 time, got %d", got)
	}
	if got := len(support.ProviderCommandRequestsForWorker(runner, "reviewer")); got != 1 {
		t.Errorf("expected reviewer called 1 time, got %d", got)
	}

	h.Assert().TokenHasTraceID("prd:complete", originTraceID)
}

func TestDispatcherWorkflow_TwoSeedFiles(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "feature-alpha"}`),
		TraceID:    "trace-alpha",
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "feature-beta"}`),
		TraceID:    "trace-beta",
	})

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(6)...)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().PlaceTokenCount("prd:complete", 2)

	if got := len(support.ProviderCommandRequestsForWorker(runner, "planner")); got != 2 {
		t.Errorf("expected planner called 2 times, got %d", got)
	}
	if got := len(support.ProviderCommandRequestsForWorker(runner, "executor")); got != 2 {
		t.Errorf("expected executor called 2 times, got %d", got)
	}
	if got := len(support.ProviderCommandRequestsForWorker(runner, "reviewer")); got != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", got)
	}
}

func TestDispatcherWorkflow_MultipleSeedFiles(t *testing.T) {
	const n = 5
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	for i := range n {
		testutil.WriteSeedFile(t, dir, "idea", fmt.Appendf(nil, `{"title": "idea-%d"}`, i))
	}

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(n * 3)...)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().PlaceTokenCount("prd:complete", n)

	if got := len(support.ProviderCommandRequestsForWorker(runner, "planner")); got != n {
		t.Errorf("expected planner called %d times, got %d", n, got)
	}
	if got := len(support.ProviderCommandRequestsForWorker(runner, "executor")); got != n {
		t.Errorf("expected executor called %d times, got %d", n, got)
	}
	if got := len(support.ProviderCommandRequestsForWorker(runner, "reviewer")); got != n {
		t.Errorf("expected reviewer called %d times, got %d", n, got)
	}
}

func TestDispatcherWorkflow_ExecutionPoolIsolation(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "file-1"}`),
		TraceID:    "trace-iso-1",
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "file-2"}`),
		TraceID:    "trace-iso-2",
	})

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(6)...)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	dispatches := support.ProviderCommandRequestsForWorker(runner, "executor")
	if len(dispatches) != 2 {
		t.Fatalf("expected 2 executor dispatches, got %d", len(dispatches))
	}

	tokenIDs := make(map[string]bool)
	for _, d := range dispatches {
		if len(d.InputTokens) == 0 {
			t.Fatal("executor dispatch has no input tokens")
		}
		tokenIDs[support.FirstInputToken(d.InputTokens).ID] = true
	}
	if len(tokenIDs) != 2 {
		t.Errorf("expected 2 distinct input token IDs in executor dispatches, got %d unique", len(tokenIDs))
	}

	h.Assert().PlaceTokenCount("prd:complete", 2)
}

func TestDispatcherWorkflow_ReviewFailurePerItem(t *testing.T) {
	support.SkipLongFunctional(t, "slow dispatcher per-item review failure sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "will-fail"}`),
		TraceID:    "trace-will-fail",
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "will-pass"}`),
		TraceID:    "trace-will-pass",
	})

	runner := &traceAwareReviewCommandRunner{
		rejectTraceID: "trace-will-fail",
		callCounts:    make(map[string]int),
	}
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().HasTokenInPlace("prd:complete")
	h.Assert().HasTokenInPlace("prd:failed")

	runner.mu.Lock()
	failCount := runner.callCounts["trace-will-fail"]
	passCount := runner.callCounts["trace-will-pass"]
	runner.mu.Unlock()

	if failCount != 3 {
		t.Errorf("expected reviewer called 3 times for failing item, got %d", failCount)
	}
	if passCount != 1 {
		t.Errorf("expected reviewer called 1 time for passing item, got %d", passCount)
	}
}

type traceAwareReviewCommandRunner struct {
	rejectTraceID string
	mu            sync.Mutex
	callCounts    map[string]int
}

func (r *traceAwareReviewCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	if req.WorkerType != "reviewer" {
		return workers.CommandResult{Stdout: []byte("Done. COMPLETE")}, nil
	}

	traceID := ""
	if len(req.InputTokens) > 0 {
		traceID = support.FirstInputToken(req.InputTokens).Color.TraceID
	}

	r.mu.Lock()
	r.callCounts[traceID]++
	r.mu.Unlock()

	if traceID == r.rejectTraceID {
		return workers.CommandResult{Stdout: []byte("needs revision")}, nil
	}

	return workers.CommandResult{Stdout: []byte("Done. COMPLETE")}, nil
}

var _ workers.CommandRunner = (*traceAwareReviewCommandRunner)(nil)
