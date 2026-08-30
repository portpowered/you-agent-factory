package subsystems

import (
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func applyPackagedTTSInvocationMetadata(
	outputShaping factorydefinitions.InvocationOutputShapingService,
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	inputColors []factorytoken.Color,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if outputShaping == nil || token == nil || !outputShaping.ShouldFormatTTSInvocationMetadata(workstation) {
		return nil
	}
	if strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	traceID := ""
	if source := firstNonResourceInput(inputColors); source != nil {
		traceID = strings.TrimSpace(source.TraceID)
	}

	backendLabel := ""
	if workstation != nil && runtimeConfig != nil {
		if lookup, ok := runtimeConfig.(factorydefinitions.RuntimeDefinitionLookup); ok {
			if worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName)); ok && worker != nil {
				backendLabel = outputShaping.TTSBackendLabelFromWorker(worker)
			}
		}
	}

	metadataContent, err := outputShaping.TTSMetadataContentFromWorkerOutput(
		workerOutput,
		traceID,
		"",
		backendLabel,
	)
	if err != nil {
		// Packaged TTS metadata is only shaped from successful audio output.
		return nil
	}

	token.Color.Content = metadataContent
	token.Color.Payload = nil
	return nil
}

func (t *TransitionerSubsystem) resolveGeneratedBatchWork(
	transition *petri.Transition,
	snapshot *factorydefinitions.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	resolved resolvedWorkResult,
	inputColors []factorytoken.Color,
) ([]work.GeneratedSubmissionBatch, int, resolvedWorkResult) {
	if resolved.outcome != workerexecution.OutcomeAccepted {
		return nil, 0, resolved
	}
	generatedBatch, detectedBatch, batchErr := t.workerEmittedBatchWork(
		resolved, inputColors, existingWorksForAdmission(snapshot),
	)
	if batchErr != nil {
		resolved.outcome = workerexecution.OutcomeFailed
		resolved.err = batchErr.Error()
		return nil, 0, resolved
	}
	if !detectedBatch {
		return nil, 0, resolved
	}
	if workstation, ok := runtimeWorkstation(transition.Name, t.runtimeConfig); ok {
		limit := effectiveGeneratedWorkItemLimit(workstation.Limits, inputColors)
		if limit > 0 && len(generatedBatch.submits) > limit {
			resolved.outcome = workerexecution.OutcomeFailed
			resolved.err = fmt.Sprintf(
				"worker-emitted work request batch contains %d Work items, exceeding workstation limit %d",
				len(generatedBatch.submits),
				limit,
			)
		}
	}
	if resolved.outcome != workerexecution.OutcomeAccepted {
		return nil, 0, resolved
	}
	return []work.GeneratedSubmissionBatch{{
		Request: generatedBatch.request, Metadata: generatedBatch.metadata, Submissions: generatedBatch.submits,
	}}, len(generatedBatch.submits), resolved
}

func applyPackagedGoalInvocationSummary(
	outputShaping factorydefinitions.InvocationOutputShapingService,
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if outputShaping == nil || token == nil || !outputShaping.ShouldFormatInvocationSummary(workstation) {
		return nil
	}
	if strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	stopToken := workstationStopToken(workstation, runtimeConfig)
	summaryContent, err := outputShaping.SummaryContentFromWorkerOutput(workerOutput, stopToken)
	if err != nil {
		return fmt.Errorf("shape packaged goal invocation summary: %w", err)
	}

	token.Color.Content = summaryContent
	token.Color.Payload = nil
	return nil
}

func applyPackagedSubagentInvocationResponse(
	outputShaping factorydefinitions.InvocationOutputShapingService,
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if outputShaping == nil || token == nil || !outputShaping.ShouldFormatInvocationResponse(workstation) {
		return nil
	}
	if strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	stopToken := workstationStopToken(workstation, runtimeConfig)
	responseContent, err := outputShaping.ResponseContentFromWorkerOutput(workerOutput, stopToken)
	if err != nil {
		return fmt.Errorf("shape packaged subagent invocation response: %w", err)
	}

	token.Color.Content = responseContent
	token.Color.Payload = nil
	return nil
}

func workstationStopToken(
	workstation *factorydefinitions.FactoryWorkstationConfig,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) string {
	if workstation == nil || runtimeConfig == nil {
		return ""
	}
	lookup, ok := runtimeConfig.(factorydefinitions.RuntimeDefinitionLookup)
	if !ok {
		return ""
	}
	worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName))
	if !ok || worker == nil {
		return ""
	}
	return strings.TrimSpace(worker.StopToken)
}

// applyRecordedOutputWorkIdentity projects the durable identity and content
// recorded for a generated Work item onto the token created by the transition.
// This output projection stays with the other token-output shaping helpers.
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
	applyRecordedOutputWorkStructuredResult(token, recorded)
}

func applyRecordedOutputWorkStructuredResult(token *factorytoken.Token, recorded work.FactoryWorkItem) {
	if jsonvalue.Present(recorded.StructuredResult, recorded.StructuredResultPresent) {
		token.Color.StructuredResult = jsonvalue.Clone(recorded.StructuredResult)
		token.Color.StructuredResultPresent = true
	}
}

// releaseResourceTokens returns consumed resource tokens to their original places.
func (t *TransitionerSubsystem) releaseResourceTokens(consumedTokens []factorytoken.Token, alreadyCovered map[string]int, transitionID string, now time.Time) []factorydefinitions.MarkingMutation {
	var mutations []factorydefinitions.MarkingMutation
	for _, consumed := range consumedTokens {
		if consumed.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		if alreadyCovered[consumed.PlaceID] > 0 {
			alreadyCovered[consumed.PlaceID]--
			continue
		}
		resourceToken := t.transformer.ReleasedResourceToken(consumed, consumed.PlaceID, now)
		mutations = append(mutations, factorydefinitions.MarkingMutation{
			Type:     factorydefinitions.MutationCreate,
			ToPlace:  consumed.PlaceID,
			NewToken: workerTokenPointer(resourceToken),
			Reason:   fmt.Sprintf("release resource %s for transition %s", consumed.PlaceID, transitionID),
		})
	}
	return mutations
}

func workerTokenPointer(value *factorytoken.Token) *workerexecution.Token {
	if value == nil {
		return nil
	}
	projected := factorytoken.ToWorker(*value)
	return &projected
}
