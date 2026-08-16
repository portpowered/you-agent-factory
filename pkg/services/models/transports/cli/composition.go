package cli

import (
	"context"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

// CompositionModelsRoot exposes the accepted Models root from injected
// composition collaborators when the invocation operation can supply it.
type CompositionModelsRoot interface {
	CompositionModelsRoot() modelinference.Service
}

// CompositionOpenCatalogScope exposes process-scoped catalog opening from
// injected composition collaborators for local list/inspect/pull behavior.
type CompositionOpenCatalogScope interface {
	CompositionOpenCatalogScope(context.Context) (InvokeRuntimeScope, error)
}

// CompositionInvokeScopeOpener exposes invoke-scope opening from injected
// composition collaborators when the invocation operation can supply it.
type CompositionInvokeScopeOpener interface {
	CompositionOpenInvokeScope(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
}

// CompositionScopeProvider is the Models transport's explicit composition
// port. Wire supplies the Models root and scope operations; the transport does
// not discover a collaborator through a Sessions operation value.
type CompositionScopeProvider interface {
	CompositionModelsRoot
	CompositionOpenCatalogScope
	CompositionInvokeScopeOpener
}

// BindService returns the composition-facing Models CLI adapter Service
// constructed from the accepted Models root. Wire and other composition roots
// inject the returned Service without constructing adapter behavior at the
// composition boundary.
func BindService(cfg Config) Service {
	return NewService(cfg)
}

// ConfigFromComposition maps composition-stable collaborator shapes onto the
// owned adapter Config without requiring composition roots to construct the
// Service directly.
func ConfigFromComposition(
	httpProtocol clihttp.Protocol,
	invocation InvocationOperation,
	providers ...CompositionScopeProvider,
) Config {
	cfg := Config{HTTP: httpProtocol}
	if invocation == nil {
		return cfg
	}
	cfg.Artifacts = compositionArtifactExporter{invocation: invocation}
	var provider CompositionScopeProvider
	if len(providers) > 0 {
		provider = providers[0]
	} else if candidate, ok := invocation.(CompositionScopeProvider); ok {
		// Keep direct transport composition usable for small embedded callers
		// that already provide the Models transport port. The canonical process
		// graph passes this port explicitly from Wire.
		provider = candidate
	}
	if provider != nil {
		cfg.Models = provider.CompositionModelsRoot()
		cfg.OpenCatalogScope = provider.CompositionOpenCatalogScope
		cfg.OpenInvokeScope = provider.CompositionOpenInvokeScope
		return cfg
	}
	// Preserve the independently injectable composition shapes used by
	// embedded callers. The process graph above supplies the aggregate port;
	// these fallbacks do not inspect or depend on Factory Sessions contracts.
	if root, ok := invocation.(CompositionModelsRoot); ok {
		cfg.Models = root.CompositionModelsRoot()
	}
	if opener, ok := invocation.(CompositionOpenCatalogScope); ok {
		cfg.OpenCatalogScope = opener.CompositionOpenCatalogScope
	}
	if opener, ok := invocation.(CompositionInvokeScopeOpener); ok {
		cfg.OpenInvokeScope = opener.CompositionOpenInvokeScope
	}
	return cfg
}

type compositionArtifactExporter struct {
	invocation InvocationOperation
}

func (exporter compositionArtifactExporter) ExportInvocationArtifact(sourcePath, destinationPath string) error {
	if exporter.invocation == nil {
		return nil
	}
	return exporter.invocation.ExportModelInvocationArtifact(sourcePath, destinationPath)
}
