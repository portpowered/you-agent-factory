// Package wire constructs the Recordings neutral replay subservice.
package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordingsreplay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay"
	replayservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/internal/service"
)

// NewService constructs the private replay owner from the lifecycle and
// projection-query capabilities selected by the Recordings root.
func NewService(
	lifecycle recordinglifecycle.Service,
	projection recordings.ProjectionService,
) recordingsreplay.Service {
	return replayservice.New(lifecycle, projection)
}

// NewServiceWithResumeSource constructs replay with the exact read-side
// source and Factory snapshot decoder selected by the owning Recordings wire.
func NewServiceWithResumeSource(
	lifecycle recordinglifecycle.Service,
	projection recordings.ProjectionService,
	readFile recordings.RecordingReadFile,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
) recordingsreplay.Service {
	return replayservice.NewWithResumeSource(
		lifecycle,
		projection,
		readFile,
		decodeFactorySnapshot,
	)
}
