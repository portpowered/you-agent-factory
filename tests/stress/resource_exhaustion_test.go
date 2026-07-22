package stress_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type workBatchSubmitter interface {
	SubmitFull(context.Context, []work.SubmitRequest)
}

func queueManyItems(t *testing.T, submitter workBatchSubmitter, workTypeID string, numItems int) {
	t.Helper()
	requests := make([]work.SubmitRequest, numItems)
	for i := range requests {
		requests[i] = work.SubmitRequest{
			WorkID:     fmt.Sprintf("%s-%d", workTypeID, i),
			Name:       fmt.Sprintf("%s-%d", workTypeID, i),
			WorkTypeID: workTypeID,
			Payload:    fmt.Appendf(nil, `{"item": %d}`, i),
			TraceID:    fmt.Sprintf("trace-%s-%d", workTypeID, i),
		}
	}
	submitter.SubmitFull(context.Background(), requests)
}

func resourceFactoryConfig(workerName string, resources ...interfaces.ResourceConfig) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Resources: resources,
		Workers:   []interfaces.FactoryWorkerConfig{{Name: workerName}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: workerName,
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
			Resources: append([]interfaces.ResourceConfig(nil), resources...),
		}},
	}
}

func startResourceProcess(
	t *testing.T,
	cfg *interfaces.FactoryConfig,
	executor workers.WorkerExecutor,
) *stressProcessHarness {
	t.Helper()
	dir := testutil.ScaffoldFactoryDir(t, cfg)
	harness := startStressProcess(t, dir, workerExecutorProvider{executor: executor})
	t.Cleanup(harness.Stop)
	return harness
}

// TestResourceExhaustionGPU proves a capacity-one public Workstation resource
// serializes execution and is fully available after every Work item completes.
func TestResourceExhaustionGPU(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	const numItems = 20
	tracker := newConcurrencyTracker(5 * time.Millisecond)
	h := startResourceProcess(t, resourceFactoryConfig(
		"gpu-worker",
		interfaces.ResourceConfig{Name: "gpu", Capacity: 1},
	), tracker)
	queueManyItems(t, h, "task", numItems)
	h.WaitForTerminalCount(numItems, 30*time.Second)

	assertWorkStateCounts(t, h.Session(), numItems, 0)
	if got := tracker.max(); got > 1 {
		t.Fatalf("maximum concurrent GPU users = %d, want at most 1", got)
	}
	assertPublicResourceUsage(t, h.Session(), "gpu", 1, 1)
}

// TestResourceExhaustionMoney proves a capacity-five public Workstation
// resource bounds execution and is fully released.
func TestResourceExhaustionMoney(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	const numItems, capacity = 10, 5
	tracker := newConcurrencyTracker(5 * time.Millisecond)
	h := startResourceProcess(t, resourceFactoryConfig(
		"money-worker",
		interfaces.ResourceConfig{Name: "money", Capacity: capacity},
	), tracker)
	queueManyItems(t, h, "task", numItems)
	h.WaitForTerminalCount(numItems, 30*time.Second)

	assertWorkStateCounts(t, h.Session(), numItems, 0)
	if got := tracker.max(); got > capacity {
		t.Fatalf("maximum concurrent money users = %d, want at most %d", got, capacity)
	}
	assertPublicResourceUsage(t, h.Session(), "money", capacity, capacity)
}

// TestResourceCapacityIsLeaseBased replaces the obsolete internal
// resource-token consumption scenario. Customer-authored Workstation resources
// are leases: capacity is returned after each dispatch and queued Work proceeds.
func TestResourceCapacityIsLeaseBased(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	const numItems, capacity = 10, 5
	h := startResourceProcess(t, resourceFactoryConfig(
		"lease-worker",
		interfaces.ResourceConfig{Name: "money", Capacity: capacity},
	), newConcurrencyTracker(2*time.Millisecond))
	queueManyItems(t, h, "task", numItems)
	h.WaitForTerminalCount(numItems, 30*time.Second)

	assertWorkStateCounts(t, h.Session(), numItems, 0)
	assertPublicResourceUsage(t, h.Session(), "money", capacity, capacity)
}

// TestResourceExhaustionNoTokenLoss proves dual public resource leases do not
// duplicate Work and both capacities return to availability.
func TestResourceExhaustionNoTokenLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	const numItems = 20
	h := startResourceProcess(t, resourceFactoryConfig(
		"dual-worker",
		interfaces.ResourceConfig{Name: "gpu", Capacity: 1},
		interfaces.ResourceConfig{Name: "money", Capacity: 5},
	), newConcurrencyTracker(2*time.Millisecond))
	queueManyItems(t, h, "task", numItems)
	h.WaitForTerminalCount(numItems, 30*time.Second)

	session := h.Session()
	assertWorkStateCounts(t, session, numItems, 0)
	assertPublicResourceUsage(t, session, "gpu", 1, 1)
	assertPublicResourceUsage(t, session, "money", 5, 5)
	assertUniquePublicWorkIDs(t, session)
}

// TestResourceExhaustionWithFailure proves a failed dispatch releases its
// public resource lease and does not block subsequent Work.
func TestResourceExhaustionWithFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	const numItems = 5
	executor := &failFirstExecutor{}
	h := startResourceProcess(t, resourceFactoryConfig(
		"failure-worker",
		interfaces.ResourceConfig{Name: "gpu", Capacity: 1},
	), executor)
	queueManyItems(t, h, "task", numItems)
	h.WaitForTerminalCount(numItems, 30*time.Second)

	session := h.Session()
	assertWorkStateCounts(t, session, numItems-1, 1)
	assertPublicResourceUsage(t, session, "gpu", 1, 1)
	assertUniquePublicWorkIDs(t, session)
}

func TestResourceExhaustionTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	h := startResourceProcess(t, resourceFactoryConfig(
		"timeout-worker",
		interfaces.ResourceConfig{Name: "gpu", Capacity: 1},
	), newConcurrencyTracker(time.Millisecond))
	queueManyItems(t, h, "task", 15)
	h.WaitForTerminalCount(15, 10*time.Second)
	assertPublicResourceUsage(t, h.Session(), "gpu", 1, 1)
}

func assertWorkStateCounts(t *testing.T, session factoryapi.FactorySession, wantComplete, wantFailed int) {
	t.Helper()
	if session.Runtime.Progress.Categories.Terminal != wantComplete ||
		session.Runtime.Progress.Categories.Failed != wantFailed ||
		session.Runtime.Progress.Categories.Initial != 0 ||
		session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf(
			"public progress = %#v, want complete=%d failed=%d and no active Work",
			session.Runtime.Progress,
			wantComplete,
			wantFailed,
		)
	}
}

func assertPublicResourceUsage(
	t *testing.T,
	session factoryapi.FactorySession,
	name string,
	wantAvailable int,
	wantTotal int,
) {
	t.Helper()
	for _, resource := range session.Runtime.Usage.Resources {
		if resource.Name != name {
			continue
		}
		if resource.Available != wantAvailable || resource.Total != wantTotal {
			t.Fatalf("resource %q usage = %#v, want available=%d total=%d", name, resource, wantAvailable, wantTotal)
		}
		return
	}
	t.Fatalf("resource %q missing from public usage %#v", name, session.Runtime.Usage.Resources)
}

func assertUniquePublicWorkIDs(t *testing.T, session factoryapi.FactorySession) {
	t.Helper()
	if session.Runtime.Petri == nil {
		t.Fatal("public Petri projection is missing")
	}
	seen := make(map[string]bool, len(session.Runtime.Petri.Marking))
	for _, token := range session.Runtime.Petri.Marking {
		if token.WorkId == "" {
			continue
		}
		if seen[token.WorkId] {
			t.Fatalf("duplicate public Work ID %q", token.WorkId)
		}
		seen[token.WorkId] = true
	}
}

type concurrencyTracker struct {
	mu      sync.Mutex
	active  int
	peak    int
	holdFor time.Duration
}

func newConcurrencyTracker(holdFor time.Duration) *concurrencyTracker {
	return &concurrencyTracker{holdFor: holdFor}
}

func (tracker *concurrencyTracker) Execute(
	ctx context.Context,
	dispatch work.WorkDispatch,
) (workers.WorkResult, error) {
	tracker.mu.Lock()
	tracker.active++
	if tracker.active > tracker.peak {
		tracker.peak = tracker.active
	}
	tracker.mu.Unlock()

	timer := time.NewTimer(tracker.holdFor)
	select {
	case <-ctx.Done():
		timer.Stop()
	case <-timer.C:
	}

	tracker.mu.Lock()
	tracker.active--
	tracker.mu.Unlock()
	return workers.WorkResult{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workers.OutcomeAccepted,
	}, nil
}

func (tracker *concurrencyTracker) max() int {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.peak
}

type failFirstExecutor struct {
	mu    sync.Mutex
	calls int
}

func (executor *failFirstExecutor) Execute(
	_ context.Context,
	dispatch work.WorkDispatch,
) (workers.WorkResult, error) {
	executor.mu.Lock()
	executor.calls++
	call := executor.calls
	executor.mu.Unlock()
	result := workers.WorkResult{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workers.OutcomeAccepted,
	}
	if call == 1 {
		result.Outcome = workers.OutcomeFailed
		result.Error = "simulated processing failure"
	}
	return result, nil
}

var (
	_ workers.WorkerExecutor = (*concurrencyTracker)(nil)
	_ workers.WorkerExecutor = (*failFirstExecutor)(nil)
)
