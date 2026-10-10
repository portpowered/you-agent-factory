// Package factorydefinition retains owner-local characterization fixtures only.
// Production composition uses the completed lifecycle constructor directly.
package factorydefinition

import (
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
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

func StubActivationGateway() factoryroot.DefinitionActivationGateway {
	return stubActivationGateway{}
}

func SessionFactoryPersistRoot(serviceRootDir string, session *factoryroot.DefinitionSession) string {
	return lifecycle.SessionFactoryPersistRoot(serviceRootDir, session)
}
