package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
)

// Distribution is the completed private distribution owner.
type Distribution = distributionservice.Service

// NewDistribution constructs only the distribution owner from direct ports.
func NewDistribution(
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	installer factorydefinitions.PackagedFactoryInstallationOperations,
	scaffold factorydefinitions.ScaffoldInitializer,
	resolveName distributionservice.ScaffoldFactoryNameResolver,
) (Distribution, error) {
	return distributionwire.NewService(catalog, installer, scaffold, resolveName)
}

// SnapshotsPortability is the completed private snapshot owner.
type SnapshotsPortability = snapshotsportability.Service

// NewSnapshotsPortability constructs only the snapshot owner from direct ports.
func NewSnapshotsPortability(
	load factorydefinitions.CanonicalFactoryJSONLoader,
	capture factorydefinitions.LoadedFactorySnapshotCapturer,
	prepare factorydefinitions.PortableFactoryConfigPreparer,
	decode factorydefinitions.FactorySnapshotJSONDecoder,
	materialize factorydefinitions.PortableBundledFilesMaterializer,
	validate factorydefinitions.PortableBundledFileWritesValidator,
) (SnapshotsPortability, error) {
	return snapshotswire.NewService(load, capture, prepare, decode, materialize, validate)
}
