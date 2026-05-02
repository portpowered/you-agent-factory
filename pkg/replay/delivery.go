package replay

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

const replaySubmissionHookName = "replay-artifact-submissions"

// SubmissionHook returns recorded submissions when their logical observation
// tick is reached.
type SubmissionHook struct {
	submissions []replaySubmission
	next        int
	mu          sync.Mutex
}

var _ factory.SubmissionHook = (*SubmissionHook)(nil)

// NewSubmissionHook builds an engine submission hook from recorded artifact
// submissions.
func NewSubmissionHook(artifact *interfaces.ReplayArtifact) (*SubmissionHook, error) {
	eventLog, err := reduceReplayEvents(artifact)
	if err != nil {
		return nil, err
	}
	submissions := append([]replaySubmission(nil), eventLog.Submissions...)
	applyReplaySubmissionDispatchDefaults(submissions, eventLog.Dispatches)
	applyReplaySubmissionDefaults(submissions, eventLog.Factory)
	return &SubmissionHook{submissions: submissions}, nil
}

func applyReplaySubmissionDispatchDefaults(submissions []replaySubmission, dispatches []replayDispatch) {
	type dispatchWorkDefaults struct {
		workID     string
		workTypeID string
		traceID    string
	}
	type namedTraceKey struct {
		name    string
		traceID string
	}
	byWorkID := make(map[string]dispatchWorkDefaults)
	byNameAndTrace := make(map[namedTraceKey]dispatchWorkDefaults)
	byTrace := make(map[string][]dispatchWorkDefaults)
	for _, dispatch := range dispatches {
		for _, token := range workers.WorkDispatchInputTokens(dispatch.dispatch) {
			if token.Color.DataType == interfaces.DataTypeResource || token.Color.WorkID == "" {
				continue
			}
			traceID := token.Color.TraceID
			if traceID == "" {
				traceID = dispatch.dispatch.Execution.TraceID
			}
			current := byWorkID[token.Color.WorkID]
			current.workID = token.Color.WorkID
			if current.workTypeID == "" {
				current.workTypeID = token.Color.WorkTypeID
			}
			if current.traceID == "" {
				current.traceID = traceID
			}
			byWorkID[token.Color.WorkID] = current

			if token.Color.Name != "" && traceID != "" {
				key := namedTraceKey{name: token.Color.Name, traceID: traceID}
				if _, exists := byNameAndTrace[key]; !exists {
					byNameAndTrace[key] = current
				}
			}
			if traceID != "" {
				byTrace[traceID] = append(byTrace[traceID], current)
			}
		}
	}
	for i := range submissions {
		for j := range submissions[i].request.Works {
			work := &submissions[i].request.Works[j]
			defaults := byWorkID[work.WorkID]
			if defaults.workID == "" && work.Name != "" && work.TraceID != "" {
				defaults = byNameAndTrace[namedTraceKey{
					name:    work.Name,
					traceID: work.TraceID,
				}]
			}
			if defaults.workID == "" && work.TraceID != "" {
				queued := byTrace[work.TraceID]
				if len(queued) > 0 {
					defaults = queued[0]
					byTrace[work.TraceID] = queued[1:]
				}
			}
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
}

func applyReplaySubmissionDefaults(submissions []replaySubmission, generatedFactory factoryapi.Factory) {
	defaultWorkType := ""
	if generatedFactory.WorkTypes != nil && len(*generatedFactory.WorkTypes) == 1 {
		defaultWorkType = (*generatedFactory.WorkTypes)[0].Name
	}
	validWorkTypes := make(map[string]bool)
	if generatedFactory.WorkTypes != nil {
		for _, workType := range *generatedFactory.WorkTypes {
			validWorkTypes[workType.Name] = true
		}
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

func (h *SubmissionHook) OnTick(_ context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var due []replaySubmission
	for h.next < len(h.submissions) && h.submissions[h.next].observedTick <= input.Snapshot.TickCount {
		recorded := h.submissions[h.next]
		due = append(due, recorded)
		h.next++
	}
	keepAlive := h.next < len(h.submissions)
	if len(due) == 0 {
		return interfaces.SubmissionHookResult{KeepAlive: keepAlive}, nil
	}
	return interfaces.SubmissionHookResult{
		GeneratedBatches: generatedBatchesFromReplaySubmissions(due),
		KeepAlive:        keepAlive,
	}, nil
}

func generatedBatchesFromReplaySubmissions(records []replaySubmission) []interfaces.GeneratedSubmissionBatch {
	batches := make([]interfaces.GeneratedSubmissionBatch, 0, len(records))
	for i, record := range records {
		request := record.request
		if request.RequestID == "" {
			request.RequestID = fmt.Sprintf("%s/%d", record.eventID, i)
		}
		batches = append(batches, interfaces.GeneratedSubmissionBatch{
			Request: request,
			Metadata: interfaces.GeneratedSubmissionBatchMetadata{
				Source: record.source,
			},
		})
	}
	return batches
}

// CompletionDeliveryPlan maps observed replay dispatches to recorded
// completion delivery ticks.
type CompletionDeliveryPlan struct {
	mu             sync.Mutex
	records        []completionDeliveryRecord
	plannedResults map[string]interfaces.WorkResult
}

type completionDeliveryRecord struct {
	dispatch      replayDispatch
	completion    *replayCompletion
	deliveryDelay int
	hasCompletion bool
	used          bool
}

var _ factory.CompletionDeliveryPlanner = (*CompletionDeliveryPlan)(nil)

// NewCompletionDeliveryPlan builds the replay completion delivery contract
// from an artifact.
func NewCompletionDeliveryPlan(artifact *interfaces.ReplayArtifact) (*CompletionDeliveryPlan, error) {
	eventLog, err := reduceReplayEvents(artifact)
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
		}
		records = append(records, completionDeliveryRecord{
			dispatch:      record.dispatch,
			completion:    record.completion,
			deliveryDelay: record.deliveryDelay,
			hasCompletion: record.hasCompletion,
		})
	}
	return &CompletionDeliveryPlan{
		records:        records,
		plannedResults: make(map[string]interfaces.WorkResult),
	}, nil
}

func (p *CompletionDeliveryPlan) DeliveryTickForDispatch(dispatch interfaces.WorkDispatch) (int, bool, error) {
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

func (p *CompletionDeliveryPlan) PlannedResultForDispatch(dispatch interfaces.WorkDispatch) (interfaces.WorkResult, bool, error) {
	if p == nil {
		return interfaces.WorkResult{}, false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	result, ok := p.plannedResults[dispatch.DispatchID]
	if !ok {
		return interfaces.WorkResult{}, false, nil
	}
	delete(p.plannedResults, dispatch.DispatchID)
	return cloneReplayPlannedResult(result), true, nil
}

func cloneReplayPlannedResult(result interfaces.WorkResult) interfaces.WorkResult {
	clone := result
	if result.SpawnedWork != nil {
		clone.SpawnedWork = append([]interfaces.TokenColor(nil), result.SpawnedWork...)
	}
	if result.RecordedOutputWork != nil {
		clone.RecordedOutputWork = cloneReplayFactoryWorkItems(result.RecordedOutputWork)
	}
	clone.ProviderFailure = cloneProviderFailureMetadata(result.ProviderFailure)
	clone.ProviderSession = cloneProviderSession(result.ProviderSession)
	clone.Diagnostics = cloneWorkDiagnostics(result.Diagnostics)
	return clone
}

func cloneReplayFactoryWorkItems(items []interfaces.FactoryWorkItem) []interfaces.FactoryWorkItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]interfaces.FactoryWorkItem, len(items))
	for i := range items {
		out[i] = items[i]
		if items[i].PreviousChainingTraceIDs != nil {
			out[i].PreviousChainingTraceIDs = append([]string(nil), items[i].PreviousChainingTraceIDs...)
		}
		if items[i].Tags != nil {
			out[i].Tags = cloneStringMap(items[i].Tags)
		}
	}
	return out
}

func cloneProviderFailureMetadata(metadata *interfaces.ProviderFailureMetadata) *interfaces.ProviderFailureMetadata {
	if metadata == nil {
		return nil
	}
	clone := *metadata
	return &clone
}

func (p *CompletionDeliveryPlan) expectedForObservedDispatchLocked(observed interfaces.WorkDispatch) (completionDeliveryRecord, bool) {
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

func recordedDispatchMatches(recorded replayDispatch, observed interfaces.WorkDispatch) bool {
	return dispatchMatches(recorded.dispatch, observed)
}

func dispatchMatches(recorded, observed interfaces.WorkDispatch) bool {
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

func tokenIDsMatch(recorded, observed []interfaces.Token) bool {
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

func hasResourceToken(tokens []interfaces.Token) bool {
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			return true
		}
	}
	return false
}

func nonResourceTokens(tokens []interfaces.Token) []interfaces.Token {
	out := make([]interfaces.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		out = append(out, token)
	}
	return out
}

func replayTokenIdentity(token interfaces.Token) string {
	if token.Color.DataType == interfaces.DataTypeResource {
		resourceID := resourceTokenName(token)
		return strings.Join([]string{"resource", resourceID}, "/")
	}
	if token.Color.WorkID != "" {
		return token.Color.WorkID
	}
	return token.ID
}

func resourceTokenName(token interfaces.Token) string {
	if token.Color.WorkTypeID != "" {
		return token.Color.WorkTypeID
	}
	if token.Color.Name != "" {
		return token.Color.Name
	}
	if before, _, ok := strings.Cut(token.ID, ":resource:"); ok && before != "" {
		return before
	}
	if before, _, ok := strings.Cut(token.PlaceID, ":"); ok && before != "" {
		return before
	}
	return token.ID
}

func dispatchSummary(dispatch interfaces.WorkDispatch) string {
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

func workTokenIDs(tokens []interfaces.Token) []string {
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.ID)
	}
	return ids
}
