// Package recordinglifecycle defines the Recordings-owned recording lifecycle.
// Consumers outside Recordings use the Recordings root service instead of this
// parent-private subservice contract.
package recordinglifecycle

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

// Snapshot is a detached private handoff to sibling Recordings capabilities.
// Replay and artifact export may read finalized facts without owning lifecycle
// state or receiving persistence handles.
type Snapshot struct {
	Status           recordings.RecordingStatusFacts
	Events           []recordings.CanonicalEvent
	SecretProvenance map[int][]recordings.RecordingSecret
}

// Service owns target selection, binding, and mutable recording lifecycle
// state behind the Recordings root.
type Service interface {
	StartRecording(recordings.StartRecordingRequest) (recordings.StartRecordingResult, error)
	BindRecording(recordings.BindRecordingRequest) (recordings.BindRecordingResult, error)
	RecordRecordingEvent(recordings.RecordRecordingEventRequest) (recordings.RecordRecordingEventResult, error)
	RecordRecordingError(recordings.RecordRecordingErrorRequest) (recordings.RecordRecordingErrorResult, error)
	FlushRecording(recordings.FlushRecordingRequest) (recordings.FlushRecordingResult, error)
	StopRecording(recordings.StopRecordingRequest) (recordings.StopRecordingResult, error)
	FinishRecording(recordings.FinishRecordingRequest) (recordings.FinishRecordingResult, error)
	QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	Snapshot(recordings.RecordingID) (Snapshot, error)
}

// CompletedFlushWatermarkReader aliases the narrow lifecycle capability so
// the implementation can expose it without adding another named interface to
// this private service root.
type CompletedFlushWatermarkReader = recordings.CompletedFlushWatermarkReader
