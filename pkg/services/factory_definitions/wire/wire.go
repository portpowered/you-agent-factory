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

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	snapshotsportabilitymaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

// NewService constructs an inert Factory Definitions root from construction and
// process-edge ports. It composes the accepted root through parent-private catalog
// Wire and the accepted service assembly without publishing owner types on the
// returned peer surface.
func NewService(
	sessionHost factorydefinitions.SessionHost,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *factoryloading.Loader,
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
) (factorydefinitions.Service, error) {
	if err := validateDependencies(
		sessionHost,
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
	); err != nil {
		return nil, err
	}

	preparePortableFactoryConfig := snapshotsportabilityprepare.NewPreparer(
		factorydefinitions.CloneFactoryConfig,
		applySupportedFiles,
		applyStarterWork,
	)
	captureFactorySnapshot := snapshotsportabilitycapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)
	snapshotsPortability, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             loader.LoadSourceFromCanonicalJSON,
		CaptureLoaded:             snapshotsportabilitycapture.NewLoaded(factorysnapshot.ObjectFromFactoryConfig),
		PreparePortable:           preparePortableFactoryConfig,
		DecodeSnapshot:            snapshotsportabilitycapture.NewJSONDecoder(factorymapping.GeneratedFactoryFromOpenAPIJSON),
		MaterializePortableFiles:  snapshotsportabilitymaterialize.NewMaterializer(portableFileSystem),
		ValidateMaterializeWrites: snapshotsportabilitymaterialize.NewWritesValidator(portableFileSystem),
	})
	if err != nil {
		return nil, err
	}

	definitions := factorydefinitionsservice.New(
		sessionHost,
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
	)
	if definitions == nil {
		return nil, fmt.Errorf("construct Factory Definitions: implementation rejected its dependencies")
	}

	attached, err := factorydefinitionsservice.AttachEffectiveCatalog(definitions, listEffective)
	if err != nil {
		return nil, err
	}
	if attached == nil {
		return nil, fmt.Errorf("construct Factory Definitions: effective catalog attachment rejected its dependencies")
	}
	withSnapshots, err := factorydefinitionsservice.AttachSnapshotsPortability(attached, snapshotsPortability)
	if err != nil {
		return nil, err
	}
	if withSnapshots == nil {
		return nil, fmt.Errorf("construct Factory Definitions: snapshots portability attachment rejected its dependencies")
	}
	return withSnapshots, nil
}

func validateDependencies(
	sessionHost factorydefinitions.SessionHost,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *factoryloading.Loader,
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
) error {
	if sessionHost == nil {
		return fmt.Errorf("construct Factory Definitions: session host is required")
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
