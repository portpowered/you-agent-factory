// Package wire constructs the Factory Definitions snapshots_portability
// subservice from exact injected snapshot and portable-materialize ports.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilityservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/service"
)

// NewService constructs the private snapshots_portability owner from direct ports.
// Construction does not select filesystem adapters or execute snapshot operations.
func NewService(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	captureLoaded factorydefinitions.LoadedFactorySnapshotCapturer,
	preparePortable factorydefinitions.PortableFactoryConfigPreparer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator,
) (snapshotsportability.Service, error) {
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
	if materializePortableFiles == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable bundled-files materializer is required")
	}
	if validateMaterializeWrites == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable bundled-file writes validator is required")
	}
	service := snapshotsportabilityservice.New(
		loadCanonical,
		captureLoaded,
		preparePortable,
		decodeSnapshot,
		materializePortableFiles,
		validateMaterializeWrites,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: implementation rejected its dependencies")
	}
	return service, nil
}
