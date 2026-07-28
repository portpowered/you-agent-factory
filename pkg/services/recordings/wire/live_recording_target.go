package wire

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

// NewLiveRecordingTargetPlanner constructs the Recordings-owned live target
// policy from exact mechanics selected by process-graph composition.
func NewLiveRecordingTargetPlanner(
	clock recordings.RecordingClock,
	newID recordings.RecordingIdentityGenerator,
	join recordings.RecordingPathJoiner,
) recordings.LiveRecordingTargetPlanner {
	return recordinglifecycle.NewLiveRecordingTargetPlanner(clock, newID, join)
}
