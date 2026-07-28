// Package service constructs the Factory Definitions service implementation.
// Concrete persistence and snapshot packages remain private to composition;
// callers depend on the factory_definitions root contract.
package service

import (
	"context"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
)

// New constructs the public Factory Definitions service.
func New(
	sessionHost factoryroot.SessionHost,
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
	options ...CompositionOption,
) factoryroot.Service {
	return NewWithAuthoringLayout(
		sessionHost,
		clock,
		versionFileSystem,
		validator,
		loadCanonical,
		loadFactory,
		readCurrentFactoryPointer,
		prepareFactoryLayoutPayload,
		persistNamedFactory,
		writeCurrentFactoryPointer,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
		replaceFactoryLayout,
		namedPaths,
		namedFactoryCatalogFileSystem,
		packagedCatalog,
		packagedInstaller,
		requiredToolChecker,
		orchestratorValidator,
		nil,
		options...,
	)
}

// NewWithAuthoringLayout constructs the public Factory Definitions service
// with the private authoring_layout subservice attached to the CTR-DEF root
// authoring slice.
func NewWithAuthoringLayout(
	sessionHost factoryroot.SessionHost,
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
	if sessionHost == nil || clock == nil || versionFileSystem == nil ||
		namedPaths == nil || namedFactoryCatalogFileSystem == nil ||
		packagedCatalog.List == nil || packagedCatalog.Resolve == nil ||
		packagedInstaller.Install == nil {
		return nil
	}
	host, err := factorydefinition.NewHost(
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
		sessionHost.GetCurrentFactorySnapshotForSession, sessionHost.WithActivationLock,
		sessionHost.RequireIdleRuntimeForSession, sessionHost.ActivateSessionEditableFactory,
		replaceFactoryLayout,
		clock.Now,
		sessionHost.RunSessionID, sessionHost.SessionForActivation,
		sessionHost.NamedFactoryActivationPaths, sessionHost.RequireIdleBeforeNamedFactoryActivation,
		sessionHost.SwapPersistedNamedFactoryRuntime,
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
	distributionService := factorydefinition.ComposeDistributionService(
		packagedCatalog,
		packagedInstaller,
		composition.scaffoldInitializer,
		composition.scaffoldFactoryNameResolver,
	)
	if distributionService == nil {
		return nil
	}
	definitions := factorydefinition.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host,
		catalogService,
		validationService,
		authoringLayout,
		distributionService,
		versionFileSystem,
	)
	sessionHost.AttachFactoryDefinitions(definitions)
	return definitions
}
