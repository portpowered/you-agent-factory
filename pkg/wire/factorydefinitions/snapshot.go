package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorysnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

// FactorySnapshotJSONDecoder binds the canonical public representation decoder
// to Factory Definitions snapshot capture.
func FactorySnapshotJSONDecoder() contracts.FactorySnapshotJSONDecoder {
	return factorysnapshotcapture.NewJSONDecoder(
		factorymapping.GeneratedFactoryFromOpenAPIJSON,
	)
}

// LoadedFactorySnapshotCapturer binds canonical snapshot representation
// mapping to the Factory Definitions capture implementation.
func LoadedFactorySnapshotCapturer() contracts.LoadedFactorySnapshotCapturer {
	return factorysnapshotcapture.NewLoaded(
		factorysnapshot.ObjectFromFactoryConfig,
	)
}

// FactorySnapshotCapturer binds canonical representation mapping to explicit
// Factory Definition snapshot capture.
func FactorySnapshotCapturer() contracts.FactorySnapshotCapturer {
	return factorysnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)
}

// FactorySnapshotDirectoryLoader composes authored Factory loading and
// snapshot capture for Recordings import paths.
func FactorySnapshotDirectoryLoader(
	loader *factoryloading.Loader,
) contracts.FactorySnapshotDirectoryLoader {
	return factorysnapshotcapture.NewDirectoryLoader(
		func(
			factoryDir string,
			workstationLoader contracts.WorkstationLoader,
		) (contracts.MutableLoadedFactorySource, error) {
			return loader.LoadSourceFromFactoryDir(factoryDir, workstationLoader)
		},
		LoadedFactorySnapshotCapturer(),
	)
}
