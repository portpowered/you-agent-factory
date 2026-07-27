// Package artifactsexport defines the Recordings-owned artifact close/export/read
// capability. Consumers outside Recordings use the Recordings root service
// instead of this parent-private subservice contract.
package artifactsexport

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

// SnapshotSource supplies finalized recording facts to artifact export without
// exposing lifecycle persistence handles.
type SnapshotSource interface {
	Snapshot(recordings.RecordingID) (recordinglifecycle.Snapshot, error)
}

// Service owns portable artifact build, validate, encode, decode, and summarize
// behind the Recordings root.
type Service interface {
	BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error)
	EncodePortableArtifact(recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error)
	DecodePortableArtifact(recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error)
	SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error)
}
