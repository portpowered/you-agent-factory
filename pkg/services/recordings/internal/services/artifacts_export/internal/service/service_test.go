package service_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

type snapshotSourceFake struct {
	snapshot recordinglifecycle.Snapshot
	err      error
}

func (fake snapshotSourceFake) Snapshot(recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	if fake.err != nil {
		return recordinglifecycle.Snapshot{}, fake.err
	}
	return fake.snapshot, nil
}

func TestBuildPortableArtifactRejectsActiveRecording(t *testing.T) {
	t.Parallel()

	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-1",
				State:       recordings.RecordingActive,
			},
		},
	})
	_, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "recording-1",
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("BuildPortableArtifact active = %v, want ErrPortableArtifactUnavailable", err)
	}
}

func TestBuildPortableArtifactProducesValidatedArtifact(t *testing.T) {
	t.Parallel()

	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	event := recordings.CanonicalEvent{
		ID: "event-1", Kind: "WORK_REQUEST",
		Sequence: 0,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-1",
			Sequence:           0,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    "{}",
	}
	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-1",
				Artifact:    "artifact:recording-1",
				Scope:       scope,
				State:       recordings.RecordingFinalized,
				FinalizedAt: &finalizedAt,
			},
			Events: []recordings.CanonicalEvent{event},
		},
	})
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "recording-1",
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if _, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: built.Artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	assertPortableArtifactRoundTrip(t, service, built.Artifact, encoded.Payload, nil)
}

func TestBuildPortableArtifactRoundTripOmitsPrivateServiceTarget(t *testing.T) {
	t.Parallel()

	const (
		privateServiceTarget = "/private/ledger/storage/recording-internal.json"
		reportedReference    = "artifact:reported-export"
	)
	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-private-export"}
	event := recordings.CanonicalEvent{
		ID: "event-private-export", Kind: "WORK_REQUEST",
		Sequence: 0,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-private-export",
			Sequence:           0,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    `{"public":true}`,
	}
	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-private-export",
				Artifact:    recordings.RecordingArtifactReference(reportedReference),
				Scope:       scope,
				State:       recordings.RecordingFinalized,
				FinalizedAt: &finalizedAt,
			},
			Events: []recordings.CanonicalEvent{event},
		},
	})
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "recording-private-export",
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if built.Artifact.Summary.Reference != recordings.RecordingArtifactReference(reportedReference) {
		t.Fatalf("summary reference = %q, want reported reference %q", built.Artifact.Summary.Reference, reportedReference)
	}
	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil {
		t.Fatalf("EncodePortableArtifact: %v", err)
	}
	assertPortableArtifactRoundTrip(
		t,
		service,
		built.Artifact,
		encoded.Payload,
		[]string{privateServiceTarget, "__factory_session_id__"},
	)
}

func assertPortableArtifactRoundTrip(
	t *testing.T,
	service *artifactsexportservice.Service,
	artifact recordings.PortableArtifact,
	payload []byte,
	forbiddenSubstrings []string,
) {
	t.Helper()

	validated, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	if !reflect.DeepEqual(validated.Summary, artifact.Summary) {
		t.Fatalf("validated summary = %#v, want %#v", validated.Summary, artifact.Summary)
	}
	decoded, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact: %v", err)
	}
	if decoded.Artifact.Integrity != artifact.Integrity ||
		decoded.Artifact.Summary.RecordingID != artifact.Summary.RecordingID ||
		decoded.Artifact.Summary.EventCount != artifact.Summary.EventCount ||
		decoded.Artifact.Summary.Reference != artifact.Summary.Reference {
		t.Fatalf("decoded public facts = %#v, want %#v", decoded.Artifact, artifact)
	}
	summarized, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: decoded.Artifact,
	})
	if err != nil {
		t.Fatalf("SummarizePortableArtifact: %v", err)
	}
	if !reflect.DeepEqual(summarized.Summary, artifact.Summary) {
		t.Fatalf("summarized summary = %#v, want %#v", summarized.Summary, artifact.Summary)
	}
	payloadText := string(payload)
	for _, forbidden := range forbiddenSubstrings {
		if strings.Contains(payloadText, forbidden) {
			t.Fatalf("encoded payload leaked %q:\n%s", forbidden, payloadText)
		}
	}
}
