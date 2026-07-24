// Package wire constructs the owner-private Factory Session invocation
// capability from explicit runtime and effect ports.
package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	legacyopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/invocation"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// NewOperation is the service-owned construction entrypoint for process-scoped
// one-shot invocation lifecycle. The implementation remains private to Factory
// Sessions and root Wire depends only on this service-local constructor.
func NewOperation(
	openRuntime *runtimeopening.Factory,
	effects runtimeopening.ExternalEffects,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter models.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (roles.InvocationOperation, error) {
	return legacyopening.NewOperation(
		openRuntime,
		effects,
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
