package http

import (
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func modelRemoveResponseFromService(result models.RemoveModelAssetsResult) factoryapi.ModelRemoveResponse {
	return factoryapi.ModelRemoveResponse{
		ModelName:    result.ModelName,
		Revision:     result.Revision,
		CachePath:    result.CachePath,
		Outcome:      factoryapi.ModelRemoveOutcome(result.Outcome),
		BytesRemoved: result.BytesRemoved,
	}
}
