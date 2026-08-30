package subsystems

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TransitionerSubsystem routes tokens to the correct arc set based on outcome,
// constructs output and fanout tokens, and handles resource release. Worker
// generated Work is returned as a canonical Work Request batch for the engine
// to submit. The subsystem reconstructs token history from raw dispatch
// records on demand instead of reading cached history snapshots.
type TransitionerSubsystem struct {
	netDefinition     *state.Net
	runtimeConfig     interfaces.RuntimeWorkstationLookup
	logger            logging.Logger
	now               func() time.Time
	transformer       *token_transformer.Transformer
	quorumPolicy      interfaces.QuorumPolicyService
	outputShaping     interfaces.InvocationOutputShapingService
	workPropagation   interfaces.WorkPropagationPolicyService
	decisionEnvelopes interfaces.DecisionEnvelopeService
}

var _ Subsystem = (*TransitionerSubsystem)(nil)

type resolvedWorkResult struct {
	dispatchID                  string
	transitionID                string
	outcome                     workerexecution.WorkOutcome
	selectedClassificationLabel string
	output                      string
	outputContent               []work.WorkContentPart
	structuredResult            any
	structuredResultPresent     bool
	recordedOutputWork          []work.FactoryWorkItem
	err                         string
	feedback                    string
	failureMetadata             *workerexecution.WorkFailureMetadata
	cancellation                *workerexecution.DispatchCancellation
}

type generatedBatchWork struct {
	request  work.WorkRequest
	submits  []work.SubmitRequest
	metadata work.GeneratedSubmissionBatchMetadata
}

type mutationCalculationInput struct {
	transition      *petri.Transition
	workstation     *interfaces.FactoryWorkstationConfig
	arcs            []petri.Arc
	consumed        []factorytoken.Token
	result          resolvedWorkResult
	now             time.Time
	history         factorytoken.History
	inputColors     []factorytoken.Color
	transformer     *token_transformer.Transformer
	runtimeConfig   interfaces.RuntimeWorkstationLookup
	quorumPolicy    interfaces.QuorumPolicyService
	outputShaping   interfaces.InvocationOutputShapingService
	workPropagation interfaces.WorkPropagationPolicyService
}

// NewTransitioner creates a TransitionerSubsystem that reads results and raw
// dispatch snapshots from the RuntimeStateSnapshot and produces routing mutations.
func NewTransitioner(
	n *state.Net,
	logger logging.Logger,
	now func() time.Time,
	transformer *token_transformer.Transformer,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
	quorumPolicy interfaces.QuorumPolicyService,
	outputShaping interfaces.InvocationOutputShapingService,
	workPropagation interfaces.WorkPropagationPolicyService,
	decisionEnvelopes ...interfaces.DecisionEnvelopeService,
) *TransitionerSubsystem {
	if now == nil {
		panic("Factory Runtime transitioner clock is required")
	}
	if transformer == nil {
		panic("Factory Runtime token transformer is required")
	}
	tr := &TransitionerSubsystem{
		netDefinition:     n,
		logger:            logging.EnsureLogger(logger),
		now:               now,
		transformer:       transformer,
		runtimeConfig:     runtimeConfig,
		quorumPolicy:      quorumPolicy,
		outputShaping:     outputShaping,
		workPropagation:   workPropagation,
		decisionEnvelopes: firstDecisionEnvelopeService(decisionEnvelopes),
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
func (t *TransitionerSubsystem) Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	if len(snapshot.Results) == 0 {
		return nil, nil
	}

	results := snapshot.Results
	t.logger.Debug("transitioner: processing results", "count", len(results))

	var mutations []interfaces.MarkingMutation
	var generatedBatches []work.GeneratedSubmissionBatch
	var completedDispatches []interfaces.CompletedDispatch
	for i := range results {
		muts, completedDispatch, batchRecords, err := t.mapToCorrespondingTokenMutations(ctx, snapshot, &results[i])
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
func (t *TransitionerSubsystem) mapToCorrespondingTokenMutations(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], result *workerexecution.WorkResult) ([]interfaces.MarkingMutation, interfaces.CompletedDispatch, []work.GeneratedSubmissionBatch, error) {
	if completed, ok := t.ignoredDispatchCompletion(snapshot, result); ok {
		return nil, completed, nil, nil
	}

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
	if resolved.cancellation != nil || resolved.outcome == workerexecution.OutcomeCanceled {
		return t.mapCanceledDispatch(snapshot, result, resolved, consumedTokens, now)
	}
	//TODO: the intermittent failure arc should be denoted as a preconstructed output, teh calculate arcs function should be a mapping of arcs for a current workstation/transition, and one such mapping would be the intermitten failure arc.
	if shouldRequeueIntermittentFailureResult(resolved) {
		t.logArcSelection(result, resolved, consumedTokens)
		mutations := t.buildIntermittentFailureRequeueMutations(consumedTokens, history, resolved, now)
		mutations = append(mutations, t.releaseResourceTokensOnFailureMutations(resolved.outcome, result.TransitionID, consumedTokens, nil, now)...)
		return mutations, t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now), nil, nil
	}
	generatedBatches, generatedWorkCount, resolved := t.resolveGeneratedBatchWork(
		currentTransition, snapshot, resolved, inputColors,
	)

	arcs, resolved, err := t.calculateArcsForResolvedResult(currentTransition, resolved, consumedTokens)
	if err != nil {
		return nil, interfaces.CompletedDispatch{}, nil, err
	}
	t.logArcSelection(result, resolved, consumedTokens)
	if len(arcs) == 0 {
		return nil, interfaces.CompletedDispatch{}, nil, transitionRoutingError(ctx, result.TransitionID, resolved.outcome)
	}

	var workstationDef *interfaces.FactoryWorkstationConfig
	if workstation, ok := runtimeWorkstation(currentTransition.Name, t.runtimeConfig); ok {
		workstationDef = workstation
	}

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:      currentTransition,
		workstation:     workstationDef,
		arcs:            arcs,
		consumed:        consumedTokens,
		result:          resolved,
		now:             now,
		history:         history,
		inputColors:     inputColors,
		transformer:     t.transformer,
		runtimeConfig:   t.runtimeConfig,
		quorumPolicy:    t.quorumPolicy,
		outputShaping:   t.outputShaping,
		workPropagation: t.workPropagation,
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
	mutations = append(mutations, t.createFanoutGuardToken(inputColors, resolved.transitionID, generatedWorkCount, now)...)

	t.logger.Info("releasing tokens", "transition", result.TransitionID, "outcome", resolved.outcome, "mutation_count", len(mutations))
	return mutations, t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now), generatedBatches, nil
}

func (t *TransitionerSubsystem) mapCanceledDispatch(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	result *workerexecution.WorkResult,
	resolved resolvedWorkResult,
	consumedTokens []factorytoken.Token,
	now time.Time,
) ([]interfaces.MarkingMutation, interfaces.CompletedDispatch, []work.GeneratedSubmissionBatch, error) {
	t.logArcSelection(result, resolved, consumedTokens)
	mutations := t.restoreCanceledDispatchMutations(snapshot, result.DispatchID, resolved, now)
	return mutations, t.buildCompletedDispatch(snapshot, result, resolved, consumedTokens, mutations, now), nil, nil
}

// restoreCanceledDispatchMutations releases the exact CONSUME claims held by
// a dispatch that was canceled before producing a business result. The
// dispatcher already removed those tokens from the marking; restoring the
// held claims makes cancellation lossless while avoiding any failure arc.
func effectiveGeneratedWorkItemLimit(limits interfaces.WorkstationLimits, inputColors []factorytoken.Color) int {
	maximum := limits.MaxGeneratedWorkItems
	argumentName := strings.TrimSpace(limits.MaxGeneratedWorkItemsArgument)
	if maximum <= 0 || argumentName == "" {
		return maximum
	}
	source := firstNonResourceInput(inputColors)
	if source == nil || source.InvocationArguments == nil {
		return maximum
	}
	argument, ok := source.InvocationArguments.Arguments[argumentName]
	if !ok || len(argument.Values) != 1 {
		return maximum
	}
	value, err := strconv.Atoi(strings.TrimSpace(argument.Values[0]))
	if err != nil || value <= 0 {
		return maximum
	}
	value += limits.MaxGeneratedWorkItemsArgumentOffset
	if value <= 0 || value > maximum {
		return maximum
	}
	return value
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
		Cancellation:                resolved.cancellation.Clone(),
		SelectedClassificationLabel: resolved.selectedClassificationLabel,
		Reason:                      completedDispatchReason(resolved),
		ArtifactVerification:        result.ArtifactVerification.Clone(),
		FailureMetadata:             failureMetadata,
		FailureDetail:               workerexecution.CloneFailureDetail(result.FailureDetail),
		ProviderSession:             (result.Continuation).SessionMetadata(),
		EndTime:                     endTime,
		ConsumedTokens:              factorytoken.ToWorkerSlice(consumedTokens),
		OutputMutations: mutationRecordsForDispatch(
			result.DispatchID,
			result.TransitionID,
			resolved.outcome,
			t.netDefinition,
			mutations,
		),
	}
	if resolved.outcome == workerexecution.OutcomeCanceled {
		completed.FailureMetadata = nil
		completed.FailureDetail = nil
	}
	if dispatchEntry == nil {
		return completed
	}

	completed.WorkstationName = dispatchEntry.WorkstationName
	completed.ExpectedArtifactContext = cloneExpectedArtifactTemplateContext(dispatchEntry.ExpectedArtifactContext)
	completed.StartTime = dispatchEntry.StartTime
	completed.Duration = completed.EndTime.Sub(dispatchEntry.StartTime)
	return completed
}

func cloneExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) *work.ExpectedArtifactTemplateContext {
	return context.Clone()
}

func completedDispatchReason(result resolvedWorkResult) string {
	switch result.outcome {
	case workerexecution.OutcomeCanceled:
		if result.cancellation != nil && result.cancellation.Reason != "" {
			return string(result.cancellation.Reason)
		}
		return string(workerexecution.DispatchCancellationReasonCanceled)
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

func firstDecisionEnvelopeService(
	services []interfaces.DecisionEnvelopeService,
) interfaces.DecisionEnvelopeService {
	if len(services) == 0 {
		return nil
	}
	return services[0]
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
	topology *state.Net,
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
		record.Terminal, record.TransitionReachable = terminalMutationFacts(topology, mutation)
		if mutation.NewToken != nil {
			runtimeToken := factorytoken.FromWorker(*mutation.NewToken)
			record.Token = workerTokenPointer(&runtimeToken)
			if record.TokenID == "" {
				record.TokenID = mutation.NewToken.ID
			}
			if record.ToPlace == "" {
				record.ToPlace = runtimeToken.PlaceID
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
			clone[i].Content = work.CloneWorkContentParts(items[i].Content)
		}
		clone[i].StructuredResult = jsonvalue.Clone(items[i].StructuredResult)
		clone[i].StructuredResultPresent = jsonvalue.Present(items[i].StructuredResult, items[i].StructuredResultPresent)
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

func resolveWorkResult(transition *petri.Transition, result *workerexecution.WorkResult, runtimeConfig interfaces.RuntimeWorkstationLookup) resolvedWorkResult {
	resolved := resolvedWorkResult{
		dispatchID:                  result.DispatchID,
		transitionID:                result.TransitionID,
		outcome:                     result.Outcome,
		output:                      result.Output,
		outputContent:               work.CloneWorkContentParts(result.OutputContent),
		structuredResult:            jsonvalue.Clone(result.StructuredResult),
		structuredResultPresent:     jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent),
		selectedClassificationLabel: result.SelectedClassificationLabel,
		recordedOutputWork:          cloneFactoryWorkItems(result.RecordedOutputWork),
		err:                         result.Error,
		feedback:                    result.Feedback,
		failureMetadata:             result.FailureMetadata,
		cancellation:                result.Cancellation.Clone(),
	}
	if resolved.cancellation != nil || resolved.outcome == workerexecution.OutcomeCanceled {
		if resolved.cancellation == nil {
			resolved.cancellation = &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonCanceled}
		}
		resolved.outcome = workerexecution.OutcomeCanceled
	}
	if workstation, ok := runtimeWorkstation(transition.Name, runtimeConfig); ok && workstation != nil &&
		resolved.cancellation == nil &&
		len(workstation.StopWords) > 0 &&
		strings.TrimSpace(workstation.OutcomeFormat) != interfaces.WorkstationOutcomeFormatDecisionEnvelope {
		resolved.outcome = evaluateStopWords(workstation.StopWords, result.Output)
	}
	return resolved
}

func shouldRequeueIntermittentFailureResult(result resolvedWorkResult) bool {
	if result.outcome != workerexecution.OutcomeFailed || result.failureMetadata == nil {
		return false
	}
	return workerexecution.FailureDecisionFromMetadata(result.failureMetadata).Retryable
}

func (t *TransitionerSubsystem) workerEmittedBatchWork(result resolvedWorkResult, inputColors []factorytoken.Color, existingWorks []work.ExistingWork) (generatedBatchWork, bool, error) {
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
	normalized, err := work.NormalizeGeneratedSubmissionBatch(batch, work.WorkRequestNormalizeOptions{
		ValidWorkTypes: t.validWorkTypes(),
		ExistingWorks:  existingWorks,
	})
	if err != nil {
		return generatedBatchWork{}, true, fmt.Errorf("worker-emitted work request batch: %w", err)
	}
	return generatedBatchWork{request: request, submits: normalized, metadata: metadata}, true, nil
}

// existingWorksForAdmission returns the point-in-time board identities visible
// to a worker-emitted batch. A dispatched Work is absent from Marking while it
// is active, so consumed dispatch tokens are included alongside marking
// tokens. The engine performs the same snapshot for external admission before
// queueing the request.
func existingWorksForAdmission(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []work.ExistingWork {
	if snapshot == nil {
		return nil
	}

	byID := make(map[string]work.ExistingWork)
	add := func(color factorytoken.Color) {
		if color.DataType == factorytoken.DataTypeResource || color.WorkID == "" {
			return
		}
		candidate := work.ExistingWork{
			WorkID:     color.WorkID,
			Name:       color.Name,
			WorkTypeID: color.WorkTypeID,
		}
		if current, exists := byID[candidate.WorkID]; exists {
			if current.Name == "" {
				current.Name = candidate.Name
			}
			if current.WorkTypeID == "" {
				current.WorkTypeID = candidate.WorkTypeID
			}
			byID[candidate.WorkID] = current
			return
		}
		byID[candidate.WorkID] = candidate
	}

	for _, token := range snapshot.Marking.Tokens {
		if token != nil {
			add(token.Color)
		}
	}
	for _, dispatch := range snapshot.Dispatches {
		if dispatch == nil {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			add(token.Color)
		}
	}

	works := make([]work.ExistingWork, 0, len(byID))
	for _, candidate := range byID {
		works = append(works, candidate)
	}
	sort.Slice(works, func(i, j int) bool { return works[i].WorkID < works[j].WorkID })
	return works
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
		if request.Works[i].InvocationArguments == nil {
			request.Works[i].InvocationArguments = work.CloneInvocationArguments(source.InvocationArguments)
		}
		if source.WorkID != "" {
			request.Works[i].RuntimeRelations = appendUniqueRuntimeRelation(request.Works[i].RuntimeRelations, work.Relation{
				Type:         work.RelationParentChild,
				TargetWorkID: source.WorkID,
			})
		}
		request.Works[i].Tags = mergedWorkerBatchTags(source.Tags, request.Works[i].Tags, source, result)
	}
}

func appendUniqueRuntimeRelation(relations []work.Relation, relation work.Relation) []work.Relation {
	for i := range relations {
		if relations[i].Type == relation.Type &&
			relations[i].TargetWorkID == relation.TargetWorkID &&
			relations[i].RequiredState == relation.RequiredState {
			return relations
		}
	}
	return append(relations, relation)
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
	case workerexecution.OutcomeCanceled:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown outcome %s", outcome)
	}
}

func (t *TransitionerSubsystem) createFanoutGuardToken(inputColors []factorytoken.Color, transitionID string, expectedCount int, now time.Time) []interfaces.MarkingMutation {
	mutations := []interfaces.MarkingMutation{}
	countPlaceID, hasFanoutGroup := t.netDefinition.FanoutGroups[transitionID]
	if expectedCount > 0 || hasFanoutGroup {
		if hasFanoutGroup {
			parentWorkID := ""
			if first := firstNonResourceInput(inputColors); first != nil {
				parentWorkID = first.WorkID
			}

			countToken := t.transformer.FanoutCountToken(countPlaceID, transitionID, parentWorkID, expectedCount, now)
			mutations = append(mutations, interfaces.MarkingMutation{
				Type:     interfaces.MutationCreate,
				ToPlace:  countPlaceID,
				NewToken: workerTokenPointer(countToken),
				Reason:   fmt.Sprintf("fanout count token for transition %s (expected %d children)", transitionID, expectedCount),
			})
		}
	}
	return mutations
}

func calculateMutations(in mutationCalculationInput) ([]interfaces.MarkingMutation, error) {
	mutations := make([]interfaces.MarkingMutation, 0)
	workOutputIndex := 0
	if in.workPropagation == nil {
		return nil, fmt.Errorf("Factory Definition Work propagation policy service is required")
	}
	workPropagationMode := in.workPropagation.Mode(in.workstation)
	workstationName := ""
	if in.workstation != nil {
		workstationName = in.workstation.Name
	}
	for arcIdx, arc := range in.arcs {
		baseInput := token_transformer.OutputTokenInput{
			ArcIndex:                arcIdx,
			Arcs:                    in.arcs,
			ConsumedTokens:          in.consumed,
			InputColors:             in.inputColors,
			Output:                  in.result.output,
			OutputContent:           work.CloneWorkContentParts(in.result.outputContent),
			StructuredResult:        in.result.structuredResult,
			StructuredResultPresent: in.result.structuredResultPresent,
			WorkPropagationMode:     workPropagationMode,
			WorkstationName:         workstationName,
			WorkstationType:         workstationType(in.workstation),
			Outcome:                 in.result.outcome,
			TransitionID:            in.result.transitionID,
			Error:                   in.result.err,
			Feedback:                in.result.feedback,
			Now:                     in.now,
			History:                 in.history,
		}
		repeatCount := mutationRepeatCountForArc(arc, in.consumed)
		for resourceTokenIndex := 0; resourceTokenIndex < repeatCount; resourceTokenIndex++ {
			tokenInput := baseInput
			tokenInput.ResourceTokenIndex = resourceTokenIndex
			newToken, err := in.transformer.OutputToken(tokenInput)
			if err != nil {
				return nil, err
			}
			if err := applyPackagedTTSInvocationMetadata(in.outputShaping, newToken, in.workstation, in.result.output, in.inputColors, in.runtimeConfig); err != nil {
				return nil, err
			}
			if in.result.outcome == workerexecution.OutcomeAccepted {
				if err := applyPackagedGoalInvocationSummary(in.outputShaping, newToken, in.workstation, in.result.output, in.runtimeConfig); err != nil {
					return nil, err
				}
			}
			if err := applyPackagedSubagentInvocationResponse(in.outputShaping, newToken, in.workstation, in.result.output, in.runtimeConfig); err != nil {
				return nil, err
			}
			applyPackagedQuorumWorkRelations(in.quorumPolicy, newToken, in.workstation, in.inputColors, in.runtimeConfig)
			if newToken.Color.DataType != factorytoken.DataTypeResource {
				if workOutputIndex < len(in.result.recordedOutputWork) {
					applyRecordedOutputWorkIdentity(newToken, in.result.recordedOutputWork[workOutputIndex])
				}
				workOutputIndex++
			}

			mutations = append(mutations, interfaces.MarkingMutation{
				Type:     interfaces.MutationCreate,
				ToPlace:  arc.PlaceID,
				NewToken: workerTokenPointer(newToken),
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
	quorumPolicy interfaces.QuorumPolicyService,
	output *factorytoken.Token,
	workstation *interfaces.FactoryWorkstationConfig,
	inputs []factorytoken.Color,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) {
	if quorumPolicy == nil || output == nil || workstation == nil {
		return
	}
	factoryConfigLookup, ok := runtimeConfig.(interfaces.RuntimeFactoryConfigLookup)
	if !ok || !quorumPolicy.IsPackagedQuorumFactory(factoryConfigLookup.FactoryConfig()) {
		return
	}
	lineage := make([]interfaces.QuorumLineageInput, len(inputs))
	for index, input := range inputs {
		lineage[index] = interfaces.QuorumLineageInput{
			WorkID: input.WorkID, WorkTypeID: input.WorkTypeID,
		}
	}
	output.Color.Relations = quorumPolicy.WorkRelations(
		workstation.Name,
		output.Color.ParentID,
		output.Color.WorkTypeID,
		lineage,
	)
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
			NewToken: workerTokenPointer(&requeued),
			Reason:   fmt.Sprintf("transitioner: requeue intermittent failure from transition %s", result.transitionID),
		})
	}
	return mutations
}

func tokenColorsFromTokens(tokens []factorytoken.Token) []factorytoken.Color {
	colors := make([]factorytoken.Color, len(tokens))
	for i, token := range tokens {
		colors[i] = token.Color
	}
	return colors
}
