package artifacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CanonicalFacts contains only public facts accepted from the canonical Factory
// Session owner. Runtime records, checkpoints, transcripts, and dispatches have
// no representation at this boundary.
type CanonicalFacts struct {
	SessionID, Status, OrchestratorKind string
	SourceRef, SourceHash, PolicyHash   string
	Arguments                           map[string]any
	ArgumentsDigest                     string
	Artifacts                           []CanonicalArtifact
	Events                              []json.RawMessage
	Checkpoint                          *CanonicalCheckpoint
	Result                              *CanonicalResult
}

type CanonicalCheckpoint struct {
	ID, Label, Summary, ArtifactID string
	Timestamp                      time.Time
}

type CanonicalArtifact struct {
	ID, Kind, Visibility, Label, ContentHash string
	SizeBytes                                int64
	CreatedAt                                time.Time
	SecretsRedacted                          int64
}

type CanonicalResult struct {
	Status, Mode  string
	PrimaryResult json.RawMessage
	ArtifactIDs   []string
	Failure       *FailureSummary
	Availability  *AvailabilityDetail
}

// Build maps canonical public facts into the portable privacy-bounded contract.
func Build(facts CanonicalFacts) (Recording, error) {
	argumentsDigest := strings.TrimSpace(facts.ArgumentsDigest)
	if argumentsDigest == "" {
		var err error
		argumentsDigest, err = digestCanonicalJSON(facts.Arguments)
		if err != nil {
			return Recording{}, fmt.Errorf("digest arguments: %w", err)
		}
	}
	artifacts := make([]ArtifactSummary, 0, len(facts.Artifacts))
	var secretsRedacted int64
	for _, artifact := range facts.Artifacts {
		artifacts = append(artifacts, ArtifactSummary{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			Label: artifact.Label, ContentHash: artifact.ContentHash,
			SizeBytes: artifact.SizeBytes, CreatedAt: artifact.CreatedAt.UTC(),
		})
		secretsRedacted = saturatingSecretsRedacted(secretsRedacted, artifact.SecretsRedacted)
	}
	events, err := eventSummaries(facts.Events)
	if err != nil {
		return Recording{}, err
	}
	value := Recording{
		RecordingKind: KindJavaScriptFactorySession, SchemaVersion: CurrentSchemaVersion,
		ReplayCompatibilityVersion: ReplayCompatibilityVersion,
		Session:                    SessionSummary{ID: facts.SessionID, Status: facts.Status, OrchestratorKind: facts.OrchestratorKind},
		Source:                     SourceSummary{Ref: facts.SourceRef, Hash: facts.SourceHash},
		ArgumentsDigest:            argumentsDigest, PolicyHash: facts.PolicyHash,
		Artifacts: artifacts, Events: events,
		Redaction: RedactionMetadata{RuntimeStateOmitted: true, CheckpointBodiesOmitted: true, ProviderTranscriptsOmitted: true, ChildDispatchesOmitted: true, SecretsRedacted: secretsRedacted},
	}
	if facts.Checkpoint != nil {
		value.Checkpoint = &CheckpointSummary{ID: facts.Checkpoint.ID, Label: facts.Checkpoint.Label, Summary: facts.Checkpoint.Summary, Timestamp: facts.Checkpoint.Timestamp.UTC(), ArtifactID: facts.Checkpoint.ArtifactID}
	}
	if facts.Result != nil {
		value.Result = &ResultProjection{
			Status: facts.Result.Status, Mode: facts.Result.Mode,
			PrimaryResult: append(json.RawMessage(nil), facts.Result.PrimaryResult...),
			ArtifactIDs:   append([]string(nil), facts.Result.ArtifactIDs...),
			Failure:       facts.Result.Failure, Availability: facts.Result.Availability,
		}
		if len(value.Result.PrimaryResult) > 0 {
			digest := sha256.Sum256(compactJSON(value.Result.PrimaryResult))
			value.Result.ContentHash = "sha256:" + hex.EncodeToString(digest[:])
		}
	}
	return value, Validate(value)
}

func saturatingSecretsRedacted(total, count int64) int64 {
	if count <= 0 || total >= MaxSecretsRedacted {
		return total
	}
	if count >= MaxSecretsRedacted-total {
		return MaxSecretsRedacted
	}
	return total + count
}

func compactJSON(value json.RawMessage) []byte {
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, value); err != nil {
		return value
	}
	return compacted.Bytes()
}

func digestCanonicalJSON(value map[string]any) (string, error) {
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

func eventSummaries(events []json.RawMessage) ([]EventSummary, error) {
	summaries := make([]EventSummary, 0, len(events))
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
		summaries = append(summaries, EventSummary{ID: event.ID, Type: event.Type, Sequence: event.Context.Sequence, Timestamp: event.Context.EventTime.UTC(), ArtifactIDs: event.Payload.ArtifactIDs, CheckpointID: checkpointID})
	}
	return summaries, nil
}
