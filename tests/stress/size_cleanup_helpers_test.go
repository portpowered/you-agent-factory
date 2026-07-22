package stress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type crossWorkflowFinding struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type recursiveScanFinding struct {
	ID          string `json:"id"`
	NeedsRescan bool   `json:"needs_rescan"`
}

func workerGeneratedBatchOutput(works []work.Work) string {
	encoded, err := json.Marshal(struct {
		Request work.WorkRequest `json:"request"`
	}{
		Request: work.WorkRequest{
			Type:  work.WorkRequestTypeFactoryRequestBatch,
			Works: works,
		},
	})
	if err != nil {
		panic(fmt.Sprintf("encode generated Work Request fixture: %v", err))
	}
	return string(encoded)
}

type multiWorkflowDef struct {
	name         string
	workType     string
	resourceName string
	resourceCap  int
	workerName   string
}

type throughputLargeScaleResult struct {
	snapshot            *factoryruntime.PetriMarkingSnapshot
	totalDuration       time.Duration
	heapGrowthMB        float64
	totalAllocMB        float64
	executorCallCount   int
	stageLatencyTracker *latencyTracker
}

func setupCrossWorkflowCodePipelineHarness(t *testing.T) *stressProcessHarness {
	t.Helper()

	dir := testutil.ScaffoldFactoryDir(t, codePipelineCfg())
	h := startStressProcessWithWorkerMux(t, dir)
	h.SetWorkerExecutor("coder", &acceptedCountingExecutor{})
	h.SetWorkerExecutor("review-submitter", &acceptedCountingExecutor{})
	h.SetWorkerExecutor("reviewer", &acceptedCountingExecutor{})
	return h
}

func setupCrossWorkflowMetaPipelineHarness(t *testing.T) *stressProcessHarness {
	t.Helper()

	dir := testutil.ScaffoldFactoryDir(t, metaPipelineCfg())
	return startStressProcessWithWorkerMux(t, dir)
}

func setupRecursiveMetaPipelineHarness(t *testing.T) *stressProcessHarness {
	t.Helper()

	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "analysis",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "scanned", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "recursive-scanner"}, {Name: "recursive-submitter"}},
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
	return startStressProcessWithWorkerMux(t, dir)
}

func installStaticFindingsExecutor(h *stressProcessHarness, findings []crossWorkflowFinding) {
	findingsJSON, _ := json.Marshal(findings)
	h.SetWorkerExecutor("scanner", &staticExecutor{
		outcome: workerexecution.OutcomeAccepted,
		output:  string(findingsJSON),
	})
}

func installWorkGeneratorExecutor(h *stressProcessHarness) {
	h.SetWorkerExecutor("work-generator", &funcExecutor{fn: func(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
		var findings []crossWorkflowFinding
		_ = json.Unmarshal([]byte(payloadFromDispatch(dispatch)), &findings)

		ids := make([]string, len(findings))
		for i, finding := range findings {
			ids[i] = finding.ID
		}
		idsJSON, _ := json.Marshal(ids)

		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       string(idsJSON),
		}, nil
	}})
}

func installCrossSubmitterExecutor(
	hMeta *stressProcessHarness,
	hCode *stressProcessHarness,
	submittedCount *atomic.Int32,
) {
	hMeta.SetWorkerExecutor("cross-submitter", &funcExecutor{fn: func(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
		var workIDs []string
		_ = json.Unmarshal([]byte(payloadFromDispatch(dispatch)), &workIDs)

		for _, id := range workIDs {
			hCode.SubmitFull(context.Background(), []work.SubmitRequest{{
				WorkTypeID: "code-change",
				TraceID:    fmt.Sprintf("meta-%s", id),
				Payload:    fmt.Appendf(nil, `{"finding_id": %q}`, id),
			}})
			submittedCount.Add(1)
		}

		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeAccepted,
		}, nil
	}})
}

func installRecursiveExecutors(
	hMeta *stressProcessHarness,
	hCode *stressProcessHarness,
	scanCount *atomic.Int32,
	submittedCount *atomic.Int32,
) {
	hMeta.SetWorkerExecutor("recursive-scanner", &funcExecutor{fn: func(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
		iteration := scanCount.Add(1)
		results := firstOrRescanFindings(iteration)
		findingsJSON, _ := json.Marshal(results)
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       string(findingsJSON),
		}, nil
	}})

	hMeta.SetWorkerExecutor("recursive-submitter", &funcExecutor{fn: func(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
		var findings []recursiveScanFinding
		_ = json.Unmarshal([]byte(payloadFromDispatch(dispatch)), &findings)

		needsRescan := submitRecursiveFindings(hCode, submittedCount, findings)
		result := workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeAccepted,
		}
		if needsRescan {
			result.Output = workerGeneratedBatchOutput([]work.Work{{
				Name:       "rescan",
				WorkID:     "rescan",
				WorkTypeID: "analysis",
				Tags:       map[string]string{"reason": "deep-issue-found"},
			}})
		}
		return result, nil
	}})
}

func firstOrRescanFindings(iteration int32) []recursiveScanFinding {
	if iteration == 1 {
		return []recursiveScanFinding{
			{ID: "refactor-x"},
			{ID: "add-test-y"},
			{ID: "deep-issue-z", NeedsRescan: true},
		}
	}
	return []recursiveScanFinding{
		{ID: "fix-lint-a"},
		{ID: "fix-lint-b"},
	}
}

func submitRecursiveFindings(
	hCode *stressProcessHarness,
	submittedCount *atomic.Int32,
	findings []recursiveScanFinding,
) bool {
	needsRescan := false
	for _, finding := range findings {
		hCode.SubmitFull(context.Background(), []work.SubmitRequest{{
			WorkTypeID: "code-change",
			TraceID:    fmt.Sprintf("recursive-meta-%s", finding.ID),
			Payload:    fmt.Appendf(nil, `{"finding_id": %q}`, finding.ID),
		}})
		submittedCount.Add(1)
		if finding.NeedsRescan {
			needsRescan = true
		}
	}
	return needsRescan
}

func payloadFromDispatch(dispatch work.WorkDispatch) string {
	if len(dispatch.InputTokens) == 0 {
		return ""
	}
	return string(firstInputToken(dispatch.InputTokens).Color.Payload)
}

func assertCrossWorkflowCodePipelineState(t *testing.T, h *stressProcessHarness, expectedTerminal int) int {
	t.Helper()

	snap := h.Marking()
	complete := len(snap.TokensInPlace("code-change:complete"))
	failed := len(snap.TokensInPlace("code-change:failed"))
	initCount := len(snap.TokensInPlace("code-change:init"))
	codingCount := len(snap.TokensInPlace("code-change:coding"))
	reviewCount := len(snap.TokensInPlace("code-change:in-review"))

	if complete+failed != expectedTerminal {
		t.Errorf("Workflow A: expected %d tokens in terminal state, got %d complete + %d failed (init=%d, coding=%d, in-review=%d)",
			expectedTerminal, complete, failed, initCount, codingCount, reviewCount)
	}
	if complete != expectedTerminal {
		t.Errorf("Workflow A: expected %d complete, got %d", expectedTerminal, complete)
	}
	return complete
}

func assertNoForeignWorkTypeTokens(t *testing.T, snap *factoryruntime.PetriMarkingSnapshot, foreignType string, workflowLabel string) {
	t.Helper()

	for id, tok := range snap.Tokens {
		if tok.Color.WorkTypeID == foreignType {
			t.Errorf("%s: found foreign token %s with WorkTypeID %q", workflowLabel, id, foreignType)
		}
	}
}

func setupMultiWorkflowHarnesses(t *testing.T, defs []multiWorkflowDef, _ int) []*stressProcessHarness {
	t.Helper()

	harnesses := make([]*stressProcessHarness, len(defs))
	for i, def := range defs {
		resource := interfaces.ResourceConfig{Name: def.resourceName, Capacity: def.resourceCap}
		cfg := &interfaces.FactoryConfig{
			WorkTypes: []interfaces.WorkTypeConfig{{
				Name: def.workType,
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			}},
			Resources: []interfaces.ResourceConfig{resource},
			Workers:   []interfaces.FactoryWorkerConfig{{Name: def.workerName}, {Name: def.workerName + "-finish"}},
			Workstations: []interfaces.FactoryWorkstationConfig{
				{
					Name: def.name + "-process", WorkerTypeName: def.workerName,
					Inputs:    []interfaces.IOConfig{{WorkTypeName: def.workType, StateName: "init"}},
					Outputs:   []interfaces.IOConfig{{WorkTypeName: def.workType, StateName: "processing"}},
					Resources: []interfaces.ResourceConfig{resource},
				},
				{
					Name: def.name + "-finish", WorkerTypeName: def.workerName + "-finish",
					Inputs:  []interfaces.IOConfig{{WorkTypeName: def.workType, StateName: "processing"}},
					Outputs: []interfaces.IOConfig{{WorkTypeName: def.workType, StateName: "complete"}},
				},
			},
		}
		dir := testutil.ScaffoldFactoryDir(t, cfg)
		harnesses[i] = startStressProcess(t, dir, workerExecutorProvider{executor: workerMuxExecutor{
			def.workerName:             &acceptedCountingExecutor{},
			def.workerName + "-finish": &acceptedCountingExecutor{},
		}})
		t.Cleanup(harnesses[i].Stop)
	}
	return harnesses
}

func submitWorkflowWorkItems(
	t *testing.T,
	harnesses []*stressProcessHarness,
	defs []multiWorkflowDef,
	itemsPerWorkflow int,
) {
	t.Helper()

	for i, def := range defs {
		for j := range itemsPerWorkflow {
			harnesses[i].SubmitFull(context.Background(), []work.SubmitRequest{{
				WorkTypeID: def.workType,
				WorkID:     fmt.Sprintf("%s-work-%d", def.name, j),
				TraceID:    fmt.Sprintf("%s-trace-%d", def.name, j),
				Payload:    fmt.Appendf(nil, `{"workflow": %q, "item": %d}`, def.name, j),
			}})
		}
	}
}

func runHarnessesConcurrently(t *testing.T, harnesses []*stressProcessHarness, timeout time.Duration) {
	t.Helper()

	var wg sync.WaitGroup
	for i := range harnesses {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			harnesses[idx].WaitForTerminalCount(5, timeout)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("multi-workflow execution did not complete within %v timeout", timeout)
	}
}

func assertMultiWorkflowStates(
	t *testing.T,
	harnesses []*stressProcessHarness,
	defs []multiWorkflowDef,
	itemsPerWorkflow int,
) {
	t.Helper()

	for i, def := range defs {
		session := harnesses[i].Session()
		completeCount := sessionPlaceCount(session, def.workType+":complete")
		initCount := sessionPlaceCount(session, def.workType+":init")
		processingCount := sessionPlaceCount(session, def.workType+":processing")

		if completeCount != itemsPerWorkflow {
			failedCount := sessionPlaceCount(session, def.workType+":failed")
			t.Errorf("workflow %q: expected %d tokens in complete, got %d (init=%d, processing=%d, failed=%d)",
				def.name, itemsPerWorkflow, completeCount, initCount, processingCount, failedCount)
		}
		if initCount != 0 {
			t.Errorf("workflow %q: expected 0 tokens in init, got %d", def.name, initCount)
		}
		if processingCount != 0 {
			t.Errorf("workflow %q: expected 0 tokens in processing, got %d", def.name, processingCount)
		}
	}
}

func assertNoCrossWorkflowContamination(t *testing.T, harnesses []*stressProcessHarness, defs []multiWorkflowDef) {
	t.Helper()

	for i, def := range defs {
		session := harnesses[i].Session()
		foreignTypes := make(map[string]string)
		for j, other := range defs {
			if j == i {
				continue
			}
			foreignTypes[other.workType] = other.name
			foreignTypes[other.resourceName] = other.name
		}
		if session.Runtime.Petri == nil {
			continue
		}
		for _, token := range session.Runtime.Petri.Marking {
			if owner, isForeign := foreignTypes[token.WorkType]; isForeign {
				t.Errorf("workflow %q: found token with WorkTypeID %q belonging to workflow %q — cross-contamination detected",
					def.name, token.WorkType, owner)
			}
		}
	}
}

func assertWorkflowResourceIsolation(t *testing.T, harnesses []*stressProcessHarness, defs []multiWorkflowDef) {
	t.Helper()

	for i, def := range defs {
		assertPublicResourceUsage(t, harnesses[i].Session(), def.resourceName, def.resourceCap, def.resourceCap)
	}
}

func runThroughputLargeScaleScenario(
	t *testing.T,
	totalItems int,
	pipelineStages int,
	workerDelay time.Duration,
	timeout time.Duration,
) throughputLargeScaleResult {
	t.Helper()

	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(pipelineStages, "pipeline-worker"))
	tracker := newLatencyTracker(pipelineStages)
	executor := &throughputExecutor{delay: workerDelay, tracker: tracker}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: executor})

	runtimeGCAndRead := func() runtimeMemSnapshot {
		return readRuntimeMemSnapshot()
	}

	memBefore := runtimeGCAndRead()
	startTime := time.Now()
	submitThroughputWorkload(h, totalItems, 10, 10, time.Millisecond)
	h.WaitForTerminalCount(totalItems, timeout-2*time.Second)
	totalDuration := time.Since(startTime)

	snapshot := h.Marking()
	h.Stop()
	memAfter := runtimeGCAndRead()

	return throughputLargeScaleResult{
		snapshot:            snapshot,
		totalDuration:       totalDuration,
		heapGrowthMB:        bytesGrowthMB(memAfter.heapAlloc, memBefore.heapAlloc),
		totalAllocMB:        bytesGrowthMB(memAfter.totalAlloc, memBefore.totalAlloc),
		executorCallCount:   executor.callCount(),
		stageLatencyTracker: tracker,
	}
}

type runtimeMemSnapshot struct {
	heapAlloc  uint64
	totalAlloc uint64
}

func readRuntimeMemSnapshot() runtimeMemSnapshot {
	var memStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	return runtimeMemSnapshot{heapAlloc: memStats.HeapAlloc, totalAlloc: memStats.TotalAlloc}
}

func submitThroughputWorkload(
	h *stressProcessHarness,
	totalItems int,
	numSubmitters int,
	yieldEvery int,
	yieldDuration time.Duration,
) {
	var submitWg sync.WaitGroup
	itemsPerSubmitter := totalItems / numSubmitters
	for g := range numSubmitters {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range itemsPerSubmitter {
				h.SubmitFull(context.Background(), []work.SubmitRequest{{
					WorkTypeID: "task",
					TraceID:    fmt.Sprintf("trace-%d-%d", gid, i),
					Payload:    fmt.Appendf(nil, `{"g":%d,"i":%d}`, gid, i),
				}})
				if yieldEvery > 0 && i%yieldEvery == yieldEvery-1 {
					time.Sleep(yieldDuration)
				}
			}
		}(g)
	}
	submitWg.Wait()
}

func assertThroughputLargeScaleResult(
	t *testing.T,
	result throughputLargeScaleResult,
	totalItems int,
	timeout time.Duration,
) {
	t.Helper()

	pipelineTerminalPlaces := []string{"task:complete", "task:failed"}
	terminalCount := countTerminalTokens(result.snapshot, pipelineTerminalPlaces)
	if terminalCount != totalItems {
		t.Errorf("expected %d terminal tokens, got %d", totalItems, terminalCount)
	}
	if len(result.snapshot.Tokens) != totalItems {
		t.Errorf("expected %d total tokens, got %d", totalItems, len(result.snapshot.Tokens))
	}
	assertAllTokensTerminal(t, result.snapshot)
	assertNoDuplicateTokenIDs(t, result.snapshot)
	if result.heapGrowthMB > 500 {
		t.Errorf("heap growth %.1fMB exceeds 500MB limit", result.heapGrowthMB)
	}
	if result.totalDuration > timeout {
		t.Errorf("execution time %v exceeds %v limit", result.totalDuration, timeout)
	}
}

func assertAllTokensTerminal(t *testing.T, snap *factoryruntime.PetriMarkingSnapshot) {
	t.Helper()

	for id, tok := range snap.Tokens {
		if tok.PlaceID != "task:complete" && tok.PlaceID != "task:failed" {
			t.Errorf("token %s stuck in non-terminal place %s", id, tok.PlaceID)
		}
	}
}

func assertNoDuplicateTokenIDs(t *testing.T, snap *factoryruntime.PetriMarkingSnapshot) {
	t.Helper()

	tokenIDs := make(map[string]bool, len(snap.Tokens))
	for _, tok := range snap.Tokens {
		if tokenIDs[tok.ID] {
			t.Errorf("duplicate token ID: %s", tok.ID)
		}
		tokenIDs[tok.ID] = true
	}
}

func logThroughputLargeScaleResult(
	t *testing.T,
	result throughputLargeScaleResult,
	totalItems int,
	pipelineStages int,
	workerDelay time.Duration,
) {
	t.Helper()

	throughput := float64(totalItems) / result.totalDuration.Seconds()
	t.Logf("=== Throughput Results ===")
	t.Logf("Total items:       %d", totalItems)
	t.Logf("Pipeline stages:   %d", pipelineStages)
	t.Logf("Worker delay:      %v", workerDelay)
	t.Logf("Total duration:    %v", result.totalDuration)
	t.Logf("Throughput:         %.1f items/sec", throughput)
	t.Logf("Heap growth:       %.1f MB", result.heapGrowthMB)
	t.Logf("Total alloc:       %.1f MB", result.totalAllocMB)
	t.Logf("Executor calls:    %d", result.executorCallCount)

	for stage := 1; stage <= pipelineStages; stage++ {
		stageName := fmt.Sprintf("step%d", stage)
		p50, p99 := result.stageLatencyTracker.percentiles(stageName)
		t.Logf("Stage %s:  p50=%v  p99=%v", stageName, p50, p99)
	}
	p50, p99 := result.stageLatencyTracker.percentiles("finish")
	t.Logf("Stage finish:  p50=%v  p99=%v", p50, p99)
}

func poisonSubmitConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "processing", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "w"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "process", WorkerTypeName: "w",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
			},
			{
				Name: "finish", WorkerTypeName: "w",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			},
		},
	}
}

func newPoisonSubmitHarness(t *testing.T) *stressProcessHarness {
	t.Helper()

	dir := testutil.ScaffoldFactoryDir(t, poisonSubmitConfig())
	return startStressProcess(t, dir, workerExecutorProvider{
		executor: testutil.NewMockExecutor(),
	})
}

func assertSingleCompletedSubmission(t *testing.T, h *stressProcessHarness, label string) {
	t.Helper()

	h.WaitForTerminalCount(1, 10*time.Second)
	snap := h.Marking()
	if completeCount := len(snap.TokensInPlace("task:complete")); completeCount != 1 {
		t.Errorf("expected 1 complete token with %s, got %d", label, completeCount)
	}
}

func assertPoisonIsolationMarking(t *testing.T, snap *factoryruntime.PetriMarkingSnapshot, wantItems int, workflowLabel string) {
	t.Helper()

	if complete := len(snap.TokensInPlace("task:complete")); complete != wantItems {
		t.Errorf("%s: expected %d complete, got %d", workflowLabel, wantItems, complete)
	}
	if len(snap.Tokens) != wantItems {
		t.Errorf("%s: expected %d total tokens, got %d", workflowLabel, wantItems, len(snap.Tokens))
	}
}

func buildTargetWorkflowNet(maxVisits int) (*factoryruntime.Net, error) {
	net := newTargetWorkflowNet()
	addTargetWorkflowPlaces(net)
	addTargetWorkflowTransitions(net, maxVisits)
	factoryruntime.NormalizeTransitionTopology(net, nil)
	return net, nil
}

func newTargetWorkflowNet() *factoryruntime.Net {
	return &factoryruntime.Net{
		ID:          "target-code-factory",
		Places:      make(map[string]*factoryruntime.PetriPlace),
		Transitions: make(map[string]*factoryruntime.PetriTransition),
		WorkTypes: map[string]*factoryruntime.WorkType{
			"task": {
				ID:   "task",
				Name: "task",
				States: []factoryruntime.StateDefinition{
					{Value: "init", Category: factoryruntime.StateCategoryInitial},
					{Value: "processing", Category: factoryruntime.StateCategoryProcessing},
					{Value: "complete", Category: factoryruntime.StateCategoryTerminal},
					{Value: "failed", Category: factoryruntime.StateCategoryFailed},
				},
			},
		},
		Resources: make(map[string]*factoryruntime.ResourceDef),
	}
}

func addTargetWorkflowPlaces(net *factoryruntime.Net) {
	for _, place := range net.WorkTypes["task"].GeneratePlaces() {
		net.Places[place.ID] = place
	}
}

func addTargetWorkflowTransitions(net *factoryruntime.Net, maxVisits int) {
	net.Transitions["execute-task"] = buildTargetWorkflowTransition(
		"execute-task",
		"executor",
		"task:init",
		"task:processing",
		nil,
		true,
	)
	net.Transitions["finish-task"] = buildTargetWorkflowTransition(
		"finish-task",
		"finisher",
		"task:processing",
		"task:complete",
		nil,
		true,
	)
	net.Transitions["auto-retry-limit"] = buildTargetWorkflowTransition(
		"auto-retry-limit",
		"",
		"task:init",
		"task:failed",
		&factoryruntime.PetriVisitCountGuard{MaxVisits: maxVisits},
		false,
	)
}

func buildTargetWorkflowTransition(
	id string,
	workerType string,
	inputPlace string,
	outputPlace string,
	guard *factoryruntime.PetriVisitCountGuard,
	withFailureArc bool,
) *factoryruntime.PetriTransition {
	transition := &factoryruntime.PetriTransition{
		ID:         id,
		Name:       id,
		Type:       factoryruntime.PetriTransitionNormal,
		WorkerType: workerType,
		InputArcs:  []factoryruntime.PetriArc{buildSingleInputArc(inputPlace, guard)},
		OutputArcs: []factoryruntime.PetriArc{{
			PlaceID:     outputPlace,
			Direction:   factoryruntime.PetriArcOutput,
			Cardinality: singleArcCardinality(),
		}},
	}
	if withFailureArc {
		transition.FailureArcs = []factoryruntime.PetriArc{{
			PlaceID:     "task:failed",
			Direction:   factoryruntime.PetriArcOutput,
			Cardinality: singleArcCardinality(),
		}}
	}
	return transition
}

func buildSingleInputArc(placeID string, guard *factoryruntime.PetriVisitCountGuard) factoryruntime.PetriArc {
	arc := factoryruntime.PetriArc{
		PlaceID:     placeID,
		Direction:   factoryruntime.PetriArcInput,
		Mode:        interfaces.ArcModeConsume,
		Guard:       guard,
		Cardinality: singleArcCardinality(),
	}
	if guard != nil {
		arc.Name = "retry-token"
	}
	return arc
}

func singleArcCardinality() factoryruntime.PetriArcCardinality {
	return factoryruntime.PetriArcCardinality{Mode: factoryruntime.PetriCardinalityOne}
}
