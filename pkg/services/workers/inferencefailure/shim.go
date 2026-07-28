// Package inferencefailure is a transitional compile shim that re-exports
// inference failure classification from the private workstations destination.
package inferencefailure

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstationinferencefailure "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/inferencefailure"
)

func ClassifyInferenceFailure(err error, ctx workers.InferenceFailureContext) (*workers.InferenceFailure, bool) {
	failure, ok := workstationinferencefailure.ClassifyInferenceFailure(
		err,
		workstationinferencefailure.InferenceFailureContext{
			ModelName:  ctx.ModelName,
			WorkerName: ctx.WorkerName,
			Operation:  ctx.Operation,
		},
	)
	if !ok {
		return nil, false
	}
	return convertInferenceFailure(failure), true
}

func ClassifyInferenceWorkResultFailure(
	result workers.WorkResult,
	ctx workers.InferenceFailureContext,
) (*workers.InferenceFailure, bool) {
	failure, ok := workstationinferencefailure.ClassifyInferenceWorkResultFailure(
		workstationinferencefailure.WorkResult{
			Outcome: workstationinferencefailure.WorkResultOutcome(result.Outcome),
			Error:   result.Error,
			FailureMetadata: convertFailureMetadata(result.FailureMetadata),
		},
		workstationinferencefailure.InferenceFailureContext{
			ModelName:  ctx.ModelName,
			WorkerName: ctx.WorkerName,
			Operation:  ctx.Operation,
		},
	)
	if !ok {
		return nil, false
	}
	return convertInferenceFailure(failure), true
}

func convertFailureMetadata(metadata *workers.WorkFailureMetadata) *workstationinferencefailure.WorkFailureMetadata {
	if metadata == nil {
		return nil
	}
	return &workstationinferencefailure.WorkFailureMetadata{
		Type: workstationinferencefailure.WorkFailureType(metadata.Type),
	}
}

func convertInferenceFailure(failure *workstationinferencefailure.InferenceFailure) *workers.InferenceFailure {
	if failure == nil {
		return nil
	}
	return &workers.InferenceFailure{
		Class:      workers.InferenceFailureClass(failure.Class),
		Message:    failure.Message,
		ModelName:  failure.ModelName,
		WorkerName: failure.WorkerName,
		Operation:  failure.Operation,
		Cause:      failure.Cause,
	}
}
