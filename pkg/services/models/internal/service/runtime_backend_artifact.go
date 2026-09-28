package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func (o *Root) resolveJoinedBackendArtifact(
	ctx context.Context,
	configuration modelseffects.ResolvedHostConfiguration,
	offline bool,
) (modelseffects.BackendArtifactSelection, error) {
	if !isJoinedManagedBackend(configuration.Backend) {
		return modelseffects.BackendArtifactSelection{}, nil
	}
	if o == nil || o.resolveBackendArtifact == nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: managed backend selector is unavailable", models.ErrHostMissingAssets)
	}
	selection, err := o.resolveBackendArtifact(ctx, configuration.Clone(), offline)
	if err != nil {
		if errors.Is(err, models.ErrAssetOffline) {
			return modelseffects.BackendArtifactSelection{}, models.ErrAssetOffline
		}
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: managed backend selection failed", models.ErrHostMissingAssets)
	}
	return validateManagedBackendSelection(selection)
}

func validateManagedBackendSelection(selection modelseffects.BackendArtifactSelection) (modelseffects.BackendArtifactSelection, error) {
	if selection.InstalledPath != "" {
		path := strings.TrimSpace(selection.InstalledPath)
		if selection.Name == "" && selection.Location == "" && selection.Bytes == 0 && selection.SHA256 == "" &&
			filepath.IsAbs(path) && filepath.Clean(path) == path {
			selection.InstalledPath = path
			return selection, nil
		}
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: installed backend path is invalid", models.ErrHostMissingAssets)
	}
	requirement := models.AssetRequirement{
		Name: selection.Name, Bytes: selection.Bytes, SHA256: selection.SHA256,
	}
	if strings.TrimSpace(selection.Location) == "" || strings.TrimSpace(selection.SHA256) == "" ||
		selection.Bytes <= 0 || requirement.Validate() != nil {
		return modelseffects.BackendArtifactSelection{}, fmt.Errorf(
			"%w: managed backend artifact facts are invalid", models.ErrHostMissingAssets)
	}
	return selection, nil
}
