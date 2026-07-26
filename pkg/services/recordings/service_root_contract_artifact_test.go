package recordings_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func peerRootDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func TestArtifactExportRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	service := recordings.Service(&peerRootServiceFake{})
	built := assertArtifactExportBuildValidate(t, service)
	assertArtifactExportSummarizeDecode(t, service, built)
	assertArtifactExportTypedFailures(t, service, built)
}

func samplePortableExportFacts() recordings.PortableRecordingCanonicalFacts {
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	return recordings.PortableRecordingCanonicalFacts{
		SessionID:        "session-export-1",
		Status:           "COMPLETED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        "workflow/export.js",
		SourceHash:       peerRootDigest('1'),
		PolicyHash:       peerRootDigest('2'),
		Artifacts: []recordings.PortableRecordingCanonicalArtifact{{
			ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC",
			Label: "Result", ContentHash: peerRootDigest('3'), SizeBytes: 21, CreatedAt: createdAt,
		}},
		Result: &recordings.PortableRecordingCanonicalResult{
			Status: "FINAL", Mode: "final",
			PrimaryResult: json.RawMessage(`{"answer":"ok"}`),
			ArtifactIDs:   []string{"artifact-result"},
			Availability: &recordings.PortableRecordingAvailability{
				Reason: "READY", Message: "available", Retryable: false,
			},
		},
	}
}

func assertArtifactExportBuildValidate(
	t *testing.T,
	service recordings.Service,
) recordings.PortableRecording {
	t.Helper()
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		Facts: samplePortableExportFacts(),
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact success path: %v", err)
	}
	if built.Recording.Session.ID != "session-export-1" {
		t.Fatalf("BuildPortableArtifact session = %#v, want session-export-1", built.Recording.Session)
	}
	if len(built.Recording.Artifacts) != 1 || built.Recording.Artifacts[0].ID != "artifact-result" {
		t.Fatalf("BuildPortableArtifact artifacts = %#v, want detached canonical artifact result", built.Recording.Artifacts)
	}
	if _, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: built.Recording,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact success path: %v", err)
	}
	return built.Recording
}

func assertArtifactExportSummarizeDecode(
	t *testing.T,
	service recordings.Service,
	built recordings.PortableRecording,
) {
	t.Helper()
	summary, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Recording: built,
	})
	if err != nil {
		t.Fatalf("SummarizePortableArtifact success path: %v", err)
	}
	if summary.SessionID != "session-export-1" || summary.Status != "COMPLETED" {
		t.Fatalf("SummarizePortableArtifact = %#v, want session/status summary", summary)
	}
	if summary.Availability == nil || summary.Availability.Reason != "READY" {
		t.Fatalf("SummarizePortableArtifact availability = %#v, want READY", summary.Availability)
	}
	if len(summary.Artifacts) != 1 || summary.Artifacts[0].ID != "artifact-result" {
		t.Fatalf("SummarizePortableArtifact artifacts = %#v, want detached artifact summaries", summary.Artifacts)
	}
	encoded, err := json.Marshal(built)
	if err != nil {
		t.Fatalf("marshal portable recording: %v", err)
	}
	decoded, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact success path: %v", err)
	}
	if decoded.Recording.Session.ID != built.Session.ID {
		t.Fatalf("DecodePortableArtifact = %#v, want detached recording", decoded.Recording.Session)
	}
}

func assertArtifactExportTypedFailures(
	t *testing.T,
	service recordings.Service,
	built recordings.PortableRecording,
) {
	t.Helper()
	_, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: recordings.PortableRecording{
			Session:         built.Session,
			Source:          built.Source,
			ArgumentsDigest: "not-a-digest",
			PolicyHash:      built.PolicyHash,
		},
	})
	if !errors.Is(err, recordings.ErrInvalidRecordingDigest) {
		t.Fatalf("invalid digest error = %v, want ErrInvalidRecordingDigest", err)
	}

	invalidSummary := built
	invalidSummary.Session.ID = ""
	_, err = service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: invalidSummary,
	})
	if !errors.Is(err, recordings.ErrInvalidRecordingSummary) {
		t.Fatalf("invalid summary error = %v, want ErrInvalidRecordingSummary", err)
	}
	if errors.Is(err, recordings.ErrInvalidRecordingDigest) {
		t.Fatalf("invalid summary must remain distinct from ErrInvalidRecordingDigest")
	}

	_, err = service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	})
	if !errors.Is(err, recordings.ErrInvalidRecordingDecode) {
		t.Fatalf("decode failure error = %v, want ErrInvalidRecordingDecode", err)
	}
	if errors.Is(err, recordings.ErrInvalidRecordingDigest) || errors.Is(err, recordings.ErrInvalidRecordingSummary) {
		t.Fatalf("decode failure must remain distinct from digest/summary typed errors")
	}
}
