// Package service is a transitional compile shim that re-exports the composed
// Factory Definitions root from pkg/services/factory_definitions/internal.
// Peers should construct through factory_definitions/wire; baseline deletion of
// this path is owned by DEL-DEF.
package service

import (
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedinstallation"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
)

// CompositionOption configures optional Factory Definitions composition ports.
type CompositionOption = factorydefinitionsinternal.CompositionOption

// EffectiveCatalogService is the read-only Factory Definitions owner used by
// transports that do not require a Factory Session.
type EffectiveCatalogService = factorydefinitionsinternal.EffectiveCatalogService

// New constructs the public Factory Definitions service.
func New(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	validator factorydefinitions.Validator,
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	readCurrentFactoryPointer factorydefinitions.CurrentFactoryPointerReader,
	prepareFactoryLayoutPayload factorydefinitions.FactoryLayoutPayloadPreparer,
	persistNamedFactory factorydefinitions.NamedFactoryPersister,
	writeCurrentFactoryPointer factorydefinitions.CurrentFactoryPointerWriter,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.FactorySnapshotCapturer,
	replaceFactoryLayout factorydefinitions.FactoryLayoutReplacer,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	options ...CompositionOption,
) factorydefinitions.Service {
	return factorydefinitionsinternal.New(
		sessionHost,
		activationGateway,
		clock,
		versionFileSystem,
		validator,
		loadCanonical,
		loadFactory,
		readCurrentFactoryPointer,
		prepareFactoryLayoutPayload,
		persistNamedFactory,
		writeCurrentFactoryPointer,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
		replaceFactoryLayout,
		namedPaths,
		namedFactoryCatalogFileSystem,
		packagedCatalog,
		packagedInstaller,
		requiredToolChecker,
		orchestratorValidator,
		options...,
	)
}

// NewWithAuthoringLayout constructs the public Factory Definitions service
// with the private authoring_layout subservice attached to the CTR-DEF root
// authoring slice.
func NewWithAuthoringLayout(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	validator factorydefinitions.Validator,
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	readCurrentFactoryPointer factorydefinitions.CurrentFactoryPointerReader,
	prepareFactoryLayoutPayload factorydefinitions.FactoryLayoutPayloadPreparer,
	persistNamedFactory factorydefinitions.NamedFactoryPersister,
	writeCurrentFactoryPointer factorydefinitions.CurrentFactoryPointerWriter,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.FactorySnapshotCapturer,
	replaceFactoryLayout factorydefinitions.FactoryLayoutReplacer,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	authoringLayout authoringlayout.Service,
	options ...CompositionOption,
) factorydefinitions.Service {
	return factorydefinitionsinternal.NewWithAuthoringLayout(
		sessionHost,
		activationGateway,
		clock,
		versionFileSystem,
		validator,
		loadCanonical,
		loadFactory,
		readCurrentFactoryPointer,
		prepareFactoryLayoutPayload,
		persistNamedFactory,
		writeCurrentFactoryPointer,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
		replaceFactoryLayout,
		namedPaths,
		namedFactoryCatalogFileSystem,
		packagedCatalog,
		packagedInstaller,
		requiredToolChecker,
		orchestratorValidator,
		authoringLayout,
		options...,
	)
}

// WithDistributionScaffold wires scaffold creation through Distribution without
// changing the primary service constructor surface used by process-root Wire.
func WithDistributionScaffold(
	scaffoldInitializer factorydefinitions.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver,
) CompositionOption {
	return factorydefinitionsinternal.WithDistributionScaffold(
		scaffoldInitializer,
		scaffoldFactoryNameResolver,
	)
}

// NewEffectiveCatalog constructs the stateless effective Factory catalog.
func NewEffectiveCatalog(
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer,
) (factorydefinitions.EffectiveFactoryCatalogOperation, error) {
	return factorydefinitionsinternal.NewEffectiveCatalog(discovery, normalize)
}

// AttachEffectiveCatalog returns the Factory Definitions service with
// effective discovery delegated to listEffective while preserving every other
// root operation.
func AttachEffectiveCatalog(
	service factorydefinitions.Service,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) (factorydefinitions.Service, error) {
	return factorydefinitionsinternal.AttachEffectiveCatalog(service, listEffective)
}

// NewEffectiveCatalogService constructs the read-only Factory Definitions
// service slice used by transports that do not require a Factory Session.
func NewEffectiveCatalogService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) (*EffectiveCatalogService, error) {
	return factorydefinitionsinternal.NewEffectiveCatalogService(listEffective)
}

// NewEffectiveCatalogDiscovery constructs read-only disk and published-package
// discovery.
func NewEffectiveCatalogDiscovery(
	listRoot factorydefinitions.EffectiveFactoryRootListing,
	read factorydefinitions.EffectiveFactoryCandidateRead,
	packaged []factorydefinitions.PackagedDefinition,
) (factorydefinitions.EffectiveFactoryCatalogDiscovery, error) {
	return factorydefinitionsinternal.NewEffectiveCatalogDiscovery(listRoot, read, packaged)
}

// AttachSnapshotsPortability returns the Factory Definitions service with
// detached snapshot capture, prepare-import, and materialize delegated to the
// nested snapshots_portability owner while preserving every other root operation.
func AttachSnapshotsPortability(
	service factorydefinitions.Service,
	snapshots snapshotsportability.Service,
) (factorydefinitions.Service, error) {
	return factorydefinitionsinternal.AttachSnapshotsPortability(service, snapshots)
}

// AttachAuthoringLayout returns the Factory Definitions root with CTR-DEF
// authoring operations delegated to the private authoring_layout subservice
// while preserving every other root operation.
func AttachAuthoringLayout(
	service factorydefinitions.Service,
	authoring authoringlayout.Service,
) (factorydefinitions.Service, error) {
	return factorydefinitionsinternal.AttachAuthoringLayout(service, authoring)
}

// NewPackagedFactoryCatalog constructs deterministic packaged Factory catalog
// operations from validated embedded definitions.
func NewPackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return factorydefinitionsinternal.NewPackagedFactoryCatalog(definitions)
}

// NewPackagedFactoryInstaller constructs packaged Factory installation operations
// from exact persistence and filesystem ports.
func NewPackagedFactoryInstaller(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) factorydefinitions.PackagedFactoryInstallationOperations {
	return factorydefinitionsinternal.NewPackagedFactoryInstaller(persistence, fileSystem)
}

// NewPackagedFactoryInstallationService constructs the private packaged
// installation service for composition paths that need the concrete type.
func NewPackagedFactoryInstallationService(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) *distributionpackagedinstallation.Service {
	return factorydefinitionsinternal.NewPackagedFactoryInstallationService(persistence, fileSystem)
}

// CaptureInitialSnapshot captures the portable Factory Definition stored with
// a newly created runtime recording.
func CaptureInitialSnapshot(
	loaded factorydefinitions.LoadedFactorySource,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.FactorySnapshotCapturer,
) (*factorydefinitions.FactorySnapshot, error) {
	return factorydefinitionsinternal.CaptureInitialSnapshot(
		loaded,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
	)
}
