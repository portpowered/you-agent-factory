// Package wire constructs the private Models Assets subservice.
package wire

import (
	"fmt"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	"go.uber.org/zap"
)

// NewService constructs an inert scoped asset inspector.
func NewService(
	scopes runtimescopes.Service,
	platform models.AssetHostPlatform,
	client modelseffects.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
	makeDirectories modelseffects.AssetMakeDirectories,
	inspectPath modelseffects.AssetInspectPath,
	resolveHome modelseffects.AssetResolveHomeDirectory,
	writeFile modelseffects.AssetWriteFile,
	renamePath modelseffects.AssetRenamePath,
	removePath modelseffects.AssetRemovePath,
	removeTree modelseffects.AssetRemoveTree,
	readFile modelseffects.AssetReadFile,
	readDirectory modelseffects.AssetReadDirectory,
	createFile modelseffects.AssetCreateFile,
	openFile modelseffects.AssetOpenFile,
	logger *zap.Logger,
	now func() time.Time,
) (assets.Service, assets.RemoveModelAssetsOperation, error) {
	if scopes == nil {
		return nil, nil, fmt.Errorf("Models Assets runtime scopes service is required")
	}
	if platform.OperatingSystem == "" || platform.Architecture == "" {
		return nil, nil, fmt.Errorf("Models Assets host platform is required")
	}
	if err := validateSourceEffects(client, endpoints); err != nil {
		return nil, nil, err
	}
	if err := validateCacheEffects(
		makeDirectories, inspectPath, resolveHome, writeFile, renamePath,
		removePath, readFile, readDirectory, createFile, openFile,
	); err != nil {
		return nil, nil, err
	}
	if removeTree == nil {
		return nil, nil, fmt.Errorf("Models Assets secure removal effect is required")
	}
	if logger == nil {
		return nil, nil, fmt.Errorf("Models Assets logger is required")
	}
	if now == nil {
		return nil, nil, fmt.Errorf("Models Assets clock is required")
	}
	service := internalservice.New(
		scopes,
		platform,
		client,
		endpoints,
		makeDirectories,
		inspectPath,
		resolveHome,
		writeFile,
		renamePath,
		removePath,
		removeTree,
		readFile,
		readDirectory,
		createFile,
		openFile,
		logger,
		now,
	)
	return service, assets.RemoveModelAssetsOperation(service.RemoveModelAssets), nil
}

func validateSourceEffects(
	client modelseffects.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
) error {
	if client == nil || endpoints.BaseURL == "" || endpoints.APIBaseURL == "" {
		return fmt.Errorf("Models Assets source effects are required")
	}
	return nil
}

func validateCacheEffects(
	makeDirectories modelseffects.AssetMakeDirectories,
	inspectPath modelseffects.AssetInspectPath,
	resolveHome modelseffects.AssetResolveHomeDirectory,
	writeFile modelseffects.AssetWriteFile,
	renamePath modelseffects.AssetRenamePath,
	removePath modelseffects.AssetRemovePath,
	readFile modelseffects.AssetReadFile,
	readDirectory modelseffects.AssetReadDirectory,
	createFile modelseffects.AssetCreateFile,
	openFile modelseffects.AssetOpenFile,
) error {
	if makeDirectories == nil || inspectPath == nil || resolveHome == nil ||
		writeFile == nil || renamePath == nil || removePath == nil ||
		readFile == nil || readDirectory == nil || createFile == nil || openFile == nil {
		return fmt.Errorf("Models Assets cache effects are required")
	}
	return nil
}
