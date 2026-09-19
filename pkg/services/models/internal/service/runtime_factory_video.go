package service

import (
	"context"
	"errors"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

func (o *Root) prepareJoinedGenericInvocation(
	ctx context.Context,
	request models.InvokeModelRequest,
	resolved models.ResolvedModelReference,
) (models.InvokeModelRequest, models.Operation, error) {
	definition, err := o.effectiveInvocationDefinition(ctx, request, resolved)
	if err != nil {
		return models.InvokeModelRequest{}, models.Operation{}, err
	}
	prepared, operation, err := models.PrepareGenericInvocation(request, definition)
	if err != nil {
		return models.InvokeModelRequest{}, models.Operation{}, err
	}
	prepared.Operation = operation.Name
	if len(prepared.Inputs) > 0 && inferenceInputIsZero(prepared.Input) {
		prepared.Input = prepared.Inputs[0].Clone()
	}
	return prepared, operation, nil
}

func (o *Root) effectiveInvocationDefinition(
	ctx context.Context,
	request models.InvokeModelRequest,
	resolved models.ResolvedModelReference,
) (models.ModelDefinition, error) {
	definition := resolved.Definition.Clone()
	if !genericRequestUsesVideo(request) ||
		!localmodels.VideoProjectorRequired(definition.Name, definition.Operations) ||
		o == nil || o.assets == nil {
		return definition, nil
	}
	inspection, err := o.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: request.Scope,
		Name:  definition.Name,
	})
	if err != nil {
		if errors.Is(err, models.ErrUnsupportedOperation) {
			// Inert/unit collaborators may not expose cache inspection. Preserve
			// the pre-existing operation path for those compatibility seams.
			return definition, nil
		}
		if !isRemovableCacheAbsence(err) {
			return models.ModelDefinition{}, err
		}
		inspection = scopedassets.RuntimeCacheInspection{}
	}
	effective, _, unavailable := localmodels.ProjectEffectiveVideoDefinition(
		definition,
		localmodels.RuntimeCacheInspectionFromScoped(inspection),
	)
	if unavailable {
		return models.ModelDefinition{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassMediaCapability,
			Message:   "video input requires a verified projector artifact",
			Model:     request.Model,
			Operation: models.OperationOMNI,
			Slot:      "video",
			Cause:     models.ErrUnsupportedOperation,
		}
	}
	return effective, nil
}

func genericRequestUsesVideo(request models.InvokeModelRequest) bool {
	for _, input := range request.Inputs {
		if strings.EqualFold(strings.TrimSpace(input.Name), "video") ||
			input.Modality == models.ModalityVideo {
			return true
		}
	}
	return false
}
