package recordings

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BuildPortableRecording maps canonical public facts into the portable
// privacy-bounded contract.
func BuildPortableRecording(facts PortableRecordingCanonicalFacts) (PortableRecording, error) {
	argumentsDigest := strings.TrimSpace(facts.ArgumentsDigest)
	if argumentsDigest == "" {
		var err error
		argumentsDigest, err = digestPortableRecordingCanonicalJSON(facts.Arguments)
		if err != nil {
			return PortableRecording{}, fmt.Errorf("digest arguments: %w", err)
		}
	}
	artifacts := make([]PortableRecordingArtifactSummary, 0, len(facts.Artifacts))
	var secretsRedacted int64
	for _, artifact := range facts.Artifacts {
		artifacts = append(artifacts, PortableRecordingArtifactSummary{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			Label: artifact.Label, ContentHash: artifact.ContentHash,
			SizeBytes: artifact.SizeBytes, CreatedAt: artifact.CreatedAt.UTC(),
		})
		secretsRedacted = saturatingPortableRecordingSecretsRedacted(secretsRedacted, artifact.SecretsRedacted)
	}
	events, err := portableRecordingEventSummaries(facts.Events)
	if err != nil {
		return PortableRecording{}, err
	}
	value := PortableRecording{
		RecordingKind: KindJavaScriptFactorySession, SchemaVersion: portableRecordingSchemaV2,
		ReplayCompatibilityVersion: portableRecordingReplayCompat,
		Session: PortableRecordingSessionSummary{
			ID: facts.SessionID, Status: facts.Status, OrchestratorKind: facts.OrchestratorKind,
		},
		Source:          PortableRecordingSourceSummary{Ref: facts.SourceRef, Hash: facts.SourceHash},
		ArgumentsDigest: argumentsDigest, PolicyHash: facts.PolicyHash,
		Artifacts: artifacts, Events: events,
		Redaction: PortableRecordingRedactionMetadata{
			RuntimeStateOmitted: true, CheckpointBodiesOmitted: true,
			ProviderTranscriptsOmitted: true, ChildDispatchesOmitted: true,
			SecretsRedacted: secretsRedacted,
		},
	}
	if facts.Checkpoint != nil {
		value.Checkpoint = &PortableRecordingCheckpointSummary{
			ID: facts.Checkpoint.ID, Label: facts.Checkpoint.Label, Summary: facts.Checkpoint.Summary,
			Timestamp: facts.Checkpoint.Timestamp.UTC(), ArtifactID: facts.Checkpoint.ArtifactID,
		}
	}
	if facts.Result != nil {
		value.Result = &PortableRecordingResult{
			Status: facts.Result.Status, Mode: facts.Result.Mode,
			PrimaryResult: append(json.RawMessage(nil), facts.Result.PrimaryResult...),
			ArtifactIDs:   append([]string(nil), facts.Result.ArtifactIDs...),
			Failure:       facts.Result.Failure, Availability: facts.Result.Availability,
		}
		if len(value.Result.PrimaryResult) > 0 {
			digest := sha256.Sum256(compactPortableRecordingJSON(value.Result.PrimaryResult))
			value.Result.ContentHash = "sha256:" + hex.EncodeToString(digest[:])
		}
	}
	return value, ValidatePortableRecording(value)
}

func saturatingPortableRecordingSecretsRedacted(total, count int64) int64 {
	if count <= 0 || total >= portableRecordingMaxSecretsRedacted {
		return total
	}
	if count >= portableRecordingMaxSecretsRedacted-total {
		return portableRecordingMaxSecretsRedacted
	}
	return total + count
}

func compactPortableRecordingJSON(value json.RawMessage) []byte {
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, value); err != nil {
		return value
	}
	return compacted.Bytes()
}

func digestPortableRecordingCanonicalJSON(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func portableRecordingEventSummaries(events []json.RawMessage) ([]PortableRecordingEventSummary, error) {
	summaries := make([]PortableRecordingEventSummary, 0, len(events))
	for index, raw := range events {
		var event struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Context struct {
				Sequence     int64     `json:"sequence"`
				EventTime    time.Time `json:"eventTime"`
				CheckpointID *string   `json:"checkpointId"`
			} `json:"context"`
			Payload struct {
				ArtifactIDs []string `json:"artifactIds"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, fmt.Errorf("summarize canonical event %d: %w", index, err)
		}
		if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" || event.Context.EventTime.IsZero() {
			return nil, fmt.Errorf("summarize canonical event %d: id, type, and eventTime are required", index)
		}
		checkpointID := ""
		if event.Context.CheckpointID != nil {
			checkpointID = strings.TrimSpace(*event.Context.CheckpointID)
		}
		summaries = append(summaries, PortableRecordingEventSummary{
			ID: event.ID, Type: event.Type, Sequence: event.Context.Sequence,
			Timestamp: event.Context.EventTime.UTC(), ArtifactIDs: event.Payload.ArtifactIDs,
			CheckpointID: checkpointID,
		})
	}
	return summaries, nil
}
