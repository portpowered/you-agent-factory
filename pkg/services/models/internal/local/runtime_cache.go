package local

import (
	"context"
	"strconv"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
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
	InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error)
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

const (
	ManagedRuntimeSourceKindUpstreamRepository = "UPSTREAM_REPOSITORY"
	ManagedRuntimeSourceKindManagedMirror      = "MANAGED_MIRROR"
)

// ManagedRuntimeSourceResolution classifies which configured backend source
// satisfies a managed runtime without exposing provider-native repository terms.
type ManagedRuntimeSourceResolution struct {
	SourceKind    string
	SourceID      string
	ResolverNotes string
}

// ManagedRuntimeSourceResolver selects a backend source for one managed runtime.
type ManagedRuntimeSourceResolver interface {
	Resolve(modelName string, resource *models.RuntimeResource) ManagedRuntimeSourceResolution
}

type defaultManagedRuntimeSourceResolver struct{}

// DefaultManagedRuntimeSourceResolver returns the production source resolver.
func DefaultManagedRuntimeSourceResolver() ManagedRuntimeSourceResolver {
	return defaultManagedRuntimeSourceResolver{}
}

func (defaultManagedRuntimeSourceResolver) Resolve(modelName string, resource *models.RuntimeResource) ManagedRuntimeSourceResolution {
	if resource != nil {
		switch strings.ToUpper(strings.TrimSpace(resource.Provider)) {
		case "MODELSCOPE", "MANAGED_MIRROR":
			return ManagedRuntimeSourceResolution{
				SourceKind:    ManagedRuntimeSourceKindManagedMirror,
				SourceID:      "managed-mirror:" + canonicalModelName(modelName),
				ResolverNotes: "assets resolve through configured managed mirror source",
			}
		}
	}
	return ManagedRuntimeSourceResolution{
		SourceKind:    ManagedRuntimeSourceKindUpstreamRepository,
		SourceID:      "upstream-repository:" + canonicalModelName(modelName),
		ResolverNotes: "assets resolve through configured upstream repository source",
	}
}

func managedRuntimeSourceDiagnostics(resolution ManagedRuntimeSourceResolution) map[string]string {
	if strings.TrimSpace(resolution.SourceKind) == "" {
		return nil
	}
	return map[string]string{
		"sourceKind":    resolution.SourceKind,
		"sourceId":      resolution.SourceID,
		"resolverNotes": resolution.ResolverNotes,
	}
}
