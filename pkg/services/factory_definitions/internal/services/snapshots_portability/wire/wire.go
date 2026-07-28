// Package wire constructs the Factory Definitions snapshots_portability
// subservice from exact injected snapshot and portable-materialize ports.
package wire

import (
	"fmt"

	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilityservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/service"
)

// NewService constructs the private snapshots_portability subservice from exact
// injected snapshot and portable-materialize ports. Callers must supply
// Dependencies; this constructor does not select host filesystem adapters,
// boundary codecs, or take Wire/root construction ownership.
func NewService(deps snapshotsportability.Dependencies) (snapshotsportability.Service, error) {
	if deps.LoadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: canonical Factory loader is required")
	}
	if deps.CaptureLoaded == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: loaded Factory snapshot capturer is required")
	}
	if deps.PreparePortable == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable Factory config preparer is required")
	}
	if deps.DecodeSnapshot == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: Factory snapshot JSON decoder is required")
	}
	if deps.MaterializePortableFiles == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable bundled-files materializer is required")
	}
	if deps.ValidateMaterializeWrites == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: portable bundled-file writes validator is required")
	}
	service := snapshotsportabilityservice.New(
		deps.LoadCanonical,
		deps.CaptureLoaded,
		deps.PreparePortable,
		deps.DecodeSnapshot,
		deps.MaterializePortableFiles,
		deps.ValidateMaterializeWrites,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: implementation rejected its dependencies")
	}
	return service, nil
}
