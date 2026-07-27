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

// PortableArtifactPublication persists completed portable artifact bytes at a
// public destination without exposing private lifecycle storage paths.
type PortableArtifactPublication interface {
	Publish(destination string, payload []byte) error
	Read(destination string) ([]byte, error)
}

// Service owns portable artifact build, validate, encode, decode, summarize,
// export, and read behind the Recordings root.
type Service interface {
	BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error)
	EncodePortableArtifact(recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error)
	DecodePortableArtifact(recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error)
	SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error)
	ExportPortableArtifact(recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error)
	ReadPortableArtifact(recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error)
}
