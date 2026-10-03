// Package wire exposes focused Recordings construction providers to canonical Wire.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
)

// NewService stores the completed owners and runtime dependencies without activating effects.
func NewService(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	lifecycle RecordingLifecycleOwner,
	artifacts ArtifactsExportOwner,
	replay ReplayOwner,
	canonical CanonicalLedgerOwner,
	historical HistoricalQueryOwner,
	clock recordings.RecordingClock,
	logger logging.Logger,
	router *RuntimeLedgerRouter,
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
) recordings.Service {
	return recordingsinternal.NewCombinedService(ledger, projection, lifecycle, artifacts, replay, canonical, historical, clock, logger, router, captureSnapshot, decodeSnapshot, decodeRuntimeConfig, replayInputs)
}
