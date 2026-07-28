// Package wire is the Factory Definitions service composition boundary.
//
// Wire performs construction only, returns the singular factorydefinitions.Service
// root interface, and starts no lifecycle components. Parent-private catalog Wire
// and the accepted service assembly stay inside the owner boundary; peers depend on
// Service rather than Definition owner internals or construction ports.
package wire

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	internalportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/canonical"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitymaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// NewService constructs an inert Factory Definitions root from construction and
// process-edge ports. It composes the accepted root through parent-private catalog
// Wire and the accepted service assembly without publishing owner types on the
// returned peer surface.
func NewService(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
	options ...CompositionOption,
) (factorydefinitions.Service, error) {
	if err := validateDependencies(
		sessionHost,
		activationGateway,
		validator,
		persistence,
		loader,
		applySupportedFiles,
		applyStarterWork,
		namedPaths,
		namedFactoryCatalogFileSystem,
		clock,
		versionFileSystem,
		listEffective,
		packagedCatalog,
		packagedInstaller,
		requiredToolChecker,
		orchestratorValidator,
		portableFileSystem,
		directoryReplacementStore,
	); err != nil {
		return nil, err
	}

	preparePortableFactoryConfig := PortableFactoryConfigPreparer(
		applySupportedFiles,
		applyStarterWork,
	)
	captureFactorySnapshot := FactorySnapshotCapturer()
	snapshotsPortability, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             loader.LoadSourceFromCanonicalJSON,
		CaptureLoaded:             LoadedFactorySnapshotCapturer(),
		PreparePortable:           preparePortableFactoryConfig,
		DecodeSnapshot:            FactorySnapshotJSONDecoder(),
		MaterializePortableFiles:  snapshotsportabilitymaterialize.NewMaterializer(portableFileSystem),
		ValidateMaterializeWrites: snapshotsportabilitymaterialize.NewWritesValidator(portableFileSystem),
	})
	if err != nil {
		return nil, err
	}

	compilation, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loader.LoadSourceFromCanonicalJSON,
		LoadFromFactoryDir: loader.LoadSourceFromFactoryDir,
		EncodeFactory:      compilationcanonical.EncodeFactoryPort(),
	})
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: %w", err)
	}
	if compilation == nil {
		return nil, fmt.Errorf("construct Factory Definitions: compilation subservice rejected its dependencies")
	}

	authoringFS, err := resolveAuthoringLayoutFilesystem(portableFileSystem)
	if err != nil {
		return nil, err
	}
	pruneRemovedDocs, err := internalportableconfig.NewPortableBundledDocsPruner(portableFileSystem)
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	authoringLayout, err := NewAuthoringLayoutService(AuthoringLayoutDependencies{
		Validator: validator,
		MapInput: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, loader.LoadSourceFromCanonicalJSON)
		},
		Loader:             loader,
		MaterializeFiles:   internalportableconfig.NewMaterializer(portableFileSystem),
		ValidateWrites:     internalportableconfig.NewWritesValidator(portableFileSystem),
		PruneRemovedDocs:   pruneRemovedDocs,
		CopySupportedFiles: internalportableconfig.NewFilesCopier(portableFileSystem),
		AuthoredWriterFS:   authoringFS,
		EnsureInbox:        inboxgitkeep.NewLocal(portableFileSystem),
		PersistenceFS:      authoringFS,
		NamedPaths:         namedPaths,
		Directories:        directoryReplacementStore,
	})
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}

	definitions := factorydefinitionsinternal.NewWithAuthoringLayout(
		sessionHost,
		activationGateway,
		clock,
		versionFileSystem,
		validator,
		loader.LoadSourceFromCanonicalJSON,
		func(
			factoryDir string,
			workstationLoader factorydefinitions.WorkstationLoader,
		) (factorydefinitions.MutableLoadedFactorySource, error) {
			return loader.LoadRuntimeSource(factoryDir, workstationLoader)
		},
		namedPaths.ReadCurrentPointer,
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			_ factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return persistence.PrepareFactoryLayout(ctx, segment, payload)
		},
		persistence.CreateNamedFactory,
		namedPaths.WriteCurrentPointer,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
		persistence.ReplaceFactoryLayout,
		namedPaths,
		namedFactoryCatalogFileSystem,
		packagedCatalog,
		packagedInstaller,
		requiredToolChecker,
		orchestratorValidator,
		authoringLayout,
		options...,
	)
	if definitions == nil {
		return nil, fmt.Errorf("construct Factory Definitions: implementation rejected its dependencies")
	}

	attached, err := factorydefinitionsinternal.AttachEffectiveCatalog(definitions, listEffective)
	if err != nil {
		return nil, err
	}
	if attached == nil {
		return nil, fmt.Errorf("construct Factory Definitions: effective catalog attachment rejected its dependencies")
	}
	withSnapshots, err := factorydefinitionsinternal.AttachSnapshotsPortability(attached, snapshotsPortability)
	if err != nil {
		return nil, err
	}
	if withSnapshots == nil {
		return nil, fmt.Errorf("construct Factory Definitions: snapshots portability attachment rejected its dependencies")
	}
	return attachCompilation(withSnapshots, compilation), nil
}

func validateDependencies(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
) error {
	if sessionHost == nil {
		return fmt.Errorf("construct Factory Definitions: session host is required")
	}
	if activationGateway == nil {
		return fmt.Errorf("construct Factory Definitions: activation gateway is required")
	}
	if validator == nil {
		return fmt.Errorf("construct Factory Definitions: validator is required")
	}
	if persistence == nil {
		return fmt.Errorf("construct Factory Definitions: persistence is required")
	}
	if loader == nil {
		return fmt.Errorf("construct Factory Definitions: loader is required")
	}
	if applySupportedFiles == nil {
		return fmt.Errorf("construct Factory Definitions: portable bundled files applier is required")
	}
	if applyStarterWork == nil {
		return fmt.Errorf("construct Factory Definitions: starter Work applier is required")
	}
	if namedPaths == nil {
		return fmt.Errorf("construct Factory Definitions: named path resolver is required")
	}
	if namedFactoryCatalogFileSystem == nil {
		return fmt.Errorf("construct Factory Definitions: named Factory catalog filesystem is required")
	}
	if clock == nil {
		return fmt.Errorf("construct Factory Definitions: clock is required")
	}
	if versionFileSystem == nil {
		return fmt.Errorf("construct Factory Definitions: version filesystem is required")
	}
	if listEffective == nil {
		return fmt.Errorf("construct Factory Definitions: effective Factory catalog is required")
	}
	if packagedCatalog.List == nil {
		return fmt.Errorf("construct Factory Definitions: packaged Factory catalog list operation is required")
	}
	if packagedCatalog.Resolve == nil {
		return fmt.Errorf("construct Factory Definitions: packaged Factory catalog resolve operation is required")
	}
	if packagedInstaller.Install == nil {
		return fmt.Errorf("construct Factory Definitions: packaged Factory installer is required")
	}
	if requiredToolChecker == nil {
		return fmt.Errorf("construct Factory Definitions: required tool checker is required")
	}
	if orchestratorValidator == nil {
		return fmt.Errorf("construct Factory Definitions: orchestrator definition validator is required")
	}
	if portableFileSystem == nil {
		return fmt.Errorf("construct Factory Definitions: portable filesystem is required")
	}
	if directoryReplacementStore == nil {
		return fmt.Errorf("construct Factory Definitions: directory replacement store is required")
	}
	return nil
}

// EffectiveFactoryDefinitionNormalizerFromMapper binds the canonical Factory
// config mapper to effective-catalog normalization for Wire composition.
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

// StaticClock returns a Factory Definitions clock backed by one fixed instant.
// It is intended for focused Wire tests that need deterministic construction ports.
func StaticClock(instant time.Time) factorydefinitions.Clock {
	return staticClock{instant: instant}
}

type staticClock struct{ instant time.Time }

func (c staticClock) Now() time.Time { return c.instant }

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
	factorydefinitions.AuthoredLayoutWriterFileSystem
	factorydefinitions.PersistenceFileSystem
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
