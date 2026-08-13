package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

func workstationDispatchResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
	executeErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatch := request.Execution.Dispatch
	proposedOutput := result.Output.Clone()
	workResult := workers.WorkResult{
		DispatchID:                  dispatch.DispatchID,
		TransitionID:                dispatch.TransitionID,
		Outcome:                     workers.OutcomeAccepted,
		Output:                      primaryOutputText(result.Output.Primary),
		Feedback:                    result.Output.Feedback,
		SelectedClassificationLabel: result.Output.Classification,
		Metrics: workers.WorkMetrics{
			Duration:   result.Metrics.Duration,
			Cost:       result.Metrics.Cost,
			RetryCount: result.Metrics.RetryCount,
		},
		ProviderSession: providerSessionFromContinuation(result.Continuation),
		Diagnostics:     result.Diagnostics.ToWorkDiagnostics(),
	}
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	switch result.Outcome {
	case workers.ExecutionOutcomeContinue:
		workResult.Outcome = workers.OutcomeContinue
	case workers.ExecutionOutcomeRejected:
		workResult.Outcome = workers.OutcomeRejected
	case workers.ExecutionOutcomeFailed:
		workResult.Outcome = workers.OutcomeFailed
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	case workers.ExecutionOutcomeCanceled:
		workResult.Outcome = workers.OutcomeFailed
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
	default:
		if result.Outcome != workers.ExecutionOutcomeAccepted {
			workResult.Outcome = workers.OutcomeFailed
			terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		}
	}
	if result.Failure != nil {
		workResult.Error = strings.TrimSpace(result.Failure.Message)
		workResult.FailureMetadata = &workers.WorkFailureMetadata{
			Family: result.Failure.Family,
			Type:   result.Failure.Type,
		}
	}
	if executeErr != nil && terminal != workers.WorkstationDispatchTerminalOutcomeCanceled {
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		workResult.Outcome = workers.OutcomeFailed
		if strings.TrimSpace(workResult.Error) == "" {
			workResult.Error = executeErr.Error()
		}
	}
	if terminal == workers.WorkstationDispatchTerminalOutcomeCanceled && strings.TrimSpace(workResult.Error) == "" {
		workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: terminal,
		Result:          workResult,
		ProposedOutput:  &proposedOutput,
	}, executeErr
}

func primaryOutputText(parts []work.WorkContentPart) string {
	for _, part := range parts {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeText:
			if strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		case work.WorkContentPartTypeJSON:
			if len(part.JSON) > 0 {
				return string(part.JSON)
			}
		case work.WorkContentPartTypeImage, work.WorkContentPartTypeAudio, work.WorkContentPartTypeBinary:
			if strings.TrimSpace(part.URL) != "" {
				return part.URL
			}
			if strings.TrimSpace(part.File) != "" {
				return part.File
			}
		}
	}
	return ""
}

func providerSessionFromContinuation(
	continuation *workers.ProviderContinuationRef,
) *workers.ProviderSessionMetadata {
	if continuation == nil {
		return nil
	}
	id := strings.TrimSpace(continuation.ProviderSessionID)
	if id == "" {
		id = strings.TrimSpace(continuation.ExternalRef)
	}
	if id == "" && strings.TrimSpace(continuation.Provider) == "" {
		return nil
	}
	return &workers.ProviderSessionMetadata{
		Provider: continuation.Provider,
		Kind:     "session",
		ID:       id,
	}
}

// materializeWorkerOutputForDispatch validates Worker-proposed output through
// Work before the result enters Runtime state. Invalid proposals flip the
// dispatch to FAILED without carrying RecordedOutputWork into mutations.
func materializeWorkerOutputForDispatch(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	request workerexecution.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
) workerexecution.WorkResult {
	return applyMaterializedWorkerOutput(
		ctx,
		workService,
		net,
		idGenerator,
		request.Execution.Dispatch,
		result,
		workerexecution.ProposedOutputFromLegacyWorkResult(result),
		false,
	)
}

// materializeWorkerOutputForDispatchWithProposal preserves the detached
// Execute proposal until Runtime has handed it to Work. Legacy WorkResult
// callers continue through the compatibility mapper above.
func materializeWorkerOutputForDispatchWithProposal(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	request workerexecution.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
	proposal *workerexecution.ProposedOutput,
) workerexecution.WorkResult {
	proposals := workerexecution.ProposedOutputFromLegacyWorkResult(result)
	fromDetachedOutput := proposal != nil
	if proposal != nil {
		proposals = proposal.Clone()
	}
	return applyMaterializedWorkerOutput(
		ctx,
		workService,
		net,
		idGenerator,
		request.Execution.Dispatch,
		result,
		proposals,
		fromDetachedOutput,
	)
}

func applyMaterializedWorkerOutput(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	dispatch work.WorkDispatch,
	result workerexecution.WorkResult,
	proposals workerexecution.ProposedOutput,
	fromDetachedOutput bool,
) workerexecution.WorkResult {
	if len(proposals.ProposedWork) == 0 &&
		(!fromDetachedOutput || !hasMaterializableOutput(proposals)) {
		return result
	}

	if workService == nil {
		failed := result
		failed.Outcome = workerexecution.OutcomeFailed
		failed.RecordedOutputWork = nil
		if strings.TrimSpace(failed.Error) == "" {
			failed.Error = "worker output materialization: Work service is required"
		} else {
			failed.Error = fmt.Sprintf("%s; worker output materialization: Work service is required", failed.Error)
		}
		return failed
	}

	materialized, err := workService.MaterializeWorkerOutput(ctx, work.MaterializeWorkerOutputRequest{
		Lineage:           lineageContextFromDispatch(dispatch),
		Primary:           proposals.Primary,
		Feedback:          proposals.Feedback,
		Classification:    proposals.Classification,
		ProposedWork:      workerexecution.WorkProposedItemsFromProposedWork(proposals.ProposedWork),
		ValidWorkTypes:    validWorkTypesFromNet(net),
		ValidStatesByType: validStatesByTypeFromNet(net),
		IDGenerator:       idGenerator,
	})
	if err != nil {
		failed := result
		failed.Outcome = workerexecution.OutcomeFailed
		failed.RecordedOutputWork = nil
		if strings.TrimSpace(failed.Error) == "" {
			failed.Error = fmt.Sprintf("worker output materialization: %v", err)
		} else {
			failed.Error = fmt.Sprintf(
				"%s; worker output materialization: %v",
				failed.Error,
				err,
			)
		}
		return failed
	}

	next := result
	if text := strings.TrimSpace(materialized.PrimaryOutput); text != "" {
		next.Output = text
	}
	if feedback := strings.TrimSpace(materialized.Feedback); feedback != "" {
		next.Feedback = feedback
	}
	if classification := strings.TrimSpace(materialized.Classification); classification != "" {
		next.SelectedClassificationLabel = classification
	}
	next.RecordedOutputWork = append([]work.FactoryWorkItem(nil), materialized.MaterializedWork...)
	return next
}

func hasMaterializableOutput(proposals workerexecution.ProposedOutput) bool {
	return len(proposals.Primary) > 0 ||
		strings.TrimSpace(proposals.Feedback) != "" ||
		strings.TrimSpace(proposals.Classification) != ""
}

func validStatesByTypeFromNet(net *state.Net) map[string]map[string]bool {
	if net == nil {
		return nil
	}
	return state.ValidStatesByType(net.WorkTypes)
}

func lineageContextFromDispatch(dispatch work.WorkDispatch) work.MaterializationLineageContext {
	parent := ""
	if len(dispatch.Execution.WorkIDs) > 0 {
		parent = dispatch.Execution.WorkIDs[0]
	}
	return work.MaterializationLineageContext{
		DispatchID:               dispatch.DispatchID,
		RequestID:                dispatch.Execution.RequestID,
		SourceWorkIDs:            append([]string(nil), dispatch.Execution.WorkIDs...),
		CurrentChainingTraceID:   dispatch.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), dispatch.PreviousChainingTraceIDs...),
		ParentWorkID:             parent,
		TraceID:                  dispatch.Execution.TraceID,
	}
}

func validWorkTypesFromNet(net *state.Net) map[string]bool {
	if net == nil || len(net.WorkTypes) == 0 {
		return nil
	}
	valid := make(map[string]bool, len(net.WorkTypes))
	for id := range net.WorkTypes {
		valid[id] = true
	}
	return valid
}
