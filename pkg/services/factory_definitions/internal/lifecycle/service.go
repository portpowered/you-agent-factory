package lifecycle

import (
	"context"
	"fmt"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
)

// ErrCurrentFactoryNotFound reports that no durable current-factory pointer
// could be resolved for canonical current-factory reads.
var ErrCurrentFactoryNotFound = factoryroot.ErrCurrentFactoryNotFound

const (
	SaveModeReplaceCurrent         = factoryroot.SaveModeReplaceCurrent
	SaveModeUpsertNamedAndActivate = factoryroot.SaveModeUpsertNamedAndActivate
)

// Service owns current and named factory definition reads, persistence, and
// activation policy. UnimplementedService keeps the CTR-DEF root slice methods
// assignable until nested IMP-DEF collaborators are wired.
type Service struct {
	nonCatalogDefaults
	catalog.Service
	validationService       validationservice.Service
	authoringLayoutService  authoringlayout.Service
	compilationService      compilationservice.Service
	distributionService     distributionservice.Service
	invocationPolicyService invocationpolicyservice.Service
}

type nonCatalogDefaults interface {
	ListEffectiveFactories(context.Context, factoryroot.ListEffectiveFactoriesRequest) (factoryroot.ListEffectiveFactoriesResult, error)
	PrepareFactoryLayout(context.Context, factoryroot.PrepareFactoryLayoutRequest) (factoryroot.PrepareFactoryLayoutResult, error)
	FlattenFactoryLayout(context.Context, factoryroot.FlattenFactoryLayoutRequest) (factoryroot.FlattenFactoryLayoutResult, error)
	ExpandFactoryLayout(context.Context, factoryroot.ExpandFactoryLayoutRequest) (factoryroot.ExpandFactoryLayoutResult, error)
	CreateNamedFactory(context.Context, factoryroot.CreateNamedFactoryRequest) (factoryroot.CreateNamedFactoryResult, error)
	ReplaceNamedFactory(context.Context, factoryroot.ReplaceNamedFactoryRequest) (factoryroot.ReplaceNamedFactoryResult, error)
	ReplaceFactoryLayoutAtDir(context.Context, factoryroot.ReplaceFactoryLayoutAtDirRequest) (factoryroot.ReplaceFactoryLayoutAtDirResult, error)
	CompileEffectiveFactorySource(context.Context, factoryroot.CompileEffectiveFactorySourceRequest) (factoryroot.CompileEffectiveFactorySourceResult, error)
	ValidateStructuralFactoryDefinition(context.Context, factoryroot.ValidateStructuralFactoryDefinitionRequest) (factoryroot.ValidateStructuralFactoryDefinitionResult, error)
	ValidateEffectiveFactoryDefinition(context.Context, factoryroot.ValidateEffectiveFactoryDefinitionRequest) (factoryroot.ValidateEffectiveFactoryDefinitionResult, error)
	CaptureFactorySnapshot(context.Context, factoryroot.CaptureFactorySnapshotRequest) (factoryroot.CaptureFactorySnapshotResult, error)
	PrepareFactorySnapshotImport(context.Context, factoryroot.PrepareFactorySnapshotImportRequest) (factoryroot.PrepareFactorySnapshotImportResult, error)
	MaterializeFactorySnapshot(context.Context, factoryroot.MaterializeFactorySnapshotRequest) (factoryroot.MaterializeFactorySnapshotResult, error)
	ListBuiltInPackagedFactories(context.Context, factoryroot.ListBuiltInPackagedFactoriesRequest) (factoryroot.ListBuiltInPackagedFactoriesResult, error)
	ResolveBuiltInPackagedFactory(context.Context, factoryroot.ResolveBuiltInPackagedFactoryRequest) (factoryroot.ResolveBuiltInPackagedFactoryResult, error)
	InstallPackagedFactory(context.Context, factoryroot.InstallPackagedFactoryRequest) (factoryroot.InstallPackagedFactoryResult, error)
	CreateFactoryScaffold(context.Context, factoryroot.CreateFactoryScaffoldRequest) (factoryroot.CreateFactoryScaffoldResult, error)
}

// New constructs the private Definitions lifecycle facade. External effects
// are owned by the seven capability services and are supplied through their
// owner-local wire packages.
func New() *Service {
	return &Service{
		nonCatalogDefaults: factoryroot.UnimplementedService{},
		Service:            factoryroot.UnimplementedService{},
	}
}

// NewWithCatalog constructs the Definitions root collaborator with private
// catalog ownership for the CTR-DEF catalog slice.
func NewWithCatalog(catalogService catalog.Service) *Service {
	service := New()
	service.Service = catalogService
	return service
}

// NewWithCatalogAndPackages constructs the Definitions root collaborator with
// both persisted and embedded catalog operations routed through Distribution.
func NewWithCatalogAndPackages(
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
) *Service {
	service := NewWithCatalog(catalogService)
	service.distributionService = ComposeDistributionService(
		packagedCatalog,
		factoryroot.PackagedFactoryInstallationOperations{},
		nil,
		nil,
	)
	return service
}

// NewWithCatalogPackagesAndInstallation constructs the complete Definitions
// root collaborator for catalog selection and canonical packaged installation.
func NewWithCatalogPackagesAndInstallation(
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
) *Service {
	service := NewWithCatalog(catalogService)
	service.distributionService = ComposeDistributionService(
		packagedCatalog,
		packagedInstaller,
		nil,
		nil,
	)
	return service
}

// NewWithCompilation constructs the Definitions root collaborator with private
// compilation ownership for the CTR-DEF compile slice.
func NewWithCompilation(compilationService compilationservice.Service) *Service {
	service := New()
	service.compilationService = compilationService
	return service
}

// NewWithValidation constructs the Definitions root collaborator with private
// validation ownership for the CTR-DEF validate slice.
func NewWithValidation(
	catalogService catalog.Service,
	validationService validationservice.Service,
) *Service {
	service := NewWithCatalog(catalogService)
	service.validationService = validationService
	return service
}

// NewWithCatalogPackagesValidationAndInstallation constructs the complete
// Definitions root collaborator with catalog, validation, and packaged
// installation ownership routed through Distribution.
func NewWithCatalogPackagesValidationAndInstallation(
	catalogService catalog.Service,
	validationService validationservice.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
) *Service {
	return NewWithCatalogPackagesValidationInstallationAndAuthoring(
		catalogService,
		validationService,
		nil,
		packagedCatalog,
		packagedInstaller,
	)
}

// NewWithCatalogPackagesValidationInstallationAndAuthoring constructs the
// complete Definitions root collaborator with catalog, validation, packaged
// installation, and private authoring_layout ownership.
func NewWithCatalogPackagesValidationInstallationAndAuthoring(
	catalogService catalog.Service,
	validationService validationservice.Service,
	authoringLayoutService authoringlayout.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
) *Service {
	service := NewWithCatalogPackagesAndInstallation(
		catalogService,
		packagedCatalog,
		packagedInstaller,
	)
	service.validationService = validationService
	service.authoringLayoutService = authoringLayoutService
	return service
}

// NewWithCatalogPackagesValidationAndDistribution constructs the complete
// Definitions root collaborator with catalog, validation, and private
// Distribution ownership for the CTR-DEF distribute slice.
func NewWithCatalogPackagesValidationAndDistribution(
	catalogService catalog.Service,
	validationService validationservice.Service,
	distributionService distributionservice.Service,
) *Service {
	service := NewWithValidation(catalogService, validationService)
	service.distributionService = distributionService
	return service
}

// NewWithCatalogPackagesValidationDistributionAndAuthoring constructs the
// complete Definitions root collaborator with catalog, validation, private
// Distribution ownership, and authoring_layout ownership.
func NewWithCatalogPackagesValidationDistributionAndAuthoring(
	catalogService catalog.Service,
	validationService validationservice.Service,
	authoringLayoutService authoringlayout.Service,
	distributionService distributionservice.Service,
	invocationPolicyService invocationpolicyservice.Service,
) *Service {
	service := NewWithCatalogPackagesValidationAndDistribution(
		catalogService,
		validationService,
		distributionService,
	)
	service.authoringLayoutService = authoringLayoutService
	service.invocationPolicyService = invocationPolicyService
	return service
}

// ComposeDistributionService constructs the private Distribution subservice from
// exact injected distribute ports for Factory Definitions composition.
func ComposeDistributionService(
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
	service, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog:             packagedCatalog,
		PackagedInstaller:           packagedInstaller,
		ScaffoldInitializer:         scaffoldInitializer,
		ScaffoldFactoryNameResolver: scaffoldFactoryNameResolver,
	})
	if err != nil {
		return nil
	}
	return service
}

func (s *Service) CompileEffectiveFactorySource(
	ctx context.Context,
	request factoryroot.CompileEffectiveFactorySourceRequest,
) (factoryroot.CompileEffectiveFactorySourceResult, error) {
	if s == nil || s.compilationService == nil {
		return factoryroot.UnimplementedService{}.CompileEffectiveFactorySource(ctx, request)
	}
	return s.compilationService.CompileEffectiveFactorySource(ctx, request)
}

func (s *Service) ValidateStructuralFactoryDefinition(
	ctx context.Context,
	request factoryroot.ValidateStructuralFactoryDefinitionRequest,
) (factoryroot.ValidateStructuralFactoryDefinitionResult, error) {
	if s == nil || s.validationService == nil {
		return factoryroot.UnimplementedService{}.ValidateStructuralFactoryDefinition(ctx, request)
	}
	return s.validationService.ValidateStructuralFactoryDefinition(ctx, request)
}

func (s *Service) ValidateEffectiveFactoryDefinition(
	ctx context.Context,
	request factoryroot.ValidateEffectiveFactoryDefinitionRequest,
) (factoryroot.ValidateEffectiveFactoryDefinitionResult, error) {
	if s == nil || s.validationService == nil {
		return factoryroot.UnimplementedService{}.ValidateEffectiveFactoryDefinition(ctx, request)
	}
	return s.validationService.ValidateEffectiveFactoryDefinition(ctx, request)
}

func (s *Service) PrepareFactoryLayout(
	ctx context.Context,
	request factoryroot.PrepareFactoryLayoutRequest,
) (factoryroot.PrepareFactoryLayoutResult, error) {
	if s == nil || s.authoringLayoutService == nil {
		return factoryroot.UnimplementedService{}.PrepareFactoryLayout(ctx, request)
	}
	return s.authoringLayoutService.PrepareFactoryLayout(ctx, request)
}

func (s *Service) FlattenFactoryLayout(
	ctx context.Context,
	request factoryroot.FlattenFactoryLayoutRequest,
) (factoryroot.FlattenFactoryLayoutResult, error) {
	if s == nil || s.authoringLayoutService == nil {
		return factoryroot.UnimplementedService{}.FlattenFactoryLayout(ctx, request)
	}
	return s.authoringLayoutService.FlattenFactoryLayout(ctx, request)
}

func (s *Service) ExpandFactoryLayout(
	ctx context.Context,
	request factoryroot.ExpandFactoryLayoutRequest,
) (factoryroot.ExpandFactoryLayoutResult, error) {
	if s == nil || s.authoringLayoutService == nil {
		return factoryroot.UnimplementedService{}.ExpandFactoryLayout(ctx, request)
	}
	return s.authoringLayoutService.ExpandFactoryLayout(ctx, request)
}

func (s *Service) CreateNamedFactory(
	ctx context.Context,
	request factoryroot.CreateNamedFactoryRequest,
) (factoryroot.CreateNamedFactoryResult, error) {
	if s == nil || s.authoringLayoutService == nil {
		return factoryroot.UnimplementedService{}.CreateNamedFactory(ctx, request)
	}
	return s.authoringLayoutService.CreateNamedFactory(ctx, request)
}

func (s *Service) ReplaceNamedFactory(
	ctx context.Context,
	request factoryroot.ReplaceNamedFactoryRequest,
) (factoryroot.ReplaceNamedFactoryResult, error) {
	if s == nil || s.authoringLayoutService == nil {
		return factoryroot.UnimplementedService{}.ReplaceNamedFactory(ctx, request)
	}
	return s.authoringLayoutService.ReplaceNamedFactory(ctx, request)
}

func (s *Service) ReplaceFactoryLayoutAtDir(
	ctx context.Context,
	request factoryroot.ReplaceFactoryLayoutAtDirRequest,
) (factoryroot.ReplaceFactoryLayoutAtDirResult, error) {
	if s == nil || s.authoringLayoutService == nil {
		return factoryroot.UnimplementedService{}.ReplaceFactoryLayoutAtDir(ctx, request)
	}
	return s.authoringLayoutService.ReplaceFactoryLayoutAtDir(ctx, request)
}

func (s *Service) ListBuiltInPackagedFactories(
	ctx context.Context,
	request factoryroot.ListBuiltInPackagedFactoriesRequest,
) (factoryroot.ListBuiltInPackagedFactoriesResult, error) {
	if s == nil || s.distributionService == nil {
		return factoryroot.UnimplementedService{}.ListBuiltInPackagedFactories(ctx, request)
	}
	return s.distributionService.ListBuiltInPackagedFactories(ctx, request)
}

func (s *Service) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request factoryroot.ResolveBuiltInPackagedFactoryRequest,
) (factoryroot.ResolveBuiltInPackagedFactoryResult, error) {
	if s == nil || s.distributionService == nil {
		return factoryroot.UnimplementedService{}.ResolveBuiltInPackagedFactory(ctx, request)
	}
	return s.distributionService.ResolveBuiltInPackagedFactory(ctx, request)
}

func (s *Service) InstallPackagedFactory(
	ctx context.Context,
	request factoryroot.InstallPackagedFactoryRequest,
) (factoryroot.InstallPackagedFactoryResult, error) {
	if s == nil || s.distributionService == nil {
		return factoryroot.UnimplementedService{}.InstallPackagedFactory(ctx, request)
	}
	return s.distributionService.InstallPackagedFactory(ctx, request)
}

func (s *Service) CreateFactoryScaffold(
	ctx context.Context,
	request factoryroot.CreateFactoryScaffoldRequest,
) (factoryroot.CreateFactoryScaffoldResult, error) {
	if s == nil || s.distributionService == nil {
		return factoryroot.UnimplementedService{}.CreateFactoryScaffold(ctx, request)
	}
	return s.distributionService.CreateFactoryScaffold(ctx, request)
}
