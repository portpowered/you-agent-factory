package snapshotcapture

import (
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// NewJSONDecoder binds a representation decoder to the Factory Definition
// snapshot contract.
func NewJSONDecoder[T any](
	decodeBoundary func([]byte) (T, error),
) factorydefinitions.FactorySnapshotJSONDecoder {
	return func(data []byte) (*factorydefinitions.FactorySnapshot, error) {
		return DecodeJSON(data, decodeBoundary)
	}
}

// DecodeJSON validates one representation boundary and captures a detached
// Factory Definition snapshot.
func DecodeJSON[T any](
	data []byte,
	decodeBoundary func([]byte) (T, error),
) (*factorydefinitions.FactorySnapshot, error) {
	if decodeBoundary == nil {
		return nil, errors.New("Factory boundary JSON decoder is required")
	}
	boundary, err := decodeBoundary(data)
	if err != nil {
		return nil, err
	}
	return factorydefinitions.NewFactorySnapshot(boundary)
}

// NewDirectoryLoader binds Factory Definition loading and snapshot capture to
// the directory-loader contract consumed by Recordings.
func NewDirectoryLoader(
	loadFactory factorydefinitions.LoadedFactoryLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) factorydefinitions.FactorySnapshotDirectoryLoader {
	return func(factoryDir string) (*factorydefinitions.FactorySnapshot, error) {
		return LoadDirectory(
			factoryDir,
			loadFactory,
			captureLoadedFactorySnapshot,
		)
	}
}

// LoadDirectory captures the effective Factory Definition rooted at one
// directory through injected owner capabilities.
func LoadDirectory(
	factoryDir string,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) (*factorydefinitions.FactorySnapshot, error) {
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
