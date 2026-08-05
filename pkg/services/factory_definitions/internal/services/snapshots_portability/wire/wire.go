// Package wire constructs the Factory Definitions snapshots_portability
// subservice from exact injected snapshot and portable-materialize ports.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportabilityservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/service"
)

// NewService constructs the focused Snapshots capability from exact injected
// Factory Definitions ports. Construction is inert: it neither loads nor
// validates a snapshot and it never touches a materialization target.
func NewService(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	captureLoaded factorydefinitions.LoadedFactorySnapshotCapturer,
	preparePortable factorydefinitions.PortableFactoryConfigPreparer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeConfig factorydefinitions.FactorySnapshotConfigDecoder,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator,
	fileSystem factorydefinitions.SnapshotMaterializationFileSystem,
	directories factorydefinitions.DirectoryReplacementStore,
) (factorydefinitions.Snapshots, error) {
	if loadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: canonical Factory loader is required")
	}
	if captureLoaded == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: loaded Factory snapshot capturer is required")
	}
	if preparePortable == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable Factory config preparer is required")
	}
	if decodeSnapshot == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: Factory snapshot JSON decoder is required")
	}
	if decodeConfig == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: Factory snapshot config decoder is required")
	}
	if materializePortableFiles == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable bundled-files materializer is required")
	}
	if validateMaterializeWrites == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable bundled-file writes validator is required")
	}
	if fileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: snapshot materialization filesystem is required")
	}
	if directories == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: directory replacement store is required")
	}
	service := snapshotsportabilityservice.New(
		loadCanonical,
		captureLoaded,
		preparePortable,
		decodeSnapshot,
		decodeConfig,
		materializePortableFiles,
		validateMaterializeWrites,
		fileSystem,
		directories,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: implementation rejected its dependencies")
	}
	return service, nil
}
