package snapshotcapture

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
)

// NewLoaded binds representation mapping and runtime-definition merging at the
// composition boundary. The returned operation consumes only Factory
// Definitions root contracts.
func NewLoaded(
	mapSnapshotObject factorydefinitions.FactorySnapshotObjectMapper,
) factorydefinitions.LoadedFactorySnapshotCapturer {
	return snapshotsportabilitycapture.NewLoaded(mapSnapshotObject)
}

// NewExplicit adapts explicit Factory Definition values to the snapshot
// capturer used by Factory Definitions persistence.
func NewExplicit(
	mapSnapshotObject factorydefinitions.FactorySnapshotObjectMapper,
) factorydefinitions.FactorySnapshotCapturer {
	return snapshotsportabilitycapture.NewExplicit(mapSnapshotObject)
}

// CaptureLoaded captures a portable snapshot without requiring the loaded
// source to know about transport representations.
func CaptureLoaded(
	source factorydefinitions.FactorySnapshotSource,
	sourceDirectory string,
	metadata map[string]string,
	mapSnapshotObject factorydefinitions.FactorySnapshotObjectMapper,
) (*factorydefinitions.FactorySnapshot, error) {
	return snapshotsportabilitycapture.CaptureLoaded(
		source,
		sourceDirectory,
		metadata,
		mapSnapshotObject,
	)
}
