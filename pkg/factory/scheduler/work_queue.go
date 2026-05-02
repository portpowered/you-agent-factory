package scheduler

import (
	"sort"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/workstationconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// WorkInQueueScheduler selects transition firings in deterministic batches.
// It prioritizes candidates consuming more customer work already in processing
// states, then customer work, workstation kind, initialized traces, completion
// age, and token-queue age.
type WorkInQueueScheduler struct {
	maxDispatches int
	runtimeConfig interfaces.RuntimeWorkstationLookup
}

// WorkInQueueSchedulerOption configures runtime lookup behavior for the
// work-in-queue scheduler.
type WorkInQueueSchedulerOption func(*WorkInQueueScheduler)

const (
	workstationPriorityLogical = -1
	workstationPriorityNormal  = 0
	workstationPriorityCron    = 1
)

// NewWorkInQueueScheduler creates a bounded scheduler that can select up to
// maxDispatches firings per tick.
func NewWorkInQueueScheduler(maxDispatches int, opts ...WorkInQueueSchedulerOption) *WorkInQueueScheduler {
	if maxDispatches <= 0 {
		maxDispatches = 1
	}
	scheduler := &WorkInQueueScheduler{maxDispatches: maxDispatches}
	for _, opt := range opts {
		if opt != nil {
			opt(scheduler)
		}
	}
	return scheduler
}

// WithRuntimeConfig lets scheduler priority derive workstation kinds from the
// authoritative runtime-config boundary instead of transition-owned copies.
func WithRuntimeConfig(runtimeConfig interfaces.RuntimeWorkstationLookup) WorkInQueueSchedulerOption {
	return func(s *WorkInQueueScheduler) {
		s.SetRuntimeConfig(runtimeConfig)
	}
}

// SetRuntimeConfig lets runtime constructors inject authoritative workstation
// metadata into an existing scheduler instance on supported custom-scheduler
// paths such as factory.WithScheduler(...).
func (s *WorkInQueueScheduler) SetRuntimeConfig(runtimeConfig interfaces.RuntimeWorkstationLookup) {
	if s != nil {
		s.runtimeConfig = runtimeConfig
	}
}

// SupportsRepeatedTransitionBindings opts WorkInQueueScheduler into receiving
// separate candidates for distinct same-transition token bindings.
func (s *WorkInQueueScheduler) SupportsRepeatedTransitionBindings() bool {
	return s != nil
}

// Select chooses up to maxDispatches transitions from enabled transitions, respecting
// token conflict safety and dispatch-history-aware trace prioritization.
func (s *WorkInQueueScheduler) Select(enabled []interfaces.EnabledTransition, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	if len(enabled) == 0 || s == nil || s.maxDispatches <= 0 {
		return nil
	}

	initializedByTrace := buildInitializedTraceRegistry(snapshot)
	activeTraces := activeTracesFromSnapshot(snapshot)
	topology := snapshotTopology(snapshot)

	candidates := make([]queuedCandidate, 0, len(enabled))
	for _, et := range enabled {
		candidate, ok := collectCandidate(et, topology, s.runtimeConfig)
		if !ok {
			continue
		}
		candidate.applyTraceHistory(initializedByTrace)
		if candidate.isCompletedTrace(activeTraces, initializedByTrace) {
			continue
		}
		candidates = append(candidates, candidate)
	}

	stableSortQueuedCandidates(candidates)

	var decisions []interfaces.FiringDecision
	claimed := make(map[string]bool)
	for _, candidate := range candidates {
		if len(decisions) >= s.maxDispatches {
			break
		}
		if hasTokenConflict(candidate.consumeTokenIDs, claimed) {
			continue
		}
		for _, tokenID := range candidate.consumeTokenIDs {
			claimed[tokenID] = true
		}
		decisions = append(decisions, interfaces.FiringDecision{
			TransitionID:  candidate.transitionID,
			ConsumeTokens: candidate.consumeTokenIDs,
			WorkerType:    candidate.workerType,
			InputBindings: candidate.inputBindings,
		})
	}

	return decisions
}

type queuedCandidate struct {
	transitionID        string
	workerType          string
	inputBindings       map[string][]string
	consumeTokenIDs     []string
	traceIDs            []string
	earliestQueueTime   time.Time
	processingWorkCount int
	workstationPriority int
	hasCustomerWork     bool
	hasInitialized      bool
	lastDispatchAt      time.Time
}

func collectCandidate(et interfaces.EnabledTransition, topology *state.Net, runtimeConfig interfaces.RuntimeWorkstationLookup) (queuedCandidate, bool) {
	arcNames := stableArcNames(et.Bindings)
	if len(arcNames) == 0 {
		return queuedCandidate{}, false
	}

	seenConsumeTokens := make(map[string]struct{})
	consumeTokenIDs := make([]string, 0, len(et.Bindings))
	inputBindings := make(map[string][]string)
	traceIDSet := make(map[string]struct{})
	var earliestQueueTime time.Time
	processingWorkCount := 0
	hasCustomerWork := false

	for _, arcName := range arcNames {
		tokens := et.Bindings[arcName]
		for _, token := range tokens {
			tokenID := token.ID
			if tokenID == "" {
				continue
			}

			if isCustomerWorkToken(token) {
				if token.Color.TraceID != "" {
					traceIDSet[token.Color.TraceID] = struct{}{}
				}
				queuedAt := token.EnteredAt
				if queuedAt.IsZero() {
					queuedAt = token.CreatedAt
				}
				if !queuedAt.IsZero() && (earliestQueueTime.IsZero() || queuedAt.Before(earliestQueueTime)) {
					earliestQueueTime = queuedAt
				}
			}

			if et.ArcModes[arcName] == interfaces.ArcModeObserve {
				continue
			}
			if _, exists := seenConsumeTokens[tokenID]; !exists {
				consumeTokenIDs = append(consumeTokenIDs, tokenID)
				seenConsumeTokens[tokenID] = struct{}{}
				if isCustomerWorkToken(token) {
					hasCustomerWork = true
				}
				if isProcessingWorkToken(token, topology) {
					processingWorkCount++
				}
			}
			inputBindings[arcName] = append(inputBindings[arcName], tokenID)
		}
	}

	if len(consumeTokenIDs) == 0 {
		return queuedCandidate{}, false
	}

	traceIDs := make([]string, 0, len(traceIDSet))
	for id := range traceIDSet {
		traceIDs = append(traceIDs, id)
	}
	sort.Strings(traceIDs)

	return queuedCandidate{
		transitionID:        et.TransitionID,
		workerType:          et.WorkerType,
		inputBindings:       inputBindings,
		consumeTokenIDs:     consumeTokenIDs,
		traceIDs:            traceIDs,
		earliestQueueTime:   earliestQueueTime,
		processingWorkCount: processingWorkCount,
		workstationPriority: workstationKindPriority(et.TransitionID, topology, runtimeConfig),
		hasCustomerWork:     hasCustomerWork,
	}, true
}

func snapshotTopology(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) *state.Net {
	if snapshot == nil {
		return nil
	}
	return snapshot.Topology
}

func isProcessingWorkToken(token interfaces.Token, topology *state.Net) bool {
	if topology == nil || !isCustomerWorkToken(token) {
		return false
	}
	return topology.StateCategoryForPlace(token.PlaceID) == state.StateCategoryProcessing
}

func isCustomerWorkToken(token interfaces.Token) bool {
	if token.Color.DataType == interfaces.DataTypeResource {
		return false
	}
	return !interfaces.IsSystemTimeWorkType(token.Color.WorkTypeID)
}

func workstationKindPriority(transitionID string, topology *state.Net, runtimeConfig interfaces.RuntimeWorkstationLookup) int {
	if topology == nil || topology.Transitions == nil {
		return workstationPriorityNormal
	}
	transition := topology.Transitions[transitionID]
	if transition == nil {
		return workstationPriorityNormal
	}
	if transition.WorkerType == "" {
		return workstationPriorityLogical
	}
	if workstationconfig.Kind(transition, runtimeConfig) == interfaces.WorkstationKindCron {
		return workstationPriorityCron
	}
	return workstationPriorityNormal
}

func hasTokenConflict(tokenIDs []string, claimed map[string]bool) bool {
	for _, tokenID := range tokenIDs {
		if claimed[tokenID] {
			return true
		}
	}
	return false
}

func (c *queuedCandidate) applyTraceHistory(initializedByTrace map[string]time.Time) {
	for _, traceID := range c.traceIDs {
		if lastDispatchAt, ok := initializedByTrace[traceID]; ok {
			c.hasInitialized = true
			if c.lastDispatchAt.IsZero() || lastDispatchAt.Before(c.lastDispatchAt) {
				c.lastDispatchAt = lastDispatchAt
			}
		}
	}
}

func (c *queuedCandidate) isCompletedTrace(activeTraces map[string]bool, initializedByTrace map[string]time.Time) bool {
	for _, traceID := range c.traceIDs {
		_, initialized := initializedByTrace[traceID]
		if !initialized {
			continue
		}
		if !activeTraces[traceID] {
			return true
		}
	}
	return false
}

func stableSortQueuedCandidates(candidates []queuedCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.processingWorkCount != right.processingWorkCount {
			return left.processingWorkCount > right.processingWorkCount
		}
		if left.hasCustomerWork != right.hasCustomerWork {
			return left.hasCustomerWork
		}
		if left.workstationPriority != right.workstationPriority {
			return left.workstationPriority < right.workstationPriority
		}
		if left.hasInitialized != right.hasInitialized {
			return left.hasInitialized
		}
		if left.hasInitialized {
			if !left.lastDispatchAt.Equal(right.lastDispatchAt) {
				if left.lastDispatchAt.IsZero() {
					return false
				}
				if right.lastDispatchAt.IsZero() {
					return true
				}
				return left.lastDispatchAt.Before(right.lastDispatchAt)
			}
		}
		if !left.earliestQueueTime.Equal(right.earliestQueueTime) {
			if left.earliestQueueTime.IsZero() {
				return false
			}
			if right.earliestQueueTime.IsZero() {
				return true
			}
			return left.earliestQueueTime.Before(right.earliestQueueTime)
		}
		if left.transitionID != right.transitionID {
			return left.transitionID < right.transitionID
		}
		if left.workerType != right.workerType {
			return left.workerType < right.workerType
		}
		if len(left.consumeTokenIDs) != len(right.consumeTokenIDs) {
			return len(left.consumeTokenIDs) < len(right.consumeTokenIDs)
		}
		for idx := range left.consumeTokenIDs {
			if left.consumeTokenIDs[idx] == right.consumeTokenIDs[idx] {
				continue
			}
			return left.consumeTokenIDs[idx] < right.consumeTokenIDs[idx]
		}
		return false
	})
}

func buildInitializedTraceRegistry(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) map[string]time.Time {
	registry := make(map[string]time.Time)
	if snapshot == nil {
		return registry
	}
	for _, dispatch := range snapshot.DispatchHistory {
		dispatchAt := dispatch.EndTime
		if dispatchAt.IsZero() {
			dispatchAt = dispatch.StartTime
		}
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.TraceID == "" || token.Color.DataType == interfaces.DataTypeResource {
				continue
			}
			if earliest, ok := registry[token.Color.TraceID]; !ok || dispatchAt.Before(earliest) {
				registry[token.Color.TraceID] = dispatchAt
			}
		}
		for _, mutation := range dispatch.OutputMutations {
			if mutation.Token == nil {
				continue
			}
			if mutation.Token.Color.TraceID == "" || mutation.Token.Color.DataType == interfaces.DataTypeResource {
				continue
			}
			if earliest, ok := registry[mutation.Token.Color.TraceID]; !ok || dispatchAt.Before(earliest) {
				registry[mutation.Token.Color.TraceID] = dispatchAt
			}
		}
	}
	return registry
}

func activeTracesFromSnapshot(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) map[string]bool {
	active := make(map[string]bool)
	if snapshot == nil {
		return active
	}

	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if token.Color.TraceID == "" {
			continue
		}
		active[token.Color.TraceID] = true
	}

	for _, dispatch := range snapshot.Dispatches {
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.DataType == interfaces.DataTypeResource {
				continue
			}
			if token.Color.TraceID == "" {
				continue
			}
			active[token.Color.TraceID] = true
		}
	}

	return active
}

func stableArcNames(bindings map[string][]interfaces.Token) []string {
	arcNames := make([]string, 0, len(bindings))
	for arcName := range bindings {
		arcNames = append(arcNames, arcName)
	}
	sort.Strings(arcNames)
	return arcNames
}
