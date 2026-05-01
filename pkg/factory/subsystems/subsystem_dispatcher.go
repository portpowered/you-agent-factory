package subsystems

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	factory_context "github.com/portpowered/agent-factory/pkg/factory/context"
	factory_throttle "github.com/portpowered/agent-factory/pkg/factory/internal/throttle"
	"github.com/portpowered/agent-factory/pkg/factory/scheduler"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/workers"
)

// DispatcherSubsystem fires transitions by consuming input tokens and producing
// WorkDispatches for worker executors. It runs at TickGroup 5 (after Scheduler).
type DispatcherSubsystem struct {
	state                 *state.Net
	sched                 scheduler.Scheduler
	wfCtx                 *factory_context.FactoryContext
	logger                logging.Logger
	evaluator             *scheduler.EnablementEvaluator
	runtimeConfig         interfaces.RuntimeDefinitionLookup
	now                   func() time.Time
	throttlePauseDuration time.Duration
	throttlePauses        map[providerModelKey]providerModelPause
}

const defaultProviderThrottlePauseDuration = 5 * time.Hour

type providerModelKey struct {
	provider string
	model    string
}

type providerModelPause struct {
	pausedAt    time.Time
	pausedUntil time.Time
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

// WithDispatcherThrottlePauseDuration overrides the internal pause window used
// for provider/model throttling. Zero or negative values keep the default.
func WithDispatcherThrottlePauseDuration(duration time.Duration) DispatcherOption {
	return func(d *DispatcherSubsystem) {
		if duration > 0 {
			d.throttlePauseDuration = duration
		}
	}
}

// NewDispatcher creates a new DispatcherSubsystem.
func NewDispatcher(n *state.Net, sched scheduler.Scheduler, wfCtx *factory_context.FactoryContext, logger logging.Logger, opts ...DispatcherOption) *DispatcherSubsystem {
	l := logging.EnsureLogger(logger)
	dispatcher := &DispatcherSubsystem{
		state:                 n,
		sched:                 sched,
		wfCtx:                 wfCtx,
		logger:                l,
		now:                   time.Now,
		throttlePauseDuration: defaultProviderThrottlePauseDuration,
		throttlePauses:        make(map[providerModelKey]providerModelPause),
	}
	for _, opt := range opts {
		opt(dispatcher)
	}
	dispatcher.evaluator = scheduler.NewEnablementEvaluator(l, scheduler.WithEnablementClock(dispatcher.now))
	return dispatcher
}

var _ Subsystem = (*DispatcherSubsystem)(nil)

// TickGroup returns Dispatcher (5).
func (d *DispatcherSubsystem) TickGroup() TickGroup {
	return Dispatcher
}

// Execute finds enabled transitions, selects firings via the scheduler,
// and produces CONSUME mutations + WorkDispatches for each firing.
// portos:func-length-exception owner=agent-factory reason=dispatcher-main-loop review=2026-07-18 removal=extract-decision-token-claim-and-dispatch-builders-before-next-dispatcher-expansion
func (d *DispatcherSubsystem) Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	d.logger.Debug("dispatcher: dispatching work based on current snapshot", "snapshot", snapshot)
	observedThrottlePauses := d.reconcileThrottlePauses(snapshot)
	enabled := d.evaluator.FindEnabledTransitions(ctx, d.state, &snapshot.Marking)
	if len(enabled) == 0 {
		d.logger.Debug("dispatcher: no enabled transitions")
		return d.throttlePauseSnapshotResult(observedThrottlePauses), nil
	}
	enabled = d.filterPausedEnabledTransitions(enabled)
	if len(enabled) == 0 {
		d.logger.Debug("dispatcher: all enabled transitions paused by provider/model throttle state")
		return d.throttlePauseSnapshotResult(observedThrottlePauses), nil
	}
	if scheduler.SupportsRepeatedTransitionBindings(d.sched) {
		expanded := scheduler.ExpandRepeatedBindings(d.state, &snapshot.Marking, enabled)
		if len(expanded) != len(enabled) {
			d.logger.Debug("dispatcher: expanded repeated transition bindings",
				"enabled", len(enabled),
				"expanded", len(expanded))
		}
		enabled = expanded
	}

	decisions := d.sched.Select(enabled, d.schedulerSnapshot(snapshot))
	if len(decisions) == 0 {
		d.logger.Debug("dispatcher: no decisions")
		return d.throttlePauseSnapshotResult(observedThrottlePauses), nil
	}

	d.logger.Debug("dispatcher: firing transitions",
		"enabled", len(enabled), "decisions", len(decisions))

	var mutations []interfaces.MarkingMutation
	var dispatchRecords []interfaces.DispatchRecord
	claimedTokens := make(map[string]bool)

	for _, decision := range decisions {
		if decision.TransitionID == "" {
			d.logger.Warn("dispatcher: skipping firing decision with missing transition id")
			continue
		}

		tr, ok := d.state.Transitions[decision.TransitionID]
		if !ok {
			d.logger.Warn("dispatcher: transition from firing decision not found in net",
				"transitionID", decision.TransitionID,
				"workerType", decision.WorkerType)
			continue
		}

		// Collect and validate all tokens before mutating state.
		inputTokens := make([]interfaces.Token, 0, len(decision.ConsumeTokens))
		seenTokens := make(map[string]bool)

		duplicateDecision := false
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
				duplicateDecision = true
				break
			}
			tok, ok := snapshot.Marking.Tokens[tokenID]
			if !ok {
				d.logger.Warn("dispatcher: token referenced by firing decision not found in snapshot",
					"transitionID", decision.TransitionID,
					"tokenID", tokenID,
					"workerType", decision.WorkerType)
				duplicateDecision = true
				break
			}
			inputTokens = append(inputTokens, *tok)
		}
		if duplicateDecision {
			continue
		}

		// CONSUME mutations for all input tokens.
		var consumeMutations []interfaces.MarkingMutation
		for _, token := range inputTokens {
			m := interfaces.MarkingMutation{
				Type:      interfaces.MutationConsume,
				TokenID:   token.ID,
				FromPlace: token.PlaceID,
				Reason:    fmt.Sprintf("consumed by transition %s", decision.TransitionID),
			}
			consumeMutations = append(consumeMutations, m)
			claimedTokens[token.ID] = true
		}
		mutations = append(mutations, consumeMutations...)
		// Determine work type for metrics.
		dispatchWorkType := d.workTypeFromTokens(inputTokens)
		dispatchWorkID := d.workIDFromTokens(inputTokens)

		// Create a WorkDispatch for the worker.
		execution := executionMetadataForDispatch(decision.TransitionID, snapshot.TickCount, inputTokens)
		dispatch := interfaces.WorkDispatch{
			DispatchID:               uuid.NewString(),
			TransitionID:             decision.TransitionID,
			WorkerType:               decision.WorkerType,
			CurrentChainingTraceID:   execution.TraceID,
			PreviousChainingTraceIDs: interfaces.PreviousChainingTraceIDsFromTokens(inputTokens),
			Execution:                execution,
			InputTokens:              workers.InputTokens(inputTokens...),
			InputBindings:            cloneDispatchInputBindings(decision.InputBindings),
			WorkstationName:          tr.Name,
		}
		if d.wfCtx != nil {
			dispatch.ProjectID = d.wfCtx.ProjectID
		}
		d.logger.Info("dispatcher: dispatching work to worker",
			workers.WorkLogFields(dispatch.Execution,
				"transition_id", decision.TransitionID,
				"worker_type", decision.WorkerType,
				"work_type", dispatchWorkType,
				"work_id", dispatchWorkID,
				"input_tokens", len(inputTokens))...)
		dispatchRecords = append(dispatchRecords, interfaces.DispatchRecord{
			Dispatch:  dispatch,
			Mutations: consumeMutations,
		})

	}

	if len(mutations) == 0 && len(dispatchRecords) == 0 {
		return d.throttlePauseSnapshotResult(observedThrottlePauses), nil
	}

	d.logger.Debug("dispatcher: mutations", "mutations", mutations)
	d.logger.Debug("dispatcher: dispatches", "dispatches", dispatchRecords)
	return &interfaces.TickResult{
		Mutations:              mutations,
		Dispatches:             dispatchRecords,
		ActiveThrottlePauses:   d.activeThrottlePauses(),
		ThrottlePausesObserved: observedThrottlePauses,
	}, nil
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

func (d *DispatcherSubsystem) reconcileThrottlePauses(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	observed := len(d.throttlePauses) > 0
	now := d.now()
	for key, pause := range d.throttlePauses {
		if !pause.pausedUntil.After(now) {
			delete(d.throttlePauses, key)
			observed = true
		}
	}
	if snapshot == nil || len(snapshot.Results) == 0 {
		return observed
	}
	for _, pause := range factory_throttle.DeriveActiveThrottlePauses(d.throttleFailureHistory(snapshot, now), d.throttlePauseDuration, now) {
		key := providerModelKey{provider: pause.Provider, model: pause.Model}
		candidateUntil := pause.PausedUntil
		if existing, exists := d.throttlePauses[key]; exists && existing.pausedUntil.After(candidateUntil) {
			continue
		}
		pausedAt := pause.PausedAt
		if existing, exists := d.throttlePauses[key]; exists && !existing.pausedAt.IsZero() {
			pausedAt = existing.pausedAt
		}
		d.throttlePauses[key] = providerModelPause{
			pausedAt:    pausedAt,
			pausedUntil: candidateUntil,
		}
		observed = true
	}
	return observed
}

func (d *DispatcherSubsystem) filterPausedEnabledTransitions(enabled []interfaces.EnabledTransition) []interfaces.EnabledTransition {
	if len(enabled) == 0 || len(d.throttlePauses) == 0 {
		return enabled
	}
	now := d.now()
	filtered := make([]interfaces.EnabledTransition, 0, len(enabled))
	for _, transition := range enabled {
		key, ok := d.providerModelKeyForWorker(transition.WorkerType)
		if !ok {
			filtered = append(filtered, transition)
			continue
		}
		if pause, paused := d.throttlePauses[key]; paused && pause.pausedUntil.After(now) {
			d.logger.Info("dispatcher: excluding paused provider/model lane before scheduling",
				"transitionID", transition.TransitionID,
				"workerType", transition.WorkerType,
				"model_provider", key.provider,
				"model", key.model,
				"paused_until", pause.pausedUntil,
			)
			continue
		}
		filtered = append(filtered, transition)
	}
	return filtered
}

func (d *DispatcherSubsystem) throttleFailureHistory(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], observedAt time.Time) []factory_throttle.FailureRecord {
	if snapshot == nil || len(snapshot.Results) == 0 {
		return nil
	}
	history := make([]factory_throttle.FailureRecord, 0, len(snapshot.Results))
	for i := range snapshot.Results {
		key, ok := d.throttlePauseKeyForResult(snapshot.Results[i])
		if !ok {
			continue
		}
		history = append(history, factory_throttle.FailureRecord{
			Provider:        key.provider,
			Model:           key.model,
			OccurredAt:      observedAt,
			ProviderFailure: snapshot.Results[i].ProviderFailure,
		})
	}
	return history
}

func (d *DispatcherSubsystem) throttleFailureHistoryFromCompletedDispatches(history []interfaces.CompletedDispatch) []factory_throttle.FailureRecord {
	if len(history) == 0 {
		return nil
	}
	records := make([]factory_throttle.FailureRecord, 0, len(history))
	for i := range history {
		if !workers.ProviderFailureDecisionFromMetadata(history[i].ProviderFailure).TriggersThrottlePause {
			continue
		}
		key, ok := d.throttlePauseKeyForCompletedDispatch(history[i])
		if !ok {
			continue
		}
		records = append(records, factory_throttle.FailureRecord{
			Provider:        key.provider,
			Model:           key.model,
			OccurredAt:      history[i].EndTime,
			ProviderFailure: history[i].ProviderFailure,
		})
	}
	sort.SliceStable(records, func(i, j int) bool {
		if !records[i].OccurredAt.Equal(records[j].OccurredAt) {
			return records[i].OccurredAt.Before(records[j].OccurredAt)
		}
		if records[i].Provider != records[j].Provider {
			return records[i].Provider < records[j].Provider
		}
		return records[i].Model < records[j].Model
	})
	return records
}

func (d *DispatcherSubsystem) throttlePauseKeyForResult(result interfaces.WorkResult) (providerModelKey, bool) {
	transition, ok := d.state.Transitions[result.TransitionID]
	if !ok {
		return providerModelKey{}, false
	}
	return d.providerModelKeyForWorker(transition.WorkerType)
}

func (d *DispatcherSubsystem) throttlePauseKeyForCompletedDispatch(dispatch interfaces.CompletedDispatch) (providerModelKey, bool) {
	transition, ok := d.state.Transitions[dispatch.TransitionID]
	if !ok {
		return providerModelKey{}, false
	}
	return d.providerModelKeyForWorker(transition.WorkerType)
}

func (d *DispatcherSubsystem) providerModelKeyForWorker(workerName string) (providerModelKey, bool) {
	if d.runtimeConfig == nil || workerName == "" {
		return providerModelKey{}, false
	}
	def, ok := d.runtimeConfig.Worker(workerName)
	if !ok || def == nil || def.ModelProvider == "" || def.Model == "" {
		return providerModelKey{}, false
	}
	return providerModelKey{
		provider: def.ModelProvider,
		model:    def.Model,
	}, true
}

func (d *DispatcherSubsystem) throttlePauseSnapshotResult(observed bool) *interfaces.TickResult {
	if !observed {
		return nil
	}
	return &interfaces.TickResult{
		ActiveThrottlePauses:   d.activeThrottlePauses(),
		ThrottlePausesObserved: true,
	}
}

func (d *DispatcherSubsystem) activeThrottlePauses() []interfaces.ActiveThrottlePause {
	if len(d.throttlePauses) == 0 {
		return nil
	}
	now := d.now()
	pauses := make([]interfaces.ActiveThrottlePause, 0, len(d.throttlePauses))
	for key, pause := range d.throttlePauses {
		if !pause.pausedUntil.After(now) {
			continue
		}
		pauses = append(pauses, interfaces.ActiveThrottlePause{
			LaneID:      providerModelLaneID(key),
			Provider:    key.provider,
			Model:       key.model,
			PausedAt:    pause.pausedAt,
			PausedUntil: pause.pausedUntil,
		})
	}
	sort.Slice(pauses, func(i, j int) bool {
		if pauses[i].Provider != pauses[j].Provider {
			return pauses[i].Provider < pauses[j].Provider
		}
		if pauses[i].Model != pauses[j].Model {
			return pauses[i].Model < pauses[j].Model
		}
		return pauses[i].LaneID < pauses[j].LaneID
	})
	return pauses
}

func providerModelLaneID(key providerModelKey) string {
	return key.provider + "/" + key.model
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
