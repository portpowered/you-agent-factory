// Package artifacts defines the portable, privacy-bounded JavaScript Factory
// Session recording contract. It contains no persistence or replay side effects.
package artifacts

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

const (
	KindJavaScriptFactorySession = recordings.KindJavaScriptFactorySession
	CurrentSchemaVersion         = "2"
	ReplayCompatibilityVersion   = "1"
	MaxSecretsRedacted           = 1_000_000
)

type (
	Recording          = recordings.PortableRecording
	SessionSummary     = recordings.PortableRecordingSessionSummary
	SourceSummary      = recordings.PortableRecordingSourceSummary
	ArtifactSummary    = recordings.PortableRecordingArtifactSummary
	EventSummary       = recordings.PortableRecordingEventSummary
	CheckpointSummary  = recordings.PortableRecordingCheckpointSummary
	ResultProjection   = recordings.PortableRecordingResult
	FailureSummary     = recordings.PortableRecordingFailureSummary
	AvailabilityDetail = recordings.PortableRecordingAvailability
	RedactionMetadata  = recordings.PortableRecordingRedactionMetadata

	DiagnosticCode = recordings.PortableRecordingDiagnosticCode
	Diagnostic     = recordings.PortableRecordingDiagnostic

	CanonicalFacts      = recordings.PortableRecordingCanonicalFacts
	CanonicalCheckpoint = recordings.PortableRecordingCanonicalCheckpoint
	CanonicalArtifact   = recordings.PortableRecordingCanonicalArtifact
	CanonicalResult     = recordings.PortableRecordingCanonicalResult

	TemporaryFile       = recordings.RecordingTemporaryFile
	MakeDirectories     = recordings.RecordingMakeDirectories
	CreateTemporaryFile = recordings.RecordingCreateTemporaryFile
	RemovePath          = recordings.RecordingRemovePath
	RenamePath          = recordings.RecordingRenamePath
	Writer              = recordings.PortableRecordingWriter
)

const (
	CodeMalformedContract  = recordings.PortableRecordingCodeMalformedContract
	CodeUnsupportedVersion = recordings.PortableRecordingCodeUnsupportedVersion
	CodeInvalidIdentity    = recordings.PortableRecordingCodeInvalidIdentity
	CodeInvalidDigest      = recordings.PortableRecordingCodeInvalidDigest
	CodeInvalidSummary     = recordings.PortableRecordingCodeInvalidSummary
)

var (
	Build             = recordings.BuildPortableRecording
	DecodeAndValidate = recordings.DecodePortableRecording
	Validate          = recordings.ValidatePortableRecording
)