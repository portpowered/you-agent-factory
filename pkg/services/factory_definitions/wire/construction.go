package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/canonical"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func newSnapshotsPortability(
	loader *compilationloading.Loader,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	portableFileSystem portablefiles.FileSystem,
) (snapshotsportability.Service, error) {
	return snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             loader.LoadSourceFromCanonicalJSON,
		CaptureLoaded:             LoadedFactorySnapshotCapturer(),
		PreparePortable:           preparePortableFactoryConfig,
		DecodeSnapshot:            FactorySnapshotJSONDecoder(),
		MaterializePortableFiles:  NewPortableBundledFilesMaterializer(portableFileSystem),
		ValidateMaterializeWrites: NewPortableBundledFileWritesValidator(portableFileSystem),
	})
}

func newCompilation(
	loader *compilationloading.Loader,
) (compilationservice.Service, error) {
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
	return compilation, nil
}

func newAuthoringLayout(
	validator factorydefinitions.Validator,
	loader *compilationloading.Loader,
	namedPaths factorydefinitions.NamedPathResolver,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
) (authoringlayout.Service, error) {
	authoringFS, err := resolveAuthoringLayoutFilesystem(portableFileSystem)
	if err != nil {
		return nil, err
	}
	pruneRemovedDocs, err := NewPortableBundledDocsPruner(portableFileSystem)
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	authoring, err := NewAuthoringLayoutService(AuthoringLayoutDependencies{
		Validator: validator,
		MapInput: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, loader.LoadSourceFromCanonicalJSON)
		},
		Loader:             loader,
		MaterializeFiles:   NewPortableBundledFilesMaterializer(portableFileSystem),
		ValidateWrites:     NewPortableBundledFileWritesValidator(portableFileSystem),
		PruneRemovedDocs:   pruneRemovedDocs,
		CopySupportedFiles: NewPortableBundledFilesCopier(portableFileSystem),
		AuthoredWriterFS:   authoringFS,
		EnsureInbox:        inboxgitkeep.NewLocal(portableFileSystem),
		PersistenceFS:      authoringFS,
		NamedPaths:         namedPaths,
		Directories:        directoryReplacementStore,
	})
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	return authoring, nil
}

func newDefinitions(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.FactorySnapshotCapturer,
	authoringLayout authoringlayout.Service,
	options ...CompositionOption,
) (factorydefinitions.Service, error) {
	definitions := factorydefinitionsinternal.NewWithAuthoringLayout(
		sessionHost, activationGateway, clock, versionFileSystem, validator,
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
		persistence.CreateNamedFactory, namedPaths.WriteCurrentPointer,
		preparePortableFactoryConfig, captureFactorySnapshot,
		persistence.ReplaceFactoryLayout, namedPaths, namedFactoryCatalogFileSystem,
		packagedCatalog, packagedInstaller, requiredToolChecker, orchestratorValidator,
		authoringLayout, options...,
	)
	if definitions == nil {
		return nil, fmt.Errorf("construct Factory Definitions: implementation rejected its dependencies")
	}
	return definitions, nil
}

func attachFactoryDefinitionCapabilities(
	definitions factorydefinitions.Service,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	snapshotsPortability snapshotsportability.Service,
	compilation compilationservice.Service,
) (factorydefinitions.Service, error) {
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
