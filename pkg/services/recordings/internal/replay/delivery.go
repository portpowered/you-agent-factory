package replay

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const replaySubmissionHookName = "replay-artifact-submissions"
const replayWorkStateChangeHookName = "replay-artifact-work-state-changes"

type MetadataMismatchWarning = recordings.MetadataMismatchWarning

// FactoryMetadataWarnings compares two Factory-owned snapshots. Replay callers
// should warn but still allow replay because the artifact remains authoritative
// for runtime configuration.
func FactoryMetadataWarnings(artifactFactory, currentFactory *interfaces.FactorySnapshot) []MetadataMismatchWarning {
	return recordings.FactoryMetadataWarnings(artifactFactory, currentFactory)
}

// NewArtifactClock returns a replay clock aligned to recorded event times when
// the artifact includes wall-clock timestamps for specific ticks.
func NewArtifactClock(artifact *interfaces.ReplayArtifact) *platformclock.Deterministic {
	if artifact == nil {
		return platformclock.NewDeterministic(time.Time{}, 0)
	}
	return platformclock.NewRecordedDeterministic(artifact.RecordedAt, 0, recordedTickTimes(artifact))
}

func recordedTickTimes(artifact *interfaces.ReplayArtifact) map[int]time.Time {
	if artifact == nil {
		return nil
	}
	tickTimes := make(map[int]time.Time)
	recordTime := artifact.RecordedAt.UTC()
	if !recordTime.IsZero() {
		tickTimes[0] = recordTime
	}
	for _, event := range artifact.Events {
		eventTime := event.Context.EventTime.UTC()
		if eventTime.IsZero() {
			continue
		}
		tick := event.Context.Tick
		existing, exists := tickTimes[tick]
		if !exists || eventTime.Before(existing) {
			tickTimes[tick] = eventTime
		}
	}
	if len(tickTimes) == 0 {
		return nil
	}
	return tickTimes
}

// SubmissionHook returns recorded submissions when their logical observation
// tick is reached.
type SubmissionHook struct {
	submissions []replaySubmission
	next        int
	mu          sync.Mutex
}

var _ recordings.ReplayHook = (*SubmissionHook)(nil)

// NewSubmissionHook builds an engine submission hook from recorded artifact
// submissions.
func NewSubmissionHook(
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig interfaces.ReplayRuntimeConfigDecoder,
	artifact *interfaces.ReplayArtifact,
) (*SubmissionHook, error) {
	eventLog, err := reduceReplayEvents(
		artifact,
		decodeFactorySnapshot,
		decodeRuntimeConfig,
	)
	if err != nil {
		return nil, err
	}
	submissions := append([]replaySubmission(nil), eventLog.Submissions...)
	applyReplaySubmissionDispatchDefaults(submissions, eventLog.Dispatches)
	applyReplaySubmissionDefaults(submissions, eventLog.RuntimeConfig.FactoryConfig())
	return &SubmissionHook{submissions: submissions}, nil
}

func applyReplaySubmissionDispatchDefaults(submissions []replaySubmission, dispatches []replayDispatch) {
	byWorkID, byNameAndTrace, byTrace := buildReplayDispatchDefaultsIndex(dispatches)
	for i := range submissions {
		applyReplaySubmissionDefaultsForRequest(submissions[i].request.Works, byWorkID, byNameAndTrace, byTrace)
	}
}

type dispatchWorkDefaults struct {
	workID     string
	workTypeID string
	traceID    string
}

type namedTraceKey struct {
	name    string
	traceID string
}

func buildReplayDispatchDefaultsIndex(dispatches []replayDispatch) (
	map[string]dispatchWorkDefaults,
	map[namedTraceKey]dispatchWorkDefaults,
	map[string][]dispatchWorkDefaults,
) {
	byWorkID := make(map[string]dispatchWorkDefaults)
	byNameAndTrace := make(map[namedTraceKey]dispatchWorkDefaults)
	byTrace := make(map[string][]dispatchWorkDefaults)
	for _, dispatch := range dispatches {
		for _, token := range workers.WorkDispatchInputTokens(dispatch.dispatch) {
			current, ok := replayDispatchDefaultsFromToken(token, dispatch.dispatch.Execution.TraceID)
			if !ok {
				continue
			}
			byWorkID[current.workID] = current
			if token.Color.Name != "" && current.traceID != "" {
				key := namedTraceKey{name: token.Color.Name, traceID: current.traceID}
				if _, exists := byNameAndTrace[key]; !exists {
					byNameAndTrace[key] = current
				}
			}
			if current.traceID != "" {
				byTrace[current.traceID] = append(byTrace[current.traceID], current)
			}
		}
	}
	return byWorkID, byNameAndTrace, byTrace
}

func replayDispatchDefaultsFromToken(token workerexecution.Token, fallbackTraceID string) (dispatchWorkDefaults, bool) {
	if token.Color.DataType == workerexecution.DataTypeResource || token.Color.WorkID == "" {
		return dispatchWorkDefaults{}, false
	}
	traceID := token.Color.TraceID
	if traceID == "" {
		traceID = fallbackTraceID
	}
	return dispatchWorkDefaults{
		workID:     token.Color.WorkID,
		workTypeID: token.Color.WorkTypeID,
		traceID:    traceID,
	}, true
}

func applyReplaySubmissionDefaultsForRequest(
	works []work.Work,
	byWorkID map[string]dispatchWorkDefaults,
	byNameAndTrace map[namedTraceKey]dispatchWorkDefaults,
	byTrace map[string][]dispatchWorkDefaults,
) {
	for i := range works {
		work := &works[i]
		defaults := replayDispatchDefaultsForWork(*work, byWorkID, byNameAndTrace, byTrace)
		if work.WorkID == "" {
			work.WorkID = defaults.workID
		}
		if work.WorkTypeID == "" {
			work.WorkTypeID = defaults.workTypeID
		}
		if work.TraceID == "" {
			work.TraceID = defaults.traceID
		}
	}
}

func replayDispatchDefaultsForWork(
	work work.Work,
	byWorkID map[string]dispatchWorkDefaults,
	byNameAndTrace map[namedTraceKey]dispatchWorkDefaults,
	byTrace map[string][]dispatchWorkDefaults,
) dispatchWorkDefaults {
	defaults := byWorkID[work.WorkID]
	if defaults.workID == "" && work.Name != "" && work.TraceID != "" {
		defaults = byNameAndTrace[namedTraceKey{name: work.Name, traceID: work.TraceID}]
	}
	if defaults.workID == "" && work.TraceID != "" {
		queued := byTrace[work.TraceID]
		if len(queued) > 0 {
			defaults = queued[0]
			byTrace[work.TraceID] = queued[1:]
		}
	}
	return defaults
}

func applyReplaySubmissionDefaults(submissions []replaySubmission, factoryConfig *interfaces.FactoryConfig) {
	if factoryConfig == nil {
		return
	}
	defaultWorkType := ""
	if len(factoryConfig.WorkTypes) == 1 {
		defaultWorkType = factoryConfig.WorkTypes[0].Name
	}
	validWorkTypes := make(map[string]bool)
	for _, workType := range factoryConfig.WorkTypes {
		validWorkTypes[workType.Name] = true
	}
	if defaultWorkType == "" && len(validWorkTypes) == 0 {
		return
	}
	for i := range submissions {
		for j := range submissions[i].request.Works {
			work := &submissions[i].request.Works[j]
			if work.WorkTypeID == "" {
				work.WorkTypeID = replayWorkTypeFromWorkID(work.WorkID, validWorkTypes)
			}
			if work.WorkTypeID == "" {
				work.WorkTypeID = defaultWorkType
			}
		}
	}
}

func replayWorkTypeFromWorkID(workID string, validWorkTypes map[string]bool) string {
	if workID == "" || len(validWorkTypes) == 0 {
		return ""
	}
	names := make([]string, 0, len(validWorkTypes))
	for name := range validWorkTypes {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})
	for _, name := range names {
		if strings.HasPrefix(workID, "work-"+name+"-") {
			return name
		}
	}
	return ""
}

func (h *SubmissionHook) Name() string {
	return replaySubmissionHookName
}

func (h *SubmissionHook) Priority() int {
	return -100
}

func (h *SubmissionHook) OnTick(ctx context.Context, input recordings.ReplaySnapshot) (recordings.ReplayHookResult, error) {
	if err := ctx.Err(); err != nil {
		return recordings.ReplayHookResult{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return recordings.ReplayHookResult{}, err
	}

	var due []replaySubmission
	for h.next < len(h.submissions) && h.submissions[h.next].observedTick <= input.Tick {
		recorded := h.submissions[h.next]
		due = append(due, recorded)
		h.next++
	}
	keepAlive := h.next < len(h.submissions)
	if len(due) == 0 {
		return recordings.ReplayHookResult{KeepAlive: keepAlive}, nil
	}
	return recordings.ReplayHookResult{
		GeneratedBatches: generatedBatchesFromReplaySubmissions(due),
		KeepAlive:        keepAlive,
	}, nil
}

func generatedBatchesFromReplaySubmissions(records []replaySubmission) []work.GeneratedSubmissionBatch {
	batches := make([]work.GeneratedSubmissionBatch, 0, len(records))
	for i, record := range records {
		request := record.request
		if request.RequestID == "" {
			request.RequestID = fmt.Sprintf("%s/%d", record.eventID, i)
		}
		batches = append(batches, work.GeneratedSubmissionBatch{
			Request: request,
			Metadata: work.GeneratedSubmissionBatchMetadata{
				Source: record.source,
			},
		})
	}
	return batches
}

// WorkStateChangeHook replays recorded operator WORK_STATE_CHANGE events at
// their observed logical ticks.
type WorkStateChangeHook struct {
	changes []replayWorkStateChange
	next    int
	mu      sync.Mutex
}

var _ recordings.ReplayHook = (*WorkStateChangeHook)(nil)

// NewWorkStateChangeHook builds an engine submission hook from recorded
// operator move events in a replay artifact.
func NewWorkStateChangeHook(
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig interfaces.ReplayRuntimeConfigDecoder,
	artifact *interfaces.ReplayArtifact,
) (*WorkStateChangeHook, error) {
	eventLog, err := reduceReplayEvents(
		artifact,
		decodeFactorySnapshot,
		decodeRuntimeConfig,
	)
	if err != nil {
		return nil, err
	}
	return &WorkStateChangeHook{changes: append([]replayWorkStateChange(nil), eventLog.WorkStateChanges...)}, nil
}

func (h *WorkStateChangeHook) Name() string {
	return replayWorkStateChangeHookName
}

func (h *WorkStateChangeHook) Priority() int {
	return -90
}

func (h *WorkStateChangeHook) OnTick(ctx context.Context, input recordings.ReplaySnapshot) (recordings.ReplayHookResult, error) {
	if err := ctx.Err(); err != nil {
		return recordings.ReplayHookResult{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return recordings.ReplayHookResult{}, err
	}

	var mutations []interfaces.MarkingMutation
	for h.next < len(h.changes) && h.changes[h.next].observedTick <= input.Tick {
		recorded := h.changes[h.next]
		h.next++
		mutation, ok := replayWorkStateChangeMutation(input, recorded)
		if !ok {
			continue
		}
		mutations = append(mutations, mutation)
	}
	keepAlive := h.next < len(h.changes)
	if len(mutations) == 0 {
		return recordings.ReplayHookResult{KeepAlive: keepAlive}, nil
	}
	return recordings.ReplayHookResult{
		MarkingMutations: mutations,
		KeepAlive:        keepAlive,
	}, nil
}

func replayWorkStateChangeMutation(
	snapshot recordings.ReplaySnapshot,
	recorded replayWorkStateChange,
) (interfaces.MarkingMutation, bool) {
	change := recorded.change
	if change.WorkID == "" || recorded.fromPlaceID == "" || recorded.toPlaceID == "" {
		return interfaces.MarkingMutation{}, false
	}
	if recorded.fromPlaceID == recorded.toPlaceID {
		return interfaces.MarkingMutation{}, false
	}
	token, ok := snapshot.TokenByWorkID[change.WorkID]
	if !ok {
		// An operator move may be recorded while the Work token is consumed by
		// an active dispatch. Runtime exposes that exact consumed token to the
		// replay hook so the move can restore and relocate it before the late
		// result is delivered.
		token, ok = snapshot.ConsumedTokenByWorkID[change.WorkID]
	}
	if !ok || token.TokenID == "" {
		return interfaces.MarkingMutation{}, false
	}
	reason := fmt.Sprintf("operator move to %s", change.ToState)
	if change.Reason != "" {
		reason = change.Reason
	}
	return interfaces.MarkingMutation{
		Type:      interfaces.MutationMove,
		TokenID:   token.TokenID,
		FromPlace: recorded.fromPlaceID,
		ToPlace:   recorded.toPlaceID,
		Reason:    reason,
	}, true
}

// CompletionDeliveryPlan maps observed replay dispatches to recorded
// completion delivery ticks.
type CompletionDeliveryPlan struct {
	mu                          sync.Mutex
	records                     []completionDeliveryRecord
	plannedResults              map[string]workerexecution.WorkResult
	workerSessionIDs            map[string]string
	workerSessionIDsByReplayKey map[string]string
}

type completionDeliveryRecord struct {
	dispatch      replayDispatch
	completion    *replayCompletion
	deliveryDelay int
	hasCompletion bool
	used          bool
}

var _ recordings.CompletionDeliveryPlanner = (*CompletionDeliveryPlan)(nil)

// NewCompletionDeliveryPlan builds the replay completion delivery contract
// from an artifact.
func NewCompletionDeliveryPlan(
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig interfaces.ReplayRuntimeConfigDecoder,
	artifact *interfaces.ReplayArtifact,
) (*CompletionDeliveryPlan, error) {
	eventLog, err := reduceReplayEvents(
		artifact,
		decodeFactorySnapshot,
		decodeRuntimeConfig,
	)
	if err != nil {
		return nil, err
	}

	dispatches := make(map[string]replayDispatch, len(eventLog.Dispatches))
	for _, dispatch := range eventLog.Dispatches {
		dispatches[dispatch.dispatchID] = dispatch
	}

	completions := make(map[string]replayCompletion, len(eventLog.Completions))
	for _, completion := range eventLog.Completions {
		if _, ok := dispatches[completion.dispatchID]; !ok {
			return nil, newDivergenceError(
				DivergenceCategoryUnknownCompletion,
				completion.observedTick,
				completion.dispatchID,
				"recorded dispatch for completion "+completion.completionID,
				"completion references unknown dispatch "+completion.dispatchID,
				withExpectedEventID(completion.eventID),
			)
		}
		completions[completion.dispatchID] = completion
	}
	for dispatchID, ignored := range eventLog.IgnoredResults {
		if _, ok := dispatches[dispatchID]; !ok {
			return nil, newDivergenceError(
				DivergenceCategoryMissingDispatch,
				ignored.observedTick,
				dispatchID,
				"recorded dispatch for ignored result",
				"ignored result references unknown dispatch",
				withExpectedEventID(ignored.eventID),
			)
		}
	}

	records := make([]completionDeliveryRecord, 0, len(eventLog.Dispatches))
	for _, dispatch := range eventLog.Dispatches {
		record := completionDeliveryRecord{dispatch: dispatch}
		if completion, ok := completions[dispatch.dispatchID]; ok {
			completionCopy := completion
			record.completion = &completionCopy
			record.deliveryDelay = completion.observedTick - dispatch.createdTick
			if record.deliveryDelay < 0 {
				record.deliveryDelay = 0
			}
			record.hasCompletion = true
		} else if ignored, ok := eventLog.IgnoredResults[dispatch.dispatchID]; ok {
			completion := replayCompletionFromIgnoredResult(dispatch, ignored)
			record.completion = &completion
			record.deliveryDelay = ignored.observedTick - dispatch.createdTick
			if record.deliveryDelay < 0 {
				record.deliveryDelay = 0
			}
			record.hasCompletion = true
		}
		records = append(records, completionDeliveryRecord{
			dispatch:      record.dispatch,
			completion:    record.completion,
			deliveryDelay: record.deliveryDelay,
			hasCompletion: record.hasCompletion,
		})
	}
	workerSessionIDs := cloneReplayWorkerSessionIDs(eventLog.WorkerSessionIDs)
	workerSessionIDsByReplayKey := make(map[string]string)
	for _, record := range records {
		workerSessionID := workerSessionIDs[record.dispatch.dispatchID]
		if strings.TrimSpace(workerSessionID) == "" || strings.TrimSpace(record.dispatch.dispatch.Execution.ReplayKey) == "" {
			continue
		}
		workerSessionIDsByReplayKey[record.dispatch.dispatch.Execution.ReplayKey] = workerSessionID
	}
	return &CompletionDeliveryPlan{
		records:                     records,
		plannedResults:              make(map[string]workerexecution.WorkResult),
		workerSessionIDs:            workerSessionIDs,
		workerSessionIDsByReplayKey: workerSessionIDsByReplayKey,
	}, nil
}

func replayCompletionFromIgnoredResult(
	dispatch replayDispatch,
	ignored replayIgnoredResult,
) replayCompletion {
	result := workerexecution.WorkResult{
		DispatchID:   dispatch.dispatchID,
		TransitionID: dispatch.dispatch.TransitionID,
		Outcome:      ignored.resultOutcome,
	}
	if ignored.resultOutcome == workerexecution.OutcomeCanceled {
		result.Cancellation = &workerexecution.DispatchCancellation{
			Reason: workerexecution.DispatchCancellationReasonCanceled,
		}
		result.Error = workers.ErrWorkstationDispatchCanceled.Error()
	}
	return replayCompletion{
		eventID:      ignored.eventID,
		completionID: ignored.eventID,
		dispatchID:   dispatch.dispatchID,
		observedTick: ignored.observedTick,
		result:       result,
	}
}

// WorkerSessionIDForDispatch returns the identity recorded alongside one
// dispatch, allowing replay to preserve the stable Worker Session address.
func (p *CompletionDeliveryPlan) WorkerSessionIDForDispatch(dispatch work.WorkDispatch) (string, bool) {
	if p == nil {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	workerSessionID, ok := p.workerSessionIDs[strings.TrimSpace(dispatch.DispatchID)]
	if !ok {
		workerSessionID, ok = p.workerSessionIDsByReplayKey[strings.TrimSpace(dispatch.Execution.ReplayKey)]
	}
	return workerSessionID, ok && strings.TrimSpace(workerSessionID) != ""
}

// DispatchIDForDispatch returns the canonical dispatch identity recorded for
// an equivalent replay dispatch. Reusing this identity is important for
// durable operator-input claims, whose approval ID is derived from dispatch
// identity and must not fork during replay.
func (p *CompletionDeliveryPlan) DispatchIDForDispatch(dispatch work.WorkDispatch) (string, bool) {
	if p == nil {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, record := range p.records {
		if strings.TrimSpace(record.dispatch.dispatchID) == "" || record.used {
			continue
		}
		if !recordedDispatchMatches(record.dispatch, dispatch) {
			continue
		}
		return record.dispatch.dispatchID, true
	}
	return "", false
}

func cloneReplayWorkerSessionIDs(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for dispatchID, workerSessionID := range values {
		clone[dispatchID] = workerSessionID
	}
	return clone
}

func (p *CompletionDeliveryPlan) DeliveryTickForDispatch(dispatch work.WorkDispatch) (int, bool, error) {
	if p == nil {
		return 0, false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.records {
		record := &p.records[i]
		if record.used {
			continue
		}
		if !recordedDispatchMatches(record.dispatch, dispatch) {
			continue
		}
		record.used = true
		if !record.hasCompletion {
			return 0, false, nil
		}
		if record.completion != nil {
			planned := cloneReplayPlannedResult(record.completion.result)
			planned.DispatchID = dispatch.DispatchID
			planned.TransitionID = dispatch.TransitionID
			p.plannedResults[dispatch.DispatchID] = planned
		}
		deliveryTick := dispatch.Execution.DispatchCreatedTick + record.deliveryDelay
		// Preserve recorded completion ordering when an equivalent dispatch is
		// observed earlier than it was in the original run.
		if record.completion != nil && deliveryTick < record.completion.observedTick {
			deliveryTick = record.completion.observedTick
		}
		return deliveryTick, true, nil
	}
	if expected, ok := p.expectedForObservedDispatchLocked(dispatch); ok {
		return 0, false, newDivergenceError(
			DivergenceCategoryDispatchMismatch,
			dispatch.Execution.DispatchCreatedTick,
			dispatch.DispatchID,
			dispatchSummary(expected.dispatch.dispatch),
			dispatchSummary(dispatch),
			withExpectedEventID(expected.dispatch.eventID),
		)
	}
	return 0, false, newDivergenceError(
		DivergenceCategoryDispatchMismatch,
		dispatch.Execution.DispatchCreatedTick,
		dispatch.DispatchID,
		"no recorded dispatch at observed tick",
		dispatchSummary(dispatch),
	)
}

// ValidateReplayTick is retained for the runtime hook contract. Replay dispatch
// matching is intentionally based on logical dispatch identity instead of exact
// ticks because repaired runs can reschedule equivalent dispatches or terminally
// drain work before later recorded dispatches are needed.
func (p *CompletionDeliveryPlan) ValidateReplayTick(currentTick int) error {
	if p == nil {
		return nil
	}
	return nil
}

func (p *CompletionDeliveryPlan) PlannedResultForDispatch(dispatch work.WorkDispatch) (workerexecution.WorkResult, bool, error) {
	if p == nil {
		return workerexecution.WorkResult{}, false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	result, ok := p.plannedResults[dispatch.DispatchID]
	if !ok {
		return workerexecution.WorkResult{}, false, nil
	}
	delete(p.plannedResults, dispatch.DispatchID)
	return cloneReplayPlannedResult(result), true, nil
}

func cloneReplayPlannedResult(result workerexecution.WorkResult) workerexecution.WorkResult {
	clone := result
	clone.Cancellation = result.Cancellation.Clone()
	clone.OutputContent = work.CloneWorkContentParts(result.OutputContent)
	clone.StructuredResult = jsonvalue.Clone(result.StructuredResult)
	clone.StructuredResultPresent = jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent)
	if result.RecordedOutputWork != nil {
		clone.RecordedOutputWork = cloneReplayFactoryWorkItems(result.RecordedOutputWork)
	}
	clone.FailureMetadata = workerexecution.CloneWorkFailureMetadata(result.FailureMetadata)
	if result.Continuation != nil {
		continuation := result.Continuation.Clone()
		clone.Continuation = &continuation
	}
	clone.Diagnostics = workerexecution.CloneWorkDiagnostics(result.Diagnostics)
	return clone
}

func cloneReplayFactoryWorkItems(items []work.FactoryWorkItem) []work.FactoryWorkItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]work.FactoryWorkItem, len(items))
	for i := range items {
		out[i] = items[i]
		if items[i].PreviousChainingTraceIDs != nil {
			out[i].PreviousChainingTraceIDs = append([]string(nil), items[i].PreviousChainingTraceIDs...)
		}
		if items[i].Content != nil {
			out[i].Content = append([]work.WorkContentPart(nil), items[i].Content...)
		}
		out[i].StructuredResult = jsonvalue.Clone(items[i].StructuredResult)
		out[i].StructuredResultPresent = jsonvalue.Present(items[i].StructuredResult, items[i].StructuredResultPresent)
		if items[i].Tags != nil {
			out[i].Tags = cloneStringMap(items[i].Tags)
		}
	}
	return out
}

func (p *CompletionDeliveryPlan) expectedForObservedDispatchLocked(observed work.WorkDispatch) (completionDeliveryRecord, bool) {
	for _, record := range p.records {
		if record.used {
			continue
		}
		if record.dispatch.createdTick == observed.Execution.DispatchCreatedTick {
			return record, true
		}
	}
	if len(p.records) == 0 {
		return completionDeliveryRecord{}, false
	}
	for _, record := range p.records {
		if !record.used {
			return record, true
		}
	}
	return completionDeliveryRecord{}, false
}

func recordedDispatchMatches(recorded replayDispatch, observed work.WorkDispatch) bool {
	return dispatchMatches(recorded.dispatch, observed)
}

func dispatchMatches(recorded, observed work.WorkDispatch) bool {
	if recorded.TransitionID != "" && observed.TransitionID != recorded.TransitionID {
		return false
	}
	if recorded.WorkerType != "" && observed.WorkerType != "" && observed.WorkerType != recorded.WorkerType {
		return false
	}
	if recorded.WorkstationName != "" && observed.WorkstationName != "" && observed.WorkstationName != recorded.WorkstationName {
		return false
	}
	if len(recorded.InputTokens) > 0 && !tokenIDsMatch(workers.WorkDispatchInputTokens(recorded), workers.WorkDispatchInputTokens(observed)) {
		return false
	}
	return executionMetadataMatches(recorded.Execution, observed.Execution)
}

func tokenIDsMatch(recorded, observed []workerexecution.Token) bool {
	if !hasResourceToken(recorded) {
		observed = nonResourceTokens(observed)
	}
	recordedIDs := make([]string, 0, len(recorded))
	for _, token := range recorded {
		recordedIDs = append(recordedIDs, replayTokenIdentity(token))
	}
	observedIDs := make([]string, 0, len(observed))
	for _, token := range observed {
		observedIDs = append(observedIDs, replayTokenIdentity(token))
	}
	sort.Strings(recordedIDs)
	sort.Strings(observedIDs)
	return reflect.DeepEqual(recordedIDs, observedIDs)
}

func hasResourceToken(tokens []workerexecution.Token) bool {
	for _, token := range tokens {
		if token.Color.DataType == workerexecution.DataTypeResource {
			return true
		}
	}
	return false
}

func nonResourceTokens(tokens []workerexecution.Token) []workerexecution.Token {
	out := make([]workerexecution.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == workerexecution.DataTypeResource {
			continue
		}
		out = append(out, token)
	}
	return out
}

func replayTokenIdentity(token workerexecution.Token) string {
	if token.Color.DataType == workerexecution.DataTypeResource {
		resourceID := resourceTokenName(token)
		return strings.Join([]string{"resource", resourceID}, "/")
	}
	if token.Color.WorkID != "" {
		return token.Color.WorkID
	}
	return token.ID
}

func resourceTokenName(token workerexecution.Token) string {
	if token.Color.WorkTypeID != "" {
		return token.Color.WorkTypeID
	}
	if token.Color.Name != "" {
		return token.Color.Name
	}
	if before, _, ok := strings.Cut(token.ID, ":resource:"); ok && before != "" {
		return before
	}
	return token.ID
}

func dispatchSummary(dispatch work.WorkDispatch) string {
	return fmt.Sprintf(
		"dispatch_id=%s transition=%s workstation=%s replay_key=%s trace_id=%s work_ids=%v input_tokens=%v",
		dispatch.DispatchID,
		dispatch.TransitionID,
		dispatch.WorkstationName,
		dispatch.Execution.ReplayKey,
		dispatch.Execution.TraceID,
		dispatch.Execution.WorkIDs,
		workTokenIDs(workers.WorkDispatchInputTokens(dispatch)),
	)
}

func workTokenIDs(tokens []workerexecution.Token) []string {
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.ID)
	}
	return ids
}
