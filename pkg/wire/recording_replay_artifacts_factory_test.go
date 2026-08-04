package wire

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type recordingReplayArtifactsFactoryLedger struct {
	recordings.Ledger
}

// TestProvideRecordingReplayArtifactsFactoryBindsTheExactWireProducedInstance
// proves the narrow RecordingReplayArtifacts capability Wire composes is
// produced explicitly at the Wire boundary -- not discovered later from a
// broader Recordings Service -- and is backed by real Recordings JSONL
// composition rather than a stub.
func TestProvideRecordingReplayArtifactsFactoryBindsTheExactWireProducedInstance(t *testing.T) {
	t.Parallel()

	factory := provideRecordingReplayArtifactsFactory(
		serviceedges.Edges{},
		provideLiveRecordingTargetPlanner(),
		platformreplay.Local{},
	)
	replayArtifacts := factory(
		&recordingReplayArtifactsFactoryLedger{},
		recordingswire.NewProjectionService(),
	)
	if replayArtifacts == nil {
		t.Fatal("provideRecordingReplayArtifactsFactory() returned nil capability")
	}

	// The Wire-produced replay/artifact capability and the lifecycle
	// capability used to seed a finalized recording below must be the exact
	// same underlying composed instance, otherwise the seeded recording
	// would be invisible to LoadReplay/BuildArtifact.
	lifecycle, ok := replayArtifacts.(recordings.RecordingLifecycle)
	if !ok {
		t.Fatal("Wire-produced RecordingReplayArtifacts does not also expose RecordingLifecycle")
	}

	exerciseWireProducedRecordingReplayArtifacts(t, lifecycle, replayArtifacts)
}

// exerciseWireProducedRecordingReplayArtifacts proves the Wire-produced
// instance is a genuinely functioning replay/artifact capability backed by
// real Recordings composition: a finalized recording's canonical facts load
// correctly, and its portable artifact round-trips through build, validate,
// encode, decode, and summarize.
func exerciseWireProducedRecordingReplayArtifacts(
	t *testing.T,
	lifecycle recordings.RecordingLifecycle,
	replayArtifacts recordings.RecordingReplayArtifacts,
) {
	t.Helper()

	recordingID := recordings.LifecycleRecordingID("wire-replay-artifacts-factory-recording")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-wire-replay-artifacts-factory"}
	lifecycleScope := recordings.LifecycleScope{FactorySessionID: scope.FactorySessionID}
	artifactPath := filepath.Join(t.TempDir(), "wire-replay-artifacts-factory.json")
	if _, err := lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: recordingID,
		Artifact:    recordings.LifecycleArtifactReference(artifactPath),
		Scope:       lifecycleScope,
	}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	event, err := wireCompositionRunRequestEvent(
		"wire-replay-artifacts-factory-run-request",
		0,
		scope,
		time.Unix(1_700_000_000, 0).UTC(),
		"generation-wire-replay-artifacts-factory",
	)
	if err != nil {
		t.Fatalf("wireCompositionRunRequestEvent: %v", err)
	}
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: recordingID,
		Event: recordings.LifecycleEvent{
			ID:          string(event.ID),
			Sequence:    int64(event.Sequence),
			FactoryTick: event.FactoryTick,
			Scope:       lifecycleScope,
			Kind:        string(event.Kind),
			Payload:     event.Payload,
			RecordedAt:  event.RecordedAt,
			Cursor: recordings.LifecycleEventCursor{
				StreamGenerationID: event.Cursor.StreamGenerationID,
				Sequence:           int64(event.Cursor.Sequence),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if _, err := lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Unix(1_700_000_300, 0).UTC(),
	}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	replayRecordingID := recordings.ReplayRecordingID(recordingID)
	loaded, err := replayArtifacts.LoadReplay(recordings.LoadReplayRequest{RecordingID: replayRecordingID})
	if err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}
	if len(loaded.Replay.Events) != 1 || loaded.Replay.Events[0].ID != string(event.ID) {
		t.Fatalf("LoadReplay() Events = %#v, want one event %q", loaded.Replay.Events, event.ID)
	}

	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: replayRecordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	if built.Artifact.SchemaVersion != recordings.ArtifactSchemaV1 || built.Artifact.Summary.EventCount != 1 {
		t.Fatalf("BuildArtifact() Artifact = %#v", built.Artifact)
	}

	if _, err := replayArtifacts.ValidateArtifact(recordings.ValidateArtifactRequest{
		Artifact: built.Artifact,
	}); err != nil {
		t.Fatalf("ValidateArtifact: %v", err)
	}

	encoded, err := replayArtifacts.EncodeArtifact(recordings.EncodeArtifactRequest{Artifact: built.Artifact})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodeArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}

	decoded, err := replayArtifacts.DecodeArtifact(recordings.DecodeArtifactRequest{Payload: encoded.Payload})
	if err != nil {
		t.Fatalf("DecodeArtifact: %v", err)
	}
	if decoded.Artifact.Integrity != built.Artifact.Integrity {
		t.Fatalf("DecodeArtifact() Integrity = %#v, want %#v", decoded.Artifact.Integrity, built.Artifact.Integrity)
	}

	summarized, err := replayArtifacts.SummarizeArtifact(recordings.SummarizeArtifactRequest{
		Artifact: decoded.Artifact,
	})
	if err != nil {
		t.Fatalf("SummarizeArtifact: %v", err)
	}
	if summarized.Summary.RecordingID != replayRecordingID || summarized.Summary.EventCount != 1 {
		t.Fatalf("SummarizeArtifact() Summary = %#v", summarized.Summary)
	}

	exported, err := replayArtifacts.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{
		RecordingID: replayRecordingID,
	})
	if err != nil {
		t.Fatalf("ExportArtifact: %v", err)
	}
	read, err := replayArtifacts.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{
		RecordingID: replayRecordingID,
		Reference:   exported.Reference,
	})
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if read.Artifact.Integrity != exported.Artifact.Integrity {
		t.Fatalf("ReadArtifact() Integrity = %#v, want %#v", read.Artifact.Integrity, exported.Artifact.Integrity)
	}
}
