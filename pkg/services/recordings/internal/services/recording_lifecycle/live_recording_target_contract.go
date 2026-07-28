package recordinglifecycle

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

// Live recording target vocabulary is owned by this subservice. Peers import
// these types from pkg/services/recordings rather than this private package.
type (
	RecordingClock             = recordings.RecordingClock
	RecordingIdentityGenerator = recordings.RecordingIdentityGenerator
	RecordingPathJoiner        = recordings.RecordingPathJoiner
	LiveRecordingTargetRequest = recordings.LiveRecordingTargetRequest
	LiveRecordingTarget        = recordings.LiveRecordingTarget
	LiveRecordingTargetPlanner = recordings.LiveRecordingTargetPlanner
	LiveRecordingTargetPlannerFunc = recordings.LiveRecordingTargetPlannerFunc
)
