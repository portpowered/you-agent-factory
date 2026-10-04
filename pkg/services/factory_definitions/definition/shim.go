// Package factorydefinition is a transitional compile shim. Production composition
// constructs the lifecycle host through pkg/services/factory_definitions/internal;
// this package forwards to internal/lifecycle for residual tests and DEL-DEF cleanup.
package factorydefinition

import (
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
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
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host,
		activationGateway,
		factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{},
		nil,
		factoryroot.UnimplementedService{},
		versionFileSystem,
		factoryroot.UnimplementedService{}.ListEffectiveFactories,
	)
}

func NewWithCatalog(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host,
		activationGateway,
		catalogService,
		factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{},
		nil,
		factoryroot.UnimplementedService{},
		versionFileSystem,
		factoryroot.UnimplementedService{}.ListEffectiveFactories,
	)
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
	compilationService lifecycle.CompilationOperations,
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
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host,
		activationGateway,
		catalogService,
		validationService,
		factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{},
		nil,
		factoryroot.UnimplementedService{},
		versionFileSystem,
		factoryroot.UnimplementedService{}.ListEffectiveFactories,
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

func StubActivationGateway() factoryroot.DefinitionActivationGateway {
	return lifecycle.StubActivationGateway()
}

func SessionFactoryPersistRoot(serviceRootDir string, session *factoryroot.DefinitionSession) string {
	return lifecycle.SessionFactoryPersistRoot(serviceRootDir, session)
}
