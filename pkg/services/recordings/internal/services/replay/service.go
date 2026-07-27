// Package recordingsreplay defines the Recordings-owned neutral replay
// capability. Consumers outside Recordings use the Recordings root service
// instead of this private subservice contract.
package recordingsreplay

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

// Service owns neutral load/plan/observe replay behavior behind the
// parent-private subservice boundary.
type Service interface {
	LoadReplayRecording(recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error)
	CreateReplayPlan(recordings.CreateReplayPlanRequest) (recordings.CreateReplayPlanResult, error)
	ObserveReplay(recordings.ObserveReplayRequest) (recordings.ObserveReplayResult, error)
}
