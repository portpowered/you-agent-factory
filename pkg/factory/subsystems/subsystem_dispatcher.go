package subsystems

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// DispatcherSubsystem fires transitions by consuming input tokens and producing
// WorkDispatches for worker executors. It runs at TickGroup 5 (after Scheduler).
type DispatcherSubsystem struct {
	state         *state.Net
	sched         scheduler.Scheduler
	wfCtx         *factory_context.FactoryContext
	logger        logging.Logger
	evaluator     *scheduler.EnablementEvaluator
	runtimeConfig interfaces.RuntimeDefinitionLookup
	now           func() time.Time
}

// DispatcherOption configures a DispatcherSubsystem.
type DispatcherOption func(*DispatcherSubsystem)

// WithDispatcherRuntimeConfig injects the authoritative runtime-loaded worker
// config so dispatcher policy can resolve provider/model lanes from worker names.
func WithDispatcherRuntimeConfig(runtimeCfg interfaces.RuntimeDefinitionLookup) DispatcherOption {
	return func(d *DispatcherSubsystem) {
		d.runtimeConfig = runtimeCfg
	}
}

// WithDispatcherClock overrides the time source used for throttle-pause expiry.
func WithDispatcherClock(now func() time.Time) DispatcherOption {
	return func(d *DispatcherSubsystem) {
		if now != nil {
			d.now = now
		}
	}
}

// NewDispatcher creates a new DispatcherSubsystem.
func NewDispatcher(n *state.Net, sched scheduler.Scheduler, wfCtx *factory_context.FactoryContext, logger logging.Logger, opts ...DispatcherOption) *DispatcherSubsystem {
	l := logging.EnsureLogger(logger)
	dispatcher := &DispatcherSubsystem{
		state:  n,
		sched:  sched,
		wfCtx:  wfCtx,
		logger: l,
		now:    time.Now,
	}
	for _, opt := range opts {
		opt(dispatcher)
	}
	dispatcher.evaluator = scheduler.NewEnablementEvaluator(
		l,
		scheduler.WithEnablementClock(dispatcher.now),
		scheduler.WithEnablementRuntimeConfig(dispatcher.runtimeConfig),
	)
	return dispatcher
}

var _ Subsystem = (*DispatcherSubsystem)(nil)

// TickGroup returns Dispatcher (5).
func (d *DispatcherSubsystem) TickGroup() TickGroup {
	return Dispatcher
}

// Execute finds enabled transitions, selects firings via the scheduler,
// and produces CONSUME mutations + WorkDispatches for each firing.
func (d *DispatcherSubsystem) Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	d.logger.Debug("dispatcher: dispatching work based on current snapshot", "snapshot", snapshot)
	activeThrottlePauses := d.activeThrottlePauses(snapshot)
	observedThrottlePauses := d.throttlePausesObserved(snapshot, activeThrottlePauses)
	decisions := d.dispatchDecisions(ctx, snapshot)
	if len(decisions) == 0 {
		d.logger.Debug("dispatcher: no enabled transitions")
		return d.throttlePauseSnapshotResult(activeThrottlePauses, observedThrottlePauses), nil
	}
	d.logger.Debug("dispatcher: firing transitions", "decisions", len(decisions))
	mutations, dispatchRecords := d.buildDispatchRecords(snapshot, decisions)
	if len(mutations) == 0 && len(dispatchRecords) == 0 {
		return d.throttlePauseSnapshotResult(activeThrottlePauses, observedThrottlePauses), nil
	}

	d.logger.Debug("dispatcher: mutations", "mutations", mutations)
	d.logger.Debug("dispatcher: dispatches", "dispatches", dispatchRecords)
	return &interfaces.TickResult{
		Mutations:              mutations,
		Dispatches:             dispatchRecords,
		ActiveThrottlePauses:   activeThrottlePauses,
		ThrottlePausesObserved: observedThrottlePauses,
	}, nil
}

func (d *DispatcherSubsystem) dispatchDecisions(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	enabled := d.evaluator.FindEnabledTransitionsWithSnapshot(ctx, d.state, d.schedulerSnapshot(snapshot))
	if len(enabled) == 0 {
		return nil
	}
	if scheduler.SupportsRepeatedTransitionBindings(d.sched) {
		expanded := scheduler.ExpandRepeatedBindings(d.state, &snapshot.Marking, enabled)
		if len(expanded) != len(enabled) {
			d.logger.Debug("dispatcher: expanded repeated transition bindings", "enabled", len(enabled), "expanded", len(expanded))
		}
		enabled = expanded
	}
	decisions := d.sched.Select(enabled, d.schedulerSnapshot(snapshot))
	if len(decisions) == 0 {
		d.logger.Debug("dispatcher: no decisions")
	}
	return decisions
}

func (d *DispatcherSubsystem) buildDispatchRecords(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], decisions []interfaces.FiringDecision) ([]interfaces.MarkingMutation, []interfaces.DispatchRecord) {
	var mutations []interfaces.MarkingMutation
	var dispatchRecords []interfaces.DispatchRecord
	claimedTokens := make(map[string]bool)

	for _, decision := range decisions {
		dispatchRecord, ok := d.dispatchRecordFromDecision(snapshot, decision, claimedTokens)
		if !ok {
			continue
		}
		mutations = append(mutations, dispatchRecord.Mutations...)
		dispatchRecords = append(dispatchRecords, dispatchRecord)
	}
	return mutations, dispatchRecords
}

func (d *DispatcherSubsystem) dispatchRecordFromDecision(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], decision interfaces.FiringDecision, claimedTokens map[string]bool) (interfaces.DispatchRecord, bool) {
	if decision.TransitionID == "" {
		d.logger.Warn("dispatcher: skipping firing decision with missing transition id")
		return interfaces.DispatchRecord{}, false
	}

	tr, ok := d.state.Transitions[decision.TransitionID]
	if !ok {
		d.logger.Warn("dispatcher: transition from firing decision not found in net",
			"transitionID", decision.TransitionID,
			"workerType", decision.WorkerType)
		return interfaces.DispatchRecord{}, false
	}

	inputTokens, ok := d.collectDecisionTokens(snapshot, decision, claimedTokens)
	if !ok {
		return interfaces.DispatchRecord{}, false
	}
	consumeMutations := consumeMutationsForDecision(decision.TransitionID, inputTokens, claimedTokens)
	dispatch := d.buildWorkDispatch(snapshot, decision, tr, inputTokens)
	d.logDispatch(decision, inputTokens, dispatch)
	return interfaces.DispatchRecord{Dispatch: dispatch, Mutations: consumeMutations}, true
}

func (d *DispatcherSubsystem) collectDecisionTokens(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], decision interfaces.FiringDecision, claimedTokens map[string]bool) ([]interfaces.Token, bool) {
	inputTokens := make([]interfaces.Token, 0, len(decision.ConsumeTokens))
	seenTokens := make(map[string]bool)
	for _, tokenID := range decision.ConsumeTokens {
		if seenTokens[tokenID] {
			continue
		}
		seenTokens[tokenID] = true
		if claimedTokens[tokenID] {
			d.logger.Warn("dispatcher: skipping decision due to duplicate token claim",
				"transitionID", decision.TransitionID,
				"tokenID", tokenID,
				"workerType", decision.WorkerType)
			return nil, false
		}
		tok, ok := snapshot.Marking.Tokens[tokenID]
		if !ok {
			d.logger.Warn("dispatcher: token referenced by firing decision not found in snapshot",
				"transitionID", decision.TransitionID,
				"tokenID", tokenID,
				"workerType", decision.WorkerType)
			return nil, false
		}
		inputTokens = append(inputTokens, *tok)
	}
	return inputTokens, true
}

func consumeMutationsForDecision(transitionID string, inputTokens []interfaces.Token, claimedTokens map[string]bool) []interfaces.MarkingMutation {
	consumeMutations := make([]interfaces.MarkingMutation, 0, len(inputTokens))
	for _, token := range inputTokens {
		consumeMutations = append(consumeMutations, interfaces.MarkingMutation{
			Type:      interfaces.MutationConsume,
			TokenID:   token.ID,
			FromPlace: token.PlaceID,
			Reason:    fmt.Sprintf("consumed by transition %s", transitionID),
		})
		claimedTokens[token.ID] = true
	}
	return consumeMutations
}

func (d *DispatcherSubsystem) buildWorkDispatch(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], decision interfaces.FiringDecision, tr *petri.Transition, inputTokens []interfaces.Token) interfaces.WorkDispatch {
	dispatch := interfaces.WorkDispatch{
		DispatchID:               uuid.NewString(),
		TransitionID:             decision.TransitionID,
		WorkerType:               decision.WorkerType,
		CurrentChainingTraceID:   interfaces.CurrentChainingTraceIDFromTokens(inputTokens),
		PreviousChainingTraceIDs: interfaces.PreviousChainingTraceIDsFromTokens(inputTokens),
		Execution:                executionMetadataForDispatch(decision.TransitionID, snapshot.TickCount, inputTokens),
		InputTokens:              workers.InputTokens(inputTokens...),
		InputBindings:            cloneDispatchInputBindings(decision.InputBindings),
		WorkstationName:          tr.Name,
	}
	if d.wfCtx != nil {
		dispatch.ProjectID = d.wfCtx.ProjectID
	}
	return dispatch
}

func (d *DispatcherSubsystem) logDispatch(decision interfaces.FiringDecision, inputTokens []interfaces.Token, dispatch interfaces.WorkDispatch) {
	d.logger.Info("dispatcher: dispatching work to worker",
		workers.WorkLogFields(dispatch.Execution,
			"transition_id", decision.TransitionID,
			"worker_type", decision.WorkerType,
			"work_type", d.workTypeFromTokens(inputTokens),
			"work_id", d.workIDFromTokens(inputTokens),
			"input_tokens", len(inputTokens))...)
}

func (d *DispatcherSubsystem) schedulerSnapshot(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	if snapshot == nil || snapshot.Topology != nil {
		return snapshot
	}
	withTopology := *snapshot
	withTopology.Topology = d.state
	return &withTopology
}

func cloneDispatchInputBindings(bindings map[string][]string) map[string][]string {
	if len(bindings) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(bindings))
	for name, tokenIDs := range bindings {
		clone[name] = append([]string(nil), tokenIDs...)
	}
	return clone
}

func (d *DispatcherSubsystem) throttlePausesObserved(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], active []interfaces.ActiveThrottlePause) bool {
	if len(active) > 0 {
		return true
	}
	return snapshot != nil && len(snapshot.ActiveThrottlePauses) > 0
}

func (d *DispatcherSubsystem) throttlePauseSnapshotResult(active []interfaces.ActiveThrottlePause, observed bool) *interfaces.TickResult {
	if !observed {
		return nil
	}
	return &interfaces.TickResult{
		ActiveThrottlePauses:   active,
		ThrottlePausesObserved: true,
	}
}

func (d *DispatcherSubsystem) activeThrottlePauses(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.ActiveThrottlePause {
	if snapshot == nil {
		return nil
	}
	activeByLane := make(map[string]interfaces.ActiveThrottlePause)
	runtime := petri.RuntimeGuardContext{
		Now:               d.now(),
		DispatchHistory:   snapshot.DispatchHistory,
		RuntimeConfig:     d.runtimeConfig,
		TransitionWorkers: transitionWorkerTypesForNet(d.state),
	}
	for _, transition := range d.state.Transitions {
		for _, arc := range transition.InputArcs {
			for _, pause := range activePausesForGuard(arc.Guard, runtime) {
				activeByLane[pause.LaneID] = pause
			}
		}
	}
	active := make([]interfaces.ActiveThrottlePause, 0, len(activeByLane))
	for _, pause := range activeByLane {
		active = append(active, pause)
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Provider != active[j].Provider {
			return active[i].Provider < active[j].Provider
		}
		if active[i].Model != active[j].Model {
			return active[i].Model < active[j].Model
		}
		return active[i].LaneID < active[j].LaneID
	})
	return active
}

func activePausesForGuard(guard petri.Guard, runtime petri.RuntimeGuardContext) []interfaces.ActiveThrottlePause {
	if guard == nil {
		return nil
	}
	if provider, ok := guard.(petri.ActivePauseProvider); ok {
		return provider.ActivePauses(runtime)
	}
	if all, ok := guard.(*petri.AllGuard); ok {
		active := make([]interfaces.ActiveThrottlePause, 0)
		for _, nested := range all.Guards {
			active = append(active, activePausesForGuard(nested, runtime)...)
		}
		return active
	}
	return nil
}

func transitionWorkerTypesForNet(net *state.Net) map[string]string {
	if net == nil || len(net.Transitions) == 0 {
		return nil
	}
	workersByTransition := make(map[string]string, len(net.Transitions))
	for transitionID, transition := range net.Transitions {
		if transition == nil || transition.WorkerType == "" {
			continue
		}
		workersByTransition[transitionID] = transition.WorkerType
	}
	return workersByTransition
}

// workTypeFromTokens extracts the work type from the first non-resource input token.
// Resource tokens (semaphores like agent-slot) are skipped to ensure metrics
// reflect the actual work being done, not the slot being consumed.
func (d *DispatcherSubsystem) workTypeFromTokens(tokens []interfaces.Token) string {
	if token := preferredIdentityToken(tokens); token != nil {
		return token.Color.WorkTypeID
	}
	return ""
}

// workIDFromTokens extracts the work ID from the first non-resource input token.
// Resource tokens (semaphores like agent-slot) are skipped to ensure metrics
// reflect the actual work being done, not the slot being consumed.
func (d *DispatcherSubsystem) workIDFromTokens(tokens []interfaces.Token) string {
	if token := preferredIdentityToken(tokens); token != nil {
		return token.Color.WorkID
	}
	return ""
}

func executionMetadataForDispatch(transitionID string, currentTick int, inputTokens []interfaces.Token) interfaces.ExecutionMetadata {
	metadata := interfaces.ExecutionMetadata{
		CurrentTick: currentTick,
	}
	for _, token := range identityTokens(inputTokens) {
		if metadata.TraceID == "" {
			metadata.TraceID = token.Color.TraceID
		}
		if metadata.RequestID == "" {
			metadata.RequestID = token.Color.RequestID
		}
		if token.Color.WorkID != "" {
			metadata.WorkIDs = append(metadata.WorkIDs, token.Color.WorkID)
		}
	}
	metadata.ReplayKey = replayKeyForDispatch(transitionID, metadata.TraceID, metadata.WorkIDs)
	return metadata
}

func preferredIdentityToken(tokens []interfaces.Token) *interfaces.Token {
	for i := range tokens {
		if isCustomerIdentityToken(tokens[i]) {
			return &tokens[i]
		}
	}
	for i := range tokens {
		if isDispatchIdentityToken(tokens[i]) {
			return &tokens[i]
		}
	}
	return nil
}

func identityTokens(tokens []interfaces.Token) []interfaces.Token {
	customerTokens := make([]interfaces.Token, 0, len(tokens))
	fallbackTokens := make([]interfaces.Token, 0, len(tokens))
	for i := range tokens {
		if !isDispatchIdentityToken(tokens[i]) {
			continue
		}
		if isCustomerIdentityToken(tokens[i]) {
			customerTokens = append(customerTokens, tokens[i])
			continue
		}
		fallbackTokens = append(fallbackTokens, tokens[i])
	}
	if len(customerTokens) > 0 {
		return customerTokens
	}
	return fallbackTokens
}

func isCustomerIdentityToken(token interfaces.Token) bool {
	return isDispatchIdentityToken(token) && token.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID
}

func isDispatchIdentityToken(token interfaces.Token) bool {
	return token.Color.DataType != interfaces.DataTypeResource
}

func replayKeyForDispatch(transitionID string, traceID string, workIDs []string) string {
	parts := []string{transitionID}
	if traceID != "" {
		parts = append(parts, traceID)
	}
	parts = append(parts, workIDs...)
	return strings.Join(parts, "/")
}
