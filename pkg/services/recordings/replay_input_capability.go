package recordings

import "time"

// LoadReplayInputRequest selects one historical replay input by filesystem
// path for RecordingReplayArtifacts.
type LoadReplayInputRequest struct {
	Path string
}

// LoadReplayInputResult contains exactly one of Portable or Legacy,
// depending on which replay input family the selected path contained. Portable
// is a directly owned, detached value rather than the compatibility
// PortableRecording alias used by the legacy writer/reader surface.
type LoadReplayInputResult struct {
	Portable *ReplayInputPortableRecording
	Legacy   *ReplayInputLegacyArtifact
}

// ReplayInputPortableRecording is the directly owned, privacy-bounded
// JavaScript Factory Session recording returned by RecordingReplayArtifacts.
// It is intentionally separate from PortableRecording, which is a
// compatibility alias retained for the existing recording writer and replay
// projection paths. Every nested value here is defined at the Recordings root
// so a peer can consume this narrow capability without importing
// recordings/internal/contracts.
type ReplayInputPortableRecording struct {
	RecordingKind              string
	SchemaVersion              string
	ReplayCompatibilityVersion string
	Session                    ReplayInputSessionSummary
	Source                     ReplayInputSourceSummary
	ArgumentsDigest            string
	PolicyHash                 string
	Artifacts                  []ReplayInputArtifactSummary
	Events                     []ReplayInputEventSummary
	Checkpoint                 *ReplayInputCheckpointSummary
	Result                     *ReplayInputResultSummary
	Redaction                  ReplayInputRedactionMetadata
}

// ReplayInputSessionSummary exposes portable Factory Session identity and
// completion facts.
type ReplayInputSessionSummary struct {
	ID               string
	Status           string
	OrchestratorKind string
}

// ReplayInputSourceSummary exposes the immutable workflow source facts used
// by the recorded Factory Session.
type ReplayInputSourceSummary struct {
	Ref  string
	Hash string
}

// ReplayInputArtifactSummary exposes one public artifact reference from a
// portable recording without granting storage authority.
type ReplayInputArtifactSummary struct {
	ID          string
	Kind        string
	Visibility  string
	Label       string
	ContentHash string
	SizeBytes   int64
	CreatedAt   time.Time
}

// ReplayInputEventSummary exposes one canonical portable-recording event in
// its recorded order.
type ReplayInputEventSummary struct {
	ID           string
	Type         string
	Sequence     int64
	Timestamp    time.Time
	ArtifactIDs  []string
	CheckpointID string
}

// ReplayInputCheckpointSummary exposes only the public checkpoint reference
// retained by a portable recording.
type ReplayInputCheckpointSummary struct {
	ID         string
	Label      string
	Summary    string
	Timestamp  time.Time
	ArtifactID string
}

// ReplayInputResultSummary exposes the public terminal result. PrimaryResult
// is detached JSON bytes; it is never a shared decoder or payload handle.
type ReplayInputResultSummary struct {
	Status        string
	Mode          string
	PrimaryResult []byte
	ContentHash   string
	ArtifactIDs   []string
	Failure       *ReplayInputFailureSummary
	Availability  *ReplayInputAvailability
}

// ReplayInputFailureSummary exposes the safe failure facts retained by a
// portable recording.
type ReplayInputFailureSummary struct {
	Reason                 string
	Message                string
	PartialResultAvailable bool
}

// ReplayInputAvailability exposes public result-availability detail.
type ReplayInputAvailability struct {
	Reason    string
	Message   string
	Retryable bool
}

// ReplayInputRedactionMetadata exposes the privacy decisions that shaped a
// portable recording without exposing any omitted contents.
type ReplayInputRedactionMetadata struct {
	RuntimeStateOmitted        bool
	CheckpointBodiesOmitted    bool
	ProviderTranscriptsOmitted bool
	ChildDispatchesOmitted     bool
	SecretsRedacted            int64
}

// ReplayInputLegacyArtifact is the directly owned compatibility view of one
// embedded-Factory replay artifact. Its serialized fields preserve the exact
// legacy runtime input while keeping the narrow capability free of aliases to
// Recordings' internal replay contracts. Factory Sessions adapts this value at
// its existing Factory Definitions compatibility boundary.
type ReplayInputLegacyArtifact struct {
	SchemaVersion       string
	RecordedAt          time.Time
	Events              []ReplayInputLegacyEvent
	FactorySnapshotJSON []byte
	DiagnosticsJSON     []byte
	WallClock           *ReplayInputWallClockMetadata
}

// ReplayInputLegacyEvent is one complete detached legacy Factory event
// envelope. EventJSON preserves the legacy schema without exposing a runtime
// event handle.
type ReplayInputLegacyEvent struct {
	EventJSON []byte
}

// ReplayInputWallClockMetadata preserves the replay timing facts used by the
// existing Factory Sessions compatibility path.
type ReplayInputWallClockMetadata struct {
	StartedAt  time.Time
	FinishedAt time.Time
}

// ReplayInputDiagnosticCode identifies one RecordingReplayArtifacts
// structured validation failure. Values mirror the existing
// portable-recording diagnostic vocabulary so consumers observe the same
// failure areas without depending on recordings/internal/contracts.
type ReplayInputDiagnosticCode string

const (
	ReplayInputDiagnosticMalformed          ReplayInputDiagnosticCode = "MALFORMED_RECORDING_CONTRACT"
	ReplayInputDiagnosticUnsupportedVersion ReplayInputDiagnosticCode = "UNSUPPORTED_REPLAY_COMPATIBILITY_VERSION"
	ReplayInputDiagnosticInvalidIdentity    ReplayInputDiagnosticCode = "INVALID_RECORDING_IDENTITY"
	ReplayInputDiagnosticInvalidDigest      ReplayInputDiagnosticCode = "INVALID_RECORDING_DIGEST"
	ReplayInputDiagnosticInvalidSummary     ReplayInputDiagnosticCode = "INVALID_RECORDING_SUMMARY"
)

// ReplayInputDiagnostic reports one structured, directly owned
// RecordingReplayArtifacts portable replay-input validation failure area.
// It is populated whenever the selected path classifies as a portable
// JavaScript Factory Session recording but fails decode or validation.
type ReplayInputDiagnostic struct {
	Code              ReplayInputDiagnosticCode
	Area              string
	Path              string
	Message           string
	SupportedVersions []string
}

// ReplayInputErrorKind distinguishes typed RecordingReplayArtifacts outcomes
// so callers can branch on classification (read, portable, or legacy)
// without depending on recordings/internal/contracts sentinel errors.
type ReplayInputErrorKind string

const (
	// ReplayInputErrorRead reports a failure to read the selected path
	// before either replay input family could be classified.
	ReplayInputErrorRead ReplayInputErrorKind = "READ_FAILED"
	// ReplayInputErrorPortable reports a portable JavaScript Factory
	// Session recording that failed decode or validation.
	ReplayInputErrorPortable ReplayInputErrorKind = "INVALID_PORTABLE_RECORDING"
	// ReplayInputErrorLegacy reports a legacy embedded-Factory replay
	// artifact that failed to load.
	ReplayInputErrorLegacy ReplayInputErrorKind = "LEGACY_LOAD_FAILED"
)

// ReplayInputError is a typed, directly owned RecordingReplayArtifacts
// failure. Diagnostic is populated only for ReplayInputErrorPortable;
// callers branch on Kind or unwrap Cause for standard errors.Is/errors.As
// matching.
type ReplayInputError struct {
	Kind       ReplayInputErrorKind
	Diagnostic *ReplayInputDiagnostic
	Message    string
	Cause      error
}

func (e *ReplayInputError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *ReplayInputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
