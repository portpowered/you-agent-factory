// Package internal composes the Factory Definitions root from parent-private
// subservices. Concrete persistence and snapshot packages remain private to
// composition; callers depend on the factory_definitions root contract.
package internal

import (
	"context"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
)

// NewWithAuthoringLayout constructs the public Factory Definitions service
// with the private authoring_layout subservice attached to the CTR-DEF root
// authoring slice.
func NewWithAuthoringLayout(
	sessionHost factoryroot.SessionHost,
	activationGateway factoryroot.DefinitionActivationGateway,
	clock factoryroot.Clock,
	versionFileSystem factoryroot.VersionFileSystem,
	validator factoryroot.Validator,
	loadCanonical factoryroot.CanonicalFactoryJSONLoader,
	loadFactory factoryroot.LoadedFactoryLoader,
	readCurrentFactoryPointer factoryroot.CurrentFactoryPointerReader,
	prepareFactoryLayoutPayload factoryroot.FactoryLayoutPayloadPreparer,
	persistNamedFactory factoryroot.NamedFactoryPersister,
	writeCurrentFactoryPointer factoryroot.CurrentFactoryPointerWriter,
	preparePortableFactoryConfig factoryroot.PortableFactoryConfigPreparer,
	captureFactorySnapshot factoryroot.FactorySnapshotCapturer,
	replaceFactoryLayout factoryroot.FactoryLayoutReplacer,
	namedPaths factoryroot.NamedPathResolver,
	namedFactoryCatalogFileSystem factoryroot.NamedFactoryCatalogFileSystem,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	requiredToolChecker factoryroot.RequiredToolChecker,
	orchestratorValidator factoryroot.OrchestratorDefinitionValidator,
	authoringLayout authoringlayout.Service,
	options ...CompositionOption,
) factoryroot.Service {
	if sessionHost == nil || activationGateway == nil || clock == nil || versionFileSystem == nil ||
		namedPaths == nil || namedFactoryCatalogFileSystem == nil ||
		packagedCatalog.List == nil || packagedCatalog.Resolve == nil ||
		packagedInstaller.Install == nil {
		return nil
	}
	host, err := lifecycle.NewHost(
		sessionHost.PersistRootDir, sessionHost.WorkstationLoader,
		loadFactory,
		readCurrentFactoryPointer,
		func(
			segment string,
			payload []byte,
		) (*factoryroot.PreparedFactoryLayoutPayload, error) {
			return prepareFactoryLayoutPayload(
				context.Background(),
				segment,
				payload,
				validator,
			)
		},
		persistNamedFactory,
		writeCurrentFactoryPointer,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
		sessionHost.CurrentRuntimeConfig, sessionHost.WorkflowID,
		namedPaths.ResolveExistingDir,
		sessionHost.RequireSession, sessionHost.SessionRuntimeConfig,
		sessionHost.SessionFactoryPersistRoot, sessionHost.ValidateEditableFactorySnapshot,
		sessionHost.GetCurrentFactorySnapshotForSession,
		replaceFactoryLayout,
	)
	if err != nil {
		return nil
	}
	// The exact ports were rejected above, which exhausts the catalog
	// constructor's failure cases.
	catalogService, _ := catalogwire.NewService(catalog.Dependencies{
		Paths:      namedPaths,
		FileSystem: namedFactoryCatalogFileSystem,
	})
	operations, _ := validator.(factoryroot.DefinitionValidationOperation)
	effective, _ := validator.(factoryroot.EffectiveDefinitionValidationOperation)
	validationService, _ := validationwire.NewService(validationservice.Dependencies{
		Operations:            operations,
		Effective:             effective,
		LoadCanonical:         loadCanonical,
		RequiredToolChecker:   requiredToolChecker,
		OrchestratorValidator: orchestratorValidator,
	})
	composition := applyCompositionOptions(options)
	distributionService := lifecycle.ComposeDistributionService(
		packagedCatalog,
		packagedInstaller,
		composition.scaffoldInitializer,
		composition.scaffoldFactoryNameResolver,
	)
	if distributionService == nil {
		return nil
	}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host,
		activationGateway,
		catalogService,
		validationService,
		authoringLayout,
		distributionService,
		versionFileSystem,
	)
}
