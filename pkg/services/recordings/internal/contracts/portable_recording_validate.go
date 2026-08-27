package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/jsoncompat"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
)

var portableRecordingSHA256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// DecodePortableRecording decodes and validates one portable recording document.
func DecodePortableRecording(reader io.Reader) (PortableRecording, error) {
	recording, _, err := CurrentPortableRecordingCodec().DecodeWithDiagnostics(reader)
	return recording, err
}

// DecodePortableRecordingWithDiagnostics decodes and validates one portable
// recording while retaining only sorted paths for ignored additive fields.
func DecodePortableRecordingWithDiagnostics(
	reader io.Reader,
) (PortableRecording, PortableRecordingDecodeDiagnostics, error) {
	return CurrentPortableRecordingCodec().DecodeWithDiagnostics(reader)
}

// DecodePortableRecordingMetadata reads only the portable recording header and
// Factory Session identity. Unknown fields, including the complete event
// summary, are streamed past by encoding/json and never retained in a
// PortableRecording value.
func DecodePortableRecordingMetadata(reader io.Reader) (string, error) {
	return CurrentPortableRecordingCodec().DecodeMetadata(reader)
}

// PortableRecordingCompatibilityPolicy defines the versions understood by a
// reader. Keeping the policy as a value makes an older-reader compatibility
// harness deterministic without changing the current Recordings policy.
type PortableRecordingCompatibilityPolicy struct {
	SupportedSchemaVersions              []string
	SupportedReplayCompatibilityVersions []string
}

// PortableRecordingCodec is a version-pinned portable recording reader and
// validator. It has no persistence or replay side effects.
type PortableRecordingCodec struct {
	policy PortableRecordingCompatibilityPolicy
}

// NewPortableRecordingCodec constructs a reader with an explicit compatibility
// matrix. Callers that need the current reader should use
// CurrentPortableRecordingCodec instead.
func NewPortableRecordingCodec(
	schemaVersions, replayCompatibilityVersions []string,
) PortableRecordingCodec {
	return PortableRecordingCodec{policy: PortableRecordingCompatibilityPolicy{
		SupportedSchemaVersions:              normalizePortableRecordingVersions(schemaVersions),
		SupportedReplayCompatibilityVersions: normalizePortableRecordingVersions(replayCompatibilityVersions),
	}}
}

// CurrentPortableRecordingCodec returns the Recordings reader for all shipped
// Factory Session recording schemas.
func CurrentPortableRecordingCodec() PortableRecordingCodec {
	return NewPortableRecordingCodec(
		portableRecordingSupportedSchemaVersions,
		portableRecordingSupportedReplayVersions,
	)
}

// Decode decodes and validates one document against the codec's pinned policy.
func (codec PortableRecordingCodec) Decode(reader io.Reader) (PortableRecording, error) {
	recording, _, err := codec.DecodeWithDiagnostics(reader)
	return recording, err
}

// DecodeWithDiagnostics decodes and validates one document against the
// codec's pinned policy, returning only safe paths for ignored fields.
func (codec PortableRecordingCodec) DecodeWithDiagnostics(
	reader io.Reader,
) (PortableRecording, PortableRecordingDecodeDiagnostics, error) {
	policy := codec.effectivePolicy()
	document, err := decodePortableRecordingDocument(reader)
	if err != nil {
		return PortableRecording{}, PortableRecordingDecodeDiagnostics{}, portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode recording: "+err.Error(),
		)
	}
	header, err := decodePortableRecordingHeader(document)
	if err != nil {
		return PortableRecording{}, PortableRecordingDecodeDiagnostics{}, portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode recording header: "+err.Error(),
		)
	}
	// An incomplete version header must remain a malformed document. Let the
	// strict decoder inspect the complete envelope first so the value validator
	// can report the missing field instead of classifying it as unsupported.
	if header.SchemaVersion != "" && header.ReplayCompatibilityVersion != "" {
		if err := validatePortableRecordingCompatibilityWithPolicy(
			policy, header.RecordingKind, header.SchemaVersion, header.ReplayCompatibilityVersion,
		); err != nil {
			return PortableRecording{}, PortableRecordingDecodeDiagnostics{}, err
		}
	}

	var recording PortableRecording
	diagnostics, err := jsoncompat.DecodeDocument(document, &recording)
	if err != nil {
		return PortableRecording{}, PortableRecordingDecodeDiagnostics{}, portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode recording: "+err.Error(),
		)
	}
	if err := codec.Validate(recording); err != nil {
		return PortableRecording{}, PortableRecordingDecodeDiagnostics{}, err
	}
	paths, err := collectPortableRecordingDecodePaths(document, recording, diagnostics.Paths())
	if err != nil {
		return PortableRecording{}, PortableRecordingDecodeDiagnostics{}, portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode recording diagnostics: "+err.Error(),
		)
	}
	return recording, PortableRecordingDecodeDiagnostics{IgnoredJSONPaths: paths}, nil
}

// DecodeMetadata reads the identity-bearing fields of one portable recording
// without materializing its event, artifact, result, or Worker-history arrays.
func (codec PortableRecordingCodec) DecodeMetadata(reader io.Reader) (string, error) {
	if reader == nil {
		return "", portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode recording: reader is required",
		)
	}
	var value struct {
		RecordingKind              string `json:"recordingKind"`
		SchemaVersion              string `json:"schemaVersion"`
		ReplayCompatibilityVersion string `json:"replayCompatibilityVersion"`
		Session                    struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&value); err != nil {
		return "", portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode recording metadata: "+err.Error(),
		)
	}
	trailing, err := decoder.Token()
	if err != io.EOF {
		if err == nil {
			return "", portableRecordingDiagnostic(
				PortableRecordingCodeMalformedContract, "document", "", fmt.Sprintf("recording must contain exactly one JSON document; found trailing token %v", trailing),
			)
		}
		return "", portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode trailing recording metadata: "+err.Error(),
		)
	}
	if err := validatePortableRecordingCompatibilityWithPolicy(
		codec.effectivePolicy(), value.RecordingKind, value.SchemaVersion, value.ReplayCompatibilityVersion,
	); err != nil {
		return "", err
	}
	if strings.TrimSpace(value.Session.ID) == "" {
		return "", portableRecordingDiagnostic(
			PortableRecordingCodeInvalidIdentity, "session", "session.id", "is required",
		)
	}
	return strings.TrimSpace(value.Session.ID), nil
}

// Validate validates one detached recording against the codec's pinned policy.
func (codec PortableRecordingCodec) Validate(recording PortableRecording) error {
	return validatePortableRecording(recording, codec.effectivePolicy())
}

// DecodePortableRecordingWithVersions is a convenience for compatibility
// tests and migration tools that need a version-pinned reader without keeping
// a codec value.
func DecodePortableRecordingWithVersions(
	reader io.Reader, schemaVersions, replayCompatibilityVersions []string,
) (PortableRecording, error) {
	return NewPortableRecordingCodec(schemaVersions, replayCompatibilityVersions).Decode(reader)
}

// ValidatePortableRecordingWithVersions validates a detached recording against
// an explicit compatibility matrix.
func ValidatePortableRecordingWithVersions(
	recording PortableRecording, schemaVersions, replayCompatibilityVersions []string,
) error {
	return NewPortableRecordingCodec(schemaVersions, replayCompatibilityVersions).Validate(recording)
}

func (codec PortableRecordingCodec) effectivePolicy() PortableRecordingCompatibilityPolicy {
	policy := codec.policy
	if len(policy.SupportedSchemaVersions) == 0 {
		policy.SupportedSchemaVersions = slices.Clone(portableRecordingSupportedSchemaVersions)
	}
	if len(policy.SupportedReplayCompatibilityVersions) == 0 {
		policy.SupportedReplayCompatibilityVersions = slices.Clone(portableRecordingSupportedReplayVersions)
	}
	return policy
}

func normalizePortableRecordingVersions(versions []string) []string {
	if len(versions) == 0 {
		return nil
	}
	result := make([]string, 0, len(versions))
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		version = strings.TrimSpace(version)
		if version == "" {
			continue
		}
		if _, exists := seen[version]; exists {
			continue
		}
		seen[version] = struct{}{}
		result = append(result, version)
	}
	return result
}

type portableRecordingHeader struct {
	RecordingKind              string `json:"recordingKind"`
	SchemaVersion              string `json:"schemaVersion"`
	ReplayCompatibilityVersion string `json:"replayCompatibilityVersion"`
}

func decodePortableRecordingDocument(reader io.Reader) ([]byte, error) {
	document, err := jsoncompat.ReadSingleDocument(reader)
	if err != nil {
		return nil, fmt.Errorf("recording must contain exactly one JSON document: %w", err)
	}
	return document, nil
}

func decodePortableRecordingHeader(document []byte) (portableRecordingHeader, error) {
	var header portableRecordingHeader
	if err := json.Unmarshal(document, &header); err != nil {
		return portableRecordingHeader{}, err
	}
	return header, nil
}

// ValidatePortableRecording validates one portable recording document.
func ValidatePortableRecording(recording PortableRecording) error {
	return CurrentPortableRecordingCodec().Validate(recording)
}

func validatePortableRecording(
	recording PortableRecording,
	policy PortableRecordingCompatibilityPolicy,
) error {
	if err := validatePortableRecordingCompatibilityForValue(policy, recording); err != nil {
		return err
	}
	if err := validatePortableRecordingSession(recording); err != nil {
		return err
	}
	if err := validatePortableRecordingDigests(recording); err != nil {
		return err
	}
	if err := validatePortableRecordingArtifacts(recording.Artifacts); err != nil {
		return err
	}
	if err := validatePortableRecordingEvents(recording.Events, recording.Artifacts); err != nil {
		return err
	}
	if err := validatePortableRecordingCheckpoint(recording); err != nil {
		return err
	}
	if err := validatePortableRecordingResult(recording); err != nil {
		return err
	}
	return validatePortableRecordingRedaction(recording.Redaction)
}

func validatePortableRecordingCheckpoint(recording PortableRecording) error {
	if recording.Checkpoint == nil {
		return nil
	}
	checkpoint := recording.Checkpoint
	if strings.TrimSpace(checkpoint.ID) == "" || checkpoint.Timestamp.IsZero() {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "checkpoint", "checkpoint", "id and timestamp are required",
		)
	}
	if checkpoint.ArtifactID != "" {
		found := false
		for _, artifact := range recording.Artifacts {
			if artifact.ID == checkpoint.ArtifactID {
				found = true
				break
			}
		}
		if !found {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "checkpoint", "checkpoint.artifactId", "references an unknown artifact",
			)
		}
	}
	for _, event := range recording.Events {
		if event.CheckpointID == checkpoint.ID {
			return nil
		}
	}
	return portableRecordingDiagnostic(
		PortableRecordingCodeInvalidSummary, "checkpoint", "checkpoint.id", "is not referenced by a canonical event summary",
	)
}

func validatePortableRecordingResult(recording PortableRecording) error {
	if err := validatePortableRecordingVersionSpecificFields(recording); err != nil {
		return err
	}
	if recording.Result == nil {
		return nil
	}
	result := recording.Result
	if !slices.Contains([]string{"NOT_READY", "PARTIAL", "FINAL", "FAILED_WITH_PARTIAL", "UNAVAILABLE"}, result.Status) {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "result", "result.status", "is not a supported result status",
		)
	}
	if !slices.Contains([]string{"final", "partial"}, result.Mode) {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "result", "result.mode", "must be final or partial",
		)
	}
	artifactIDs := make(map[string]struct{}, len(recording.Artifacts))
	for _, artifact := range recording.Artifacts {
		artifactIDs[artifact.ID] = struct{}{}
	}
	for index, id := range result.ArtifactIDs {
		if _, ok := artifactIDs[id]; !ok {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "result", fmt.Sprintf("result.artifactIds[%d]", index), "references an unknown artifact",
			)
		}
	}
	if len(result.PrimaryResult) == 0 {
		if result.ContentHash != "" {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidDigest, "result", "result.contentHash", "must be absent when primaryResult is absent",
			)
		}
	} else {
		if !json.Valid(result.PrimaryResult) {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "result", "result.primaryResult", "must be valid JSON",
			)
		}
		digest := sha256.Sum256(compactPortableRecordingJSON(result.PrimaryResult))
		if result.ContentHash != "sha256:"+hex.EncodeToString(digest[:]) {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidDigest, "result", "result.contentHash", "does not match primaryResult",
			)
		}
	}
	if recording.Session.Status == "FAILED" && result.Status == "FINAL" {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "result", "result.status", "failed sessions cannot declare a final result",
		)
	}
	return nil
}

func validatePortableRecordingCompatibilityWithPolicy(
	policy PortableRecordingCompatibilityPolicy,
	recordingKind, schemaVersion, replayCompatibilityVersion string,
) error {
	if recordingKind != KindJavaScriptFactorySession {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidIdentity, "identity", "recordingKind",
			fmt.Sprintf("must be %q", KindJavaScriptFactorySession),
		)
	}
	if strings.TrimSpace(schemaVersion) == "" {
		return portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "compatibility", "schemaVersion", "is required",
		)
	}
	if strings.TrimSpace(replayCompatibilityVersion) == "" {
		return portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "compatibility", "replayCompatibilityVersion", "is required",
		)
	}
	if !slices.Contains(policy.SupportedReplayCompatibilityVersions, replayCompatibilityVersion) {
		return &PortableRecordingDiagnostic{
			Code: PortableRecordingCodeUnsupportedVersion, Area: "compatibility",
			Path:               "replayCompatibilityVersion",
			Message:            "unsupported replay compatibility version; upgrade the reader or migrate the recording to a supported version",
			EncounteredVersion: replayCompatibilityVersion,
			SupportedVersions:  slices.Clone(policy.SupportedReplayCompatibilityVersions),
			Action:             PortableRecordingCompatibilityAction,
		}
	}
	if !slices.Contains(policy.SupportedSchemaVersions, schemaVersion) {
		return &PortableRecordingDiagnostic{
			Code: PortableRecordingCodeUnsupportedSchema, Area: "compatibility", Path: "schemaVersion",
			Message:            "unsupported recording schema version; upgrade the reader or migrate the recording to a supported version",
			EncounteredVersion: schemaVersion,
			SupportedVersions:  slices.Clone(policy.SupportedSchemaVersions),
			Action:             PortableRecordingCompatibilityAction,
		}
	}
	return nil
}

func validatePortableRecordingCompatibilityForValue(
	policy PortableRecordingCompatibilityPolicy,
	recording PortableRecording,
) error {
	return validatePortableRecordingCompatibilityWithPolicy(
		policy,
		recording.RecordingKind, recording.SchemaVersion, recording.ReplayCompatibilityVersion,
	)
}

func validatePortableRecordingVersionSpecificFields(recording PortableRecording) error {
	switch recording.SchemaVersion {
	case PortableRecordingSchemaV1:
		// Schema 1 predates the result projection. An omitted result is part of
		// its contract and must not be defaulted into a success or failure.
		if recording.WorkerHistory != nil {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "compatibility", "workerHistory",
				"Worker history is only supported by the current schema version; export the recording again",
			)
		}
		return nil
	case PortableRecordingSchemaV2:
		if recording.WorkerHistory != nil {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "compatibility", "workerHistory",
				"Worker history is only supported by the current schema version; export the recording again",
			)
		}
		if recording.Result == nil {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "result", "result", "is required for the current schema version",
			)
		}
		return nil
	case PortableRecordingSchemaV3:
		if recording.Result == nil {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "result", "result", "is required for the current schema version",
			)
		}
		return validatePortableRecordingWorkerHistory(recording)
	default:
		return validatePortableRecordingCompatibilityForValue(
			PortableRecordingCompatibilityPolicy{
				SupportedSchemaVersions:              portableRecordingSupportedSchemaVersions,
				SupportedReplayCompatibilityVersions: portableRecordingSupportedReplayVersions,
			}, recording,
		)
	}
}

func validatePortableRecordingWorkerHistory(recording PortableRecording) error {
	history := recording.WorkerHistory
	if history == nil {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "workerHistory", "workerHistory",
			"is required for the current schema version",
		)
	}
	switch history.Availability {
	case PortableRecordingWorkerHistoryUnavailable:
		if history.WorkerPortableRecording != nil {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "workerHistory", "workerHistory.recording",
				"must be absent when Worker history is unavailable",
			)
		}
		if strings.TrimSpace(history.Reason) == "" {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "workerHistory", "workerHistory.reason",
				"is required when Worker history is unavailable",
			)
		}
		return nil
	case PortableRecordingWorkerHistoryAvailable:
		if history.WorkerPortableRecording == nil {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "workerHistory", "workerHistory.recording",
				"is required when Worker history is available",
			)
		}
		if strings.TrimSpace(history.Reason) != "" {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "workerHistory", "workerHistory.reason",
				"must be absent when Worker history is available",
			)
		}
		if err := (workerrecording.WorkerRecordingCodec{}).ValidateWorkerPortableRecording(*history.WorkerPortableRecording); err != nil {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "workerHistory", "workerHistory",
				"contains an invalid canonical Worker recording",
			)
		}
		factorySessionID := strings.TrimSpace(history.Correlation.FactorySessionID)
		if factorySessionID != "" && factorySessionID != recording.Session.ID {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidIdentity, "workerHistory", "workerHistory.correlation.factorySessionId",
				"does not match the Factory Session identity",
			)
		}
		return nil
	default:
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "workerHistory", "workerHistory.availability",
			"must be AVAILABLE or UNAVAILABLE",
		)
	}
}

func validatePortableRecordingSession(recording PortableRecording) error {
	if strings.TrimSpace(recording.Session.ID) == "" {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidIdentity, "session", "session.id", "is required",
		)
	}
	if !slices.Contains(
		[]string{"QUEUED", "AWAITING_APPROVAL", "RUNNING", "PAUSED", "RESUMING", "SUCCEEDED", "COMPLETED", "FAILED", "CANCELING", "CANCELED", "TIMED_OUT", "TERMINATED", "INTERRUPTED"},
		recording.Session.Status,
	) {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "session", "session.status", "is not a supported Factory Session status",
		)
	}
	if recording.Session.OrchestratorKind != "JAVASCRIPT" {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "session", "session.orchestratorKind", "must be JAVASCRIPT",
		)
	}
	if strings.TrimSpace(recording.Source.Ref) == "" {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "source", "source.ref", "is required",
		)
	}
	return nil
}

func validatePortableRecordingDigests(recording PortableRecording) error {
	for path, value := range map[string]string{
		"source.hash": recording.Source.Hash, "argumentsDigest": recording.ArgumentsDigest, "policyHash": recording.PolicyHash,
	} {
		if !portableRecordingSHA256Pattern.MatchString(value) {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidDigest, "digest", path, "must be a lowercase sha256 digest",
			)
		}
	}
	return nil
}

func validatePortableRecordingRedaction(redaction PortableRecordingRedactionMetadata) error {
	if !redaction.RuntimeStateOmitted || !redaction.CheckpointBodiesOmitted ||
		!redaction.ProviderTranscriptsOmitted || !redaction.ChildDispatchesOmitted {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "redaction", "redaction", "all prohibited runtime detail omission flags must be true",
		)
	}
	if redaction.SecretsRedacted < 0 || redaction.SecretsRedacted > portableRecordingMaxSecretsRedacted {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidSummary, "redaction", "redaction.secretsRedacted",
			fmt.Sprintf("must be between 0 and %d", portableRecordingMaxSecretsRedacted),
		)
	}
	return nil
}

func validatePortableRecordingArtifacts(artifacts []PortableRecordingArtifactSummary) error {
	seen := make(map[string]struct{}, len(artifacts))
	for index, artifact := range artifacts {
		path := fmt.Sprintf("artifacts[%d]", index)
		if strings.TrimSpace(artifact.ID) == "" || strings.TrimSpace(artifact.Kind) == "" ||
			strings.TrimSpace(artifact.Visibility) == "" || artifact.CreatedAt.IsZero() {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "artifacts", path, "id, kind, visibility, and createdAt are required",
			)
		}
		if _, exists := seen[artifact.ID]; exists {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidIdentity, "artifacts", path+".id", "must be unique",
			)
		}
		seen[artifact.ID] = struct{}{}
		if !portableRecordingSHA256Pattern.MatchString(artifact.ContentHash) {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidDigest, "artifacts", path+".contentHash", "must be a lowercase sha256 digest",
			)
		}
		if artifact.SizeBytes < 0 {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "artifacts", path+".sizeBytes", "must be non-negative",
			)
		}
	}
	return nil
}

func validatePortableRecordingEvents(
	events []PortableRecordingEventSummary,
	artifacts []PortableRecordingArtifactSummary,
) error {
	artifactIDs := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		artifactIDs[artifact.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(events))
	var previous int64 = -1
	for index, event := range events {
		path := fmt.Sprintf("events[%d]", index)
		if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" || event.Timestamp.IsZero() {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "events", path, "id, type, and timestamp are required",
			)
		}
		if _, exists := seen[event.ID]; exists {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidIdentity, "events", path+".id", "must be unique",
			)
		}
		seen[event.ID] = struct{}{}
		if event.Sequence < 0 || event.Sequence <= previous {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidOrder, "events", path+".sequence", "must be non-negative and strictly increasing",
			)
		}
		previous = event.Sequence
		for artifactIndex, id := range event.ArtifactIDs {
			if _, exists := artifactIDs[id]; !exists {
				return portableRecordingDiagnostic(
					PortableRecordingCodeInvalidSummary, "events",
					fmt.Sprintf("%s.artifactIds[%d]", path, artifactIndex), "references an unknown artifact",
				)
			}
		}
	}
	return nil
}

func portableRecordingDiagnostic(
	code PortableRecordingDiagnosticCode,
	area, path, message string,
) *PortableRecordingDiagnostic {
	return &PortableRecordingDiagnostic{Code: code, Area: area, Path: path, Message: message}
}
