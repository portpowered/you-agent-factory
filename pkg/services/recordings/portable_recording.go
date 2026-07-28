package recordings

import (
	"encoding/json"
	"io/fs"
	"time"
)

const (
	// KindJavaScriptFactorySession identifies the portable JavaScript Factory
	// Session recording contract.
	KindJavaScriptFactorySession = "you.factory-session.javascript.recording"
	portableRecordingSchemaV2    = "2"
	portableRecordingReplayCompat  = "1"
	// portableRecordingMaxSecretsRedacted bounds the aggregate count exposed by
	// a recording. Counts at or above this limit are reported as the limit.
	portableRecordingMaxSecretsRedacted int64 = 1_000_000
)

var (
	portableRecordingSupportedSchemaVersions = []string{"1", portableRecordingSchemaV2}
	portableRecordingSupportedReplayVersions = []string{portableRecordingReplayCompat}
)

// PortableRecording is the privacy-bounded JavaScript Factory Session recording
// contract. It contains no persistence or replay side effects.
type PortableRecording struct {
	RecordingKind              string                            `json:"recordingKind"`
	SchemaVersion              string                            `json:"schemaVersion"`
	ReplayCompatibilityVersion string                            `json:"replayCompatibilityVersion"`
	Session                    PortableRecordingSessionSummary   `json:"session"`
	Source                     PortableRecordingSourceSummary    `json:"source"`
	ArgumentsDigest            string                            `json:"argumentsDigest"`
	PolicyHash                 string                            `json:"policyHash"`
	Artifacts                  []PortableRecordingArtifactSummary `json:"artifacts"`
	Events                     []PortableRecordingEventSummary   `json:"events"`
	Checkpoint                 *PortableRecordingCheckpointSummary `json:"checkpoint,omitempty"`
	Result                     *PortableRecordingResult          `json:"result,omitempty"`
	Redaction                  PortableRecordingRedactionMetadata `json:"redaction"`
}

// PortableRecordingSessionSummary exposes session facts in a portable recording.
type PortableRecordingSessionSummary struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	OrchestratorKind string `json:"orchestratorKind"`
}

// PortableRecordingSourceSummary exposes source facts in a portable recording.
type PortableRecordingSourceSummary struct {
	Ref  string `json:"ref"`
	Hash string `json:"hash"`
}

// PortableRecordingArtifactSummary exposes one artifact summary in a portable
// recording document.
type PortableRecordingArtifactSummary struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Visibility  string    `json:"visibility"`
	Label       string    `json:"label,omitempty"`
	ContentHash string    `json:"contentHash"`
	SizeBytes   int64     `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
}

// PortableRecordingEventSummary exposes one canonical event summary.
type PortableRecordingEventSummary struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Sequence     int64     `json:"sequence"`
	Timestamp    time.Time `json:"timestamp"`
	ArtifactIDs  []string  `json:"artifactIds,omitempty"`
	CheckpointID string    `json:"checkpointId,omitempty"`
}

// PortableRecordingCheckpointSummary exposes only the public checkpoint reference
// used for historical inspection.
type PortableRecordingCheckpointSummary struct {
	ID         string    `json:"id"`
	Label      string    `json:"label,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	ArtifactID string    `json:"artifactId,omitempty"`
}

// PortableRecordingResult contains only the canonical public result read model.
type PortableRecordingResult struct {
	Status        string                        `json:"status"`
	Mode          string                        `json:"mode"`
	PrimaryResult json.RawMessage               `json:"primaryResult,omitempty"`
	ContentHash   string                        `json:"contentHash,omitempty"`
	ArtifactIDs   []string                      `json:"artifactIds,omitempty"`
	Failure       *PortableRecordingFailureSummary `json:"failure,omitempty"`
	Availability  *PortableRecordingAvailability   `json:"availability,omitempty"`
}

// PortableRecordingFailureSummary exposes a safe failure summary.
type PortableRecordingFailureSummary struct {
	Reason                 string `json:"reason"`
	Message                string `json:"message,omitempty"`
	PartialResultAvailable bool   `json:"partialResultAvailable"`
}

// PortableRecordingAvailability exposes availability detail for a result.
type PortableRecordingAvailability struct {
	Reason    string `json:"reason"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable"`
}

// PortableRecordingRedactionMetadata exposes redaction metadata in a portable
// recording.
type PortableRecordingRedactionMetadata struct {
	RuntimeStateOmitted        bool  `json:"runtimeStateOmitted"`
	CheckpointBodiesOmitted    bool  `json:"checkpointBodiesOmitted"`
	ProviderTranscriptsOmitted bool  `json:"providerTranscriptsOmitted"`
	ChildDispatchesOmitted     bool  `json:"childDispatchesOmitted"`
	SecretsRedacted            int64 `json:"secretsRedacted"`
}

// PortableRecordingDiagnosticCode identifies a portable recording validation
// failure.
type PortableRecordingDiagnosticCode string

const (
	PortableRecordingCodeMalformedContract  PortableRecordingDiagnosticCode = "MALFORMED_RECORDING_CONTRACT"
	PortableRecordingCodeUnsupportedVersion PortableRecordingDiagnosticCode = "UNSUPPORTED_REPLAY_COMPATIBILITY_VERSION"
	PortableRecordingCodeInvalidIdentity    PortableRecordingDiagnosticCode = "INVALID_RECORDING_IDENTITY"
	PortableRecordingCodeInvalidDigest      PortableRecordingDiagnosticCode = "INVALID_RECORDING_DIGEST"
	PortableRecordingCodeInvalidSummary     PortableRecordingDiagnosticCode = "INVALID_RECORDING_SUMMARY"
)

// PortableRecordingDiagnostic reports one validation failure area.
type PortableRecordingDiagnostic struct {
	Code              PortableRecordingDiagnosticCode `json:"code"`
	Area              string                          `json:"area"`
	Path              string                          `json:"path,omitempty"`
	Message           string                          `json:"message"`
	SupportedVersions []string                        `json:"supportedVersions,omitempty"`
}

func (diagnostic *PortableRecordingDiagnostic) Error() string {
	if diagnostic == nil {
		return ""
	}
	if diagnostic.Path == "" {
		return diagnostic.Message
	}
	return diagnostic.Path + ": " + diagnostic.Message
}

// PortableRecordingCanonicalFacts contains only public facts accepted from the
// canonical Factory Session owner.
type PortableRecordingCanonicalFacts struct {
	SessionID, Status, OrchestratorKind string
	SourceRef, SourceHash, PolicyHash   string
	Arguments                           map[string]any
	ArgumentsDigest                     string
	Artifacts                           []PortableRecordingCanonicalArtifact
	Events                              []json.RawMessage
	Checkpoint                          *PortableRecordingCanonicalCheckpoint
	Result                              *PortableRecordingCanonicalResult
}

// PortableRecordingCanonicalCheckpoint carries canonical checkpoint facts.
type PortableRecordingCanonicalCheckpoint struct {
	ID, Label, Summary, ArtifactID string
	Timestamp                      time.Time
}

// PortableRecordingCanonicalArtifact carries canonical artifact facts.
type PortableRecordingCanonicalArtifact struct {
	ID, Kind, Visibility, Label, ContentHash string
	SizeBytes                                int64
	CreatedAt                                time.Time
	SecretsRedacted                          int64
}

// PortableRecordingCanonicalResult carries canonical result facts.
type PortableRecordingCanonicalResult struct {
	Status, Mode  string
	PrimaryResult json.RawMessage
	ArtifactIDs   []string
	Failure       *PortableRecordingFailureSummary
	Availability  *PortableRecordingAvailability
}

// RecordingTemporaryFile is the portable recording writer temporary file seam.
type RecordingTemporaryFile interface {
	Write([]byte) (int, error)
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

// RecordingMakeDirectories creates directories for portable recording writes.
type RecordingMakeDirectories func(string, fs.FileMode) error

// RecordingCreateTemporaryFile creates a temporary file for portable recording
// writes.
type RecordingCreateTemporaryFile func(string, string) (RecordingTemporaryFile, error)

// RecordingRemovePath removes a path during portable recording writes.
type RecordingRemovePath func(string) error

// RecordingRenamePath renames a path during portable recording writes.
type RecordingRenamePath func(string, string) error

// PortableRecordingWriter validates and atomically persists one portable
// recording.
type PortableRecordingWriter interface {
	Write(string, PortableRecording) error
}
