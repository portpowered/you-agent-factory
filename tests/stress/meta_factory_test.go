package stress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	agentstate "github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/state/validation"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestMetaFactoryWorkflow proves the meta-factory pattern: a workflow that
// analyzes execution statistics and produces workflow modifications, which
// are then validated and applied.
//
// Meta-workflow: analyze-stats:init → optimization-proposal:init → validation:init → apply-changes:init → complete
//
// Assertions:
//   - Meta-workflow completes successfully
//   - The modified workflow definition is valid (passes all validators)
//   - Meta-workflow terminates (does not loop indefinitely)
func TestMetaFactoryWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, buildMetaFactoryCfg(5))
	h := testutil.NewServiceTestHarness(t, dir)

	tracker := &metaFactoryTracker{}

	h.SetCustomExecutor("analyzer", &analyzerExecutor{tracker: tracker})
	h.SetCustomExecutor("proposal-emitter", &proposalEmitterExecutor{tracker: tracker})
	h.SetCustomExecutor("validator-worker", &validatorExecutor{tracker: tracker})
	h.SetCustomExecutor("apply-emitter", &applyEmitterExecutor{tracker: tracker})
	h.SetCustomExecutor("applier", &applierExecutor{tracker: tracker})
	h.MockWorker("finalizer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	// Submit an analysis request.
	h.SubmitWork("analyze-stats", []byte(`{"factory_id": "code-factory", "metric": "transition_latency"}`))

	h.RunUntilComplete(t, 10*time.Second)

	// Assert: all tokens in terminal state.
	h.Assert().
		PlaceTokenCount("analyze-stats:complete", 1).
		PlaceTokenCount("optimization-proposal:complete", 1).
		PlaceTokenCount("apply-changes:complete", 1).
		HasNoTokenInPlace("analyze-stats:init").
		HasNoTokenInPlace("optimization-proposal:init").
		HasNoTokenInPlace("apply-changes:init").
		HasNoTokenInPlace("analyze-stats:failed").
		HasNoTokenInPlace("optimization-proposal:failed").
		HasNoTokenInPlace("apply-changes:failed")

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

	// Assert: the modified workflow passes all validators.
	modifiedNet := tracker.getModifiedNet()
	if modifiedNet == nil {
		t.Fatal("applier did not produce a modified net")
	}

	validator := validation.NewCompositeValidator(
		&validation.ReachabilityValidator{},
		&validation.CompletenessValidator{},
		&validation.BoundednessValidator{},
		&validation.TypeSafetyValidator{},
	)
	violations := validator.Validate(modifiedNet)
	for _, v := range violations {
		if v.Level == validation.ViolationError {
			t.Errorf("modified net has ERROR violation: %s — %s (at %s)", v.Code, v.Message, v.Location)
		}
	}
}

// TestMetaFactoryWithRejectionLoop verifies the meta-factory handles
// validation rejection (rejected proposal loops back) and eventually succeeds.
func TestMetaFactoryWithRejectionLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, buildMetaFactoryCfg(5))
	h := testutil.NewServiceTestHarness(t, dir)
	tracker := &metaFactoryTracker{}

	h.SetCustomExecutor("analyzer", &analyzerExecutor{tracker: tracker})
	h.SetCustomExecutor("proposal-emitter", &proposalEmitterExecutor{tracker: tracker})
	// Validator rejects the first 2 attempts, accepts the 3rd.
	h.SetCustomExecutor("validator-worker", &rejectingValidatorExecutor{
		tracker:      tracker,
		rejectUntilN: 3,
	})
	h.SetCustomExecutor("apply-emitter", &applyEmitterExecutor{tracker: tracker})
	h.SetCustomExecutor("applier", &applierExecutor{tracker: tracker})
	h.MockWorker("finalizer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	h.SubmitWork("analyze-stats", []byte(`{"factory_id": "code-factory", "metric": "error_rate"}`))

	h.RunUntilComplete(t, 10*time.Second)

	// Assert: workflow completed through rejection loop.
	h.Assert().
		PlaceTokenCount("analyze-stats:complete", 1).
		PlaceTokenCount("optimization-proposal:complete", 1).
		PlaceTokenCount("apply-changes:complete", 1)

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
	h := testutil.NewServiceTestHarness(t, dir)
	tracker := &metaFactoryTracker{}

	h.SetCustomExecutor("analyzer", &analyzerExecutor{tracker: tracker})
	h.SetCustomExecutor("proposal-emitter", &proposalEmitterExecutor{tracker: tracker})
	// Validator always rejects -> guarded loop breaker should terminate the loop.
	h.SetCustomExecutor("validator-worker", &alwaysRejectingValidatorExecutor{tracker: tracker})
	h.SetCustomExecutor("apply-emitter", &applyEmitterExecutor{tracker: tracker})
	h.SetCustomExecutor("applier", &applierExecutor{tracker: tracker})
	h.MockWorker("finalizer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	h.SubmitWork("analyze-stats", []byte(`{"factory_id": "code-factory"}`))

	h.RunUntilComplete(t, 10*time.Second)

	// Assert: analyze-stats completed but optimization-proposal failed via guarded loop breaker.
	h.Assert().
		PlaceTokenCount("analyze-stats:complete", 1).
		PlaceTokenCount("optimization-proposal:failed", 1).
		HasNoTokenInPlace("optimization-proposal:init").
		HasNoTokenInPlace("optimization-proposal:validated").
		HasNoTokenInPlace("apply-changes:init"). // never reached
		HasNoTokenInPlace("apply-changes:applied")

	// Validator was called 3 times (the max visits before the loop breaker fired).
	if calls := tracker.validatorCalls(); calls != 3 {
		t.Errorf("expected validator called 3 times before guarded loop breaker, got %d", calls)
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "max-validation-retries", "optimization-proposal:failed")
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
		h := testutil.NewServiceTestHarness(t, dir)
		tracker := &metaFactoryTracker{}

		h.SetCustomExecutor("analyzer", &analyzerExecutor{tracker: tracker})
		h.SetCustomExecutor("proposal-emitter", &proposalEmitterExecutor{tracker: tracker})
		h.SetCustomExecutor("validator-worker", &validatorExecutor{tracker: tracker})
		h.SetCustomExecutor("apply-emitter", &applyEmitterExecutor{tracker: tracker})
		h.SetCustomExecutor("applier", &applierExecutor{tracker: tracker})
		h.MockWorker("finalizer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

		h.SubmitWork("analyze-stats", []byte(`{"factory_id": "test"}`))

		h.RunUntilComplete(t, 10*time.Second)
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
		Workers: []interfaces.WorkerConfig{
			{Name: "analyzer"}, {Name: "proposal-emitter"}, {Name: "validator-worker"},
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

// --- Shared tracker ---

// metaFactoryTracker tracks executor calls and artifacts across the meta-factory pipeline.
type metaFactoryTracker struct {
	mu             sync.Mutex
	analyzerCount  int
	validatorCount int
	applierCount   int
	storedNet      *agentstate.Net
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

func (t *metaFactoryTracker) storeModifiedNet(n *agentstate.Net) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.storedNet = n
}

func (t *metaFactoryTracker) getModifiedNet() *agentstate.Net {
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

func (e *analyzerExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
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

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       string(proposalJSON),
	}, nil
}

// proposalEmitterExecutor spawns an optimization-proposal work item.
type proposalEmitterExecutor struct {
	tracker *metaFactoryTracker
}

func (e *proposalEmitterExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	proposalJSON := ""
	traceID := ""
	if len(dispatch.InputTokens) > 0 {
		proposalJSON = string(firstInputToken(dispatch.InputTokens).Color.Payload)
		traceID = firstInputToken(dispatch.InputTokens).Color.TraceID
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		SpawnedWork: []interfaces.TokenColor{{
			WorkTypeID: "optimization-proposal",
			WorkID:     fmt.Sprintf("proposal-%s", traceID),
			Tags: map[string]string{
				"proposal": proposalJSON,
			},
			Payload: []byte("0"),
		}},
	}, nil
}

// validatorExecutor checks the proposal is structurally valid (always accepts valid proposals).
type validatorExecutor struct {
	tracker *metaFactoryTracker
}

func (e *validatorExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.tracker.recordValidator()

	proposalJSON := ""
	if len(dispatch.InputTokens) > 0 {
		if tags := firstInputToken(dispatch.InputTokens).Color.Tags; tags != nil {
			proposalJSON = tags["proposal"]
		}
	}

	var proposal workflowModification
	if err := json.Unmarshal([]byte(proposalJSON), &proposal); err != nil {
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeRejected,
			Feedback:     fmt.Sprintf("invalid proposal JSON: %v", err),
		}, nil
	}

	if proposal.TransitionID == "" || proposal.Field == "" {
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeRejected,
			Feedback:     "proposal missing required fields: transition_id and field",
		}, nil
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

// rejectingValidatorExecutor rejects the first N-1 attempts, accepts the Nth.
type rejectingValidatorExecutor struct {
	tracker      *metaFactoryTracker
	rejectUntilN int
}

func (e *rejectingValidatorExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.tracker.recordValidator()

	callNum := e.tracker.validatorCalls()
	if callNum < e.rejectUntilN {
		iteration := "0"
		if len(dispatch.InputTokens) > 0 {
			n, _ := strconv.Atoi(string(firstInputToken(dispatch.InputTokens).Color.Payload))
			iteration = strconv.Itoa(n + 1)
		}

		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeRejected,
			Feedback:     fmt.Sprintf("proposal needs refinement (attempt %d/%d)", callNum, e.rejectUntilN),
			Output:       iteration,
		}, nil
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

// alwaysRejectingValidatorExecutor always rejects — used to test guarded loop-breaker termination.
type alwaysRejectingValidatorExecutor struct {
	tracker *metaFactoryTracker
}

func (e *alwaysRejectingValidatorExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.tracker.recordValidator()

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeRejected,
		Feedback:     "proposal is structurally invalid — always rejected for testing",
	}, nil
}

// applyEmitterExecutor spawns an apply-changes work item.
type applyEmitterExecutor struct {
	tracker *metaFactoryTracker
}

func (e *applyEmitterExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	proposalJSON := ""
	traceID := ""
	if len(dispatch.InputTokens) > 0 {
		if tags := firstInputToken(dispatch.InputTokens).Color.Tags; tags != nil {
			proposalJSON = tags["proposal"]
		}
		traceID = firstInputToken(dispatch.InputTokens).Color.TraceID
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		SpawnedWork: []interfaces.TokenColor{{
			WorkTypeID: "apply-changes",
			WorkID:     fmt.Sprintf("apply-%s", traceID),
			Tags: map[string]string{
				"proposal": proposalJSON,
			},
		}},
	}, nil
}

// applierExecutor applies the modification to a copy of a target workflow
// definition and validates the result using all CPN validators.
type applierExecutor struct {
	tracker *metaFactoryTracker
}

func (e *applierExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
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
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeFailed,
			Error:        fmt.Sprintf("failed to build modified workflow: %v", err),
		}, nil
	}

	// Store the modified net for post-hoc validation in the test.
	e.tracker.storeModifiedNet(modifiedNet)

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       fmt.Sprintf("set %s.%s = %d (was %d)", proposal.TransitionID, proposal.Field, proposal.NewValue, proposal.OldValue),
	}, nil
}

// portos:func-length-exception owner=agent-factory reason=petri-net-fixture-builder review=2026-07-19 removal=split-target-net-builder-before-next-meta-factory-stress-change
func buildTargetWorkflowNet(maxVisits int) (*agentstate.Net, error) {
	net := &agentstate.Net{
		ID:          "target-code-factory",
		Places:      make(map[string]*petri.Place),
		Transitions: make(map[string]*petri.Transition),
		WorkTypes: map[string]*agentstate.WorkType{
			"task": {
				ID:   "task",
				Name: "task",
				States: []agentstate.StateDefinition{
					{Value: "init", Category: agentstate.StateCategoryInitial},
					{Value: "processing", Category: agentstate.StateCategoryProcessing},
					{Value: "complete", Category: agentstate.StateCategoryTerminal},
					{Value: "failed", Category: agentstate.StateCategoryFailed},
				},
			},
		},
		Resources: make(map[string]*agentstate.ResourceDef),
	}

	for _, place := range net.WorkTypes["task"].GeneratePlaces() {
		net.Places[place.ID] = place
	}

	net.Transitions["execute-task"] = &petri.Transition{
		ID:         "execute-task",
		Name:       "execute-task",
		Type:       petri.TransitionNormal,
		WorkerType: "executor",
		InputArcs: []petri.Arc{
			{
				PlaceID:   "task:init",
				Direction: petri.ArcInput,
				Mode:      interfaces.ArcModeConsume,
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
		OutputArcs: []petri.Arc{
			{
				PlaceID:   "task:processing",
				Direction: petri.ArcOutput,
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
		FailureArcs: []petri.Arc{
			{
				PlaceID:   "task:failed",
				Direction: petri.ArcOutput,
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
	}

	net.Transitions["finish-task"] = &petri.Transition{
		ID:         "finish-task",
		Name:       "finish-task",
		Type:       petri.TransitionNormal,
		WorkerType: "finisher",
		InputArcs: []petri.Arc{
			{
				PlaceID:   "task:processing",
				Direction: petri.ArcInput,
				Mode:      interfaces.ArcModeConsume,
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
		OutputArcs: []petri.Arc{
			{
				PlaceID:   "task:complete",
				Direction: petri.ArcOutput,
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
		FailureArcs: []petri.Arc{
			{
				PlaceID:   "task:failed",
				Direction: petri.ArcOutput,
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
	}

	net.Transitions["auto-retry-limit"] = &petri.Transition{
		ID:   "auto-retry-limit",
		Name: "auto-retry-limit",
		Type: petri.TransitionNormal,
		InputArcs: []petri.Arc{
			{
				Name:      "retry-token",
				PlaceID:   "task:init",
				Direction: petri.ArcInput,
				Mode:      interfaces.ArcModeConsume,
				Guard:     &petri.VisitCountGuard{MaxVisits: maxVisits},
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
		OutputArcs: []petri.Arc{
			{
				PlaceID:   "task:failed",
				Direction: petri.ArcOutput,
				Cardinality: petri.ArcCardinality{
					Mode: petri.CardinalityOne,
				},
			},
		},
	}

	agentstate.NormalizeTransitionTopology(net)

	validator := validation.NewCompositeValidator(
		&validation.ReachabilityValidator{},
		&validation.CompletenessValidator{},
		&validation.BoundednessValidator{},
		&validation.TypeSafetyValidator{},
	)
	violations := validator.Validate(net)
	for _, violation := range violations {
		if violation.Level == validation.ViolationError {
			return nil, fmt.Errorf("net validation failed: %s - %s (at %s)", violation.Code, violation.Message, violation.Location)
		}
	}

	return net, nil
}
