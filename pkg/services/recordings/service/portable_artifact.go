package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
)

func (service *combinedService) BuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	snapshot, err := service.Snapshot(request.RecordingID)
	if err != nil || snapshot.Status.FinalizedAt == nil {
		return recordings.BuildPortableArtifactResult{}, recordings.ErrPortableArtifactUnavailable
	}
	artifact := recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary:       portableArtifactSummary(snapshot.Status, snapshot.Events),
		Events:        cloneCanonicalEvents(snapshot.Events),
		Integrity: recordings.PortableArtifactIntegrity{
			Algorithm: recordings.PortableArtifactIntegritySHA256,
		},
	}
	digest, err := portableArtifactDigest(artifact)
	if err != nil {
		return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
	}
	artifact.Integrity.Digest = digest
	if err := validatePortableArtifact(artifact); err != nil {
		return recordings.BuildPortableArtifactResult{}, err
	}
	return recordings.BuildPortableArtifactResult{Artifact: artifact}, nil
}

func (service *combinedService) ValidatePortableArtifact(
	request recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	if err := validatePortableArtifact(request.Artifact); err != nil {
		return recordings.ValidatePortableArtifactResult{}, err
	}
	return recordings.ValidatePortableArtifactResult{
		Summary: clonePortableArtifactSummary(request.Artifact.Summary),
	}, nil
}

func (service *combinedService) EncodePortableArtifact(
	request recordings.EncodePortableArtifactRequest,
) (recordings.EncodePortableArtifactResult, error) {
	if err := validatePortableArtifact(request.Artifact); err != nil {
		return recordings.EncodePortableArtifactResult{}, err
	}
	payload, err := json.Marshal(request.Artifact)
	if err != nil {
		return recordings.EncodePortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
	}
	return recordings.EncodePortableArtifactResult{Payload: payload}, nil
}

func (service *combinedService) DecodePortableArtifact(
	request recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(request.Payload))
	decoder.DisallowUnknownFields()
	var artifact recordings.PortableArtifact
	if len(request.Payload) == 0 || decoder.Decode(&artifact) != nil {
		return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
	}
	if err := validatePortableArtifact(artifact); err != nil {
		return recordings.DecodePortableArtifactResult{}, err
	}
	return recordings.DecodePortableArtifactResult{
		Artifact: clonePortableArtifact(artifact),
	}, nil
}

func (service *combinedService) SummarizePortableArtifact(
	request recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	if err := validatePortableArtifact(request.Artifact); err != nil {
		return recordings.SummarizePortableArtifactResult{}, err
	}
	return recordings.SummarizePortableArtifactResult{
		Summary: clonePortableArtifactSummary(request.Artifact.Summary),
	}, nil
}

func portableArtifactSummary(
	status recordings.RecordingStatusFacts,
	events []recordings.CanonicalEvent,
) recordings.PortableArtifactSummary {
	summary := recordings.PortableArtifactSummary{
		RecordingID: status.RecordingID,
		Reference:   status.Artifact,
		Scope:       status.Scope,
		State:       status.State,
		EventCount:  len(events),
		Failures:    append([]recordings.RecordingFailure{}, status.Failures...),
		Available:   true,
	}
	if len(events) > 0 {
		first := events[0].Cursor
		last := events[len(events)-1].Cursor
		summary.FirstCursor = &first
		summary.LastCursor = &last
	}
	return summary
}

func validatePortableArtifact(artifact recordings.PortableArtifact) error {
	if artifact.SchemaVersion != recordings.PortableArtifactSchemaV1 {
		return recordings.ErrUnsupportedPortableArtifactSchema
	}
	if err := validatePortableArtifactSummary(artifact); err != nil {
		return err
	}
	if err := validatePortableArtifactEvents(artifact); err != nil {
		return err
	}
	if artifact.Integrity.Algorithm != recordings.PortableArtifactIntegritySHA256 {
		return recordings.ErrInvalidPortableArtifactIntegrity
	}
	expected, err := portableArtifactDigest(artifact)
	if err != nil || artifact.Integrity.Digest != expected {
		return recordings.ErrInvalidPortableArtifactIntegrity
	}
	return nil
}

func validatePortableArtifactSummary(artifact recordings.PortableArtifact) error {
	summary := artifact.Summary
	if strings.TrimSpace(string(summary.RecordingID)) == "" ||
		!summary.Available || summary.EventCount != len(artifact.Events) {
		return recordings.ErrInvalidPortableArtifact
	}
	if summary.State != recordings.RecordingFinalized &&
		summary.State != recordings.RecordingFailed {
		return recordings.ErrPortableArtifactUnavailable
	}
	if len(artifact.Events) == 0 {
		if summary.FirstCursor != nil || summary.LastCursor != nil {
			return recordings.ErrInvalidPortableArtifactOrder
		}
		return nil
	}
	if summary.FirstCursor == nil || summary.LastCursor == nil ||
		*summary.FirstCursor != artifact.Events[0].Cursor ||
		*summary.LastCursor != artifact.Events[len(artifact.Events)-1].Cursor {
		return recordings.ErrInvalidPortableArtifactOrder
	}
	return nil
}

func validatePortableArtifactEvents(artifact recordings.PortableArtifact) error {
	var previous recordings.CanonicalEvent
	for index, event := range artifact.Events {
		if !canonical.ValidAppendEvent(event) || event.Scope != artifact.Summary.Scope ||
			event.Cursor.StreamGenerationID == "" ||
			event.Cursor.Sequence != event.Sequence ||
			event.Sequence < 0 {
			return recordings.ErrInvalidPortableArtifactOrder
		}
		if index > 0 {
			if event.Cursor.StreamGenerationID != previous.Cursor.StreamGenerationID ||
				event.Sequence <= previous.Sequence {
				return recordings.ErrInvalidPortableArtifactOrder
			}
			if artifact.Summary.Scope.FactorySessionID == "" &&
				event.Sequence != previous.Sequence+1 {
				return recordings.ErrInvalidPortableArtifactOrder
			}
		}
		previous = event
	}
	return nil
}

func portableArtifactDigest(artifact recordings.PortableArtifact) (string, error) {
	artifact.Integrity.Digest = ""
	payload, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return recordings.PortableArtifactIntegritySHA256 + ":" +
		hex.EncodeToString(digest[:]), nil
}

func clonePortableArtifact(artifact recordings.PortableArtifact) recordings.PortableArtifact {
	artifact.Events = cloneCanonicalEvents(artifact.Events)
	artifact.Summary = clonePortableArtifactSummary(artifact.Summary)
	return artifact
}

func clonePortableArtifactSummary(
	summary recordings.PortableArtifactSummary,
) recordings.PortableArtifactSummary {
	summary.Failures = append([]recordings.RecordingFailure{}, summary.Failures...)
	if summary.FirstCursor != nil {
		first := *summary.FirstCursor
		summary.FirstCursor = &first
	}
	if summary.LastCursor != nil {
		last := *summary.LastCursor
		summary.LastCursor = &last
	}
	return summary
}

func cloneCanonicalEvents(events []recordings.CanonicalEvent) []recordings.CanonicalEvent {
	return append([]recordings.CanonicalEvent{}, events...)
}
