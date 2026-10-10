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
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitymaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
	internalportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// NewService constructs an inert Factory Definitions root from completed catalog,
// validation, compilation and runtime snapshot owners plus construction and process-edge ports. Private owner
// types remain behind the returned peer surface.
func NewService(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	compilation Compilation,
	validationService Validation,
	runtimeSnapshot RuntimeSnapshot,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	catalogService Catalog,
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
	return composeService(
		sessionHost, activationGateway, validator, persistence, loader, compilation, validationService, runtimeSnapshot,
		applySupportedFiles, applyStarterWork, namedPaths,
		catalogService, clock, versionFileSystem, listEffective,
		packagedCatalog, packagedInstaller, requiredToolChecker,
		orchestratorValidator, portableFileSystem, directoryReplacementStore,
		options...,
	)
}

func composeService(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	compilation Compilation,
	validationService Validation,
	runtimeSnapshot RuntimeSnapshot,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	catalogService Catalog,
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
	preparePortableFactoryConfig, captureFactorySnapshot, snapshotsPortability, authoringLayout, err := composeFactoryDefinitionSupport(
		loader,
		applySupportedFiles,
		applyStarterWork,
		validator,
		namedPaths,
		portableFileSystem,
		directoryReplacementStore,
	)
	if err != nil {
		return nil, err
	}

	definitions := factorydefinitionsinternal.NewWithAuthoringLayout(
		sessionHost,
		activationGateway,
		clock,
		versionFileSystem,
		validator,
		validationService,
		runtimeSnapshot,
		compilation,
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
		catalogService,
		packagedCatalog,
		packagedInstaller,
		requiredToolChecker,
		orchestratorValidator,
		authoringLayout,
		listEffective,
		snapshotsPortability,
		options...,
	)
	if definitions == nil {
		return nil, fmt.Errorf("construct Factory Definitions: implementation rejected its dependencies")
	}

	return definitions, nil
}

func composeFactoryDefinitionSupport(
	loader *compilationloading.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	validator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
) (
	factorydefinitions.PortableFactoryConfigPreparer,
	factorydefinitions.FactorySnapshotCapturer,
	snapshotsportability.Service,
	authoringlayout.Service,
	error,
) {
	preparePortableFactoryConfig := PortableFactoryConfigPreparer(applySupportedFiles, applyStarterWork)
	captureFactorySnapshot := FactorySnapshotCapturer()
	snapshotsPortability, err := snapshotsportabilitywire.NewService(
		loader.LoadSourceFromCanonicalJSON,
		LoadedFactorySnapshotCapturer(),
		preparePortableFactoryConfig,
		FactorySnapshotJSONDecoder(),
		snapshotsportabilitymaterialize.NewMaterializer(portableFileSystem),
		snapshotsportabilitymaterialize.NewWritesValidator(portableFileSystem),
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	authoringFS, err := resolveAuthoringLayoutFilesystem(portableFileSystem)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	pruneRemovedDocs, err := internalportableconfig.NewPortableBundledDocsPruner(portableFileSystem)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	mapper := factorymapping.NewFactoryConfigMapper()
	writer := NewAuthoredLayoutWriter(authoringFS, inboxgitkeep.NewLocal(portableFileSystem), AuthoredAgentsFileWriter(authoringFS))
	materializeFiles := internalportableconfig.NewMaterializer(portableFileSystem)
	validateWrites := internalportableconfig.NewWritesValidator(portableFileSystem)
	authoringLayout, err := NewAuthoringLayout(
		validator, validationentry.MapFactoryJSONForPersistence, mapper.Expand,
		authoredmapping.AuthoredFactoryConfigForExpandedLayout, mapper.Flatten,
		PreparedAuthoredLayoutWriter(writer, materializeFiles, pruneRemovedDocs),
		AuthoredLayoutValidator(loader, validateWrites), loader.FlattenFactoryConfig,
		AuthoredLayoutExpander(loader, writer, validateWrites, materializeFiles, internalportableconfig.NewFilesCopier(portableFileSystem)),
		authoringFS, namedPaths.RequireDefinitionDir, directoryReplacementStore,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	return preparePortableFactoryConfig, captureFactorySnapshot, snapshotsPortability, authoringLayout, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func validateDependencies(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
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
