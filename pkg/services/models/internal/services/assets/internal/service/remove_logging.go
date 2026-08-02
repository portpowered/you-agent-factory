package service

import (
	"errors"
	"fmt"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

const assetRemovalOperation = "models.remove_model_assets"

func (s *service) logAssetRemovalStart(modelIdentity string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info(
		"model asset removal started",
		zap.String("operation", assetRemovalOperation),
		zap.String("phase", "start"),
		zap.String("model", modelIdentity),
	)
}

func (s *service) logAssetRemovalTerminal(
	modelIdentity string,
	start time.Time,
	result models.RemoveModelAssetsResult,
	err error,
	changed bool,
	partialDeletion bool,
) {
	if s == nil || s.logger == nil {
		return
	}
	duration := s.operationNow().Sub(start)
	if duration < 0 {
		duration = 0
	}
	outcome := "ERROR"
	if err == nil {
		outcome = string(result.Outcome)
	}
	fields := []zap.Field{
		zap.String("operation", assetRemovalOperation),
		zap.String("phase", "terminal"),
		zap.String("model", modelIdentity),
		zap.String("outcome", outcome),
		zap.Int64("duration_ms", duration.Milliseconds()),
		zap.String("error_classification", assetRemovalErrorClassification(err)),
		zap.Bool("cancelled", errors.Is(err, models.ErrAssetCancelled)),
		zap.Bool("changed", changed),
		zap.Bool("partial_deletion", partialDeletion),
	}
	if err != nil {
		// Error text can contain a filesystem path. The type and bounded
		// classification are enough for correlation without leaking it.
		fields = append(fields, zap.String("error_type", fmt.Sprintf("%T", err)))
	}
	s.logger.Info("model asset removal finished", fields...)
}

func assetRemovalErrorClassification(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, models.ErrAssetCancelled):
		return "cancelled"
	case errors.Is(err, models.ErrNotFound):
		return "invalid_model_identity"
	case errors.Is(err, models.ErrRuntimeScopeInvalid),
		errors.Is(err, models.ErrRuntimeScopeForeign),
		errors.Is(err, models.ErrRuntimeScopeStale),
		errors.Is(err, models.ErrRuntimeScopeClosed):
		return "scope"
	case errors.Is(err, models.ErrAssetSourceUnsupported):
		return "unsupported_model_identity"
	case errors.Is(err, models.ErrAssetUnavailable):
		return "filesystem"
	default:
		return "error"
	}
}
