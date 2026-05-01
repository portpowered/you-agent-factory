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

	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/factory/token_transformer"
	"github.com/portpowered/agent-factory/pkg/factory/workstationconfig"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/workers"
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
	dispatchID         string
	transitionID       string
	outcome            interfaces.WorkOutcome
	output             string
	spawnedWork        []interfaces.TokenColor
	recordedOutputWork []interfaces.FactoryWorkItem
	err                string
	feedback           string
	providerFailure    *interfaces.ProviderFailureMetadata
}

type generatedBatchWork struct {
	request  interfaces.WorkRequest
	submits  []interfaces.SubmitRequest
	metadata interfaces.GeneratedSubmissionBatchMetadata
}

type mutationCalculationInput struct {
	transition  *petri.Transition
	arcs        []petri.Arc
	consumed    []interfaces.Token
	result      resolvedWorkResult
	now         time.Time
	history     interfaces.TokenHistory
	inputColors []interfaces.TokenColor
	transformer *token_transformer.Transformer
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
	var generatedBatches []interfaces.GeneratedSubmissionBatch
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
func (t *TransitionerSubsystem) mapToCorrespondingTokenMutations(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], result *interfaces.WorkResult) ([]interfaces.MarkingMutation, interfaces.CompletedDispatch, []interfaces.GeneratedSubmissionBatch, error) {
	currentTransition, ok := t.netDefinition.Transitions[result.TransitionID]
	if !ok {
		t.logger.Error("transitioner: unknown transition in result", "transitionID", result.TransitionID)
		return nil, interfaces.CompletedDispatch{}, nil, fmt.Errorf("unknown transition %s", result.TransitionID)
	}

	resolved := resolveWorkResult(currentTransition, result, t.runtimeConfig)
	consumedTokens := consumedTokensForResult(snapshot, result)
	history := buildHistory(consumedTokens, result)
	now := t.now()
	inputColors := tokenColorsFromTokens(consumedTokens)
	//TODO: the intermittent failure arc should be denoted as a preconstructed output, teh calculate arcs function should be a mapping of arcs for a current workstation/transition, and one such mapping would be the intermitten failure arc.

	if shouldRequeueIntermittentFailureResult(resolved) {
		mutations := t.buildIntermittentFailureRequeueMutations(consumedTokens, history, resolved, now)
		mutations = append(mutations, t.releaseResourceTokensOnFailureMutations(resolved.outcome, result.TransitionID, consumedTokens, nil, now)...)
		return mutations, t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now), nil, nil
	}

	if resolved.outcome == interfaces.OutcomeAccepted {
		generatedBatch, detectedBatch, batchErr := t.workerEmittedBatchWork(resolved, inputColors)
		if batchErr != nil {
			resolved.outcome = interfaces.OutcomeFailed
			resolved.err = batchErr.Error()
		} else if detectedBatch {
			mutations := t.releaseResourceTokens(consumedTokens, map[string]bool{}, result.TransitionID, now)
			completed := t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now)
			batch := interfaces.GeneratedSubmissionBatch{
				Request:     generatedBatch.request,
				Metadata:    generatedBatch.metadata,
				Submissions: generatedBatch.submits,
			}
			return mutations, completed, []interfaces.GeneratedSubmissionBatch{batch}, nil
		}
	}

	arcs, err := calculateArcs(currentTransition, resolved.outcome)
	if err != nil {
		return nil, interfaces.CompletedDispatch{}, nil, err
	}
	t.logArcSelection(result.TransitionID, resolved.outcome)
	if len(arcs) == 0 {
		return nil, interfaces.CompletedDispatch{}, nil, fmt.Errorf("transition %s has no arcs for outcome %s", result.TransitionID, resolved.outcome)
	}

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  currentTransition,
		arcs:        arcs,
		consumed:    consumedTokens,
		result:      resolved,
		now:         now,
		history:     history,
		inputColors: inputColors,
		transformer: t.transformer,
	})
	if err != nil {
		return nil, interfaces.CompletedDispatch{}, nil, err
	}
	mutations = append(mutations, t.releaseResourceTokensOnFailureMutations(resolved.outcome, result.TransitionID, consumedTokens, arcs, now)...)
	mutations = append(mutations, t.getSpawnedWorkMutations(resolved, now)...)
	mutations = append(mutations, t.createFanoutGuardToken(inputColors, resolved, now)...)

	t.logger.Info("releasing tokens", "transition", result.TransitionID, "outcome", resolved.outcome, "mutation_count", len(mutations))
	return mutations, t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now), nil, nil
}

func (t *TransitionerSubsystem) buildCompletedDispatch(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	result *interfaces.WorkResult,
	resolved resolvedWorkResult,
	consumedTokens []interfaces.Token,
	mutations []interfaces.MarkingMutation,
	endTime time.Time,
) interfaces.CompletedDispatch {
	dispatchEntry := completedDispatchEntry(snapshot, result.DispatchID)
	completed := interfaces.CompletedDispatch{
		DispatchID:      result.DispatchID,
		TransitionID:    result.TransitionID,
		Outcome:         resolved.outcome,
		Reason:          completedDispatchReason(resolved),
		ProviderFailure: cloneProviderFailureMetadata(result.ProviderFailure),
		ProviderSession: cloneProviderSession(result.ProviderSession),
		EndTime:         endTime,
		ConsumedTokens:  cloneTokens(consumedTokens),
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
	case interfaces.OutcomeFailed:
		return result.err
	case interfaces.OutcomeContinue:
		return result.feedback
	case interfaces.OutcomeRejected:
		return result.feedback
	default:
		return ""
	}
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
	outcome interfaces.WorkOutcome,
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
			tokenCopy := cloneToken(*mutation.NewToken)
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

func cloneTokens(tokens []interfaces.Token) []interfaces.Token {
	if len(tokens) == 0 {
		return nil
	}

	clones := make([]interfaces.Token, len(tokens))
	for i := range tokens {
		clones[i] = cloneToken(tokens[i])
	}
	return clones
}

func cloneFactoryWorkItems(items []interfaces.FactoryWorkItem) []interfaces.FactoryWorkItem {
	if len(items) == 0 {
		return nil
	}

	clone := make([]interfaces.FactoryWorkItem, len(items))
	for i := range items {
		clone[i] = items[i]
		if items[i].PreviousChainingTraceIDs != nil {
			clone[i].PreviousChainingTraceIDs = append([]string(nil), items[i].PreviousChainingTraceIDs...)
		}
		if items[i].Tags != nil {
			clone[i].Tags = cloneTags(items[i].Tags)
		}
	}
	return clone
}

func cloneToken(token interfaces.Token) interfaces.Token {
	clone := token
	if token.Color.Tags != nil {
		clone.Color.Tags = cloneTags(token.Color.Tags)
	}
	if token.Color.Relations != nil {
		clone.Color.Relations = cloneRelations(token.Color.Relations)
	}
	if token.Color.Payload != nil {
		clone.Color.Payload = append([]byte(nil), token.Color.Payload...)
	}
	if token.History.TotalVisits != nil {
		clone.History.TotalVisits = cloneIntMap(token.History.TotalVisits)
	}
	if token.History.ConsecutiveFailures != nil {
		clone.History.ConsecutiveFailures = cloneIntMap(token.History.ConsecutiveFailures)
	}
	if token.History.PlaceVisits != nil {
		clone.History.PlaceVisits = cloneIntMap(token.History.PlaceVisits)
	}
	if token.History.FailureLog != nil {
		clone.History.FailureLog = append([]interfaces.FailureRecord(nil), token.History.FailureLog...)
	}
	return clone
}

func cloneIntMap(input map[string]int) map[string]int {
	clone := make(map[string]int, len(input))
	for key, value := range input {
		clone[key] = value
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

func cloneRelations(relations []interfaces.Relation) []interfaces.Relation {
	if relations == nil {
		return nil
	}

	clone := make([]interfaces.Relation, len(relations))
	copy(clone, relations)
	return clone
}

func cloneProviderSession(session *interfaces.ProviderSessionMetadata) *interfaces.ProviderSessionMetadata {
	if session == nil {
		return nil
	}

	clone := *session
	return &clone
}

func cloneProviderFailureMetadata(metadata *interfaces.ProviderFailureMetadata) *interfaces.ProviderFailureMetadata {
	if metadata == nil {
		return nil
	}

	clone := *metadata
	return &clone
}

func resolveWorkResult(transition *petri.Transition, result *interfaces.WorkResult, runtimeConfig interfaces.RuntimeWorkstationLookup) resolvedWorkResult {
	resolved := resolvedWorkResult{
		dispatchID:         result.DispatchID,
		transitionID:       result.TransitionID,
		outcome:            result.Outcome,
		output:             result.Output,
		spawnedWork:        result.SpawnedWork,
		recordedOutputWork: cloneFactoryWorkItems(result.RecordedOutputWork),
		err:                result.Error,
		feedback:           result.Feedback,
		providerFailure:    result.ProviderFailure,
	}
	if workstation, ok := workstationconfig.Workstation(transition, runtimeConfig); ok && workstation != nil && len(workstation.StopWords) > 0 {
		resolved.outcome = evaluateStopWords(workstation.StopWords, result.Output)
	}
	return resolved
}

func shouldRequeueIntermittentFailureResult(result resolvedWorkResult) bool {
	if result.outcome != interfaces.OutcomeFailed || result.providerFailure == nil {
		return false
	}
	return workers.ProviderFailureDecisionFromMetadata(result.providerFailure).Retryable
}

func (t *TransitionerSubsystem) workerEmittedBatchWork(result resolvedWorkResult, inputColors []interfaces.TokenColor) (generatedBatchWork, bool, error) {
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

	var request interfaces.WorkRequest
	if err := json.Unmarshal(rawRequest, &request); err != nil {
		if strings.Contains(string(rawRequest), string(interfaces.WorkRequestTypeFactoryRequestBatch)) {
			return generatedBatchWork{}, true, fmt.Errorf("worker-emitted work request batch: %w", err)
		}
		return generatedBatchWork{}, false, nil
	}
	if request.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
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

	metadata := interfaces.GeneratedSubmissionBatchMetadata{Source: "worker-output:" + result.dispatchID}
	if envelope.Metadata != nil {
		metadata = *envelope.Metadata
		if metadata.Source == "" {
			metadata.Source = "worker-output:" + result.dispatchID
		}
	}
	batch := interfaces.GeneratedSubmissionBatch{
		Request:     request,
		Metadata:    metadata,
		Submissions: envelope.Submissions,
	}
	normalized, err := factory.NormalizeGeneratedSubmissionBatch(batch, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: t.validWorkTypes(),
	})
	if err != nil {
		return generatedBatchWork{}, true, fmt.Errorf("worker-emitted work request batch: %w", err)
	}
	return generatedBatchWork{request: request, submits: normalized, metadata: metadata}, true, nil
}

type workerEmittedBatchEnvelope struct {
	Request     interfaces.WorkRequest                       `json:"request"`
	Submissions []interfaces.SubmitRequest                   `json:"submissions"`
	Metadata    *interfaces.GeneratedSubmissionBatchMetadata `json:"metadata"`
}

func workerEmittedBatchRequestPayload(output string) (json.RawMessage, bool, error) {
	var rawEnvelope struct {
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal([]byte(output), &rawEnvelope); err != nil {
		if strings.Contains(output, `"request"`) && strings.Contains(output, string(interfaces.WorkRequestTypeFactoryRequestBatch)) {
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

func enrichWorkerEmittedBatchRequest(request *interfaces.WorkRequest, inputColors []interfaces.TokenColor, result resolvedWorkResult) {
	source := firstNonResourceInput(inputColors)
	previousChainingTraceIDs := interfaces.PreviousChainingTraceIDsFromTokenColors(inputColors)
	for i := range request.Works {
		if request.Works[i].RequestID == "" {
			request.Works[i].RequestID = request.RequestID
		}
		request.Works[i].PreviousChainingTraceIDs = previousChainingTraceIDs
		if source == nil {
			continue
		}
		if request.Works[i].TraceID == "" {
			request.Works[i].TraceID = source.TraceID
		}
		if request.Works[i].CurrentChainingTraceID == "" {
			request.Works[i].CurrentChainingTraceID = request.Works[i].TraceID
		}
		request.Works[i].Tags = mergedWorkerBatchTags(source.Tags, request.Works[i].Tags, source, result)
	}
}

func mergedWorkerBatchTags(sourceTags map[string]string, itemTags map[string]string, source *interfaces.TokenColor, result resolvedWorkResult) map[string]string {
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

func (t *TransitionerSubsystem) logArcSelection(transitionID string, outcome interfaces.WorkOutcome) {
	switch outcome {
	case interfaces.OutcomeAccepted:
		t.logger.Info("transitioner: result accepted", "transitionID", transitionID)
	case interfaces.OutcomeContinue:
		t.logger.Info("transitioner: result continued", "transitionID", transitionID)
	case interfaces.OutcomeRejected:
		t.logger.Info("transitioner: result rejected", "transitionID", transitionID)
	case interfaces.OutcomeFailed:
		t.logger.Info("transitioner: result failed", "transitionID", transitionID)
	}
}

func (t *TransitionerSubsystem) releaseResourceTokensOnFailureMutations(outcome interfaces.WorkOutcome, transitionID string, consumedTokens []interfaces.Token, arcs []petri.Arc, now time.Time) []interfaces.MarkingMutation {
	mutations := []interfaces.MarkingMutation{}
	if outcome == interfaces.OutcomeFailed || outcome == interfaces.OutcomeContinue || outcome == interfaces.OutcomeRejected {
		covered := make(map[string]bool, len(arcs))
		for _, a := range arcs {
			covered[a.PlaceID] = true
		}
		mutations = append(mutations, t.releaseResourceTokens(consumedTokens, covered, transitionID, now)...)
	}
	return mutations
}

func (t *TransitionerSubsystem) getSpawnedWorkMutations(result resolvedWorkResult, now time.Time) []interfaces.MarkingMutation {
	// Implementation for getting spawned work mutations
	mutations := []interfaces.MarkingMutation{}
	for i := range result.spawnedWork {
		spawnMuts := t.createSpawnedTokens(&result.spawnedWork[i], result.transitionID, now)
		mutations = append(mutations, spawnMuts...)
	}
	return mutations
}

func calculateArcs(currentTransition *petri.Transition, outcome interfaces.WorkOutcome) ([]petri.Arc, error) {
	switch outcome {
	case interfaces.OutcomeAccepted:
		return currentTransition.OutputArcs, nil
	case interfaces.OutcomeContinue:
		if len(currentTransition.ContinueArcs) > 0 {
			return currentTransition.ContinueArcs, nil
		}
		return currentTransition.RejectionArcs, nil
	case interfaces.OutcomeRejected:
		return currentTransition.RejectionArcs, nil
	case interfaces.OutcomeFailed:
		return currentTransition.FailureArcs, nil
	default:
		return nil, fmt.Errorf("unknown outcome %s", outcome)
	}
}

func (t *TransitionerSubsystem) createFanoutGuardToken(inputColors []interfaces.TokenColor, result resolvedWorkResult, now time.Time) []interfaces.MarkingMutation {
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
	for arcIdx, arc := range in.arcs {
		newToken, err := in.transformer.OutputToken(token_transformer.OutputTokenInput{
			ArcIndex:       arcIdx,
			Arcs:           in.arcs,
			ConsumedTokens: in.consumed,
			InputColors:    in.inputColors,
			Output:         in.result.output,
			Outcome:        in.result.outcome,
			TransitionID:   in.result.transitionID,
			Error:          in.result.err,
			Feedback:       in.result.feedback,
			Now:            in.now,
			History:        in.history,
		})
		if err != nil {
			return nil, err
		}
		if newToken.Color.DataType != interfaces.DataTypeResource {
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
	return mutations, nil
}

func applyRecordedOutputWorkIdentity(token *interfaces.Token, recorded interfaces.FactoryWorkItem) {
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
	if recorded.TraceID != "" {
		token.Color.TraceID = recorded.TraceID
	}
	if recorded.ParentID != "" {
		token.Color.ParentID = recorded.ParentID
	}
	if len(recorded.Tags) > 0 {
		token.Color.Tags = cloneTags(recorded.Tags)
	}
}

func (t *TransitionerSubsystem) buildIntermittentFailureRequeueMutations(
	consumedTokens []interfaces.Token,
	history interfaces.TokenHistory,
	result resolvedWorkResult,
	now time.Time,
) []interfaces.MarkingMutation {
	mutations := make([]interfaces.MarkingMutation, 0, len(consumedTokens))
	for i := range consumedTokens {
		consumed := consumedTokens[i]
		if consumed.Color.DataType == interfaces.DataTypeResource {
			continue
		}

		requeued := cloneToken(consumed)
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
	history interfaces.TokenHistory,
	result resolvedWorkResult,
	now time.Time,
) interfaces.TokenHistory {
	cloned := interfaces.TokenHistory{
		TotalDuration:       history.TotalDuration,
		LastError:           result.err,
		TotalVisits:         cloneIntMap(history.TotalVisits),
		ConsecutiveFailures: cloneIntMap(history.ConsecutiveFailures),
		PlaceVisits:         cloneIntMap(history.PlaceVisits),
	}
	if history.FailureLog != nil {
		cloned.FailureLog = append([]interfaces.FailureRecord(nil), history.FailureLog...)
	}
	cloned.FailureLog = append(cloned.FailureLog, interfaces.FailureRecord{
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
func (t *TransitionerSubsystem) releaseResourceTokens(consumedTokens []interfaces.Token, alreadyCovered map[string]bool, transitionID string, now time.Time) []interfaces.MarkingMutation {
	var mutations []interfaces.MarkingMutation
	for i := range consumedTokens {
		consumed := consumedTokens[i]
		if consumed.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		if alreadyCovered[consumed.PlaceID] {
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
func (t *TransitionerSubsystem) createSpawnedTokens(spawnColor *interfaces.TokenColor, parentTransitionID string, now time.Time) []interfaces.MarkingMutation {
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

func tokenColorsFromTokens(tokens []interfaces.Token) []interfaces.TokenColor {
	colors := make([]interfaces.TokenColor, len(tokens))
	for i, token := range tokens {
		colors[i] = token.Color
	}
	return colors
}

func firstNonResourceInput(inputs []interfaces.TokenColor) *interfaces.TokenColor {
	for i := range inputs {
		if inputs[i].DataType != interfaces.DataTypeResource && inputs[i].WorkTypeID != interfaces.SystemTimeWorkTypeID {
			return &inputs[i]
		}
	}
	for i := range inputs {
		if inputs[i].DataType != interfaces.DataTypeResource {
			return &inputs[i]
		}
	}
	return nil
}
