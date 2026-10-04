package service_test

import (
	"encoding/json"
	"strings"
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

func TestPortableArtifactExportRedactsDeclaredFactoryPaths(t *testing.T) {
	t.Parallel()
	finishedAt := time.Date(2026, 8, 24, 12, 1, 0, 0, time.UTC)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-runtime-provenance"}
	event := recordings.CanonicalEvent{
		ID: "initial-event", Kind: "RUN_REQUEST", Scope: scope,
		RecordedAt: finishedAt.Add(-time.Minute),
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "runtime-provenance"},
		Payload:    `{"factory":{"credential":"runtime-secret","items":[{"token":"item-secret","label":"visible"}],"a/b":{"~key":"escaped-secret"},"scalar":"leaf"}}`,
	}
	snapshot := recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID: "runtime-provenance", Scope: scope,
			State: recordings.RecordingFinalized, FinalizedAt: &finishedAt,
		},
		Events: []recordings.CanonicalEvent{event},
		SecretProvenance: map[int][]recordings.RecordingSecret{0: {
			{JSONPointer: "/factory/credential", Provenance: recordings.RecordingSecretProvenanceDeclared},
			{JSONPointer: "/factory/items/0/token", Provenance: recordings.RecordingSecretProvenanceDeclared},
			{JSONPointer: "/factory/a~1b/~0key", Provenance: recordings.RecordingSecretProvenanceDeclared},
		}},
	}
	service := artifactsexportservice.New(snapshotSourceFake{snapshot: snapshot}, nil)
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: "runtime-provenance"})
	if err != nil || len(built.Artifact.Events) != 1 {
		t.Fatalf("BuildPortableArtifact = (%#v, %v), want one redacted event", built, err)
	}
	assertDeclaredFactoryPathsRedacted(t, built.Artifact.Events[0].Payload)
	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{Artifact: built.Artifact})
	if err != nil {
		t.Fatalf("EncodePortableArtifact: %v", err)
	}
	var persisted recordings.PortableArtifact
	if err := json.Unmarshal(encoded.Payload, &persisted); err != nil {
		t.Fatalf("decode encoded artifact: %v", err)
	}
	assertDeclaredFactoryPathsRedacted(t, persisted.Events[0].Payload)
	if snapshot.Events[0].Payload != event.Payload || built.Artifact.SecretProvenance != nil {
		t.Fatal("export changed source payload or retained private provenance")
	}
}

func assertDeclaredFactoryPathsRedacted(t *testing.T, rawPayload string) {
	t.Helper()
	for _, forbidden := range []string{"runtime-secret", "item-secret", "escaped-secret", `"value":`} {
		if strings.Contains(rawPayload, forbidden) {
			t.Fatalf("redacted payload retained %q", forbidden)
		}
	}
	var payload struct {
		Factory struct {
			Credential recordings.RecordingRedactedValue `json:"credential"`
			Items      []struct {
				Token recordings.RecordingRedactedValue `json:"token"`
				Label string                            `json:"label"`
			} `json:"items"`
			Escaped struct {
				Key recordings.RecordingRedactedValue `json:"~key"`
			} `json:"a/b"`
			Scalar  string          `json:"scalar"`
			Missing json.RawMessage `json:"missing"`
		} `json:"factory"`
	}
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("decode initial event payload: %v", err)
	}
	factory := payload.Factory
	if len(factory.Items) != 1 || factory.Items[0].Label != "visible" || factory.Scalar != "leaf" || factory.Missing != nil {
		t.Fatalf("unclassified Factory paths changed: %#v", factory)
	}
	for _, marker := range []recordings.RecordingRedactedValue{factory.Credential, factory.Items[0].Token, factory.Escaped.Key} {
		if err := marker.Validate(); err != nil || marker.Provenance != recordings.RecordingSecretProvenanceDeclared {
			t.Fatalf("declared redaction marker = %#v, error = %v", marker, err)
		}
	}
}
