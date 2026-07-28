package scheduler

import (
	"sort"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

// WorkInQueueScheduler selects transition firings in deterministic batches.
// It prioritizes candidates consuming more customer work already in processing
// states, then customer work, workstation kind, initialized traces, completion
// age, and token-queue age.
type WorkInQueueScheduler struct {
	maxDispatches int
	runtimeConfig interfaces.RuntimeWorkstationLookup
}

const (
	workstationPriorityLogical = -1
	workstationPriorityNormal  = 0
	workstationPriorityCron    = 1
)

// NewWorkInQueueScheduler creates a bounded scheduler that can select up to
// maxDispatches firings per tick.
func NewWorkInQueueScheduler(
	maxDispatches int,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) *WorkInQueueScheduler {
	if maxDispatches <= 0 {
		maxDispatches = 1
	}
	return &WorkInQueueScheduler{
		maxDispatches: maxDispatches,
		runtimeConfig: runtimeConfig,
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

	analysis := newQueuedCandidateAnalysis(et, topology)
	for _, arcName := range arcNames {
		analysis.consumeArc(arcName, et.Bindings[arcName])
	}
	if len(analysis.consumeTokenIDs) == 0 {
		return queuedCandidate{}, false
	}

	return queuedCandidate{
		transitionID:        et.TransitionID,
		workerType:          et.WorkerType,
		inputBindings:       analysis.inputBindings,
		consumeTokenIDs:     analysis.consumeTokenIDs,
		traceIDs:            analysis.traceIDs(),
		earliestQueueTime:   analysis.earliestQueueTime,
		processingWorkCount: analysis.processingWorkCount,
		workstationPriority: workstationKindPriority(et.TransitionID, topology, runtimeConfig),
		hasCustomerWork:     analysis.hasCustomerWork,
	}, true
}

type queuedCandidateAnalysis struct {
	arcModes            map[string]interfaces.ArcMode
	topology            *state.Net
	seenConsumeTokens   map[string]struct{}
	consumeTokenIDs     []string
	inputBindings       map[string][]string
	traceIDSet          map[string]struct{}
	earliestQueueTime   time.Time
	processingWorkCount int
	hasCustomerWork     bool
}

func newQueuedCandidateAnalysis(et interfaces.EnabledTransition, topology *state.Net) *queuedCandidateAnalysis {
	return &queuedCandidateAnalysis{
		arcModes:          et.ArcModes,
		topology:          topology,
		seenConsumeTokens: make(map[string]struct{}),
		consumeTokenIDs:   make([]string, 0, len(et.Bindings)),
		inputBindings:     make(map[string][]string),
		traceIDSet:        make(map[string]struct{}),
	}
}

func (a *queuedCandidateAnalysis) consumeArc(arcName string, tokens []factorytoken.Token) {
	for _, token := range tokens {
		a.consumeToken(arcName, token)
	}
}

func (a *queuedCandidateAnalysis) consumeToken(arcName string, token factorytoken.Token) {
	tokenID := token.ID
	if tokenID == "" {
		return
	}
	a.observeTraceCandidate(token)
	if a.arcModes[arcName] == interfaces.ArcModeObserve {
		return
	}
	a.inputBindings[arcName] = append(a.inputBindings[arcName], tokenID)
	if _, exists := a.seenConsumeTokens[tokenID]; exists {
		return
	}
	a.seenConsumeTokens[tokenID] = struct{}{}
	a.consumeTokenIDs = append(a.consumeTokenIDs, tokenID)
	if isCustomerWorkToken(token) {
		a.hasCustomerWork = true
	}
	if isProcessingWorkToken(token, a.topology) {
		a.processingWorkCount++
	}
}

func (a *queuedCandidateAnalysis) observeTraceCandidate(token factorytoken.Token) {
	if !isCustomerWorkToken(token) {
		return
	}
	if token.Color.TraceID != "" {
		a.traceIDSet[token.Color.TraceID] = struct{}{}
	}
	queuedAt := token.EnteredAt
	if queuedAt.IsZero() {
		queuedAt = token.CreatedAt
	}
	if !queuedAt.IsZero() && (a.earliestQueueTime.IsZero() || queuedAt.Before(a.earliestQueueTime)) {
		a.earliestQueueTime = queuedAt
	}
}

func (a *queuedCandidateAnalysis) traceIDs() []string {
	traceIDs := make([]string, 0, len(a.traceIDSet))
	for id := range a.traceIDSet {
		traceIDs = append(traceIDs, id)
	}
	sort.Strings(traceIDs)
	return traceIDs
}

func snapshotTopology(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) *state.Net {
	if snapshot == nil {
		return nil
	}
	return snapshot.Topology
}

func isProcessingWorkToken(token factorytoken.Token, topology *state.Net) bool {
	if topology == nil || !isCustomerWorkToken(token) {
		return false
	}
	return topology.StateCategoryForPlace(token.PlaceID) == state.StateCategoryProcessing
}

func isCustomerWorkToken(token factorytoken.Token) bool {
	if token.Color.DataType == factorytoken.DataTypeResource {
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
	if runtimeWorkstationKind(transition.Name, runtimeConfig) == interfaces.WorkstationKindCron {
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
		return queuedCandidateLess(candidates[i], candidates[j])
	})
}

func queuedCandidateLess(left queuedCandidate, right queuedCandidate) bool {
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
	if left.hasInitialized && !left.lastDispatchAt.Equal(right.lastDispatchAt) {
		return earlierNonZeroTime(left.lastDispatchAt, right.lastDispatchAt)
	}
	if !left.earliestQueueTime.Equal(right.earliestQueueTime) {
		return earlierNonZeroTime(left.earliestQueueTime, right.earliestQueueTime)
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
	return orderedTokenIDsLess(left.consumeTokenIDs, right.consumeTokenIDs)
}

func earlierNonZeroTime(left time.Time, right time.Time) bool {
	if left.IsZero() {
		return false
	}
	if right.IsZero() {
		return true
	}
	return left.Before(right)
}

func orderedTokenIDsLess(left []string, right []string) bool {
	for idx := range left {
		if left[idx] == right[idx] {
			continue
		}
		return left[idx] < right[idx]
	}
	return false
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
			if token.Color.TraceID == "" || token.Color.DataType == factorytoken.DataTypeResource {
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
			if mutation.Token.Color.TraceID == "" || mutation.Token.Color.DataType == factorytoken.DataTypeResource {
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
		if token == nil || token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if token.Color.TraceID == "" {
			continue
		}
		active[token.Color.TraceID] = true
	}

	for _, dispatch := range snapshot.Dispatches {
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.DataType == factorytoken.DataTypeResource {
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

func stableArcNames(bindings map[string][]factorytoken.Token) []string {
	arcNames := make([]string, 0, len(bindings))
	for arcName := range bindings {
		arcNames = append(arcNames, arcName)
	}
	sort.Strings(arcNames)
	return arcNames
}
