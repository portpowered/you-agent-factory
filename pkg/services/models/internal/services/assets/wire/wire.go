// Package wire constructs the private Models Assets subservice.
package wire

import (
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewService constructs an inert scoped asset inspector.
func NewService(
	scopes runtimescopes.Service,
	platform models.AssetHostPlatform,
	client models.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
	makeDirectories models.AssetMakeDirectories,
	inspectPath models.AssetInspectPath,
	resolveHome models.AssetResolveHomeDirectory,
	writeFile models.AssetWriteFile,
	renamePath models.AssetRenamePath,
	removePath models.AssetRemovePath,
	readFile models.AssetReadFile,
	readDirectory models.AssetReadDirectory,
	createFile models.AssetCreateFile,
	openFile models.AssetOpenFile,
) (assets.Service, error) {
	if scopes == nil {
		return nil, fmt.Errorf("Models Assets runtime scopes service is required")
	}
	if platform.OperatingSystem == "" || platform.Architecture == "" {
		return nil, fmt.Errorf("Models Assets host platform is required")
	}
	if err := validateSourceEffects(client, endpoints); err != nil {
		return nil, err
	}
	if err := validateCacheEffects(
		makeDirectories, inspectPath, resolveHome, writeFile, renamePath,
		removePath, readFile, readDirectory, createFile, openFile,
	); err != nil {
		return nil, err
	}
	return internalservice.New(
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
		readFile,
		readDirectory,
		createFile,
		openFile,
	), nil
}

func validateSourceEffects(
	client models.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
) error {
	if client == nil || endpoints.BaseURL == "" || endpoints.APIBaseURL == "" {
		return fmt.Errorf("Models Assets source effects are required")
	}
	return nil
}

func validateCacheEffects(
	makeDirectories models.AssetMakeDirectories,
	inspectPath models.AssetInspectPath,
	resolveHome models.AssetResolveHomeDirectory,
	writeFile models.AssetWriteFile,
	renamePath models.AssetRenamePath,
	removePath models.AssetRemovePath,
	readFile models.AssetReadFile,
	readDirectory models.AssetReadDirectory,
	createFile models.AssetCreateFile,
	openFile models.AssetOpenFile,
) error {
	if makeDirectories == nil || inspectPath == nil || resolveHome == nil ||
		writeFile == nil || renamePath == nil || removePath == nil ||
		readFile == nil || readDirectory == nil || createFile == nil || openFile == nil {
		return fmt.Errorf("Models Assets cache effects are required")
	}
	return nil
}
