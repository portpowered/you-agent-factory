package localmodels

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	ManagedRuntimeSourceKindUpstreamRepository = "UPSTREAM_REPOSITORY"
	ManagedRuntimeSourceKindManagedMirror        = "MANAGED_MIRROR"
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
	Resolve(modelName string, resource *interfaces.ResourceConfig) ManagedRuntimeSourceResolution
}

type defaultManagedRuntimeSourceResolver struct{}

// DefaultManagedRuntimeSourceResolver returns the production source resolver.
func DefaultManagedRuntimeSourceResolver() ManagedRuntimeSourceResolver {
	return defaultManagedRuntimeSourceResolver{}
}

func (defaultManagedRuntimeSourceResolver) Resolve(modelName string, resource *interfaces.ResourceConfig) ManagedRuntimeSourceResolution {
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
