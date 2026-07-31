package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	)
}

// materializeExecuteResultForDispatch is the post-Execute materialization path
// used when Runtime already holds a correlated workers.ExecuteResult.
func materializeExecuteResultForDispatch(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	dispatch work.WorkDispatch,
	result workerexecution.WorkResult,
	executeResult workerexecution.ExecuteResult,
) workerexecution.WorkResult {
	return applyMaterializedWorkerOutput(
		ctx,
		workService,
		net,
		idGenerator,
		dispatch,
		result,
		executeResult.Output.Clone(),
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
) workerexecution.WorkResult {
	if len(proposals.ProposedWork) == 0 {
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
