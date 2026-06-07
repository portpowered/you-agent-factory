package localmodels

import (
	"context"
	"strconv"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// RuntimeCacheInspection reports local managed-cache state without contacting
// upstream asset sources.
type RuntimeCacheInspection struct {
	Supported          bool
	Installed          bool
	Revision           string
	CachePath          string
	InstalledFileCount int
	MissingAssets      []string
	PartialArtifacts   bool
}

// RuntimeCacheInspector probes installed managed-runtime assets from local cache.
type RuntimeCacheInspector interface {
	InspectRuntimeCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (RuntimeCacheInspection, error)
}

func runtimeCacheInspectDiagnostics(inspection RuntimeCacheInspection, forInspect bool) map[string]string {
	if !inspection.Supported {
		return nil
	}
	diagnostics := make(map[string]string)
	if len(inspection.MissingAssets) > 0 {
		diagnostics["missingAssets"] = strings.Join(inspection.MissingAssets, ",")
	}
	if !forInspect {
		return diagnostics
	}
	if inspection.Installed {
		diagnostics["installedFileCount"] = strconv.Itoa(inspection.InstalledFileCount)
	}
	if inspection.Revision != "" {
		diagnostics["revision"] = inspection.Revision
	}
	if inspection.CachePath != "" {
		diagnostics["cachePath"] = inspection.CachePath
	}
	return diagnostics
}
