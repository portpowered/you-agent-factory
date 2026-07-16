package subsystems

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/factory/packages/quorum"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/factory/workstationconfig"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

// TransitionerSubsystem routes tokens to the correct arc set based on outcome,
// constructs output and fanout tokens, handles resource release, and spawns
// child work tokens. It reconstructs token history from raw dispatch records
// on demand instead of reading cached history snapshots.
type TransitionerSubsystem struct {
	netDefinition *state.Net
	runtimeConfig interfaces.RuntimeWorkstationLookup
	logger        logging.Logger
	now           func() time.Time
	transformer   *token_transformer.Transformer
}

var _ Subsystem = (*TransitionerSubsystem)(nil)

type resolvedWorkResult struct {
	dispatchID                  string
	transitionID                string
	outcome                     workerexecution.WorkOutcome
	selectedClassificationLabel string
	output                      string
	spawnedWork                 []factorytoken.Color
	recordedOutputWork          []work.FactoryWorkItem
	err                         string
	feedback                    string
	failureMetadata             *workerexecution.WorkFailureMetadata
}

type generatedBatchWork struct {
	request  work.WorkRequest
	submits  []work.SubmitRequest
	metadata work.GeneratedSubmissionBatchMetadata
}

type mutationCalculationInput struct {
	transition    *petri.Transition
	workstation   *interfaces.FactoryWorkstationConfig
	arcs          []petri.Arc
	consumed      []factorytoken.Token
	result        resolvedWorkResult
	now           time.Time
	history       factorytoken.History
	inputColors   []factorytoken.Color
	transformer   *token_transformer.Transformer
	runtimeConfig interfaces.RuntimeWorkstationLookup
}

// TransitionerOption configures a TransitionerSubsystem.
type TransitionerOption func(*TransitionerSubsystem)

// WithTransitionerClock overrides the time source used for token lifecycle
// timestamps so tests can assert exact CreatedAt and EnteredAt values.
func WithTransitionerClock(now func() time.Time) TransitionerOption {
	return func(t *TransitionerSubsystem) {
		if now != nil {
			t.now = now
		}
	}
}

// WithTokenTransformer injects the token conversion component used by the transitioner.
func WithTokenTransformer(transformer *token_transformer.Transformer) TransitionerOption {
	return func(t *TransitionerSubsystem) {
		if transformer != nil {
			t.transformer = transformer
		}
	}
}

// WithTransitionerRuntimeConfig injects the runtime workstation config used to
// derive config-owned workstation metadata during result handling.
func WithTransitionerRuntimeConfig(runtimeConfig interfaces.RuntimeWorkstationLookup) TransitionerOption {
	return func(t *TransitionerSubsystem) {
		if runtimeConfig != nil {
			t.runtimeConfig = runtimeConfig
		}
	}
}

// NewTransitioner creates a TransitionerSubsystem that reads results and raw
// dispatch snapshots from the RuntimeStateSnapshot and produces routing mutations.
func NewTransitioner(n *state.Net, logger logging.Logger, opts ...TransitionerOption) *TransitionerSubsystem {
	tr := &TransitionerSubsystem{
		netDefinition: n,
		logger:        logging.EnsureLogger(logger),
		now:           time.Now,
	}
	for _, opt := range opts {
		opt(tr)
	}
	if tr.transformer == nil {
		tr.transformer = token_transformer.New(n.Places, n.WorkTypes)
	}
	return tr
}

// TickGroup returns Transitioner (12).
func (t *TransitionerSubsystem) TickGroup() TickGroup {
	return Transitioner
}

// TODO: this thing needs more tests.
// Execute reads results and raw dispatch snapshots from the RuntimeStateSnapshot
// and produces marking mutations for token routing.
func (t *TransitionerSubsystem) Execute(_ context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	if len(snapshot.Results) == 0 {
		return nil, nil
	}

	results := snapshot.Results
	t.logger.Debug("transitioner: processing results", "count", len(results))

	var mutations []interfaces.MarkingMutation
	var generatedBatches []work.GeneratedSubmissionBatch
	var completedDispatches []interfaces.CompletedDispatch
	for i := range results {
		muts, completedDispatch, batchRecords, err := t.mapToCorrespondingTokenMutations(snapshot, &results[i])
		if err != nil {
			t.logger.Error("transitioner: error processing result", "error", err, "transition", results[i].TransitionID)
			return nil, fmt.Errorf("processing result for transition %s: %w", results[i].TransitionID, err)
		}
		mutations = append(mutations, muts...)
		generatedBatches = append(generatedBatches, batchRecords...)
		completedDispatches = append(completedDispatches, completedDispatch)
	}

	if len(mutations) == 0 && len(completedDispatches) == 0 {
		return nil, nil
	}

	return &interfaces.TickResult{
		Mutations:           mutations,
		GeneratedBatches:    generatedBatches,
		CompletedDispatches: completedDispatches,
	}, nil
}

// mapToCorrespondingTokenMutations handles a single WorkResult: routes tokens via the appropriate
// arc set and creates new tokens with embedded history.
// TODO: we should break out the logic here to be referentially transparent and testable independent of the subsystem. Right now its too reliant on internal state.
// Break out dependency on ID generation as well as the logger/mocker.
func (t *TransitionerSubsystem) mapToCorrespondingTokenMutations(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], result *workerexecution.WorkResult) ([]interfaces.MarkingMutation, interfaces.CompletedDispatch, []work.GeneratedSubmissionBatch, error) {
	currentTransition, ok := t.netDefinition.Transitions[result.TransitionID]
	if !ok {
		t.logger.Error("transitioner: unknown transition in result", "transitionID", result.TransitionID)
		return nil, interfaces.CompletedDispatch{}, nil, fmt.Errorf("unknown transition %s", result.TransitionID)
	}

	resolved := resolveWorkResult(currentTransition, result, t.runtimeConfig)
	consumedTokens := consumedTokensForResult(snapshot, result)
	history := buildHistory(consumedTokens, result, candidateWorkID(t.netDefinition, result.TransitionID, consumedTokens))
	now := t.now()
	inputColors := tokenColorsFromTokens(consumedTokens)
	//TODO: the intermittent failure arc should be denoted as a preconstructed output, teh calculate arcs function should be a mapping of arcs for a current workstation/transition, and one such mapping would be the intermitten failure arc.

	if shouldRequeueIntermittentFailureResult(resolved) {
		mutations := t.buildIntermittentFailureRequeueMutations(consumedTokens, history, resolved, now)
		mutations = append(mutations, t.releaseResourceTokensOnFailureMutations(resolved.outcome, result.TransitionID, consumedTokens, nil, now)...)
		return mutations, t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now), nil, nil
	}

	if resolved.outcome == workerexecution.OutcomeAccepted {
		generatedBatch, detectedBatch, batchErr := t.workerEmittedBatchWork(resolved, inputColors)
		if batchErr != nil {
			resolved.outcome = workerexecution.OutcomeFailed
			resolved.err = batchErr.Error()
		} else if detectedBatch {
			mutations := t.releaseResourceTokens(consumedTokens, map[string]int{}, result.TransitionID, now)
			completed := t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now)
			batch := work.GeneratedSubmissionBatch{
				Request:     generatedBatch.request,
				Metadata:    generatedBatch.metadata,
				Submissions: generatedBatch.submits,
			}
			return mutations, completed, []work.GeneratedSubmissionBatch{batch}, nil
		}
	}

	arcs, resolved, err := t.calculateArcsForResolvedResult(currentTransition, resolved)
	if err != nil {
		return nil, interfaces.CompletedDispatch{}, nil, err
	}
	t.logArcSelection(result.TransitionID, resolved.outcome)
	if len(arcs) == 0 {
		return nil, interfaces.CompletedDispatch{}, nil, fmt.Errorf("transition %s has no arcs for outcome %s", result.TransitionID, resolved.outcome)
	}

	var workstationDef *interfaces.FactoryWorkstationConfig
	if workstation, ok := workstationconfig.Workstation(currentTransition, t.runtimeConfig); ok {
		workstationDef = workstation
	}

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:    currentTransition,
		workstation:   workstationDef,
		arcs:          arcs,
		consumed:      consumedTokens,
		result:        resolved,
		now:           now,
		history:       history,
		inputColors:   inputColors,
		transformer:   t.transformer,
		runtimeConfig: t.runtimeConfig,
	})
	if err != nil {
		return nil, interfaces.CompletedDispatch{}, nil, err
	}
	if reconcileMutations := executorReviewReconcileMutations(
		&snapshot.Marking,
		currentTransition.Name,
		resolved.outcome,
		consumedTokens,
		arcs,
		now,
	); len(reconcileMutations) > 0 {
		mutations = append(reconcileMutations, mutations...)
	}
	mutations = append(mutations, t.releaseResourceTokensOnFailureMutations(resolved.outcome, result.TransitionID, consumedTokens, arcs, now)...)
	spawnedMutations, err := t.getSpawnedWorkMutations(workstationDef, inputColors, resolved, now)
	if err != nil {
		return nil, interfaces.CompletedDispatch{}, nil, err
	}
	mutations = append(mutations, spawnedMutations...)
	mutations = append(mutations, t.createFanoutGuardToken(inputColors, resolved, now)...)

	t.logger.Info("releasing tokens", "transition", result.TransitionID, "outcome", resolved.outcome, "mutation_count", len(mutations))
	return mutations, t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now), nil, nil
}

func (t *TransitionerSubsystem) buildCompletedDispatch(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	result *workerexecution.WorkResult,
	resolved resolvedWorkResult,
	consumedTokens []factorytoken.Token,
	mutations []interfaces.MarkingMutation,
	endTime time.Time,
) interfaces.CompletedDispatch {
	dispatchEntry := completedDispatchEntry(snapshot, result.DispatchID)
	failureMetadata := workerexecution.CloneWorkFailureMetadata(result.FailureMetadata)
	completed := interfaces.CompletedDispatch{
		DispatchID:                  result.DispatchID,
		TransitionID:                result.TransitionID,
		Outcome:                     resolved.outcome,
		SelectedClassificationLabel: resolved.selectedClassificationLabel,
		Reason:                      completedDispatchReason(resolved),
		FailureMetadata:             failureMetadata,
		ProviderSession:             workerexecution.CloneProviderSessionMetadata(result.ProviderSession),
		EndTime:                     endTime,
		ConsumedTokens:              factorytoken.CloneSlice(consumedTokens),
		OutputMutations: mutationRecordsForDispatch(
			result.DispatchID,
			result.TransitionID,
			resolved.outcome,
			mutations,
		),
	}
	if dispatchEntry == nil {
		return completed
	}

	completed.WorkstationName = dispatchEntry.WorkstationName
	completed.StartTime = dispatchEntry.StartTime
	completed.Duration = completed.EndTime.Sub(dispatchEntry.StartTime)
	return completed
}

func completedDispatchReason(result resolvedWorkResult) string {
	switch result.outcome {
	case workerexecution.OutcomeFailed:
		return result.err
	case workerexecution.OutcomeContinue:
		return result.feedback
	case workerexecution.OutcomeRejected:
		return result.feedback
	default:
		return ""
	}
}

func (t *TransitionerSubsystem) calculateArcsForResolvedResult(currentTransition *petri.Transition, resolved resolvedWorkResult) ([]petri.Arc, resolvedWorkResult, error) {
	workstation, ok := workstationconfig.Workstation(currentTransition, t.runtimeConfig)
	if ok && workstation != nil && goal.UsesGoalRoutingDecisionEnvelope(workstation) {
		if resolved.outcome == workerexecution.OutcomeAccepted {
			return matchClassificationLabelArcs(currentTransition, resolved.selectedClassificationLabel, resolved, "decision %q did not match any authored routing route")
		}
		arcs, err := calculateArcs(currentTransition, resolved.outcome)
		return arcs, resolved, err
	}
	if !ok || workstation == nil || workstation.Type != interfaces.WorkstationTypeClassify || resolved.outcome != workerexecution.OutcomeAccepted {
		arcs, err := calculateArcs(currentTransition, resolved.outcome)
		return arcs, resolved, err
	}

	return matchClassificationLabelArcs(currentTransition, resolved.output, resolved, "classifier label %q did not match any authored classification route")
}

func matchClassificationLabelArcs(currentTransition *petri.Transition, label string, resolved resolvedWorkResult, unknownLabelFmt string) ([]petri.Arc, resolvedWorkResult, error) {
	matchedArcs := make([]petri.Arc, 0, len(currentTransition.OutputArcs))
	matchedRoute := false
	for _, arc := range currentTransition.OutputArcs {
		if arc.ClassificationLabel == "" {
			matchedArcs = append(matchedArcs, arc)
			continue
		}
		if arc.ClassificationLabel == label {
			matchedRoute = true
			matchedArcs = append(matchedArcs, arc)
		}
	}
	if matchedRoute {
		resolved.selectedClassificationLabel = label
		return matchedArcs, resolved, nil
	}

	resolved.outcome = workerexecution.OutcomeFailed
	if label == "" {
		resolved.err = "decision envelope: routing label is required"
	} else {
		resolved.err = fmt.Sprintf(unknownLabelFmt, label)
	}
	resolved.selectedClassificationLabel = ""
	arcs, err := calculateArcs(currentTransition, resolved.outcome)
	return arcs, resolved, err
}

func completedDispatchEntry(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], dispatchID string) *interfaces.DispatchEntry {
	if snapshot == nil || snapshot.Dispatches == nil {
		return nil
	}
	return snapshot.Dispatches[dispatchID]
}

func mutationRecordsForDispatch(
	dispatchID string,
	transitionID string,
	outcome workerexecution.WorkOutcome,
	mutations []interfaces.MarkingMutation,
) []interfaces.TokenMutationRecord {
	if len(mutations) == 0 {
		return nil
	}

	records := make([]interfaces.TokenMutationRecord, 0, len(mutations))
	for _, mutation := range mutations {
		record := interfaces.TokenMutationRecord{
			DispatchID:   dispatchID,
			TransitionID: transitionID,
			Outcome:      outcome,
			Type:         mutation.Type,
			TokenID:      mutation.TokenID,
			FromPlace:    mutation.FromPlace,
			ToPlace:      mutation.ToPlace,
			Reason:       mutation.Reason,
		}
		if mutation.NewToken != nil {
			tokenCopy := factorytoken.Clone(*mutation.NewToken)
			record.Token = &tokenCopy
			if record.TokenID == "" {
				record.TokenID = mutation.NewToken.ID
			}
			if record.ToPlace == "" {
				record.ToPlace = mutation.NewToken.PlaceID
			}
		}
		records = append(records, record)
	}
	return records
}

func cloneFactoryWorkItems(items []work.FactoryWorkItem) []work.FactoryWorkItem {
	if len(items) == 0 {
		return nil
	}

	clone := make([]work.FactoryWorkItem, len(items))
	for i := range items {
		clone[i] = items[i]
		if items[i].PreviousChainingTraceIDs != nil {
			clone[i].PreviousChainingTraceIDs = append([]string(nil), items[i].PreviousChainingTraceIDs...)
		}
		if items[i].Content != nil {
			clone[i].Content = append([]work.WorkContentPart(nil), items[i].Content...)
		}
		if items[i].Tags != nil {
			clone[i].Tags = cloneTags(items[i].Tags)
		}
	}
	return clone
}

func cloneTags(tags map[string]string) map[string]string {
	if tags == nil {
		return nil
	}

	clone := make(map[string]string, len(tags))
	for key, value := range tags {
		clone[key] = value
	}
	return clone
}

func cloneRuntimeTokenColors(colors []factorytoken.Color) []factorytoken.Color {
	if len(colors) == 0 {
		return nil
	}

	cloned := make([]factorytoken.Color, len(colors))
	for i := range colors {
		cloned[i] = colors[i]
		cloned[i].PreviousChainingTraceIDs = append([]string(nil), colors[i].PreviousChainingTraceIDs...)
		cloned[i].Tags = factory.CloneRuntimeTags(colors[i].Tags)
		cloned[i].Relations = factory.CloneRuntimeRelations(colors[i].Relations)
		cloned[i].Content = work.CloneWorkContentParts(colors[i].Content)
		cloned[i].Payload = factory.CloneRuntimePayload(colors[i].Payload)
		cloned[i].InvocationArguments = work.CloneInvocationArguments(colors[i].InvocationArguments)
	}
	return cloned
}

func resolveWorkResult(transition *petri.Transition, result *workerexecution.WorkResult, runtimeConfig interfaces.RuntimeWorkstationLookup) resolvedWorkResult {
	resolved := resolvedWorkResult{
		dispatchID:                  result.DispatchID,
		transitionID:                result.TransitionID,
		outcome:                     result.Outcome,
		output:                      result.Output,
		selectedClassificationLabel: result.SelectedClassificationLabel,
		spawnedWork:                 cloneRuntimeTokenColors(result.SpawnedWork),
		recordedOutputWork:          cloneFactoryWorkItems(result.RecordedOutputWork),
		err:                         result.Error,
		feedback:                    result.Feedback,
		failureMetadata:             result.FailureMetadata,
	}
	if workstation, ok := workstationconfig.Workstation(transition, runtimeConfig); ok && workstation != nil && len(workstation.StopWords) > 0 {
		resolved.outcome = evaluateStopWords(workstation.StopWords, result.Output)
	}
	return resolved
}

func shouldRequeueIntermittentFailureResult(result resolvedWorkResult) bool {
	if result.outcome != workerexecution.OutcomeFailed || result.failureMetadata == nil {
		return false
	}
	return workerprovider.WorkFailureDecisionFromMetadata(result.failureMetadata).Retryable
}

func (t *TransitionerSubsystem) workerEmittedBatchWork(result resolvedWorkResult, inputColors []factorytoken.Color) (generatedBatchWork, bool, error) {
	output := strings.TrimSpace(result.output)
	if output == "" || !strings.HasPrefix(output, "{") {
		return generatedBatchWork{}, false, nil
	}

	rawRequest, ok, err := workerEmittedBatchRequestPayload(output)
	if err != nil {
		return generatedBatchWork{}, true, err
	}
	if !ok {
		return generatedBatchWork{}, false, nil
	}

	var request work.WorkRequest
	if err := json.Unmarshal(rawRequest, &request); err != nil {
		if strings.Contains(string(rawRequest), string(work.WorkRequestTypeFactoryRequestBatch)) {
			return generatedBatchWork{}, true, fmt.Errorf("worker-emitted work request batch: %w", err)
		}
		return generatedBatchWork{}, false, nil
	}
	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
		return generatedBatchWork{}, false, nil
	}

	envelope, err := decodeWorkerEmittedBatchEnvelope(output)
	if err != nil {
		return generatedBatchWork{}, true, err
	}
	request = envelope.Request
	if request.RequestID == "" {
		request.RequestID = deterministicWorkerBatchRequestID(result, output)
	}
	enrichWorkerEmittedBatchRequest(&request, inputColors, result)

	metadata := work.GeneratedSubmissionBatchMetadata{Source: "worker-output:" + result.dispatchID}
	if envelope.Metadata != nil {
		metadata = *envelope.Metadata
		if metadata.Source == "" {
			metadata.Source = "worker-output:" + result.dispatchID
		}
	}
	batch := work.GeneratedSubmissionBatch{
		Request:     request,
		Metadata:    metadata,
		Submissions: envelope.Submissions,
	}
	normalized, err := requests.NormalizeGeneratedSubmissionBatch(batch, work.WorkRequestNormalizeOptions{
		ValidWorkTypes: t.validWorkTypes(),
	})
	if err != nil {
		return generatedBatchWork{}, true, fmt.Errorf("worker-emitted work request batch: %w", err)
	}
	return generatedBatchWork{request: request, submits: normalized, metadata: metadata}, true, nil
}

type workerEmittedBatchEnvelope struct {
	Request     work.WorkRequest                       `json:"request"`
	Submissions []work.SubmitRequest                   `json:"submissions"`
	Metadata    *work.GeneratedSubmissionBatchMetadata `json:"metadata"`
}

func workerEmittedBatchRequestPayload(output string) (json.RawMessage, bool, error) {
	var rawEnvelope struct {
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal([]byte(output), &rawEnvelope); err != nil {
		if strings.Contains(output, `"request"`) && strings.Contains(output, string(work.WorkRequestTypeFactoryRequestBatch)) {
			return nil, false, fmt.Errorf("worker-emitted work request batch: %w", err)
		}
		return nil, false, nil
	}
	if len(rawEnvelope.Request) == 0 || string(rawEnvelope.Request) == "null" {
		return nil, false, nil
	}
	return rawEnvelope.Request, true, nil
}

func decodeWorkerEmittedBatchEnvelope(output string) (workerEmittedBatchEnvelope, error) {
	var envelope workerEmittedBatchEnvelope
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		return workerEmittedBatchEnvelope{}, fmt.Errorf("worker-emitted work request batch: %w", err)
	}
	return envelope, nil
}

func deterministicWorkerBatchRequestID(result resolvedWorkResult, output string) string {
	sum := sha256.Sum256([]byte(result.dispatchID + "\x00" + result.transitionID + "\x00" + output))
	return "generated-request-" + hex.EncodeToString(sum[:8])
}

func enrichWorkerEmittedBatchRequest(request *work.WorkRequest, inputColors []factorytoken.Color, result resolvedWorkResult) {
	source := firstNonResourceInput(inputColors)
	previousChainingTraceIDs := factorytoken.PreviousChainingTraceIDsFromColors(inputColors)
	chainingTraceDepth := factorytoken.ChainingTraceDepthFromColors(inputColors)
	for i := range request.Works {
		if request.Works[i].RequestID == "" {
			request.Works[i].RequestID = request.RequestID
		}
		if request.Works[i].ChainingTraceDepth == 0 {
			request.Works[i].ChainingTraceDepth = chainingTraceDepth
		}
		request.Works[i].PreviousChainingTraceIDs = previousChainingTraceIDs
		if source == nil {
			continue
		}
		if request.Works[i].TraceID == "" {
			request.Works[i].TraceID = source.TraceID
		}
		if request.Works[i].CurrentChainingTraceID == "" {
			request.Works[i].CurrentChainingTraceID = source.TraceID
			if source.CurrentChainingTraceID != "" {
				request.Works[i].CurrentChainingTraceID = source.CurrentChainingTraceID
			}
		}
		request.Works[i].Tags = mergedWorkerBatchTags(source.Tags, request.Works[i].Tags, source, result)
	}
}

func mergedWorkerBatchTags(sourceTags map[string]string, itemTags map[string]string, source *factorytoken.Color, result resolvedWorkResult) map[string]string {
	tags := make(map[string]string, len(sourceTags)+len(itemTags)+4)
	maps.Copy(tags, sourceTags)
	maps.Copy(tags, itemTags)
	if source.WorkID != "" {
		tags["_parent_work_id"] = source.WorkID
	}
	if source.RequestID != "" {
		tags["_parent_request_id"] = source.RequestID
	}
	if result.dispatchID != "" {
		tags["_source_dispatch_id"] = result.dispatchID
	}
	if result.transitionID != "" {
		tags["_source_transition_id"] = result.transitionID
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func (t *TransitionerSubsystem) validWorkTypes() map[string]bool {
	valid := make(map[string]bool, len(t.netDefinition.WorkTypes))
	for workTypeID := range t.netDefinition.WorkTypes {
		valid[workTypeID] = true
	}
	return valid
}

func (t *TransitionerSubsystem) logArcSelection(transitionID string, outcome workerexecution.WorkOutcome) {
	switch outcome {
	case workerexecution.OutcomeAccepted:
		t.logger.Info("transitioner: result accepted", "transitionID", transitionID)
	case workerexecution.OutcomeContinue:
		t.logger.Info("transitioner: result continued", "transitionID", transitionID)
	case workerexecution.OutcomeRejected:
		t.logger.Info("transitioner: result rejected", "transitionID", transitionID)
	case workerexecution.OutcomeFailed:
		t.logger.Info("transitioner: result failed", "transitionID", transitionID)
	}
}

func (t *TransitionerSubsystem) releaseResourceTokensOnFailureMutations(outcome workerexecution.WorkOutcome, transitionID string, consumedTokens []factorytoken.Token, arcs []petri.Arc, now time.Time) []interfaces.MarkingMutation {
	mutations := []interfaces.MarkingMutation{}
	if outcome == workerexecution.OutcomeFailed || outcome == workerexecution.OutcomeContinue || outcome == workerexecution.OutcomeRejected {
		covered := make(map[string]int, len(arcs))
		for _, a := range arcs {
			covered[a.PlaceID] += arcCoverageCount(a)
		}
		mutations = append(mutations, t.releaseResourceTokens(consumedTokens, covered, transitionID, now)...)
	}
	return mutations
}

func arcCoverageCount(arc petri.Arc) int {
	if arc.Cardinality.Mode == petri.CardinalityN && arc.Cardinality.Count > 0 {
		return arc.Cardinality.Count
	}
	return 1
}

func (t *TransitionerSubsystem) getSpawnedWorkMutations(
	workstation *interfaces.FactoryWorkstationConfig,
	inputColors []factorytoken.Color,
	result resolvedWorkResult,
	now time.Time,
) ([]interfaces.MarkingMutation, error) {
	mutations := []interfaces.MarkingMutation{}
	workPropagationMode := workstationconfig.WorkPropagationMode(workstation)
	workstationName := ""
	if workstation != nil {
		workstationName = workstation.Name
	}
	for i := range result.spawnedWork {
		spawnColor := factorytoken.CloneColor(result.spawnedWork[i])
		if workPropagationMode == interfaces.WorkPropagationModePreserveInput {
			if err := token_transformer.ApplyPreservedInputToColor(&spawnColor, inputColors, spawnColor.WorkTypeID, workstationName); err != nil {
				return nil, err
			}
		}
		spawnMuts := t.createSpawnedTokens(&spawnColor, result.transitionID, now)
		mutations = append(mutations, spawnMuts...)
	}
	return mutations, nil
}

func calculateArcs(currentTransition *petri.Transition, outcome workerexecution.WorkOutcome) ([]petri.Arc, error) {
	switch outcome {
	case workerexecution.OutcomeAccepted:
		return currentTransition.OutputArcs, nil
	case workerexecution.OutcomeContinue:
		if len(currentTransition.ContinueArcs) > 0 {
			return currentTransition.ContinueArcs, nil
		}
		return currentTransition.RejectionArcs, nil
	case workerexecution.OutcomeRejected:
		return currentTransition.RejectionArcs, nil
	case workerexecution.OutcomeFailed:
		return currentTransition.FailureArcs, nil
	default:
		return nil, fmt.Errorf("unknown outcome %s", outcome)
	}
}

func (t *TransitionerSubsystem) createFanoutGuardToken(inputColors []factorytoken.Color, result resolvedWorkResult, now time.Time) []interfaces.MarkingMutation {
	mutations := []interfaces.MarkingMutation{}
	if len(result.spawnedWork) > 0 || t.hasFanoutGroup(result.transitionID) {
		if countPlaceID, ok := t.netDefinition.FanoutGroups[result.transitionID]; ok {
			parentWorkID := ""
			if first := firstNonResourceInput(inputColors); first != nil {
				parentWorkID = first.WorkID
			}

			countToken := t.transformer.FanoutCountToken(countPlaceID, result.transitionID, parentWorkID, len(result.spawnedWork), now)
			mutations = append(mutations, interfaces.MarkingMutation{
				Type:     interfaces.MutationCreate,
				ToPlace:  countPlaceID,
				NewToken: countToken,
				Reason:   fmt.Sprintf("fanout count token for transition %s (expected %d children)", result.transitionID, len(result.spawnedWork)),
			})
		}
	}
	return mutations
}

func calculateMutations(in mutationCalculationInput) ([]interfaces.MarkingMutation, error) {
	mutations := make([]interfaces.MarkingMutation, 0)
	workOutputIndex := 0
	workPropagationMode := workstationconfig.WorkPropagationMode(in.workstation)
	workstationName := ""
	if in.workstation != nil {
		workstationName = in.workstation.Name
	}
	for arcIdx, arc := range in.arcs {
		baseInput := token_transformer.OutputTokenInput{
			ArcIndex:            arcIdx,
			Arcs:                in.arcs,
			ConsumedTokens:      in.consumed,
			InputColors:         in.inputColors,
			Output:              in.result.output,
			WorkPropagationMode: workPropagationMode,
			WorkstationName:     workstationName,
			WorkstationType:     workstationType(in.workstation),
			Outcome:             in.result.outcome,
			TransitionID:        in.result.transitionID,
			Error:               in.result.err,
			Feedback:            in.result.feedback,
			Now:                 in.now,
			History:             in.history,
		}
		repeatCount := mutationRepeatCountForArc(arc, in.consumed)
		for resourceTokenIndex := 0; resourceTokenIndex < repeatCount; resourceTokenIndex++ {
			tokenInput := baseInput
			tokenInput.ResourceTokenIndex = resourceTokenIndex
			newToken, err := in.transformer.OutputToken(tokenInput)
			if err != nil {
				return nil, err
			}
			if err := applyPackagedTTSInvocationMetadata(newToken, in.workstation, in.result.output, in.inputColors, in.runtimeConfig); err != nil {
				return nil, err
			}
			if err := applyPackagedGoalInvocationSummary(newToken, in.workstation, in.result.output, in.runtimeConfig); err != nil {
				return nil, err
			}
			if err := applyPackagedSubagentInvocationResponse(newToken, in.workstation, in.result.output, in.runtimeConfig); err != nil {
				return nil, err
			}
			applyPackagedQuorumWorkRelations(newToken, in.workstation, in.inputColors, in.runtimeConfig)
			if newToken.Color.DataType != factorytoken.DataTypeResource {
				if workOutputIndex < len(in.result.recordedOutputWork) {
					applyRecordedOutputWorkIdentity(newToken, in.result.recordedOutputWork[workOutputIndex])
				}
				workOutputIndex++
			}

			mutations = append(mutations, interfaces.MarkingMutation{
				Type:     interfaces.MutationCreate,
				ToPlace:  arc.PlaceID,
				NewToken: newToken,
				Reason:   fmt.Sprintf("transitioner: %s from transition %s", in.result.outcome, in.transition.ID),
			})
		}
	}
	return mutations, nil
}

// applyPackagedQuorumWorkRelations limits quorum's fixed public relation
// topology to the shipped package. Customer factories own their relations,
// even if they independently choose the same workstation or Work type names.
func applyPackagedQuorumWorkRelations(
	output *factorytoken.Token,
	workstation *interfaces.FactoryWorkstationConfig,
	inputs []factorytoken.Color,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) {
	factoryConfigLookup, ok := runtimeConfig.(interfaces.RuntimeFactoryConfigLookup)
	if !ok || !quorum.IsPackagedFactory(factoryConfigLookup.FactoryConfig()) {
		return
	}
	quorum.ApplyWorkRelations(output, workstation, inputs)
}

func workstationType(workstation *interfaces.FactoryWorkstationConfig) string {
	if workstation == nil {
		return ""
	}
	return strings.TrimSpace(workstation.Type)
}

func mutationRepeatCountForArc(
	arc petri.Arc,
	consumedTokens []factorytoken.Token,
) int {
	if arc.Cardinality.Mode != petri.CardinalityN || arc.Cardinality.Count <= 0 {
		return 1
	}
	repeatCount := arc.Cardinality.Count
	available := consumedResourceTokenCountForPlace(consumedTokens, arc.PlaceID)
	if available > 0 && repeatCount > available {
		return available
	}
	return repeatCount
}

func consumedResourceTokenCountForPlace(consumedTokens []factorytoken.Token, placeID string) int {
	count := 0
	for i := range consumedTokens {
		if consumedTokens[i].Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		if consumedTokens[i].PlaceID == placeID {
			count++
		}
	}
	return count
}

func applyRecordedOutputWorkIdentity(token *factorytoken.Token, recorded work.FactoryWorkItem) {
	if token == nil {
		return
	}
	if recorded.WorkTypeID != "" && token.Color.WorkTypeID != "" && recorded.WorkTypeID != token.Color.WorkTypeID {
		return
	}
	if recorded.ID != "" {
		token.ID = recorded.ID
		token.Color.WorkID = recorded.ID
	}
	if recorded.WorkTypeID != "" {
		token.Color.WorkTypeID = recorded.WorkTypeID
	}
	if recorded.DisplayName != "" {
		token.Color.Name = recorded.DisplayName
	}
	if recorded.CurrentChainingTraceID != "" {
		token.Color.CurrentChainingTraceID = recorded.CurrentChainingTraceID
	}
	if len(recorded.PreviousChainingTraceIDs) > 0 {
		token.Color.PreviousChainingTraceIDs = append([]string(nil), recorded.PreviousChainingTraceIDs...)
	}
	if recorded.ChainingTraceDepth > 0 {
		token.Color.ChainingTraceDepth = recorded.ChainingTraceDepth
	}
	if recorded.TraceID != "" {
		token.Color.TraceID = recorded.TraceID
	}
	if recorded.ParentID != "" {
		token.Color.ParentID = recorded.ParentID
	}
	if len(recorded.Tags) > 0 {
		token.Color.Tags = cloneTags(recorded.Tags)
	}
	if len(recorded.Content) > 0 {
		token.Color.Content = work.CloneWorkContentParts(recorded.Content)
	}
}

func (t *TransitionerSubsystem) buildIntermittentFailureRequeueMutations(
	consumedTokens []factorytoken.Token,
	history factorytoken.History,
	result resolvedWorkResult,
	now time.Time,
) []interfaces.MarkingMutation {
	mutations := make([]interfaces.MarkingMutation, 0, len(consumedTokens))
	for i := range consumedTokens {
		consumed := consumedTokens[i]
		if consumed.Color.DataType == factorytoken.DataTypeResource {
			continue
		}

		requeued := factorytoken.Clone(consumed)
		requeued.PlaceID = consumed.PlaceID
		requeued.EnteredAt = now
		requeued.History = cloneHistoryForIntermittentFailureRequeue(history, result, now)

		mutations = append(mutations, interfaces.MarkingMutation{
			Type:     interfaces.MutationCreate,
			ToPlace:  consumed.PlaceID,
			NewToken: &requeued,
			Reason:   fmt.Sprintf("transitioner: requeue intermittent failure from transition %s", result.transitionID),
		})
	}
	return mutations
}

func cloneHistoryForIntermittentFailureRequeue(
	history factorytoken.History,
	result resolvedWorkResult,
	now time.Time,
) factorytoken.History {
	cloned := factorytoken.CloneHistory(history)
	cloned.LastError = result.err
	cloned.FailureLog = append(cloned.FailureLog, factorytoken.Failure{
		TransitionID: result.transitionID,
		Timestamp:    now,
		Error:        result.err,
		Attempt:      history.TotalVisits[result.transitionID],
	})
	return cloned
}

// hasFanoutGroup checks if a transition has a fanout group configured.
func (t *TransitionerSubsystem) hasFanoutGroup(transitionID string) bool {
	if t.netDefinition.FanoutGroups == nil {
		return false
	}
	_, ok := t.netDefinition.FanoutGroups[transitionID]
	return ok
}

// releaseResourceTokens returns consumed resource tokens back to their original resource places.
func (t *TransitionerSubsystem) releaseResourceTokens(consumedTokens []factorytoken.Token, alreadyCovered map[string]int, transitionID string, now time.Time) []interfaces.MarkingMutation {
	var mutations []interfaces.MarkingMutation
	for i := range consumedTokens {
		consumed := consumedTokens[i]
		if consumed.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		if alreadyCovered[consumed.PlaceID] > 0 {
			alreadyCovered[consumed.PlaceID]--
			continue
		}
		resourceToken := t.transformer.ReleasedResourceToken(consumed, consumed.PlaceID, now)
		mutations = append(mutations, interfaces.MarkingMutation{
			Type:     interfaces.MutationCreate,
			ToPlace:  consumed.PlaceID,
			NewToken: resourceToken,
			Reason:   fmt.Sprintf("release resource %s for transition %s", consumed.PlaceID, transitionID),
		})
	}
	return mutations
}

// createSpawnedTokens creates new tokens in INITIAL places for spawned work.
func (t *TransitionerSubsystem) createSpawnedTokens(spawnColor *factorytoken.Color, parentTransitionID string, now time.Time) []interfaces.MarkingMutation {
	newToken, err := t.transformer.SpawnedToken(*spawnColor, parentTransitionID, now)
	if err != nil {
		return nil
	}

	return []interfaces.MarkingMutation{{
		Type:     interfaces.MutationCreate,
		ToPlace:  newToken.PlaceID,
		NewToken: newToken,
		Reason:   fmt.Sprintf("spawned by transition %s", parentTransitionID),
	}}
}

func tokenColorsFromTokens(tokens []factorytoken.Token) []factorytoken.Color {
	colors := make([]factorytoken.Color, len(tokens))
	for i, token := range tokens {
		colors[i] = token.Color
	}
	return colors
}
