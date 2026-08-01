package wire

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
)

// NewLiveRecordingTargetPlanner constructs the Recordings-owned live target
// policy from exact mechanics selected by process-graph composition.
func NewLiveRecordingTargetPlanner(
	clock recordings.RecordingClock,
	newID recordings.RecordingIdentityGenerator,
	join recordings.RecordingPathJoiner,
) recordings.LiveRecordingTargetPlanner {
	return recordinglifecyclewire.NewTargetPlanner(clock, newID, join)
}
