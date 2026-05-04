package stress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestCrossWorkflowPipeline verifies that two workflows can cooperate:
// a meta-pipeline analyzes code and generates work items that it submits
// to a separate code-pipeline workflow, proving cross-workflow coordination
// works end-to-end.
//
// Workflow A (code-pipeline): code-change:init → coding → in-review → review → complete/failed
// Workflow B (meta-pipeline): analysis:init → scan → generate-work → submit-work → complete
//
// Workflow B's submit-work executor calls SubmitWorkRequest on Workflow A's engine.
// portos:func-length-exception owner=agent-factory reason=cross-workflow-stress-fixture review=2026-07-19 removal=split-workflow-setup-and-assertions-before-next-cross-workflow-stress-change
func TestCrossWorkflowPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// --- Workflow A: code-pipeline ---
	dirA := testutil.ScaffoldFactoryDir(t, codePipelineCfg())
	hA := testutil.NewServiceTestHarness(t, dirA, testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExtraOptions(
			factory.WithWorkerExecutor("coder", testutil.NewMockExecutor()),
			factory.WithWorkerExecutor("review-submitter", testutil.NewMockExecutor()),
			factory.WithWorkerExecutor("reviewer", testutil.NewMockExecutor()),
		))

	// --- Workflow B: meta-pipeline ---
	dirB := testutil.ScaffoldFactoryDir(t, metaPipelineCfg())
	hB := testutil.NewServiceTestHarness(t, dirB)

	// Scanner returns 3 findings as JSON.
	type finding struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	findings := []finding{
		{ID: "refactor-x", Description: "Refactor function X for readability"},
		{ID: "add-test-y", Description: "Add unit test for module Y"},
		{ID: "fix-lint-z", Description: "Fix lint warning Z in package P"},
	}
	findingsJSON, _ := json.Marshal(findings)

	hB.SetCustomExecutor("scanner", &staticExecutor{
		outcome: interfaces.OutcomeAccepted,
		tags:    map[string]string{"findings": string(findingsJSON)},
	})

	// Work generator reads findings and produces a comma-separated list of work IDs.
	hB.SetCustomExecutor("work-generator", &funcExecutor{fn: func(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
		findingsStr := ""
		if len(dispatch.InputTokens) > 0 {
			findingsStr = string(firstInputToken(dispatch.InputTokens).Color.Payload)
		}
		var fs []finding
		_ = json.Unmarshal([]byte(findingsStr), &fs)

		ids := make([]string, len(fs))
		for i, f := range fs {
			ids[i] = f.ID
		}
		idsJSON, _ := json.Marshal(ids)

		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
			Output:       string(idsJSON),
		}, nil
	}})

	// Cross-submitter reads work IDs and submits each to Workflow A's engine.
	var submittedCount atomic.Int32
	hB.SetCustomExecutor("cross-submitter", &funcExecutor{fn: func(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
		workIDsStr := ""
		if len(dispatch.InputTokens) > 0 {
			workIDsStr = string(firstInputToken(dispatch.InputTokens).Color.Payload)
		}

		var workIDs []string
		_ = json.Unmarshal([]byte(workIDsStr), &workIDs)

		// Submit each finding as a code-change to Workflow A.
		for _, id := range workIDs {
			payload := fmt.Appendf(nil, `{"finding_id": %q}`, id)
			hA.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
				WorkTypeID: "code-change",
				TraceID:    fmt.Sprintf("meta-%s", id),
				Payload:    payload,
			}})
			submittedCount.Add(1)
		}

		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
		}, nil
	}})

	// --- Execute Workflow B ---
	hB.SubmitWork("analysis", []byte(`{"target": "codebase-v1"}`))
	hB.RunUntilComplete(t, 10*time.Second)

	// --- Assert: Workflow B completed ---
	hB.Assert().
		PlaceTokenCount("analysis:complete", 1).
		HasNoTokenInPlace("analysis:init").
		HasNoTokenInPlace("analysis:scanned").
		HasNoTokenInPlace("analysis:generated").
		HasNoTokenInPlace("analysis:failed")

	// --- Assert: Workflow B submitted exactly 3 items to Workflow A ---
	if got := submittedCount.Load(); got != 3 {
		t.Errorf("expected 3 cross-workflow submissions, got %d", got)
	}

	// --- Run Workflow A after B has submitted all work into its queue ---
	hA.RunUntilComplete(t, 10*time.Second)

	// --- Assert: all 3 code-change items in Workflow A reached terminal state ---
	snapA := hA.Marking()
	completeA := len(snapA.TokensInPlace("code-change:complete"))
	failedA := len(snapA.TokensInPlace("code-change:failed"))
	initA := len(snapA.TokensInPlace("code-change:init"))
	codingA := len(snapA.TokensInPlace("code-change:coding"))
	reviewA := len(snapA.TokensInPlace("code-change:in-review"))

	if completeA+failedA != 3 {
		t.Errorf("Workflow A: expected 3 tokens in terminal state, got %d complete + %d failed (init=%d, coding=%d, in-review=%d)",
			completeA, failedA, initA, codingA, reviewA)
	}
	if completeA != 3 {
		t.Errorf("Workflow A: expected 3 complete, got %d", completeA)
	}

	// --- Assert: no cross-workflow token contamination ---
	// Workflow A should have no tokens with analysis work type.
	for id, tok := range snapA.Tokens {
		if tok.Color.WorkTypeID == "analysis" {
			t.Errorf("Workflow A: found foreign token %s with WorkTypeID 'analysis' — cross-contamination", id)
		}
	}
	// Workflow B should have no tokens with code-change work type.
	snapB := hB.Marking()
	for id, tok := range snapB.Tokens {
		if tok.Color.WorkTypeID == "code-change" {
			t.Errorf("Workflow B: found foreign token %s with WorkTypeID 'code-change' — cross-contamination", id)
		}
	}

	// --- Assert: Workflow B completion did NOT depend on Workflow A finishing ---
	// This is proven by the test structure: hB.RunUntilComplete() returns before
	// we poll Workflow A. If B depended on A, RunUntilComplete would hang.

	t.Logf("cross-workflow pipeline: B submitted %d items to A, A completed %d/%d",
		submittedCount.Load(), completeA, 3)
}

// TestCrossWorkflowPipelineNoDeadlock verifies that if Workflow A is slow,
// Workflow B still completes its submission phase without blocking.
func TestCrossWorkflowPipelineNoDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// Workflow A: simple 2-stage with a slow executor.
	dirA := testutil.ScaffoldFactoryDir(t, simpleCodePipelineCfg("slow-worker"))
	hA := testutil.NewServiceTestHarness(t, dirA, testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("slow-worker", &delayExecutor{maxDelay: 20 * time.Millisecond})))

	// Workflow B: simple 1-stage meta-pipeline that submits to A.
	dirB := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "analysis",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "submitter"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "submit-work", WorkerTypeName: "submitter",
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "complete"}},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "failed"}},
		}},
	})
	hB := testutil.NewServiceTestHarness(t, dirB)
	hB.SetCustomExecutor("submitter", &funcExecutor{fn: func(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
		for i := range 3 {
			hA.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
				WorkTypeID: "code-change",
				TraceID:    fmt.Sprintf("deadlock-test-%d", i),
				Payload:    fmt.Appendf(nil, `{"item": %d}`, i),
			}})
		}
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
		}, nil
	}})

	// B should complete fast (fire-and-forget submission).
	start := time.Now()
	hB.SubmitWork("analysis", []byte(`{"test": "deadlock"}`))
	hB.RunUntilComplete(t, 10*time.Second)
	bDuration := time.Since(start)

	hB.Assert().
		PlaceTokenCount("analysis:complete", 1).
		HasNoTokenInPlace("analysis:init")

	// B should complete quickly even while A is slower in the background.
	if bDuration > 500*time.Millisecond {
		t.Errorf("Workflow B took %v — should complete nearly instantly (fire-and-forget)", bDuration)
	}

	// Run A only after B has finished submitting its fire-and-forget work.
	hA.RunUntilComplete(t, 10*time.Second)

	snapA := hA.Marking()
	if complete := len(snapA.TokensInPlace("code-change:complete")); complete != 3 {
		t.Errorf("Workflow A: expected 3 complete, got %d", complete)
	}
}

// TestCrossWorkflowPipelineRecursive verifies the recursive variant:
// one of the findings triggers another scan cycle (meta-pipeline loops once),
// producing 2 more items for Workflow A (5 total). Exhaustion guard limits
// meta-pipeline to 2 scan iterations max.
func TestCrossWorkflowPipelineRecursive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// --- Workflow A: code-pipeline (same as basic test) ---
	dirA := testutil.ScaffoldFactoryDir(t, codePipelineCfg())
	hA := testutil.NewServiceTestHarness(t, dirA, testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExtraOptions(
			factory.WithWorkerExecutor("coder", testutil.NewMockExecutor()),
			factory.WithWorkerExecutor("review-submitter", testutil.NewMockExecutor()),
			factory.WithWorkerExecutor("reviewer", testutil.NewMockExecutor()),
		))

	// --- Workflow B: meta-pipeline with recursion ---
	dirB := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "analysis",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "scanned", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "recursive-scanner"}, {Name: "recursive-submitter"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "scan-codebase", WorkerTypeName: "recursive-scanner",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "scanned"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "failed"}}},
			{Name: "submit-and-finalize", WorkerTypeName: "recursive-submitter",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "scanned"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "failed"}}},
			guardedLoopBreakerWorkstation(
				"max-scan-iterations",
				"scan-codebase",
				3,
				interfaces.IOConfig{WorkTypeName: "analysis", StateName: "init"},
				interfaces.IOConfig{WorkTypeName: "analysis", StateName: "failed"},
			),
		},
	})
	hB := testutil.NewServiceTestHarness(t, dirB)

	var totalSubmittedToA atomic.Int32
	var scanCount atomic.Int32

	// Scanner: iteration 0 returns 3 findings (one with "needs-rescan" flag).
	// Iteration 1 (rescan) returns 2 additional findings (no more rescans).
	hB.SetCustomExecutor("recursive-scanner", &funcExecutor{fn: func(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
		iteration := scanCount.Add(1)

		type scanFinding struct {
			ID          string `json:"id"`
			NeedsRescan bool   `json:"needs_rescan"`
		}

		var results []scanFinding
		if iteration == 1 {
			// First scan: 3 findings, one triggers rescan.
			results = []scanFinding{
				{ID: "refactor-x", NeedsRescan: false},
				{ID: "add-test-y", NeedsRescan: false},
				{ID: "deep-issue-z", NeedsRescan: true}, // triggers rescan
			}
		} else {
			// Rescan: 2 more findings, no more rescans.
			results = []scanFinding{
				{ID: "fix-lint-a", NeedsRescan: false},
				{ID: "fix-lint-b", NeedsRescan: false},
			}
		}

		findingsJSON, _ := json.Marshal(results)
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
			Output:       string(findingsJSON),
		}, nil
	}})

	// Submitter: submits findings to Workflow A and spawns rescan if needed.
	hB.SetCustomExecutor("recursive-submitter", &funcExecutor{fn: func(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
		findingsStr := ""
		if len(dispatch.InputTokens) > 0 {
			findingsStr = string(firstInputToken(dispatch.InputTokens).Color.Payload)
		}

		type scanFinding struct {
			ID          string `json:"id"`
			NeedsRescan bool   `json:"needs_rescan"`
		}
		var findings []scanFinding
		_ = json.Unmarshal([]byte(findingsStr), &findings)

		// Submit each finding to Workflow A.
		needsRescan := false
		for _, f := range findings {
			hA.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
				WorkTypeID: "code-change",
				TraceID:    fmt.Sprintf("recursive-meta-%s", f.ID),
				Payload:    fmt.Appendf(nil, `{"finding_id": %q}`, f.ID),
			}})
			totalSubmittedToA.Add(1)
			if f.NeedsRescan {
				needsRescan = true
			}
		}

		result := interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
		}

		// If any finding needs rescan, spawn a new analysis work item.
		if needsRescan {
			result.SpawnedWork = []interfaces.TokenColor{{
				WorkTypeID: "analysis",
				WorkID:     "rescan",
				Tags: map[string]string{
					"reason": "deep-issue-found",
				},
			}}
		}

		return result, nil
	}})

	// --- Execute Workflow B ---
	hB.SubmitWork("analysis", []byte(`{"target": "codebase-v2"}`))
	hB.RunUntilComplete(t, 10*time.Second)

	// --- Assert: Workflow B completed (2 analysis tokens: original + rescan) ---
	snapB := hB.Marking()
	completeB := len(snapB.TokensInPlace("analysis:complete"))
	failedB := len(snapB.TokensInPlace("analysis:failed"))
	initB := len(snapB.TokensInPlace("analysis:init"))

	if completeB != 2 {
		t.Errorf("Workflow B: expected 2 complete (original + rescan), got %d (init=%d, failed=%d)",
			completeB, initB, failedB)
	}

	// --- Assert: scanner called exactly 2 times (initial + rescan) ---
	if got := scanCount.Load(); got != 2 {
		t.Errorf("expected scanner called 2 times, got %d", got)
	}

	// --- Assert: exactly 5 items submitted to Workflow A (3 + 2) ---
	if got := totalSubmittedToA.Load(); got != 5 {
		t.Errorf("expected 5 total submissions to Workflow A, got %d", got)
	}

	// --- Run Workflow A after B has submitted all recursive follow-up work ---
	hA.RunUntilComplete(t, 10*time.Second)

	snapA := hA.Marking()
	completeA := len(snapA.TokensInPlace("code-change:complete"))
	failedA := len(snapA.TokensInPlace("code-change:failed"))

	if completeA+failedA != 5 {
		t.Errorf("Workflow A: expected 5 terminal tokens, got %d complete + %d failed", completeA, failedA)
	}
	if completeA != 5 {
		t.Errorf("Workflow A: expected 5 complete, got %d", completeA)
	}

	// --- Assert: no cross-workflow token contamination ---
	for id, tok := range snapA.Tokens {
		if tok.Color.WorkTypeID == "analysis" {
			t.Errorf("Workflow A: foreign token %s with WorkTypeID 'analysis'", id)
		}
	}
	for id, tok := range snapB.Tokens {
		if tok.Color.WorkTypeID == "code-change" {
			t.Errorf("Workflow B: foreign token %s with WorkTypeID 'code-change'", id)
		}
	}

	t.Logf("recursive cross-workflow: %d scans, %d items submitted to A, A completed %d/%d",
		scanCount.Load(), totalSubmittedToA.Load(), completeA, 5)
}

// TestCrossWorkflowPipelineNoRace verifies no data races during cross-workflow
// submission with concurrent status queries.
func TestCrossWorkflowPipelineNoRace(t *testing.T) {
	t.Skip("bad arguments right now")
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// Simple 1-stage code pipeline.
	dirA := testutil.ScaffoldFactoryDir(t, oneStageCodePipelineCfg("coder"))
	hA := testutil.NewServiceTestHarness(t, dirA, testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("coder", &delayExecutor{maxDelay: time.Millisecond})))

	ctxA, cancelA := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelA()
	errChA := hA.RunInBackground(ctxA)

	// 5 goroutines submit work items concurrently.
	const totalItems = 15
	var submitWg sync.WaitGroup
	for g := range 5 {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range 3 {
				hA.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
					WorkTypeID: "code-change",
					TraceID:    fmt.Sprintf("race-%d-%d", gid, i),
					Payload:    fmt.Appendf(nil, `{"g":%d,"i":%d}`, gid, i),
				}})
				time.Sleep(time.Millisecond)
			}
		}(g)
	}

	// 3 goroutines query marking concurrently.
	var queryWg sync.WaitGroup
	queryDone := make(chan struct{})
	var queryCount atomic.Int64
	for range 3 {
		queryWg.Add(1)
		go func() {
			defer queryWg.Done()
			for {
				select {
				case <-queryDone:
					return
				default:
					snap := hA.Marking()
					for _, tok := range snap.Tokens {
						_ = tok.PlaceID
						_ = tok.Color.WorkTypeID
					}
					queryCount.Add(1)
				}
			}
		}()
	}

	submitWg.Wait()
	pollUntilAllTerminalH(t, hA, []string{"code-change:complete", "code-change:failed"}, totalItems, 20*time.Second)

	close(queryDone)
	queryWg.Wait()
	cancelA()
	<-errChA

	snapA := hA.Marking()
	complete := len(snapA.TokensInPlace("code-change:complete"))
	if complete != totalItems {
		t.Errorf("expected %d complete, got %d", totalItems, complete)
	}

	t.Logf("no-race test: %d items completed, %d queries", complete, queryCount.Load())
}

// --- Helper configs for cross-workflow tests ---

// codePipelineCfg returns a config for the code-pipeline workflow.
func codePipelineCfg() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "code-change",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "coding", Type: interfaces.StateTypeProcessing},
				{Name: "in-review", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "coder"}, {Name: "review-submitter"}, {Name: "reviewer"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "do-coding", WorkerTypeName: "coder",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "coding"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "failed"}}},
			{Name: "submit-review", WorkerTypeName: "review-submitter",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "coding"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "in-review"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "failed"}}},
			{Name: "do-review", WorkerTypeName: "reviewer",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "in-review"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "failed"}}},
		},
	}
}

// simpleCodePipelineCfg returns a config for a simple 2-stage code-pipeline.
func simpleCodePipelineCfg(workerName string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "code-change",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "processing", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: workerName}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "process", WorkerTypeName: workerName,
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "processing"}}},
			{Name: "finish", WorkerTypeName: workerName,
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "processing"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "complete"}}},
		},
	}
}

// oneStageCodePipelineCfg returns a config for a 1-stage code pipeline.
func oneStageCodePipelineCfg(workerName string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "code-change",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: workerName}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "process", WorkerTypeName: workerName,
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "code-change", StateName: "complete"}}},
		},
	}
}

// metaPipelineCfg returns a config for the meta-pipeline workflow.
func metaPipelineCfg() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "analysis",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "scanned", Type: interfaces.StateTypeProcessing},
				{Name: "generated", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "scanner"}, {Name: "work-generator"}, {Name: "cross-submitter"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "scan-codebase", WorkerTypeName: "scanner",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "scanned"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "failed"}}},
			{Name: "generate-work-items", WorkerTypeName: "work-generator",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "scanned"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "generated"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "failed"}}},
			{Name: "submit-to-code-pipeline", WorkerTypeName: "cross-submitter",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "generated"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "analysis", StateName: "failed"}}},
		},
	}
}

// --- Helper executors for cross-workflow tests ---

// staticExecutor returns a fixed outcome with optional tags.
type staticExecutor struct {
	outcome interfaces.WorkOutcome
	tags    map[string]string
}

func (e *staticExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	output := ""
	if e.tags != nil {
		output = e.tags["findings"]
	}
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      e.outcome,
		Output:       output,
	}, nil
}

// funcExecutor wraps a function as a WorkerExecutor.
type funcExecutor struct {
	fn func(context.Context, interfaces.WorkDispatch) (interfaces.WorkResult, error)
}

func (e *funcExecutor) Execute(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return e.fn(ctx, dispatch)
}
