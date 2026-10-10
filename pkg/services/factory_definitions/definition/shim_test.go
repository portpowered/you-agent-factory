// Package factorydefinition retains owner-local characterization fixtures only.
// Production composition uses the completed lifecycle constructor directly.
package factorydefinition

import (
	"context"
	"fmt"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
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
		factoryroot.UnimplementedService{},
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
		factoryroot.UnimplementedService{},
	)
}

func NewWithCatalogAndPackages(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return NewWithCatalogPackagesValidationInstallationAndAuthoring(
		host, activationGateway, catalogService, factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{}, packagedCatalog,
		factoryroot.PackagedFactoryInstallationOperations{}, versionFileSystems...,
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
	return NewWithCatalogPackagesValidationInstallationAndAuthoring(
		host, activationGateway, catalogService, factoryroot.UnimplementedService{},
		factoryroot.UnimplementedService{}, packagedCatalog, packagedInstaller, versionFileSystems...,
	)
}

func NewWithCompilation(
	host Host,
	compilationService lifecycle.CompilationOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	disabled := factoryroot.UnimplementedService{}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host, StubActivationGateway(), disabled, disabled, disabled, disabled,
		nil, compilationService, versionFileSystem, disabled.ListEffectiveFactories, disabled,
	)
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
		factoryroot.UnimplementedService{},
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
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	distribution := testDistributionService(packagedCatalog, packagedInstaller, nil, nil)
	disabled := factoryroot.UnimplementedService{}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host, activationGateway, catalogService, validationService, authoringLayoutService,
		distribution, nil, disabled, versionFileSystem, disabled.ListEffectiveFactories, disabled,
	)
}

func StubActivationGateway() factoryroot.DefinitionActivationGateway {
	return stubActivationGateway{}
}

func SessionFactoryPersistRoot(serviceRootDir string, session *factoryroot.DefinitionSession) string {
	return lifecycle.SessionFactoryPersistRoot(serviceRootDir, session)
}

// testDistributionService constructs the private Distribution subservice from
// exact injected distribute ports for Factory Definitions composition.
func testDistributionService(
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	scaffoldInitializer factoryroot.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver,
) distributionservice.Service {
	if packagedInstaller.Install == nil {
		packagedInstaller = factoryroot.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factoryroot.PackagedFactoryInstallParams,
			) (factoryroot.PackagedFactoryInstallResult, error) {
				return factoryroot.PackagedFactoryInstallResult{},
					fmt.Errorf("%w: packaged Factory installation collaborator is required",
						factoryroot.ErrFactoryDistributeFailed)
			},
		}
	}
	service, err := distributionwire.NewService(
		packagedCatalog,
		packagedInstaller,
		scaffoldInitializer,
		scaffoldFactoryNameResolver,
	)
	if err != nil {
		return nil
	}
	return service
}
