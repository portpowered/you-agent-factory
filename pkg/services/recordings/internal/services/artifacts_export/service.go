// Package artifactsexport defines the Recordings-owned artifact close/export/read
// capability. Consumers outside Recordings use the Recordings root service
// instead of this parent-private subservice contract.
package artifactsexport

import (
	"context"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Service owns portable artifact build, validate, encode, decode, summarize,
// export, and read behind the Recordings root.
type Service interface {
	BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error)
	EncodePortableArtifact(recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error)
	DecodePortableArtifact(recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error)
	SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error)
	ExportPortableArtifact(context.Context, recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error)
	ReadPortableArtifact(context.Context, recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error)
}
