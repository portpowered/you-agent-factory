package wire_test

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (recordingAssetsService) PreflightModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}
