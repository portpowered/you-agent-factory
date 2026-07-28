package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// FactorySnapshotJSONDecoder binds the canonical public representation decoder
// to Factory Definitions snapshot capture.
func FactorySnapshotJSONDecoder() contracts.FactorySnapshotJSONDecoder {
	return factorydefinitionswire.FactorySnapshotJSONDecoder()
}

// LoadedFactorySnapshotCapturer binds canonical snapshot representation
// mapping to the Factory Definitions capture implementation.
func LoadedFactorySnapshotCapturer() contracts.LoadedFactorySnapshotCapturer {
	return factorydefinitionswire.LoadedFactorySnapshotCapturer()
}

// FactorySnapshotCapturer binds canonical representation mapping to explicit
// Factory Definition snapshot capture.
func FactorySnapshotCapturer() contracts.FactorySnapshotCapturer {
	return factorydefinitionswire.FactorySnapshotCapturer()
}

// FactorySnapshotDirectoryLoader composes authored Factory loading and
// snapshot capture for Recordings import paths.
func FactorySnapshotDirectoryLoader(
	loader *factoryloading.Loader,
) contracts.FactorySnapshotDirectoryLoader {
	return factorydefinitionswire.FactorySnapshotDirectoryLoader(loader)
}
