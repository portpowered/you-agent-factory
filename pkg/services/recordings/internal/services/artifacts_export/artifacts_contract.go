package artifactsexport

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

// Portable recording vocabulary is owned by this subservice. Peers import
// these types from pkg/services/recordings rather than this private package.
type (
	PortableRecording                  = recordings.PortableRecording
	PortableRecordingArtifactSummary     = recordings.PortableRecordingArtifactSummary
	PortableRecordingAvailability        = recordings.PortableRecordingAvailability
	PortableRecordingCanonicalArtifact   = recordings.PortableRecordingCanonicalArtifact
	PortableRecordingCanonicalCheckpoint = recordings.PortableRecordingCanonicalCheckpoint
	PortableRecordingCanonicalFacts      = recordings.PortableRecordingCanonicalFacts
	PortableRecordingCanonicalResult     = recordings.PortableRecordingCanonicalResult
	PortableRecordingCheckpointSummary   = recordings.PortableRecordingCheckpointSummary
	PortableRecordingDiagnostic          = recordings.PortableRecordingDiagnostic
	PortableRecordingEventSummary        = recordings.PortableRecordingEventSummary
	PortableRecordingFailureSummary      = recordings.PortableRecordingFailureSummary
	PortableRecordingResult              = recordings.PortableRecordingResult
	PortableRecordingWriter              = recordings.PortableRecordingWriter
)

const (
	KindJavaScriptFactorySession = recordings.KindJavaScriptFactorySession
)

var (
	BuildPortableRecording    = recordings.BuildPortableRecording
	DecodePortableRecording   = recordings.DecodePortableRecording
	ValidatePortableRecording = recordings.ValidatePortableRecording
)
