package replay

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestSaveRedactsDeclaredSecretsBeforeReplayStorage(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "run.replay.json")
	if err := Save(
		testReplayStorage(),
		path,
		secretReplayArtifact(t),
		[]recordings.RecordingSecret{{
			JSONPointer: "/events/1/payload/credential",
			Provenance:  recordings.RecordingSecretProvenanceDeclared,
		}},
	); err != nil {
		t.Fatalf("Save: %v", err)
	}

	payload, err := testReplayStorage().ReadFile(path)
	if err != nil {
		t.Fatalf("read saved replay artifact: %v", err)
	}
	assertReplayArtifactPayloadRedacted(t, payload, 1, "replay-control")
}

func TestRecorderRedactsEveryStreamingReplaySnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stream.replay.json")
	recorder, err := NewRecorder(
		testReplayStorage(),
		path,
		minimalValidArtifact(time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)),
		time.Hour,
		[]recordings.RecordingSecret{{
			JSONPointer: "/events/1/payload/credential",
			Provenance:  recordings.RecordingSecretProvenanceDeclared,
		}},
	)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	recorder.RecordEvent(secretReplayEvent())
	if err := recorder.Flush(); err != nil {
		t.Fatalf("Recorder.Flush: %v", err)
	}

	payload, err := testReplayStorage().ReadFile(path)
	if err != nil {
		t.Fatalf("read streamed replay artifact: %v", err)
	}
	assertReplayArtifactPayloadRedacted(t, payload, 1, "replay-control")
}

func TestSaveRejectsClassifiedReplayPathBeforeStorage(t *testing.T) {
	t.Parallel()

	const declaredSecret = "replay-write-failure-secret-002"
	path := filepath.Join(t.TempDir(), "rejected.replay.json")
	err := Save(
		testReplayStorage(),
		path,
		secretReplayArtifactWithSecret(t, declaredSecret),
		[]recordings.RecordingSecret{{
			JSONPointer: "/events/9/payload/credential",
			Provenance:  recordings.RecordingSecretProvenanceDeclared,
		}},
	)
	if !errors.Is(err, recordings.ErrRecordingSecretPathNotFound) {
		t.Fatalf("Save error = %v, want missing classified path", err)
	}
	if strings.Contains(err.Error(), declaredSecret) {
		t.Fatalf("Save error exposed declared secret: %v", err)
	}
	if _, readErr := testReplayStorage().ReadFile(path); readErr == nil {
		t.Fatal("rejected Save published a replay artifact")
	}
}

func secretReplayArtifact(t *testing.T) *interfaces.ReplayArtifact {
	return secretReplayArtifactWithSecret(t, "replay-write-secret-002")
}

func secretReplayArtifactWithSecret(t *testing.T, secret string) *interfaces.ReplayArtifact {
	t.Helper()
	artifact := minimalValidArtifact(time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))
	artifact.Events = append(artifact.Events, secretReplayEventWithSecret(secret))
	return artifact
}

func secretReplayEvent() interfaces.FactoryEvent {
	return secretReplayEventWithSecret("replay-write-secret-002")
}

func secretReplayEventWithSecret(secret string) interfaces.FactoryEvent {
	return interfaces.FactoryEvent{
		Id:            "replay-secret-event",
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeWorkRequest,
		Context:       interfaces.FactoryEventContext{EventTime: time.Date(2026, 8, 24, 12, 1, 0, 0, time.UTC)},
		Payload:       json.RawMessage(`{"credential":"` + secret + `","control":"replay-control"}`),
	}
}

func assertReplayArtifactPayloadRedacted(
	t *testing.T,
	payload []byte,
	eventIndex int,
	wantControl string,
) {
	t.Helper()
	var artifact struct {
		Events []struct {
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(payload, &artifact); err != nil {
		t.Fatalf("decode replay artifact: %v", err)
	}
	if eventIndex < 0 || eventIndex >= len(artifact.Events) {
		t.Fatalf("event index %d is absent from %d events", eventIndex, len(artifact.Events))
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(artifact.Events[eventIndex].Payload, &fields); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	var marker recordings.RecordingRedactedValue
	if err := json.Unmarshal(fields["credential"], &marker); err != nil {
		t.Fatalf("decode credential marker: %v", err)
	}
	if err := marker.Validate(); err != nil {
		t.Fatalf("credential marker: %v", err)
	}
	var control string
	if err := json.Unmarshal(fields["control"], &control); err != nil || control != wantControl {
		t.Fatalf("control = %q, want %q (err=%v)", control, wantControl, err)
	}
}
