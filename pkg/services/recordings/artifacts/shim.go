package artifacts

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/artifacts"
)

// Preserve factory_definitions import for vocabulary boundary tests that assert
// the shim still depends on the published Factory contract.
var _ = interfaces.FactorySessionJavaScriptRuntimeState{}

// Re-export the transitional shim surface through recordings-owned portable
// recording vocabulary until DEL-REC removes the artifacts/ package.
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
	AtomicWriter        = artifactsimpl.AtomicWriter
)

const (
	KindJavaScriptFactorySession = recordings.KindJavaScriptFactorySession
	CurrentSchemaVersion         = "2"
	ReplayCompatibilityVersion   = "1"
	MaxSecretsRedacted             = 1_000_000

	CodeMalformedContract  = recordings.PortableRecordingCodeMalformedContract
	CodeUnsupportedVersion = recordings.PortableRecordingCodeUnsupportedVersion
	CodeInvalidIdentity    = recordings.PortableRecordingCodeInvalidIdentity
	CodeInvalidDigest      = recordings.PortableRecordingCodeInvalidDigest
	CodeInvalidSummary     = recordings.PortableRecordingCodeInvalidSummary
)

var (
	Build                          = recordings.BuildPortableRecording
	DecodeAndValidate              = recordings.DecodePortableRecording
	Validate                       = recordings.ValidatePortableRecording
	ApplyJavaScriptProjectionFacts = artifactsimpl.ApplyJavaScriptProjectionFacts
	NewAtomicWriter                = artifactsimpl.NewAtomicWriter
)
