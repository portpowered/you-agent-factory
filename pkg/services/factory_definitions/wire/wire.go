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
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
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
		sessionHost, activationGateway, validator, persistence, loader,
		applySupportedFiles, applyStarterWork, namedPaths, namedFactoryCatalogFileSystem, clock,
		versionFileSystem, listEffective, packagedCatalog, packagedInstaller, requiredToolChecker,
		orchestratorValidator, portableFileSystem, directoryReplacementStore,
	); err != nil {
		return nil, err
	}

	preparePortableFactoryConfig := PortableFactoryConfigPreparer(
		applySupportedFiles,
		applyStarterWork,
	)
	captureFactorySnapshot := FactorySnapshotCapturer()
	snapshotsPortability, err := newSnapshotsPortability(
		loader, preparePortableFactoryConfig, portableFileSystem,
	)
	if err != nil {
		return nil, err
	}

	compilation, err := newCompilation(loader)
	if err != nil {
		return nil, err
	}

	authoringLayout, err := newAuthoringLayout(
		validator, loader, namedPaths, portableFileSystem, directoryReplacementStore,
	)
	if err != nil {
		return nil, err
	}

	definitions, err := newDefinitions(
		sessionHost, activationGateway, clock, versionFileSystem, validator, persistence, loader,
		namedPaths, namedFactoryCatalogFileSystem, packagedCatalog, packagedInstaller,
		requiredToolChecker, orchestratorValidator, preparePortableFactoryConfig,
		captureFactorySnapshot, authoringLayout, options...,
	)
	if err != nil {
		return nil, err
	}

	return attachFactoryDefinitionCapabilities(definitions, listEffective, snapshotsPortability, compilation)
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
