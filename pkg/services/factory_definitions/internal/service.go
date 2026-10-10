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
	runtimesnapshot "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/runtime_snapshot"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
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
	validationService validationservice.Service,
	runtimeSnapshot runtimesnapshot.Service,
	compilation lifecycle.CompilationOperations,
	loadFactory factoryroot.LoadedFactoryLoader,
	readCurrentFactoryPointer factoryroot.CurrentFactoryPointerReader,
	prepareFactoryLayoutPayload factoryroot.FactoryLayoutPayloadPreparer,
	persistNamedFactory factoryroot.NamedFactoryPersister,
	writeCurrentFactoryPointer factoryroot.CurrentFactoryPointerWriter,
	preparePortableFactoryConfig factoryroot.PortableFactoryConfigPreparer,
	captureFactorySnapshot factoryroot.FactorySnapshotCapturer,
	replaceFactoryLayout factoryroot.FactoryLayoutReplacer,
	namedPaths factoryroot.NamedPathResolver,
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	requiredToolChecker factoryroot.RequiredToolChecker,
	orchestratorValidator factoryroot.OrchestratorDefinitionValidator,
	authoringLayout authoringlayout.Service,
	listEffective factoryroot.EffectiveFactoryCatalogOperation,
	snapshotsPortability snapshotsportability.Service,
	options ...CompositionOption,
) factoryroot.Service {
	if sessionHost == nil || activationGateway == nil || clock == nil || versionFileSystem == nil ||
		namedPaths == nil ||
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
		runtimeSnapshot,
		compilation,
		versionFileSystem,
		listEffective,
		snapshotsPortability,
	)
}
