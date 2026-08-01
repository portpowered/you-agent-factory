// Package wire is the Factory Definitions service composition boundary.
//
// Wire constructs the singular Factory Definitions root and starts no
// lifecycle components. Session/runtime effects are deliberately absent from
// this bundle; Factory Sessions owns session-scoped operations.
package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// Dependencies are the exact process-edge ports required to construct the
// private Factory Definitions capabilities. Session/runtime ports intentionally
// do not appear here; those operations belong to Factory Sessions.
type Dependencies struct {
	Validator                     factorydefinitions.Validator
	DefinitionValidation          factorydefinitions.DefinitionValidationOperation
	EffectiveDefinitionValidation factorydefinitions.EffectiveDefinitionValidationOperation
	Loader                        *compilationwire.Loader
	ApplySupportedFiles           factorydefinitions.PortableBundledFilesApplier
	ApplyStarterWork              factorydefinitions.FactoryStarterWorkApplier
	NamedPaths                    factoryeffect.NamedPathResolver
	NamedFactoryCatalogFileSystem factoryeffect.NamedFactoryCatalogFileSystem
	ListEffective                 factorydefinitions.EffectiveFactoryCatalogOperation
	PackagedCatalog               factorydefinitions.PackagedFactoryCatalogOperations
	PackagedInstaller             factorydefinitions.PackagedFactoryInstallationOperations
	RequiredToolChecker           factorydefinitions.RequiredToolChecker
	OrchestratorValidator         factorydefinitions.OrchestratorDefinitionValidator
	PortableFileSystem            portablefiles.FileSystem
	DirectoryReplacementStore     factoryeffect.DirectoryReplacementStore
	InvocationPolicyPorts         InvocationPolicyPorts
}

// NewService constructs an inert Factory Definitions root from one dependency
// bundle. Private subservices are composed exactly once and no lifecycle is
// started.
func NewService(deps Dependencies, options ...CompositionOption) (factorydefinitions.Service, error) {
	if err := validateDependencies(deps); err != nil {
		return nil, err
	}

	invocationPolicy, err := invocationPolicyService(deps.InvocationPolicyPorts)
	if err != nil {
		return nil, err
	}
	preparePortableFactoryConfig := PortableFactoryConfigPreparer(
		deps.ApplySupportedFiles,
		deps.ApplyStarterWork,
	)
	snapshotsPortability, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             deps.Loader.LoadSourceFromCanonicalJSON,
		CaptureLoaded:             LoadedFactorySnapshotCapturer(),
		PreparePortable:           preparePortableFactoryConfig,
		DecodeSnapshot:            NewFactorySnapshotJSONDecoder(),
		MaterializePortableFiles:  snapshotsportabilitywire.NewMaterializer(deps.PortableFileSystem),
		ValidateMaterializeWrites: snapshotsportabilitywire.NewWritesValidator(deps.PortableFileSystem),
	})
	if err != nil {
		return nil, err
	}

	compilation, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      deps.Loader.LoadSourceFromCanonicalJSON,
		LoadFromFactoryDir: deps.Loader.LoadSourceFromFactoryDir,
		EncodeFactory:      compilationwire.EncodeFactoryPort(),
	})
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: %w", err)
	}
	if compilation == nil {
		return nil, fmt.Errorf("construct Factory Definitions: compilation subservice rejected its dependencies")
	}

	authoringFS, err := resolveAuthoringLayoutFilesystem(deps.PortableFileSystem)
	if err != nil {
		return nil, err
	}
	pruneRemovedDocs, err := snapshotsportabilitywire.NewPortableBundledDocsPruner(deps.PortableFileSystem)
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	authoringLayout, err := NewAuthoringLayoutService(AuthoringLayoutDependencies{
		Validator: deps.Validator,
		MapInput: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, deps.Loader.LoadSourceFromCanonicalJSON)
		},
		Loader:             deps.Loader,
		MaterializeFiles:   snapshotsportabilitywire.NewMaterializer(deps.PortableFileSystem),
		ValidateWrites:     snapshotsportabilitywire.NewWritesValidator(deps.PortableFileSystem),
		PruneRemovedDocs:   pruneRemovedDocs,
		CopySupportedFiles: snapshotsportabilitywire.NewPortableBundledFilesCopier(deps.PortableFileSystem),
		AuthoredWriterFS:   authoringFS,
		EnsureInbox:        inboxgitkeep.NewLocal(deps.PortableFileSystem),
		PersistenceFS:      authoringFS,
		NamedPaths:         deps.NamedPaths,
		Directories:        deps.DirectoryReplacementStore,
	})
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}

	definitions, err := factorydefinitionsinternal.NewService(factorydefinitionsinternal.Dependencies{
		Validator:                     deps.Validator,
		DefinitionValidation:          deps.DefinitionValidation,
		EffectiveDefinitionValidation: deps.EffectiveDefinitionValidation,
		LoadCanonical:                 deps.Loader.LoadSourceFromCanonicalJSON,
		NamedPaths:                    deps.NamedPaths,
		NamedFactoryCatalogFileSystem: deps.NamedFactoryCatalogFileSystem,
		PackagedCatalog:               deps.PackagedCatalog,
		PackagedInstaller:             deps.PackagedInstaller,
		RequiredToolChecker:           deps.RequiredToolChecker,
		OrchestratorValidator:         deps.OrchestratorValidator,
		AuthoringLayout:               authoringLayout,
		InvocationPolicy:              invocationPolicy,
	}, options...)
	if err != nil {
		return nil, err
	}

	attached, err := factorydefinitionsinternal.AttachEffectiveCatalog(definitions, deps.ListEffective)
	if err != nil {
		return nil, err
	}
	withSnapshots, err := factorydefinitionsinternal.AttachSnapshotsPortability(attached, snapshotsPortability)
	if err != nil {
		return nil, err
	}
	return attachCompilation(withSnapshots, compilation), nil
}

func validateDependencies(deps Dependencies) error {
	if deps.Validator == nil {
		return fmt.Errorf("construct Factory Definitions: validator is required")
	}
	if deps.Loader == nil {
		return fmt.Errorf("construct Factory Definitions: loader is required")
	}
	if deps.ApplySupportedFiles == nil {
		return fmt.Errorf("construct Factory Definitions: portable bundled files applier is required")
	}
	if deps.ApplyStarterWork == nil {
		return fmt.Errorf("construct Factory Definitions: starter Work applier is required")
	}
	if deps.NamedPaths == nil {
		return fmt.Errorf("construct Factory Definitions: named path resolver is required")
	}
	if deps.NamedFactoryCatalogFileSystem == nil {
		return fmt.Errorf("construct Factory Definitions: named Factory catalog filesystem is required")
	}
	if deps.ListEffective == nil {
		return fmt.Errorf("construct Factory Definitions: effective Factory catalog is required")
	}
	if deps.PackagedCatalog.List == nil {
		return fmt.Errorf("construct Factory Definitions: packaged Factory catalog list operation is required")
	}
	if deps.PackagedCatalog.Resolve == nil {
		return fmt.Errorf("construct Factory Definitions: packaged Factory catalog resolve operation is required")
	}
	if deps.PackagedInstaller.Install == nil {
		return fmt.Errorf("construct Factory Definitions: packaged Factory installer is required")
	}
	if deps.RequiredToolChecker == nil {
		return fmt.Errorf("construct Factory Definitions: required tool checker is required")
	}
	if deps.OrchestratorValidator == nil {
		return fmt.Errorf("construct Factory Definitions: orchestrator definition validator is required")
	}
	if deps.PortableFileSystem == nil {
		return fmt.Errorf("construct Factory Definitions: portable filesystem is required")
	}
	if deps.DirectoryReplacementStore == nil {
		return fmt.Errorf("construct Factory Definitions: directory replacement store is required")
	}
	return nil
}

// LocalFactoryNameResolver returns the distribution-owned resolver used by
// scaffold composition. The process root receives this owner adapter through
// the public Factory Definitions wire boundary; it does not construct a
// second scaffold policy.
func LocalFactoryNameResolver() func(string) (string, error) {
	return distributionwire.LocalFactoryNameResolver()
}

// EffectiveFactoryDefinitionNormalizerFromMapper binds the canonical Factory
// config mapper to effective-catalog normalization for wire composition.
func EffectiveFactoryDefinitionNormalizerFromMapper() factorydefinitions.EffectiveFactoryDefinitionNormalizer {
	mapper := factorymapping.NewFactoryConfigMapper()
	return func(
		ctx context.Context,
		candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
	) (*factorydefinitions.FactoryConfig, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		definition, err := mapper.Expand(candidate.Canonical)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return definition, err
	}
}

type compilationAttachedService struct {
	factorydefinitions.Service
	compilation compilationservice.Service
}

func attachCompilation(
	service factorydefinitions.Service,
	compilation compilationservice.Service,
) factorydefinitions.Service {
	if service == nil || compilation == nil {
		return service
	}
	return compilationAttachedService{
		Service:     service,
		compilation: compilation,
	}
}

func (s compilationAttachedService) CompileEffectiveFactorySource(
	ctx context.Context,
	request factorydefinitions.CompileEffectiveFactorySourceRequest,
) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
	return s.compilation.CompileEffectiveFactorySource(ctx, request)
}

type authoringLayoutFilesystem interface {
	portablefiles.FileSystem
	factoryeffect.AuthoredLayoutWriterFileSystem
	factoryeffect.PersistenceFileSystem
}

func resolveAuthoringLayoutFilesystem(portableFileSystem portablefiles.FileSystem) (authoringLayoutFilesystem, error) {
	authoringFS, ok := portableFileSystem.(authoringLayoutFilesystem)
	if !ok {
		return nil, fmt.Errorf(
			"construct Factory Definitions: portable filesystem must support authoring_layout persistence",
		)
	}
	return authoringFS, nil
}
