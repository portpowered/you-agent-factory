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
	inspectPath models.AssetInspectPath,
	resolveHome models.AssetResolveHomeDirectory,
	readFile models.AssetReadFile,
	readDirectory models.AssetReadDirectory,
) (assets.Service, error) {
	if scopes == nil {
		return nil, fmt.Errorf("Models Assets runtime scopes service is required")
	}
	if platform.OperatingSystem == "" || platform.Architecture == "" {
		return nil, fmt.Errorf("Models Assets host platform is required")
	}
	if inspectPath == nil || resolveHome == nil || readFile == nil || readDirectory == nil {
		return nil, fmt.Errorf("Models Assets cache inspection effects are required")
	}
	return internalservice.New(
		scopes,
		platform,
		inspectPath,
		resolveHome,
		readFile,
		readDirectory,
	), nil
}
