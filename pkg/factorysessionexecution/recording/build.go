package recording

import (
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
	Artifacts                           []CanonicalArtifact
	Events                              []json.RawMessage
}

type CanonicalArtifact struct {
	ID, Kind, Visibility, Label, ContentHash string
	SizeBytes                                int64
	CreatedAt                                time.Time
	SecretsRedacted                          int64
}

// Build maps canonical public facts into the portable privacy-bounded contract.
func Build(facts CanonicalFacts) (Recording, error) {
	argumentsDigest, err := digestCanonicalJSON(facts.Arguments)
	if err != nil {
		return Recording{}, fmt.Errorf("digest arguments: %w", err)
	}
	artifacts := make([]ArtifactSummary, 0, len(facts.Artifacts))
	var secretsRedacted int64
	for _, artifact := range facts.Artifacts {
		artifacts = append(artifacts, ArtifactSummary{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			Label: artifact.Label, ContentHash: artifact.ContentHash,
			SizeBytes: artifact.SizeBytes, CreatedAt: artifact.CreatedAt.UTC(),
		})
		secretsRedacted += artifact.SecretsRedacted
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
	return value, Validate(value)
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
				Sequence  int64     `json:"sequence"`
				EventTime time.Time `json:"eventTime"`
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
		summaries = append(summaries, EventSummary{ID: event.ID, Type: event.Type, Sequence: event.Context.Sequence, Timestamp: event.Context.EventTime.UTC(), ArtifactIDs: event.Payload.ArtifactIDs})
	}
	return summaries, nil
}
