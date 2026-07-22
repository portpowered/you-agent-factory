// Package wire constructs the owner-private Factory Session invocation
// capability from explicit runtime and effect ports.
package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimeopening"
	legacyopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimeopening/invocation"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// NewOperation is the service-owned construction entrypoint for process-scoped
// one-shot invocation lifecycle. The implementation remains private to Factory
// Sessions and root Wire depends only on this service-local constructor.
func NewOperation(
	openRuntime *runtimeopening.Factory,
	edges serviceedges.Edges,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter models.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessions.InvocationOperation, error) {
	return legacyopening.NewOperation(
		openRuntime,
		edges,
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
	)
}

// New constructs an inert invocation service and exposes only its contract.
func New(deps invocationservice.Dependencies) (invocationservice.Service, error) {
	service, err := internalservice.New(deps)
	if err != nil {
		return nil, err
	}
	return service, nil
}
