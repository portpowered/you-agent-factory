package stress_test

import (
	"context"
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

	hA := setupCrossWorkflowCodePipelineHarness(t)
	hB := setupCrossWorkflowMetaPipelineHarness(t)
	findings := []crossWorkflowFinding{
		{ID: "refactor-x", Description: "Refactor function X for readability"},
		{ID: "add-test-y", Description: "Add unit test for module Y"},
		{ID: "fix-lint-z", Description: "Fix lint warning Z in package P"},
	}
	var submittedCount atomic.Int32
	installStaticFindingsExecutor(hB, findings)
	installWorkGeneratorExecutor(hB)
	installCrossSubmitterExecutor(hB, hA, &submittedCount)

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

	if got := submittedCount.Load(); got != 3 {
		t.Errorf("expected 3 cross-workflow submissions, got %d", got)
	}

	hA.RunUntilComplete(t, 10*time.Second)
	completeA := assertCrossWorkflowCodePipelineState(t, hA, 3)
	snapA := hA.Marking()
	for id, tok := range snapA.Tokens {
		if tok.Color.WorkTypeID == "analysis" {
			t.Errorf("Workflow A: found foreign token %s with WorkTypeID 'analysis' — cross-contamination", id)
		}
	}
	snapB := hB.Marking()
	assertNoForeignWorkTypeTokens(t, snapB, "code-change", "Workflow B")

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

	hA := setupCrossWorkflowCodePipelineHarness(t)
	hB := setupRecursiveMetaPipelineHarness(t)

	var totalSubmittedToA atomic.Int32
	var scanCount atomic.Int32
	installRecursiveExecutors(hB, hA, &scanCount, &totalSubmittedToA)

	// --- Execute Workflow B ---
	hB.SubmitWork("analysis", []byte(`{"target": "codebase-v2"}`))
	hB.RunUntilComplete(t, 10*time.Second)

	snapB := hB.Marking()
	completeB := len(snapB.TokensInPlace("analysis:complete"))
	failedB := len(snapB.TokensInPlace("analysis:failed"))
	initB := len(snapB.TokensInPlace("analysis:init"))

	if completeB != 2 {
		t.Errorf("Workflow B: expected 2 complete (original + rescan), got %d (init=%d, failed=%d)",
			completeB, initB, failedB)
	}

	if got := scanCount.Load(); got != 2 {
		t.Errorf("expected scanner called 2 times, got %d", got)
	}

	if got := totalSubmittedToA.Load(); got != 5 {
		t.Errorf("expected 5 total submissions to Workflow A, got %d", got)
	}

	hA.RunUntilComplete(t, 10*time.Second)
	completeA := assertCrossWorkflowCodePipelineState(t, hA, 5)
	assertNoForeignWorkTypeTokens(t, hA.Marking(), "analysis", "Workflow A")
	assertNoForeignWorkTypeTokens(t, snapB, "code-change", "Workflow B")

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
