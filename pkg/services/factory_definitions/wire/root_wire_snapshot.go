package wire

import (
	"errors"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

// FactorySnapshotJSONDecoder binds the canonical public representation decoder
// to Factory Definitions snapshot capture.
func FactorySnapshotJSONDecoder() contracts.FactorySnapshotJSONDecoder {
	return snapshotsportabilitycapture.NewJSONDecoder(
		factorymapping.GeneratedFactoryFromOpenAPIJSON,
	)
}

// LoadedFactorySnapshotCapturer binds canonical snapshot representation
// mapping to the Factory Definitions capture implementation.
func LoadedFactorySnapshotCapturer() contracts.LoadedFactorySnapshotCapturer {
	return snapshotsportabilitycapture.NewLoaded(
		factorysnapshot.ObjectFromFactoryConfig,
	)
}

// FactorySnapshotCapturer binds canonical representation mapping to explicit
// Factory Definition snapshot capture.
func FactorySnapshotCapturer() contracts.FactorySnapshotCapturer {
	return snapshotsportabilitycapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)
}

// FactorySnapshotDirectoryLoader composes authored Factory loading and
// snapshot capture for Recordings import paths.
func FactorySnapshotDirectoryLoader(
	loader *factoryloading.Loader,
) contracts.FactorySnapshotDirectoryLoader {
	loadFactory := func(
		factoryDir string,
		workstationLoader contracts.WorkstationLoader,
	) (contracts.MutableLoadedFactorySource, error) {
		return loader.LoadSourceFromFactoryDir(factoryDir, workstationLoader)
	}
	captureLoadedFactorySnapshot := LoadedFactorySnapshotCapturer()
	return func(factoryDir string) (*contracts.FactorySnapshot, error) {
		if loadFactory == nil {
			return nil, errors.New("Factory Definition loader is required")
		}
		if captureLoadedFactorySnapshot == nil {
			return nil, errors.New("loaded Factory snapshot capturer is required")
		}
		loaded, err := loadFactory(factoryDir, nil)
		if err != nil {
			return nil, err
		}
		return captureLoadedFactorySnapshot(
			loaded,
			loaded.FactoryDir(),
			nil,
		)
	}
}
