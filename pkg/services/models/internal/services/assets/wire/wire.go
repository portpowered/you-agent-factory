// Package wire constructs the private Models Assets subservice.
package wire

import (
	"context"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
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
	readFile modelseffects.AssetReadFile,
	readDirectory modelseffects.AssetReadDirectory,
	createFile modelseffects.AssetCreateFile,
	openFile modelseffects.AssetOpenFile,
	resolveEnvironment modelseffects.AssetResolveEnvironment,
	resolveRevision func(context.Context, string) (string, error),
	coordination modelseffects.AssetStagingCoordination,
) (assets.Service, error) {
	if platform.OperatingSystem == "" || platform.Architecture == "" {
		return nil, fmt.Errorf("Models Assets host platform is required")
	}
	if endpoints.BaseURL == "" || endpoints.APIBaseURL == "" {
		return nil, fmt.Errorf("Models Assets source effects are required")
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
		resolveEnvironment, resolveRevision, coordination,
	), nil
}
