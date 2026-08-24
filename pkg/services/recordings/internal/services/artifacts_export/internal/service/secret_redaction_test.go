package service_test

import (
	"encoding/json"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

func TestPortableArtifactExportRedactsBeforeBuildAndEncode(t *testing.T) {
	t.Parallel()

	finalizedAt := time.Date(2026, 8, 24, 12, 10, 0, 0, time.UTC)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-portable-secret"}
	event := recordings.CanonicalEvent{
		ID:         "portable-secret-event",
		Kind:       "WORK_REQUEST",
		Sequence:   0,
		Scope:      scope,
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "generation-portable-secret", Sequence: 0},
		RecordedAt: finalizedAt.Add(-time.Minute),
		Payload:    `{"credential":"portable-artifact-secret-002","control":"portable-artifact-control"}`,
	}
	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-portable-secret",
				Artifact:    "artifact:portable-secret",
				Scope:       scope,
				State:       recordings.RecordingFinalized,
				FinalizedAt: &finalizedAt,
			},
			Events: []recordings.CanonicalEvent{event},
			SecretProvenance: map[int][]recordings.RecordingSecret{
				0: {{
					JSONPointer: "/credential",
					Provenance:  recordings.RecordingSecretProvenanceDeclared,
				}},
			},
		},
	}, nil)

	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "recording-portable-secret",
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if built.Artifact.SecretProvenance != nil {
		t.Fatal("built artifact retained secret provenance handoff")
	}
	assertPortableArtifactEventRedacted(t, built.Artifact.Events[0].Payload, "portable-artifact-control")

	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil {
		t.Fatalf("EncodePortableArtifact: %v", err)
	}
	var persisted recordings.PortableArtifact
	if err := json.Unmarshal(encoded.Payload, &persisted); err != nil {
		t.Fatalf("decode encoded portable artifact: %v", err)
	}
	assertPortableArtifactEventRedacted(t, persisted.Events[0].Payload, "portable-artifact-control")
}

func assertPortableArtifactEventRedacted(t *testing.T, payload string, wantControl string) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		t.Fatalf("decode portable artifact event: %v", err)
	}
	var marker recordings.RecordingRedactedValue
	if err := json.Unmarshal(fields["credential"], &marker); err != nil {
		t.Fatalf("decode portable artifact marker: %v", err)
	}
	if err := marker.Validate(); err != nil {
		t.Fatalf("portable artifact marker: %v", err)
	}
	var control string
	if err := json.Unmarshal(fields["control"], &control); err != nil || control != wantControl {
		t.Fatalf("control = %q, want %q (err=%v)", control, wantControl, err)
	}
}
