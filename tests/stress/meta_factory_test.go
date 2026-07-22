package stress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestMetaFactoryWorkflow proves the meta-factory pattern: a workflow that
// analyzes execution statistics and produces workflow modifications, which
// are then validated and applied.
//
// Meta-workflow: analyze-stats:init → optimization-proposal:init → validation:init → apply-changes:init → complete
//
// Assertions:
//   - Meta-workflow completes successfully
//   - The applier produces a modified workflow definition
//   - Meta-workflow terminates (does not loop indefinitely)
func TestMetaFactoryWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, buildMetaFactoryCfg(5))
	tracker := &metaFactoryTracker{}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: metaFactoryExecutors(
		tracker,
		&validatorExecutor{tracker: tracker},
	)})
	t.Cleanup(h.Stop)

	// Submit an analysis request.
	h.SubmitWork("analyze-stats", []byte(`{"factory_id": "code-factory", "metric": "transition_latency"}`))

	h.WaitForTerminalCount(3, 10*time.Second)

	session := h.Session()
	for _, place := range []string{"analyze-stats:complete", "optimization-proposal:complete", "apply-changes:complete"} {
		assertSessionPlaceCount(t, session, place, 1)
	}
	for _, place := range []string{
		"analyze-stats:init", "optimization-proposal:init", "apply-changes:init",
		"analyze-stats:failed", "optimization-proposal:failed", "apply-changes:failed",
	} {
		assertSessionPlaceCount(t, session, place, 0)
	}

	// Assert: each stage was called exactly once.
	if tracker.analyzerCalls() != 1 {
		t.Errorf("expected analyzer called 1 time, got %d", tracker.analyzerCalls())
	}
	if tracker.validatorCalls() != 1 {
		t.Errorf("expected validator called 1 time, got %d", tracker.validatorCalls())
	}
	if tracker.applierCalls() != 1 {
		t.Errorf("expected applier called 1 time, got %d", tracker.applierCalls())
	}

	// Assert: the root-process workflow reached the injected applier and it
	// produced the modified definition. Static net-policy matrices belong to
	// Factory Runtime and are not re-run from this customer-scale stress test.
	modifiedNet := tracker.getModifiedNet()
	if modifiedNet == nil {
		t.Fatal("applier did not produce a modified net")
	}
}

// TestMetaFactoryWithRejectionLoop verifies the meta-factory handles
// validation rejection (rejected proposal loops back) and eventually succeeds.
func TestMetaFactoryWithRejectionLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, buildMetaFactoryCfg(5))
	tracker := &metaFactoryTracker{}

	// Validator rejects the first 2 attempts, accepts the 3rd.
	h := startStressProcess(t, dir, workerExecutorProvider{executor: metaFactoryExecutors(
		tracker,
		&rejectingValidatorExecutor{tracker: tracker, rejectUntilN: 3},
	)})
	t.Cleanup(h.Stop)

	h.SubmitWork("analyze-stats", []byte(`{"factory_id": "code-factory", "metric": "error_rate"}`))

	h.WaitForTerminalCount(3, 10*time.Second)

	session := h.Session()
	assertSessionPlaceCount(t, session, "analyze-stats:complete", 1)
	assertSessionPlaceCount(t, session, "optimization-proposal:complete", 1)
	assertSessionPlaceCount(t, session, "apply-changes:complete", 1)

	// Validator was called 3 times (2 rejections + 1 accept).
	if tracker.validatorCalls() != 3 {
		t.Errorf("expected validator called 3 times, got %d", tracker.validatorCalls())
	}
}

// TestMetaFactoryGuardedLoopBreakerTerminatesRejectedValidationLoop verifies
// that the guarded loop breaker fires when the validation loop exceeds max
// iterations.
func TestMetaFactoryGuardedLoopBreakerTerminatesRejectedValidationLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, buildMetaFactoryCfg(3))
	tracker := &metaFactoryTracker{}

	// Validator always rejects -> guarded loop breaker should terminate the loop.
	h := startStressProcess(t, dir, workerExecutorProvider{executor: metaFactoryExecutors(
		tracker,
		&alwaysRejectingValidatorExecutor{tracker: tracker},
	)})
	t.Cleanup(h.Stop)

	h.SubmitWork("analyze-stats", []byte(`{"factory_id": "code-factory"}`))

	h.WaitForTerminalCount(2, 10*time.Second)

	session := h.Session()
	assertSessionPlaceCount(t, session, "analyze-stats:complete", 1)
	assertSessionPlaceCount(t, session, "optimization-proposal:failed", 1)
	assertSessionPlaceCount(t, session, "optimization-proposal:init", 0)
	assertSessionPlaceCount(t, session, "optimization-proposal:validated", 0)
	assertSessionPlaceCount(t, session, "apply-changes:init", 0)
	assertSessionPlaceCount(t, session, "apply-changes:applied", 0)

	// Validator was called 3 times (the max visits before the loop breaker fired).
	if calls := tracker.validatorCalls(); calls != 3 {
		t.Errorf("expected validator called 3 times before guarded loop breaker, got %d", calls)
	}

}

// TestMetaFactoryTimeout verifies the meta-factory completes within a
// reasonable time bound.
func TestMetaFactoryTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		dir := testutil.ScaffoldFactoryDir(t, buildMetaFactoryCfg(5))
		tracker := &metaFactoryTracker{}
		h := startStressProcess(t, dir, workerExecutorProvider{executor: metaFactoryExecutors(
			tracker,
			&validatorExecutor{tracker: tracker},
		)})
		defer h.Stop()

		h.SubmitWork("analyze-stats", []byte(`{"factory_id": "test"}`))

		h.WaitForTerminalCount(3, 10*time.Second)
	}()

	select {
	case <-done:
		// Completed within timeout.
	case <-time.After(10 * time.Second):
		t.Fatal("meta-factory workflow did not complete within 10s timeout")
	}
}

// --- Helpers ---

// buildMetaFactoryCfg constructs the meta-factory config with the given
// max visits on the validation rejection loop.
func buildMetaFactoryCfg(maxVisits int) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "analyze-stats", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "analyzed", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			}},
			{Name: "optimization-proposal", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "validated", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			}},
			{Name: "apply-changes", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "applied", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			}},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "analyzer"}, {Name: "proposal-emitter"}, {Name: "validator-worker", StopToken: "COMPLETE"},
			{Name: "apply-emitter"}, {Name: "applier"}, {Name: "finalizer"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "run-analysis", WorkerTypeName: "analyzer",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "analyze-stats", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "analyze-stats", StateName: "analyzed"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "analyze-stats", StateName: "failed"}}},
			{Name: "emit-proposal", WorkerTypeName: "proposal-emitter",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "analyze-stats", StateName: "analyzed"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "analyze-stats", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "analyze-stats", StateName: "failed"}}},
			{Name: "validate-proposal", WorkerTypeName: "validator-worker",
				Inputs:      []interfaces.IOConfig{{WorkTypeName: "optimization-proposal", StateName: "init"}},
				Outputs:     []interfaces.IOConfig{{WorkTypeName: "optimization-proposal", StateName: "validated"}},
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "optimization-proposal", StateName: "init"}},
				OnFailure:   []interfaces.IOConfig{{WorkTypeName: "optimization-proposal", StateName: "failed"}}},
			{Name: "emit-apply", WorkerTypeName: "apply-emitter",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "optimization-proposal", StateName: "validated"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "optimization-proposal", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "optimization-proposal", StateName: "failed"}}},
			{Name: "apply-modification", WorkerTypeName: "applier",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "apply-changes", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "apply-changes", StateName: "applied"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "apply-changes", StateName: "failed"}}},
			{Name: "finalize-apply", WorkerTypeName: "finalizer",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "apply-changes", StateName: "applied"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "apply-changes", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "apply-changes", StateName: "failed"}}},
			guardedLoopBreakerWorkstation(
				"max-validation-retries",
				"validate-proposal",
				maxVisits,
				interfaces.IOConfig{WorkTypeName: "optimization-proposal", StateName: "init"},
				interfaces.IOConfig{WorkTypeName: "optimization-proposal", StateName: "failed"},
			),
		},
	}
}

func metaFactoryExecutors(
	tracker *metaFactoryTracker,
	validator workerexecution.WorkerExecutor,
) workerMuxExecutor {
	return workerMuxExecutor{
		"analyzer":         &analyzerExecutor{tracker: tracker},
		"proposal-emitter": &proposalEmitterExecutor{tracker: tracker},
		"validator-worker": validator,
		"apply-emitter":    &applyEmitterExecutor{tracker: tracker},
		"applier":          &applierExecutor{tracker: tracker},
		"finalizer":        &acceptedCountingExecutor{},
	}
}

// --- Shared tracker ---

// metaFactoryTracker tracks executor calls and artifacts across the meta-factory pipeline.
type metaFactoryTracker struct {
	mu             sync.Mutex
	analyzerCount  int
	validatorCount int
	applierCount   int
	storedNet      *factoryruntime.Net
}

func (t *metaFactoryTracker) recordAnalyzer()  { t.mu.Lock(); t.analyzerCount++; t.mu.Unlock() }
func (t *metaFactoryTracker) recordValidator() { t.mu.Lock(); t.validatorCount++; t.mu.Unlock() }
func (t *metaFactoryTracker) recordApplier()   { t.mu.Lock(); t.applierCount++; t.mu.Unlock() }
func (t *metaFactoryTracker) analyzerCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.analyzerCount
}
func (t *metaFactoryTracker) validatorCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.validatorCount
}
func (t *metaFactoryTracker) applierCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.applierCount
}

func (t *metaFactoryTracker) storeModifiedNet(n *factoryruntime.Net) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.storedNet = n
}

func (t *metaFactoryTracker) getModifiedNet() *factoryruntime.Net {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.storedNet
}

// --- Executor implementations ---

// workflowModification represents a proposed change to a workflow definition.
type workflowModification struct {
	TransitionID string `json:"transition_id"`
	Field        string `json:"field"`
	OldValue     int    `json:"old_value"`
	NewValue     int    `json:"new_value"`
	Reason       string `json:"reason"`
}

// analyzerExecutor simulates reading factory stats and returning a proposed modification.
type analyzerExecutor struct {
	tracker *metaFactoryTracker
}

func (e *analyzerExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.tracker.recordAnalyzer()

	// Simulate: analyzed stats, found that transition X has high retry rate.
	proposal := workflowModification{
		TransitionID: "execute-task",
		Field:        "max_retries",
		OldValue:     3,
		NewValue:     5,
		Reason:       "High retry rate (42%) indicates transient failures — increasing retries will improve completion rate",
	}
	proposalJSON, _ := json.Marshal(proposal)

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       string(proposalJSON),
	}, nil
}

// proposalEmitterExecutor spawns an optimization-proposal work item.
type proposalEmitterExecutor struct {
	tracker *metaFactoryTracker
}

func (e *proposalEmitterExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	proposalJSON := ""
	traceID := ""
	if len(dispatch.InputTokens) > 0 {
		proposalJSON = string(firstInputToken(dispatch.InputTokens).Color.Payload)
		traceID = firstInputToken(dispatch.InputTokens).Color.TraceID
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output: workerGeneratedBatchOutput([]work.Work{{
			Name:       fmt.Sprintf("proposal-%s", traceID),
			WorkTypeID: "optimization-proposal",
			WorkID:     fmt.Sprintf("proposal-%s", traceID),
			Tags: map[string]string{
				"proposal": proposalJSON,
			},
			Payload: "0",
		}}),
	}, nil
}

// validatorExecutor checks the proposal is structurally valid (always accepts valid proposals).
type validatorExecutor struct {
	tracker *metaFactoryTracker
}

func (e *validatorExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.tracker.recordValidator()

	proposalJSON := ""
	if len(dispatch.InputTokens) > 0 {
		if tags := firstInputToken(dispatch.InputTokens).Color.Tags; tags != nil {
			proposalJSON = tags["proposal"]
		}
	}

	var proposal workflowModification
	if err := json.Unmarshal([]byte(proposalJSON), &proposal); err != nil {
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeRejected,
			Feedback:     fmt.Sprintf("invalid proposal JSON: %v", err),
		}, nil
	}

	if proposal.TransitionID == "" || proposal.Field == "" {
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeRejected,
			Feedback:     "proposal missing required fields: transition_id and field",
		}, nil
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

// rejectingValidatorExecutor rejects the first N-1 attempts, accepts the Nth.
type rejectingValidatorExecutor struct {
	tracker      *metaFactoryTracker
	rejectUntilN int
}

func (e *rejectingValidatorExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.tracker.recordValidator()

	callNum := e.tracker.validatorCalls()
	if callNum < e.rejectUntilN {
		iteration := "0"
		if len(dispatch.InputTokens) > 0 {
			n, _ := strconv.Atoi(string(firstInputToken(dispatch.InputTokens).Color.Payload))
			iteration = strconv.Itoa(n + 1)
		}

		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeRejected,
			Feedback:     fmt.Sprintf("proposal needs refinement (attempt %d/%d)", callNum, e.rejectUntilN),
			Output:       iteration,
		}, nil
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

// alwaysRejectingValidatorExecutor always rejects — used to test guarded loop-breaker termination.
type alwaysRejectingValidatorExecutor struct {
	tracker *metaFactoryTracker
}

func (e *alwaysRejectingValidatorExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.tracker.recordValidator()

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeRejected,
		Feedback:     "proposal is structurally invalid — always rejected for testing",
	}, nil
}

// applyEmitterExecutor spawns an apply-changes work item.
type applyEmitterExecutor struct {
	tracker *metaFactoryTracker
}

func (e *applyEmitterExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	proposalJSON := ""
	traceID := ""
	if len(dispatch.InputTokens) > 0 {
		if tags := firstInputToken(dispatch.InputTokens).Color.Tags; tags != nil {
			proposalJSON = tags["proposal"]
		}
		traceID = firstInputToken(dispatch.InputTokens).Color.TraceID
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output: workerGeneratedBatchOutput([]work.Work{{
			Name:       fmt.Sprintf("apply-%s", traceID),
			WorkTypeID: "apply-changes",
			WorkID:     fmt.Sprintf("apply-%s", traceID),
			Tags: map[string]string{
				"proposal": proposalJSON,
			},
		}}),
	}, nil
}

// applierExecutor applies the modification to a copy of a target workflow
// definition and validates the result using all CPN validators.
type applierExecutor struct {
	tracker *metaFactoryTracker
}

func (e *applierExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.tracker.recordApplier()

	proposalJSON := ""
	if len(dispatch.InputTokens) > 0 {
		if tags := firstInputToken(dispatch.InputTokens).Color.Tags; tags != nil {
			proposalJSON = tags["proposal"]
		}
	}

	var proposal workflowModification
	if proposalJSON != "" {
		_ = json.Unmarshal([]byte(proposalJSON), &proposal)
	}

	// Build a "target" workflow and apply the proposed modification.
	// Simulates loading code-factory workflow and modifying max_retries.
	modifiedNet, err := buildTargetWorkflowNet(proposal.NewValue)
	if err != nil {
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        fmt.Sprintf("failed to build modified workflow: %v", err),
		}, nil
	}

	// Store the modified net for post-hoc validation in the test.
	e.tracker.storeModifiedNet(modifiedNet)

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       fmt.Sprintf("set %s.%s = %d (was %d)", proposal.TransitionID, proposal.Field, proposal.NewValue, proposal.OldValue),
	}, nil
}
