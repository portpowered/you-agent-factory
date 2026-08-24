package contracts

import (
	"encoding/json"
	"io"
	"io/fs"
	"time"

	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
)

const (
	// KindJavaScriptFactorySession identifies the portable JavaScript Factory
	// Session recording contract.
	KindJavaScriptFactorySession = "you.factory-session.javascript.recording"

	// PortableRecordingSchemaV1 is the first shipped Factory Session recording
	// schema. It intentionally permits a recording without a result projection.
	PortableRecordingSchemaV1 = "1"
	// PortableRecordingSchemaV2 adds the required public result projection while
	// preserving the established Factory Session and event summaries.
	PortableRecordingSchemaV2 = "2"
	// PortableRecordingSchemaV3 adds the explicit Worker-history outcome and,
	// when available, the detached canonical Worker recording. Earlier schema
	// versions retain their original meaning and cannot carry this field.
	PortableRecordingSchemaV3 = "3"
	// PortableRecordingCurrentSchemaVersion is the version emitted by the
	// Worker-history-capable Factory Session exporter.
	PortableRecordingCurrentSchemaVersion = PortableRecordingSchemaV3

	// PortableRecordingReplayCompatibilityV1 identifies the replay vocabulary
	// shared by every shipped Factory Session recording schema.
	PortableRecordingReplayCompatibilityV1 = "1"

	// PortableRecordingWorkerHistoryReasonNotCaptured is the explicit outcome
	// used by a new export that has the Worker-history-capable schema but no
	// canonical Worker capture to attach.
	PortableRecordingWorkerHistoryReasonNotCaptured = "CANONICAL_WORKER_HISTORY_NOT_CAPTURED"

	// PortableRecordingCompatibilityAction is safe, stable guidance for a
	// reader that cannot consume a recording's declared version.
	PortableRecordingCompatibilityAction = "UPGRADE_READER_OR_MIGRATE_RECORDING_TO_A_SUPPORTED_VERSION"

	// portableRecordingMaxSecretsRedacted bounds the aggregate count exposed by
	// a recording. Counts at or above this limit are reported as the limit.
	portableRecordingMaxSecretsRedacted int64 = 1_000_000
)

var (
	portableRecordingSupportedSchemaVersions = []string{
		PortableRecordingSchemaV1, PortableRecordingSchemaV2, PortableRecordingSchemaV3,
	}
	portableRecordingSupportedReplayVersions = []string{PortableRecordingReplayCompatibilityV1}
)

// PortableRecording is the privacy-bounded JavaScript Factory Session recording
// contract. It contains no persistence or replay side effects.
type PortableRecording struct {
	RecordingKind              string                              `json:"recordingKind"`
	SchemaVersion              string                              `json:"schemaVersion"`
	ReplayCompatibilityVersion string                              `json:"replayCompatibilityVersion"`
	Session                    PortableRecordingSessionSummary     `json:"session"`
	Source                     PortableRecordingSourceSummary      `json:"source"`
	ArgumentsDigest            string                              `json:"argumentsDigest"`
	PolicyHash                 string                              `json:"policyHash"`
	Artifacts                  []PortableRecordingArtifactSummary  `json:"artifacts"`
	Events                     []PortableRecordingEventSummary     `json:"events"`
	Checkpoint                 *PortableRecordingCheckpointSummary `json:"checkpoint,omitempty"`
	Result                     *PortableRecordingResult            `json:"result,omitempty"`
	WorkerHistory              *PortableRecordingWorkerHistory     `json:"workerHistory,omitempty"`
	Redaction                  PortableRecordingRedactionMetadata  `json:"redaction"`
	// SecretProvenance is an in-memory write-boundary handoff. It is never
	// serialized; classified locations are replaced before persistence.
	SecretProvenance []RecordingSecret `json:"-"`
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
	Status        string                           `json:"status"`
	Mode          string                           `json:"mode"`
	PrimaryResult json.RawMessage                  `json:"primaryResult,omitempty"`
	ContentHash   string                           `json:"contentHash,omitempty"`
	ArtifactIDs   []string                         `json:"artifactIds,omitempty"`
	Failure       *PortableRecordingFailureSummary `json:"failure,omitempty"`
	Availability  *PortableRecordingAvailability   `json:"availability,omitempty"`
}

// PortableRecordingWorkerHistoryAvailability identifies whether a portable
// recording can make a canonical Worker-history claim.
type PortableRecordingWorkerHistoryAvailability string

const (
	PortableRecordingWorkerHistoryAvailable   PortableRecordingWorkerHistoryAvailability = "AVAILABLE"
	PortableRecordingWorkerHistoryUnavailable PortableRecordingWorkerHistoryAvailability = "UNAVAILABLE"

	// PortableRecordingWorkerHistoryReasonLegacySchema is stable compatibility
	// metadata for recordings that predate canonical Worker-history capture.
	PortableRecordingWorkerHistoryReasonLegacySchema = "SCHEMA_DID_NOT_RECORD_CANONICAL_WORKER_HISTORY"
	PortableRecordingWorkerHistoryUnavailableReason  = PortableRecordingWorkerHistoryReasonLegacySchema
)

// PortableRecordingWorkerHistory is the detached Worker-history availability
// projection for a Factory Session recording. The embedded Worker recording
// is flattened into this object so callers can inspect ordered records and
// their lifecycle, correlation, provenance, and fidelity facts without a
// second envelope or a live service dependency. Legacy schemas intentionally
// contain only the unavailable outcome and its compatibility reason; they do
// not acquire fabricated records, terminal, fidelity, provider, or complete
// facts while being read.
type PortableRecordingWorkerHistory struct {
	Availability PortableRecordingWorkerHistoryAvailability `json:"availability"`
	Reason       string                                     `json:"reason"`
	*workerrecording.WorkerPortableRecording
}

// NormalizePortableRecordingWorkerHistory maps the shipped pre-Worker-history
// schemas to their honest read-time outcome. It is pure and does not consult
// Workers, Providers, Events, or provider transcripts.
func NormalizePortableRecordingWorkerHistory(recording PortableRecording) PortableRecordingWorkerHistory {
	switch recording.SchemaVersion {
	case PortableRecordingSchemaV1, PortableRecordingSchemaV2:
		return PortableRecordingWorkerHistory{
			Availability: PortableRecordingWorkerHistoryUnavailable,
			Reason:       PortableRecordingWorkerHistoryReasonLegacySchema,
		}
	case PortableRecordingSchemaV3:
		if recording.WorkerHistory == nil {
			return PortableRecordingWorkerHistory{}
		}
		return clonePortableRecordingWorkerHistory(*recording.WorkerHistory)
	default:
		return PortableRecordingWorkerHistory{}
	}
}

func clonePortableRecordingWorkerHistory(
	history PortableRecordingWorkerHistory,
) PortableRecordingWorkerHistory {
	clone := history
	if history.WorkerPortableRecording != nil {
		recording := cloneWorkerPortableRecording(*history.WorkerPortableRecording)
		clone.WorkerPortableRecording = &recording
	}
	return clone
}

func cloneWorkerPortableRecording(
	recording workerrecording.WorkerPortableRecording,
) workerrecording.WorkerPortableRecording {
	clone := recording
	clone.Records = append([]workerrecording.WorkerPortableRecord(nil), recording.Records...)
	for index := range clone.Records {
		clone.Records[index].Payload = append(json.RawMessage(nil), recording.Records[index].Payload...)
		clone.Records[index].Provenance = recording.Records[index].Provenance
	}
	clone.Correlation.WorkIDs = append([]string(nil), recording.Correlation.WorkIDs...)
	if recording.Correlation.Continuation != nil {
		continuation := *recording.Correlation.Continuation
		clone.Correlation.Continuation = &continuation
	}
	if recording.Correlation.ProviderSelection != nil {
		selection := *recording.Correlation.ProviderSelection
		clone.Correlation.ProviderSelection = &selection
	}
	if recording.Lifecycle.OpeningTimestamp != nil {
		timestamp := *recording.Lifecycle.OpeningTimestamp
		clone.Lifecycle.OpeningTimestamp = &timestamp
	}
	if recording.Lifecycle.Terminal != nil {
		terminal := *recording.Lifecycle.Terminal
		clone.Lifecycle.Terminal = &terminal
	}
	return clone
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
	PortableRecordingCodeUnsupportedSchema  PortableRecordingDiagnosticCode = "UNSUPPORTED_RECORDING_SCHEMA_VERSION"
	PortableRecordingCodeInvalidIdentity    PortableRecordingDiagnosticCode = "INVALID_RECORDING_IDENTITY"
	PortableRecordingCodeInvalidDigest      PortableRecordingDiagnosticCode = "INVALID_RECORDING_DIGEST"
	PortableRecordingCodeInvalidSummary     PortableRecordingDiagnosticCode = "INVALID_RECORDING_SUMMARY"
	PortableRecordingCodeInvalidOrder       PortableRecordingDiagnosticCode = "INVALID_RECORDING_EVENT_ORDER"
)

// PortableRecordingDiagnostic reports one validation failure area.
type PortableRecordingDiagnostic struct {
	Code               PortableRecordingDiagnosticCode `json:"code"`
	Area               string                          `json:"area"`
	Path               string                          `json:"path,omitempty"`
	Message            string                          `json:"message"`
	EncounteredVersion string                          `json:"encounteredVersion,omitempty"`
	SupportedVersions  []string                        `json:"supportedVersions,omitempty"`
	Action             string                          `json:"action,omitempty"`
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
	WorkerHistory                       *PortableRecordingWorkerHistory
	// SecretProvenance identifies classified values in the portable recording
	// document before it is persisted. It contains locations and provenance,
	// never the classified values themselves.
	SecretProvenance []RecordingSecret
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

// RecordingReadFile reads a published portable recording or artifact.
type RecordingReadFile func(string) ([]byte, error)

// RecordingOpenFile opens a published recording for bounded-memory inspection.
type RecordingOpenFile func(string) (io.ReadCloser, error)

// PortableRecordingWriter validates and atomically persists one portable
// recording.
type PortableRecordingWriter interface {
	Write(string, PortableRecording) error
}
