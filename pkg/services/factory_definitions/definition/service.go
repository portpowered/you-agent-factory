package factorydefinition

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
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
	validationService        validationservice.Service
	authoringLayoutService   authoringlayout.Service
	compilationService       compilationservice.Service
	host                     Host
	activationGateway        factoryroot.DefinitionActivationGateway
	versionFileSystem        factoryroot.VersionFileSystem
	distributionService      distributionservice.Service
}

type nonCatalogDefaults interface {
	ListEffectiveFactories(context.Context, factoryroot.ListEffectiveFactoriesRequest) (factoryroot.ListEffectiveFactoriesResult, error)
	PrepareFactoryLayout(context.Context, factoryroot.PrepareFactoryLayoutRequest) (factoryroot.PrepareFactoryLayoutResult, error)
	FlattenFactoryLayout(context.Context, factoryroot.FlattenFactoryLayoutRequest) (factoryroot.FlattenFactoryLayoutResult, error)
	ExpandFactoryLayout(context.Context, factoryroot.ExpandFactoryLayoutRequest) (factoryroot.ExpandFactoryLayoutResult, error)
	CreateNamedFactory(context.Context, factoryroot.CreateNamedFactoryRequest) (factoryroot.CreateNamedFactoryResult, error)
	ReplaceNamedFactory(context.Context, factoryroot.ReplaceNamedFactoryRequest) (factoryroot.ReplaceNamedFactoryResult, error)
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

// New constructs a factory-definition read collaborator with explicit dependencies.
func New(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	return &Service{
		nonCatalogDefaults: factoryroot.UnimplementedService{},
		Service:            factoryroot.UnimplementedService{},
		host:               host,
		activationGateway:  activationGateway,
		versionFileSystem:  versionFileSystem,
	}
}

// NewWithCatalog constructs the Definitions root collaborator with private
// catalog ownership for the CTR-DEF catalog slice.
func NewWithCatalog(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	service := New(host, activationGateway, versionFileSystems...)
	service.Service = catalogService
	return service
}

// NewWithCatalogAndPackages constructs the Definitions root collaborator with
// both persisted and embedded catalog operations routed through Distribution.
func NewWithCatalogAndPackages(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	service := NewWithCatalog(host, activationGateway, catalogService, versionFileSystems...)
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
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	service := NewWithCatalog(host, activationGateway, catalogService, versionFileSystems...)
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
func NewWithCompilation(
	host Host,
	compilationService compilationservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	service := New(host, StubActivationGateway(), versionFileSystems...)
	service.compilationService = compilationService
	return service
}

// NewWithValidation constructs the Definitions root collaborator with private
// validation ownership for the CTR-DEF validate slice.
func NewWithValidation(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	service := NewWithCatalog(host, activationGateway, catalogService, versionFileSystems...)
	service.validationService = validationService
	return service
}

// NewWithCatalogPackagesValidationAndInstallation constructs the complete
// Definitions root collaborator with catalog, validation, and packaged
// installation ownership routed through Distribution.
func NewWithCatalogPackagesValidationAndInstallation(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	packagedCatalog factoryroot.PackagedFactoryCatalogOperations,
	packagedInstaller factoryroot.PackagedFactoryInstallationOperations,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	return NewWithCatalogPackagesValidationInstallationAndAuthoring(
		host,
		activationGateway,
		catalogService,
		validationService,
		nil,
		packagedCatalog,
		packagedInstaller,
		versionFileSystems...,
	)
}

// NewWithCatalogPackagesValidationInstallationAndAuthoring constructs the
// complete Definitions root collaborator with catalog, validation, packaged
// installation, and private authoring_layout ownership.
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
	service := NewWithCatalogPackagesAndInstallation(
		host,
		activationGateway,
		catalogService,
		packagedCatalog,
		packagedInstaller,
		versionFileSystems...,
	)
	service.validationService = validationService
	service.authoringLayoutService = authoringLayoutService
	return service
}

// NewWithCatalogPackagesValidationAndDistribution constructs the complete
// Definitions root collaborator with catalog, validation, and private
// Distribution ownership for the CTR-DEF distribute slice.
func NewWithCatalogPackagesValidationAndDistribution(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	distributionService distributionservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	service := NewWithValidation(host, activationGateway, catalogService, validationService, versionFileSystems...)
	service.distributionService = distributionService
	return service
}

// NewWithCatalogPackagesValidationDistributionAndAuthoring constructs the
// complete Definitions root collaborator with catalog, validation, private
// Distribution ownership, and authoring_layout ownership.
func NewWithCatalogPackagesValidationDistributionAndAuthoring(
	host Host,
	activationGateway factoryroot.DefinitionActivationGateway,
	catalogService catalog.Service,
	validationService validationservice.Service,
	authoringLayoutService authoringlayout.Service,
	distributionService distributionservice.Service,
	versionFileSystems ...factoryroot.VersionFileSystem,
) *Service {
	service := NewWithCatalogPackagesValidationAndDistribution(
		host,
		activationGateway,
		catalogService,
		validationService,
		distributionService,
		versionFileSystems...,
	)
	service.authoringLayoutService = authoringLayoutService
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

// Save coordinates the session-scoped definition submission pipeline for the
// requested persistence and activation policy.
func (s *Service) Save(ctx context.Context, sessionID string, mode factoryroot.SaveMode, request EditableFactory) (EditableFactory, error) {
	if s == nil || s.host == nil {
		return EditableFactory{}, fmt.Errorf("factory definition service is required")
	}
	if mode == SaveModeUpsertNamedAndActivate {
		return s.SaveUpsertNamedSnapshotAndActivateForSession(ctx, sessionID, request)
	}
	snapshot, err := s.SaveReplaceCurrentSnapshotForSession(ctx, sessionID, request)
	if err != nil {
		return EditableFactory{}, err
	}
	return EditableFactory{Snapshot: snapshot}, nil
}

// GetCurrentNamedFactory returns the durable current named-factory snapshot
// resolved from the persisted pointer and canonical on-disk layout.
func (s *Service) GetCurrentNamedFactory(context.Context) (*factoryroot.FactorySnapshot, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	rootDir := s.host.PersistRootDir()
	name, err := readCurrentFactoryPointerFromHost(s.host, rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := s.host.CurrentRuntimeConfig()
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return s.serializeNamedFactory(factoryroot.DefaultCurrentFactoryName, currentRuntime, true)
			}
			return nil, ErrCurrentFactoryNotFound
		}
		return nil, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := resolveExistingFactoryDirFromHost(s.host, rootDir, name)
	if err != nil {
		return nil, fmt.Errorf("resolve current factory %q: %w", name, err)
	}
	current, err := loadFactoryFromHost(s.host, factoryDir, s.host.WorkstationLoader())
	if err != nil {
		return nil, fmt.Errorf("load current factory %q: %w", name, err)
	}
	return s.serializeNamedFactory(name, current, true)
}

// GetCurrentFactoryForSession returns the editable Factory snapshot and durable
// optimistic-concurrency version for one live session.
func (s *Service) GetCurrentFactoryForSession(_ context.Context, sessionID string) (EditableFactory, error) {
	if s == nil || s.host == nil {
		return EditableFactory{}, fmt.Errorf("factory definition service is required")
	}
	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	runtimeCfg, err := s.host.SessionRuntimeConfig(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	rootDir := sessionFactoryRootDir(s.host.PersistRootDir(), session)
	factoryName := factoryName(rootDir, runtimeCfg)
	versionRootDir := rootDir
	if persistRoot := s.host.SessionFactoryPersistRoot(session); persistRoot != "" {
		if pointerName, pointerErr := readCurrentFactoryPointerFromHost(s.host, persistRoot); pointerErr == nil {
			if session.IsDefault || pointerName == factoryName {
				factoryName = pointerName
			}
		}
		if sameFactoryDir(persistRoot, rootDir) {
			versionRootDir = persistRoot
		}
	}
	snapshot, err := s.serializeNamedFactory(factoryName, runtimeCfg, true)
	if err != nil {
		return EditableFactory{}, err
	}
	version, err := s.CurrentFactoryDefinitionVersionAtRoot(versionRootDir, factoryName)
	if err != nil {
		return EditableFactory{}, err
	}
	return EditableFactory{Name: factoryName, Snapshot: snapshot, Version: &version}, nil
}

// CurrentFactorySnapshotForSession reads one editable session definition
// without requiring a Definition service to refer back to itself through its
// host adapter.
func CurrentFactorySnapshotForSession(
	ctx context.Context,
	host Host,
	sessionID string,
) (*factoryroot.FactorySnapshot, error) {
	current, err := New(host, StubActivationGateway()).GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if current.Snapshot == nil {
		return nil, fmt.Errorf("current factory snapshot is unavailable")
	}
	return current.Snapshot, nil
}

// CurrentFactoryDefinitionVersionAtRoot returns optimistic-concurrency metadata.
func (s *Service) CurrentFactoryDefinitionVersionAtRoot(rootDir, name string) (factoryroot.FactoryVersion, error) {
	if s == nil {
		return factoryroot.FactoryVersion{}, fmt.Errorf("factory definition service is required")
	}
	return s.currentFactoryDefinitionVersionAtRoot(rootDir, name)
}

// SerializeNamedFactory returns the canonical editable Factory snapshot.
func (s *Service) SerializeNamedFactory(name string, current factoryroot.LoadedFactorySource, inlineBundledFiles bool) (*factoryroot.FactorySnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	return s.serializeNamedFactory(name, current, inlineBundledFiles)
}

func sameFactoryDir(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func sessionFactoryRootDir(
	serviceRootDir string,
	session *factoryroot.DefinitionSession,
) string {
	if session == nil {
		return ""
	}
	rootDir := session.FolderPath
	if session.FolderPath == "" {
		return rootDir
	}
	if session.FactoryDir == "" || !sameFactoryDir(session.FactoryDir, session.FolderPath) {
		return rootDir
	}
	serviceRoot := filepath.Clean(serviceRootDir)
	if serviceRoot != "" && filepath.Dir(session.FactoryDir) == serviceRoot {
		return serviceRoot
	}
	return rootDir
}

func factoryName(
	rootDir string,
	runtimeCfg factoryroot.RuntimeConfigLookup,
) string {
	if runtimeCfg == nil {
		return factoryroot.DefaultCurrentFactoryName
	}
	factoryDir := runtimeCfg.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if sameFactoryDir(factoryDir, cleanRoot) {
		return factoryroot.DefaultCurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		name := filepath.Base(factoryDir)
		if _, err := namedfactorypath.PathSegments(name); err == nil {
			return name
		}
	}
	cfg := runtimeCfg.FactoryConfig()
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Name); name != "" {
			return name
		}
		if project := strings.TrimSpace(cfg.Project); project != "" {
			return project
		}
	}
	return "factory"
}

// SessionFactoryPersistRoot resolves the on-disk factory root for session-scoped definition persistence.
func SessionFactoryPersistRoot(serviceRootDir string, session *factoryroot.DefinitionSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return sessionFactoryRootDir(serviceRootDir, session)
}
