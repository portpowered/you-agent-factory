package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

type DiagnosticCode string

const (
	CodeMalformedContract  DiagnosticCode = "MALFORMED_RECORDING_CONTRACT"
	CodeUnsupportedVersion DiagnosticCode = "UNSUPPORTED_REPLAY_COMPATIBILITY_VERSION"
	CodeInvalidIdentity    DiagnosticCode = "INVALID_RECORDING_IDENTITY"
	CodeInvalidDigest      DiagnosticCode = "INVALID_RECORDING_DIGEST"
	CodeInvalidSummary     DiagnosticCode = "INVALID_RECORDING_SUMMARY"
)

type Diagnostic struct {
	Code              DiagnosticCode `json:"code"`
	Area              string         `json:"area"`
	Path              string         `json:"path,omitempty"`
	Message           string         `json:"message"`
	SupportedVersions []string       `json:"supportedVersions,omitempty"`
}

func (d *Diagnostic) Error() string {
	if d == nil {
		return ""
	}
	if d.Path == "" {
		return d.Message
	}
	return d.Path + ": " + d.Message
}

var sha256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func DecodeAndValidate(reader io.Reader) (Recording, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var recording Recording
	if err := decoder.Decode(&recording); err != nil {
		return Recording{}, diagnostic(CodeMalformedContract, "document", "", "decode recording: "+err.Error())
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Recording{}, diagnostic(CodeMalformedContract, "document", "", "recording must contain exactly one JSON document")
	}
	if err := Validate(recording); err != nil {
		return Recording{}, err
	}
	return recording, nil
}

func Validate(r Recording) error {
	if err := validateCompatibility(r); err != nil {
		return err
	}
	if err := validateSession(r); err != nil {
		return err
	}
	if err := validateDigests(r); err != nil {
		return err
	}
	if err := validateArtifacts(r.Artifacts); err != nil {
		return err
	}
	if err := validateEvents(r.Events, r.Artifacts); err != nil {
		return err
	}
	if err := validateCheckpoint(r); err != nil {
		return err
	}
	if err := validateResult(r); err != nil {
		return err
	}
	return validateRedaction(r.Redaction)
}

func validateCheckpoint(r Recording) error {
	if r.Checkpoint == nil {
		return nil
	}
	checkpoint := r.Checkpoint
	if strings.TrimSpace(checkpoint.ID) == "" || checkpoint.Timestamp.IsZero() {
		return diagnostic(CodeInvalidSummary, "checkpoint", "checkpoint", "id and timestamp are required")
	}
	if checkpoint.ArtifactID != "" {
		found := false
		for _, artifact := range r.Artifacts {
			if artifact.ID == checkpoint.ArtifactID {
				found = true
				break
			}
		}
		if !found {
			return diagnostic(CodeInvalidSummary, "checkpoint", "checkpoint.artifactId", "references an unknown artifact")
		}
	}
	for _, event := range r.Events {
		if event.CheckpointID == checkpoint.ID {
			return nil
		}
	}
	return diagnostic(CodeInvalidSummary, "checkpoint", "checkpoint.id", "is not referenced by a canonical event summary")
}

func validateResult(r Recording) error {
	if r.Result == nil {
		if r.SchemaVersion == CurrentSchemaVersion {
			return diagnostic(CodeInvalidSummary, "result", "result", "is required for the current schema version")
		}
		return nil
	}
	result := r.Result
	if !slices.Contains([]string{"NOT_READY", "PARTIAL", "FINAL", "FAILED_WITH_PARTIAL", "UNAVAILABLE"}, result.Status) {
		return diagnostic(CodeInvalidSummary, "result", "result.status", "is not a supported result status")
	}
	if !slices.Contains([]string{"final", "partial"}, result.Mode) {
		return diagnostic(CodeInvalidSummary, "result", "result.mode", "must be final or partial")
	}
	artifactIDs := make(map[string]struct{}, len(r.Artifacts))
	for _, artifact := range r.Artifacts {
		artifactIDs[artifact.ID] = struct{}{}
	}
	for index, id := range result.ArtifactIDs {
		if _, ok := artifactIDs[id]; !ok {
			return diagnostic(CodeInvalidSummary, "result", fmt.Sprintf("result.artifactIds[%d]", index), "references an unknown artifact")
		}
	}
	if len(result.PrimaryResult) == 0 {
		if result.ContentHash != "" {
			return diagnostic(CodeInvalidDigest, "result", "result.contentHash", "must be absent when primaryResult is absent")
		}
	} else {
		if !json.Valid(result.PrimaryResult) {
			return diagnostic(CodeInvalidSummary, "result", "result.primaryResult", "must be valid JSON")
		}
		digest := sha256.Sum256(compactJSON(result.PrimaryResult))
		if result.ContentHash != "sha256:"+hex.EncodeToString(digest[:]) {
			return diagnostic(CodeInvalidDigest, "result", "result.contentHash", "does not match primaryResult")
		}
	}
	if r.Session.Status == "FAILED" && result.Status == "FINAL" {
		return diagnostic(CodeInvalidSummary, "result", "result.status", "failed sessions cannot declare a final result")
	}
	return nil
}

func validateCompatibility(r Recording) error {
	if r.RecordingKind != KindJavaScriptFactorySession {
		return diagnostic(CodeInvalidIdentity, "identity", "recordingKind", fmt.Sprintf("must be %q", KindJavaScriptFactorySession))
	}
	if !slices.Contains(supportedReplayCompatibilityVersions, r.ReplayCompatibilityVersion) {
		return &Diagnostic{Code: CodeUnsupportedVersion, Area: "compatibility", Path: "replayCompatibilityVersion", Message: "unsupported replay compatibility version; use a supported version or migrate the recording", SupportedVersions: slices.Clone(supportedReplayCompatibilityVersions)}
	}
	if !slices.Contains(supportedSchemaVersions, r.SchemaVersion) {
		return &Diagnostic{Code: CodeUnsupportedVersion, Area: "compatibility", Path: "schemaVersion", Message: "unsupported recording schema version; migrate the recording before replay", SupportedVersions: slices.Clone(supportedSchemaVersions)}
	}
	return nil
}

func validateSession(r Recording) error {
	if strings.TrimSpace(r.Session.ID) == "" {
		return diagnostic(CodeInvalidIdentity, "session", "session.id", "is required")
	}
	if !slices.Contains([]string{"QUEUED", "AWAITING_APPROVAL", "RUNNING", "PAUSED", "RESUMING", "SUCCEEDED", "COMPLETED", "FAILED", "CANCELING", "CANCELED", "TIMED_OUT", "TERMINATED", "INTERRUPTED"}, r.Session.Status) {
		return diagnostic(CodeInvalidSummary, "session", "session.status", "is not a supported Factory Session status")
	}
	if r.Session.OrchestratorKind != "JAVASCRIPT" {
		return diagnostic(CodeInvalidSummary, "session", "session.orchestratorKind", "must be JAVASCRIPT")
	}
	if strings.TrimSpace(r.Source.Ref) == "" {
		return diagnostic(CodeInvalidSummary, "source", "source.ref", "is required")
	}
	return nil
}

func validateDigests(r Recording) error {
	for path, value := range map[string]string{"source.hash": r.Source.Hash, "argumentsDigest": r.ArgumentsDigest, "policyHash": r.PolicyHash} {
		if !sha256Pattern.MatchString(value) {
			return diagnostic(CodeInvalidDigest, "digest", path, "must be a lowercase sha256 digest")
		}
	}
	return nil
}

func validateRedaction(redaction RedactionMetadata) error {
	if !redaction.RuntimeStateOmitted || !redaction.CheckpointBodiesOmitted || !redaction.ProviderTranscriptsOmitted || !redaction.ChildDispatchesOmitted {
		return diagnostic(CodeInvalidSummary, "redaction", "redaction", "all prohibited runtime detail omission flags must be true")
	}
	if redaction.SecretsRedacted < 0 || redaction.SecretsRedacted > MaxSecretsRedacted {
		return diagnostic(CodeInvalidSummary, "redaction", "redaction.secretsRedacted", fmt.Sprintf("must be between 0 and %d", MaxSecretsRedacted))
	}
	return nil
}

func validateArtifacts(artifacts []ArtifactSummary) error {
	seen := make(map[string]struct{}, len(artifacts))
	for i, artifact := range artifacts {
		path := fmt.Sprintf("artifacts[%d]", i)
		if strings.TrimSpace(artifact.ID) == "" || strings.TrimSpace(artifact.Kind) == "" || strings.TrimSpace(artifact.Visibility) == "" || artifact.CreatedAt.IsZero() {
			return diagnostic(CodeInvalidSummary, "artifacts", path, "id, kind, visibility, and createdAt are required")
		}
		if _, exists := seen[artifact.ID]; exists {
			return diagnostic(CodeInvalidIdentity, "artifacts", path+".id", "must be unique")
		}
		seen[artifact.ID] = struct{}{}
		if !sha256Pattern.MatchString(artifact.ContentHash) {
			return diagnostic(CodeInvalidDigest, "artifacts", path+".contentHash", "must be a lowercase sha256 digest")
		}
		if artifact.SizeBytes < 0 {
			return diagnostic(CodeInvalidSummary, "artifacts", path+".sizeBytes", "must be non-negative")
		}
	}
	return nil
}

func validateEvents(events []EventSummary, artifacts []ArtifactSummary) error {
	artifactIDs := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		artifactIDs[artifact.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(events))
	var previous int64 = -1
	for i, event := range events {
		path := fmt.Sprintf("events[%d]", i)
		if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" || event.Timestamp.IsZero() {
			return diagnostic(CodeInvalidSummary, "events", path, "id, type, and timestamp are required")
		}
		if _, exists := seen[event.ID]; exists {
			return diagnostic(CodeInvalidIdentity, "events", path+".id", "must be unique")
		}
		seen[event.ID] = struct{}{}
		if event.Sequence < 0 || event.Sequence <= previous {
			return diagnostic(CodeInvalidSummary, "events", path+".sequence", "must be non-negative and strictly increasing")
		}
		previous = event.Sequence
		for j, id := range event.ArtifactIDs {
			if _, exists := artifactIDs[id]; !exists {
				return diagnostic(CodeInvalidSummary, "events", fmt.Sprintf("%s.artifactIds[%d]", path, j), "references an unknown artifact")
			}
		}
	}
	return nil
}

func diagnostic(code DiagnosticCode, area, path, message string) *Diagnostic {
	return &Diagnostic{Code: code, Area: area, Path: path, Message: message}
}
