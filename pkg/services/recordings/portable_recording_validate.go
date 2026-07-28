package recordings

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

var portableRecordingSHA256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// DecodePortableRecording decodes and validates one portable recording document.
func DecodePortableRecording(reader io.Reader) (PortableRecording, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var recording PortableRecording
	if err := decoder.Decode(&recording); err != nil {
		return PortableRecording{}, portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "decode recording: "+err.Error(),
		)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return PortableRecording{}, portableRecordingDiagnostic(
			PortableRecordingCodeMalformedContract, "document", "", "recording must contain exactly one JSON document",
		)
	}
	if err := ValidatePortableRecording(recording); err != nil {
		return PortableRecording{}, err
	}
	return recording, nil
}

// ValidatePortableRecording validates one portable recording document.
func ValidatePortableRecording(recording PortableRecording) error {
	if err := validatePortableRecordingCompatibility(recording); err != nil {
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
	if recording.Result == nil {
		if recording.SchemaVersion == portableRecordingSchemaV2 {
			return portableRecordingDiagnostic(
				PortableRecordingCodeInvalidSummary, "result", "result", "is required for the current schema version",
			)
		}
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

func validatePortableRecordingCompatibility(recording PortableRecording) error {
	if recording.RecordingKind != KindJavaScriptFactorySession {
		return portableRecordingDiagnostic(
			PortableRecordingCodeInvalidIdentity, "identity", "recordingKind",
			fmt.Sprintf("must be %q", KindJavaScriptFactorySession),
		)
	}
	if !slices.Contains(portableRecordingSupportedReplayVersions, recording.ReplayCompatibilityVersion) {
		return &PortableRecordingDiagnostic{
			Code: PortableRecordingCodeUnsupportedVersion, Area: "compatibility",
			Path: "replayCompatibilityVersion",
			Message: "unsupported replay compatibility version; use a supported version or migrate the recording",
			SupportedVersions: slices.Clone(portableRecordingSupportedReplayVersions),
		}
	}
	if !slices.Contains(portableRecordingSupportedSchemaVersions, recording.SchemaVersion) {
		return &PortableRecordingDiagnostic{
			Code: PortableRecordingCodeUnsupportedVersion, Area: "compatibility", Path: "schemaVersion",
			Message: "unsupported recording schema version; migrate the recording before replay",
			SupportedVersions: slices.Clone(portableRecordingSupportedSchemaVersions),
		}
	}
	return nil
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
				PortableRecordingCodeInvalidSummary, "events", path+".sequence", "must be non-negative and strictly increasing",
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
