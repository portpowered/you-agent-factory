// Package factorydefinition is a transitional compile shim. Production composition
// constructs the lifecycle host through pkg/services/factory_definitions/internal;
// this package forwards to internal/lifecycle for residual tests and DEL-DEF cleanup.
package factorydefinition

import (
	"context"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
)

type (
	Service         = lifecycle.Service
	Host            = lifecycle.Host
	EditableFactory = lifecycle.EditableFactory
)

var ErrCurrentFactoryNotFound = lifecycle.ErrCurrentFactoryNotFound

const (
	SaveModeReplaceCurrent         = lifecycle.SaveModeReplaceCurrent
	SaveModeUpsertNamedAndActivate = lifecycle.SaveModeUpsertNamedAndActivate
)

func New(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.New(host, activationGateway, versionFileSystems...)
}

func NewWithCatalog(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCatalog(host, activationGateway, catalogService, versionFileSystems...)
}

func NewWithCatalogAndPackages(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCatalogAndPackages(
		host,
		activationGateway,
		catalogService,
		packagedCatalog,
		versionFileSystems...,
	)
}

func NewWithCatalogPackagesAndInstallation(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCatalogPackagesAndInstallation(
		host,
		activationGateway,
		catalogService,
		packagedCatalog,
		packagedInstaller,
		versionFileSystems...,
	)
}

func NewWithCompilation(
	host Host,
	compilationService compilationservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCompilation(host, compilationService, versionFileSystems...)
}

func NewWithValidation(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithValidation(
		host,
		activationGateway,
		catalogService,
		validationService,
		versionFileSystems...,
	)
}

func NewWithCatalogPackagesValidationAndInstallation(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCatalogPackagesValidationAndInstallation(
		host,
		activationGateway,
		catalogService,
		validationService,
		packagedCatalog,
		packagedInstaller,
		versionFileSystems...,
	)
}

func NewWithCatalogPackagesValidationInstallationAndAuthoring(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	authoringLayoutService authoringlayout.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCatalogPackagesValidationInstallationAndAuthoring(
		host,
		activationGateway,
		catalogService,
		validationService,
		authoringLayoutService,
		packagedCatalog,
		packagedInstaller,
		versionFileSystems...,
	)
}

func NewWithCatalogPackagesValidationAndDistribution(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	distributionService distributionservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCatalogPackagesValidationAndDistribution(
		host,
		activationGateway,
		catalogService,
		validationService,
		distributionService,
		versionFileSystems...,
	)
}

func NewWithCatalogPackagesValidationDistributionAndAuthoring(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	authoringLayoutService authoringlayout.Service,
	distributionService distributionservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host,
		activationGateway,
		catalogService,
		validationService,
		authoringLayoutService,
		distributionService,
		versionFileSystems...,
	)
}

func ComposeDistributionService(
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	scaffoldInitializer factoryroot.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver,
) distributionservice.Service {
	return lifecycle.ComposeDistributionService(
		packagedCatalog,
		packagedInstaller,
		scaffoldInitializer,
		scaffoldFactoryNameResolver,
	)
}

func NewHost(
	persistRootDir func() string,
	workstationLoader func() factoryroot.WorkstationLoader,
	loadFactory factoryroot.LoadedFactoryLoader,
	readCurrentFactoryPointer func(string) (string, error),
	prepareFactoryLayoutPayload func(string, []byte) (*factoryroot.PreparedFactoryLayoutPayload, error),
	persistNamedFactoryWithPrepared func(string, string, *factoryroot.PreparedFactoryLayoutPayload) (string, error),
	writeCurrentFactoryPointer func(string, string) error,
	preparePortableFactoryConfig factoryroot.PortableFactoryConfigPreparer,
	captureFactorySnapshot factoryroot.FactorySnapshotCapturer,
	currentRuntimeConfig func() factoryroot.LoadedFactorySource,
	workflowID func() string,
	resolveExistingFactoryDir func(string, string) (string, error),
	requireSession func(string) (*factoryroot.DefinitionSession, error),
	sessionRuntimeConfig func(string) (factoryroot.LoadedFactorySource, error),
	sessionFactoryPersistRoot func(*factoryroot.DefinitionSession) string,
	validateEditableFactorySnapshot func(context.Context, *factoryroot.FactorySnapshot) error,
	getCurrentFactorySnapshotForSession func(context.Context, string) (*factoryroot.FactorySnapshot, error),
	replaceFactoryLayoutAtDir func(string, *factoryroot.PreparedFactoryLayoutPayload) (*factoryroot.FactorySplitLayoutReplaceResult, error),
) (Host, error) {
	return lifecycle.NewHost(
		persistRootDir,
		workstationLoader,
		loadFactory,
		readCurrentFactoryPointer,
		prepareFactoryLayoutPayload,
		persistNamedFactoryWithPrepared,
		writeCurrentFactoryPointer,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
		currentRuntimeConfig,
		workflowID,
		resolveExistingFactoryDir,
		requireSession,
		sessionRuntimeConfig,
		sessionFactoryPersistRoot,
		validateEditableFactorySnapshot,
		getCurrentFactorySnapshotForSession,
		replaceFactoryLayoutAtDir,
	)
}

func StubActivationGateway() factoryroot.DefinitionActivationGateway {
	return lifecycle.StubActivationGateway()
}

func CurrentFactorySnapshotForSession(
	ctx context.Context,
	host Host,
	sessionID string,
) (*factoryroot.FactorySnapshot, error) {
	return lifecycle.CurrentFactorySnapshotForSession(ctx, host, sessionID)
}

func SessionFactoryPersistRoot(serviceRootDir string, session *factoryroot.DefinitionSession) string {
	return lifecycle.SessionFactoryPersistRoot(serviceRootDir, session)
}
