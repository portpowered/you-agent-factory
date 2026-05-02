package stress_test

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// TestThroughputLargeScale verifies the engine handles a few hundred work items through
// a 3-stage pipeline without degrading. Uses engine Run() with real concurrency
// (raceSyncDispatcher with 1ms delay per item).
//
// Asserts: all items reach terminal state, no tokens lost or duplicated,
// reasonable memory usage (<500MB), total execution time <10s.
// Logs: throughput (items/sec), p50/p99 latency per stage.
func TestThroughputLargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		totalItems     = 200
		pipelineStages = 3
		workerDelay    = 1 * time.Millisecond
		timeout        = 8 * time.Second
	)

	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(pipelineStages, "pipeline-worker"))

	// Track per-stage latencies.
	tracker := newLatencyTracker(pipelineStages)

	executor := &throughputExecutor{delay: workerDelay, tracker: tracker}
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("pipeline-worker", executor)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Capture baseline memory.
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()

	// Run engine in background.
	errCh := h.RunInBackground(ctx)

	// Submit items in batches to avoid blocking on submitCh (buffer 16).
	var submitWg sync.WaitGroup
	const numSubmitters = 10
	itemsPerSubmitter := totalItems / numSubmitters
	for g := range numSubmitters {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range itemsPerSubmitter {
				h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
					WorkTypeID: "task",
					TraceID:    fmt.Sprintf("trace-%d-%d", gid, i),
					Payload:    fmt.Appendf(nil, `{"g":%d,"i":%d}`, gid, i),
				}})
				// Yield to let the engine drain the submit channel (buffer 16).
				if i%10 == 9 {
					time.Sleep(time.Millisecond)
				}
			}
		}(g)
	}
	submitWg.Wait()

	// Poll until all tokens reach terminal state.
	pipelineTerminalPlaces := []string{"task:complete", "task:failed"}
	pollUntilAllTerminalH(t, h, pipelineTerminalPlaces, totalItems, timeout-2*time.Second)

	totalDuration := time.Since(startTime)

	// Stop engine.
	cancel()
	<-errCh

	// Capture peak memory.
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// --- Assertions ---

	// All items in terminal state.
	snap := h.Marking()
	terminalCount := countTerminalTokens(snap, pipelineTerminalPlaces)
	if terminalCount != totalItems {
		t.Errorf("expected %d terminal tokens, got %d", totalItems, terminalCount)
	}

	// No tokens lost or duplicated.
	if len(snap.Tokens) != totalItems {
		t.Errorf("expected %d total tokens, got %d", totalItems, len(snap.Tokens))
	}

	// No tokens stuck in non-terminal places.
	for id, tok := range snap.Tokens {
		if tok.PlaceID != "task:complete" && tok.PlaceID != "task:failed" {
			t.Errorf("token %s stuck in non-terminal place %s", id, tok.PlaceID)
		}
	}

	// No duplicate token IDs.
	tokenIDs := make(map[string]bool, len(snap.Tokens))
	for _, tok := range snap.Tokens {
		if tokenIDs[tok.ID] {
			t.Errorf("duplicate token ID: %s", tok.ID)
		}
		tokenIDs[tok.ID] = true
	}

	// Memory usage check: heap growth should be < 500MB.
	heapGrowthMB := bytesGrowthMB(memAfter.HeapAlloc, memBefore.HeapAlloc)
	totalAllocMB := bytesGrowthMB(memAfter.TotalAlloc, memBefore.TotalAlloc)
	if heapGrowthMB > 500 {
		t.Errorf("heap growth %.1fMB exceeds 500MB limit", heapGrowthMB)
	}

	// Execution time check.
	if totalDuration > timeout {
		t.Errorf("execution time %v exceeds %v limit", totalDuration, timeout)
	}

	// --- Logging ---
	throughput := float64(totalItems) / totalDuration.Seconds()
	t.Logf("=== Throughput Results ===")
	t.Logf("Total items:       %d", totalItems)
	t.Logf("Pipeline stages:   %d", pipelineStages)
	t.Logf("Worker delay:      %v", workerDelay)
	t.Logf("Total duration:    %v", totalDuration)
	t.Logf("Throughput:         %.1f items/sec", throughput)
	t.Logf("Heap growth:       %.1f MB", heapGrowthMB)
	t.Logf("Total alloc:       %.1f MB", totalAllocMB)
	t.Logf("Executor calls:    %d", executor.callCount())

	// Per-stage latency percentiles.
	for stage := 1; stage <= pipelineStages; stage++ {
		stageName := fmt.Sprintf("step%d", stage)
		p50, p99 := tracker.percentiles(stageName)
		t.Logf("Stage %s:  p50=%v  p99=%v", stageName, p50, p99)
	}
	// Finish stage.
	p50, p99 := tracker.percentiles("finish")
	t.Logf("Stage finish:  p50=%v  p99=%v", p50, p99)
}

// TestThroughputNoTokenLoss is a focused variant that verifies token integrity
// at scale: every submitted work ID appears exactly once in the terminal marking.
func TestThroughputNoTokenLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		totalItems     = 200
		pipelineStages = 3
		timeout        = 15 * time.Second
	)

	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(pipelineStages, "pipeline-worker"))

	executor := &throughputExecutor{delay: 500 * time.Microsecond, tracker: newLatencyTracker(0)}
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("pipeline-worker", executor)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := h.RunInBackground(ctx)

	submitted := make(map[string]submittedWorkToken, totalItems)
	var mu sync.Mutex

	var submitWg sync.WaitGroup
	const numSubmitters = 10
	for g := range numSubmitters {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range totalItems / numSubmitters {
				traceID := fmt.Sprintf("trace-%d-%d", gid, i)
				workID := fmt.Sprintf("work-stress-%d-%d", gid, i)
				mu.Lock()
				submitted[workID] = submittedWorkToken{
					WorkID:  workID,
					TraceID: traceID,
				}
				mu.Unlock()
				h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
					WorkTypeID: "task",
					WorkID:     workID,
					TraceID:    traceID,
					Payload:    fmt.Appendf(nil, `{"g":%d,"i":%d}`, gid, i),
				}})
				// Yield to let the engine drain the submit channel.
				if i%10 == 9 {
					time.Sleep(time.Millisecond)
				}
			}
		}(g)
	}
	submitWg.Wait()

	pipelineTerminalPlaces := []string{"task:complete", "task:failed"}
	pollUntilAllTerminalH(t, h, pipelineTerminalPlaces, totalItems, timeout-2*time.Second)

	cancel()
	<-errCh

	snap := h.Marking()
	diagnostics := diagnoseSubmittedWorkPreservation(snap, submitted, pipelineTerminalPlaces)
	expectedExecutorCalls := totalItems * (pipelineStages + 1)
	actualExecutorCalls := executor.callCount()
	t.Logf("token integrity: %s, executor calls=%d", diagnostics.summary(), actualExecutorCalls)

	if diagnostics.hasFailures() {
		t.Error(diagnostics.failureMessage())
	}
	if actualExecutorCalls != expectedExecutorCalls {
		t.Errorf("expected custom executor to handle every pipeline transition: got %d calls, want %d", actualExecutorCalls, expectedExecutorCalls)
	}
}

func TestThroughputNoTokenLossRegression_CustomExecutorPreservesSubmittedWorkIdentities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		totalItems     = 40
		pipelineStages = 3
		numSubmitters  = 4
		timeout        = 5 * time.Second
	)

	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(pipelineStages, "pipeline-worker"))

	executor := &throughputExecutor{delay: 100 * time.Microsecond, tracker: newLatencyTracker(0)}
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("pipeline-worker", executor)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := h.RunInBackground(ctx)

	submitted := make(map[string]submittedWorkToken, totalItems)
	var mu sync.Mutex
	var submitWg sync.WaitGroup
	for g := range numSubmitters {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range totalItems / numSubmitters {
				traceID := fmt.Sprintf("regression-trace-%d-%d", gid, i)
				workID := fmt.Sprintf("regression-work-%d-%d", gid, i)
				mu.Lock()
				submitted[workID] = submittedWorkToken{WorkID: workID, TraceID: traceID}
				mu.Unlock()
				h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
					WorkTypeID: "task",
					WorkID:     workID,
					TraceID:    traceID,
					Payload:    fmt.Appendf(nil, `{"g":%d,"i":%d}`, gid, i),
				}})
				if i%2 == 1 {
					time.Sleep(250 * time.Microsecond)
				}
			}
		}(g)
	}
	submitWg.Wait()

	pipelineTerminalPlaces := []string{"task:complete", "task:failed"}
	pollUntilAllTerminalH(t, h, pipelineTerminalPlaces, totalItems, timeout-1*time.Second)

	cancel()
	<-errCh

	diagnostics := diagnoseSubmittedWorkPreservation(h.Marking(), submitted, pipelineTerminalPlaces)
	if diagnostics.hasFailures() {
		t.Error(diagnostics.failureMessage())
	}

	expectedExecutorCalls := totalItems * (pipelineStages + 1)
	if actualExecutorCalls := executor.callCount(); actualExecutorCalls != expectedExecutorCalls {
		t.Errorf("expected custom executor to handle every pipeline transition: got %d calls, want %d", actualExecutorCalls, expectedExecutorCalls)
	}
}

func TestSubmittedWorkPreservationDiagnostics_ReportsIdentityFailures(t *testing.T) {
	submitted := map[string]submittedWorkToken{
		"work-kept":    {WorkID: "work-kept", TraceID: "trace-kept"},
		"work-missing": {WorkID: "work-missing", TraceID: "trace-missing"},
		"work-dup":     {WorkID: "work-dup", TraceID: "trace-dup"},
		"work-stuck":   {WorkID: "work-stuck", TraceID: "trace-stuck"},
	}
	snap := &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-kept": {
			ID:      "tok-kept",
			PlaceID: "task:complete",
			Color:   interfaces.TokenColor{DataType: interfaces.DataTypeWork, WorkTypeID: "task", WorkID: "work-kept", TraceID: "trace-kept"},
		},
		"tok-dup-a": {
			ID:      "tok-dup-a",
			PlaceID: "task:complete",
			Color:   interfaces.TokenColor{DataType: interfaces.DataTypeWork, WorkTypeID: "task", WorkID: "work-dup", TraceID: "trace-dup"},
		},
		"tok-dup-b": {
			ID:      "tok-dup-b",
			PlaceID: "task:failed",
			Color:   interfaces.TokenColor{DataType: interfaces.DataTypeWork, WorkTypeID: "task", WorkID: "work-dup", TraceID: "trace-dup"},
		},
		"tok-stuck": {
			ID:      "tok-stuck",
			PlaceID: "task:step2",
			Color:   interfaces.TokenColor{DataType: interfaces.DataTypeWork, WorkTypeID: "task", WorkID: "work-stuck", TraceID: "trace-stuck"},
		},
		"resource-token": {
			ID:      "resource-token",
			PlaceID: "executor:available",
			Color:   interfaces.TokenColor{DataType: interfaces.DataTypeResource, WorkID: "executor:0"},
		},
	}}

	diagnostics := diagnoseSubmittedWorkPreservation(snap, submitted, []string{"task:complete", "task:failed"})
	message := diagnostics.failureMessage()
	for _, want := range []string{
		"submitted=4",
		"final_work_tokens=4",
		"terminal=3",
		"missing_work_ids=[work-missing(trace=trace-missing)]",
		"duplicate_work_ids=[work-dup(trace=trace-dup tokens=tok-dup-a@task:complete,tok-dup-b@task:failed)]",
		"non_terminal=[work-stuck(trace=trace-stuck token=tok-stuck place=task:step2)]",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("diagnostic message missing %q:\n%s", want, message)
		}
	}
}

func TestBytesGrowthMB_ClampsLowerAfterValueToZero(t *testing.T) {
	if got := bytesGrowthMB(1024, 2048); got != 0 {
		t.Fatalf("expected zero growth when after is lower than before, got %.1f", got)
	}
}

// TestThroughputTimeout verifies the throughput test completes within 10s
// (no deadlocks or infinite loops at scale).
func TestThroughputTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		totalItems = 200
		timeout    = 8 * time.Second
	)

	done := make(chan struct{})
	go func() {
		defer close(done)

		dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(3, "pipeline-worker"))
		executor := &throughputExecutor{delay: 100 * time.Microsecond, tracker: newLatencyTracker(0)}
		h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
		h.SetCustomExecutor("pipeline-worker", executor)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		errCh := h.RunInBackground(ctx)

		// Submit all items from a single goroutine with interleaved sleeps.
		for i := range totalItems {
			h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
				WorkTypeID: "task",
				TraceID:    fmt.Sprintf("trace-%d", i),
				Payload:    fmt.Appendf(nil, `{"i":%d}`, i),
			}})
			if i%25 == 24 {
				time.Sleep(time.Millisecond)
			}
		}

		pollUntilAllTerminalH(t, h, []string{"task:complete", "task:failed"}, totalItems, timeout-2*time.Second)
		cancel()
		<-errCh
	}()

	select {
	case <-done:
		// Completed within timeout.
	case <-time.After(10 * time.Second):
		t.Fatal("throughput test did not complete within 10s — possible deadlock")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type submittedWorkToken struct {
	WorkID  string
	TraceID string
}

type submittedTokenObservation struct {
	TokenID string
	PlaceID string
	TraceID string
}

type tokenPreservationDiagnostics struct {
	submittedCount int
	finalWorkCount int
	terminalCount  int
	missing        []string
	duplicates     []string
	nonTerminal    []string
}

func (d tokenPreservationDiagnostics) hasFailures() bool {
	return len(d.missing) > 0 || len(d.duplicates) > 0 || len(d.nonTerminal) > 0 ||
		d.finalWorkCount != d.submittedCount || d.terminalCount != d.submittedCount
}

func (d tokenPreservationDiagnostics) summary() string {
	return fmt.Sprintf("submitted=%d, final_work_tokens=%d, terminal=%d, missing=%d, duplicated=%d, non_terminal=%d",
		d.submittedCount, d.finalWorkCount, d.terminalCount, len(d.missing), len(d.duplicates), len(d.nonTerminal))
}

func (d tokenPreservationDiagnostics) failureMessage() string {
	return fmt.Sprintf("submitted work token preservation failed: submitted=%d final_work_tokens=%d terminal=%d missing_work_ids=%s duplicate_work_ids=%s non_terminal=%s",
		d.submittedCount, d.finalWorkCount, d.terminalCount, formatList(d.missing), formatList(d.duplicates), formatList(d.nonTerminal))
}

func diagnoseSubmittedWorkPreservation(
	snap *petri.MarkingSnapshot,
	submitted map[string]submittedWorkToken,
	terminalPlaces []string,
) tokenPreservationDiagnostics {
	terminalSet := make(map[string]bool, len(terminalPlaces))
	for _, p := range terminalPlaces {
		terminalSet[p] = true
	}

	observedByWorkID := make(map[string][]submittedTokenObservation, len(submitted))
	finalWorkCount := 0
	terminalCount := 0
	for _, tok := range snap.Tokens {
		if tok.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if tok.Color.WorkID == "" {
			continue
		}
		finalWorkCount++
		if terminalSet[tok.PlaceID] {
			terminalCount++
		}
		observedByWorkID[tok.Color.WorkID] = append(observedByWorkID[tok.Color.WorkID], submittedTokenObservation{
			TokenID: tok.ID,
			PlaceID: tok.PlaceID,
			TraceID: tok.Color.TraceID,
		})
	}

	expectedWorkIDs := make([]string, 0, len(submitted))
	for workID := range submitted {
		expectedWorkIDs = append(expectedWorkIDs, workID)
	}
	sort.Strings(expectedWorkIDs)

	diagnostics := tokenPreservationDiagnostics{
		submittedCount: len(submitted),
		finalWorkCount: finalWorkCount,
		terminalCount:  terminalCount,
	}
	for _, workID := range expectedWorkIDs {
		expected := submitted[workID]
		observed := observedByWorkID[workID]
		switch len(observed) {
		case 0:
			diagnostics.missing = append(diagnostics.missing, fmt.Sprintf("%s(trace=%s)", expected.WorkID, expected.TraceID))
		case 1:
			if !terminalSet[observed[0].PlaceID] {
				diagnostics.nonTerminal = append(diagnostics.nonTerminal, fmt.Sprintf("%s(trace=%s token=%s place=%s)",
					expected.WorkID, expected.TraceID, observed[0].TokenID, observed[0].PlaceID))
			}
		default:
			diagnostics.duplicates = append(diagnostics.duplicates, fmt.Sprintf("%s(trace=%s tokens=%s)",
				expected.WorkID, expected.TraceID, formatObservedTokens(observed)))
			for _, token := range observed {
				if !terminalSet[token.PlaceID] {
					diagnostics.nonTerminal = append(diagnostics.nonTerminal, fmt.Sprintf("%s(trace=%s token=%s place=%s)",
						expected.WorkID, expected.TraceID, token.TokenID, token.PlaceID))
				}
			}
		}
	}

	return diagnostics
}

func formatObservedTokens(tokens []submittedTokenObservation) string {
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, fmt.Sprintf("%s@%s", token.TokenID, token.PlaceID))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func formatList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	return "[" + strings.Join(values, " ") + "]"
}

func bytesGrowthMB(after, before uint64) float64 {
	if after <= before {
		return 0
	}
	return float64(after-before) / (1024 * 1024)
}

// throughputExecutor is a WorkerExecutor with a fixed delay and per-stage
// latency tracking for throughput measurement.
type throughputExecutor struct {
	delay   time.Duration
	tracker *latencyTracker
	mu      sync.Mutex
	calls   int
}

func (e *throughputExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	start := time.Now()

	if e.delay > 0 {
		time.Sleep(e.delay)
	}

	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	elapsed := time.Since(start)
	e.tracker.record(dispatch.TransitionID, elapsed)

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

func (e *throughputExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

// latencyTracker records per-stage latencies for percentile computation.
type latencyTracker struct {
	mu     sync.Mutex
	stages map[string][]time.Duration
}

func newLatencyTracker(_ int) *latencyTracker {
	return &latencyTracker{
		stages: make(map[string][]time.Duration),
	}
}

func (lt *latencyTracker) record(stage string, d time.Duration) {
	lt.mu.Lock()
	lt.stages[stage] = append(lt.stages[stage], d)
	lt.mu.Unlock()
}

func (lt *latencyTracker) percentiles(stage string) (p50, p99 time.Duration) {
	lt.mu.Lock()
	durations := make([]time.Duration, len(lt.stages[stage]))
	copy(durations, lt.stages[stage])
	lt.mu.Unlock()

	if len(durations) == 0 {
		return 0, 0
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	p50idx := len(durations) / 2
	p99idx := len(durations) * 99 / 100
	if p99idx >= len(durations) {
		p99idx = len(durations) - 1
	}

	return durations[p50idx], durations[p99idx]
}

var (
	_ workers.WorkerExecutor = (*throughputExecutor)(nil)
)
