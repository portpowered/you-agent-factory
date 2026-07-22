package service

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

// CaptureInitialSnapshot captures the portable Factory Definition stored with
// a newly created runtime recording.
func CaptureInitialSnapshot(
	loaded factorydefinitions.LoadedFactorySource,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.FactorySnapshotCapturer,
) (*factorydefinitions.FactorySnapshot, error) {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return nil, fmt.Errorf("loaded factory config is unavailable")
	}
	if preparePortableFactoryConfig == nil || captureFactorySnapshot == nil {
		return nil, fmt.Errorf("Factory snapshot adapters are required")
	}
	factoryCfg, err := preparePortableFactoryConfig(
		loaded.FactoryDir(),
		loaded.FactoryConfig(),
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("prepare initial Factory snapshot: %w", err)
	}
	return captureFactorySnapshot(
		loaded.FactoryDir(),
		factoryCfg,
		loaded,
		loaded.FactoryDir(),
		nil,
	)
}
