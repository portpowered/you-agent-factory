package service_test

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (availableInferenceAssets) PreflightModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (*recordingInferenceAssets) PreflightModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}
