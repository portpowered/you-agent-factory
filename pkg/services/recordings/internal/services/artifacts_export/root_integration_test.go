package artifactsexport_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

type exportSnapshots map[recordings.RecordingID]recordinglifecycle.Snapshot

func (snapshots exportSnapshots) Snapshot(id recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	snapshot, ok := snapshots[id]
	if !ok {
		return recordinglifecycle.Snapshot{}, recordings.ErrMissingRecordingTarget
	}
	return snapshot, nil
}

type exportPublication struct {
	payload    []byte
	publishErr error
	publishes  int
	reads      int
}

func (publication *exportPublication) Publish(_ context.Context, _ string, payload []byte) error {
	publication.publishes++
	if publication.publishErr != nil {
		return publication.publishErr
	}
	publication.payload = append([]byte(nil), payload...)
	return nil
}

func (publication *exportPublication) Read(context.Context, string) ([]byte, error) {
	publication.reads++
	if len(publication.payload) == 0 {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), publication.payload...), nil
}

func exportSnapshot(id recordings.RecordingID, reference recordings.RecordingArtifactReference) recordinglifecycle.Snapshot {
	finalizedAt := time.Unix(1_700_000_001, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-export"}
	return recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{RecordingID: id, Artifact: reference, Scope: scope,
			State: recordings.RecordingFinalized, FinalizedAt: &finalizedAt},
		Events: []recordings.CanonicalEvent{{ID: "export-event", Kind: "WORK_REQUEST", Scope: scope,
			RecordedAt: time.Unix(1_700_000_000, 0).UTC(), Payload: `{"public":true}`,
			Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "generation-export"}}},
	}
}

func newExportOwner(snapshots exportSnapshots, publication *exportPublication) artifactsexport.Service {
	return artifactsexportwire.NewService(snapshots, publication)
}

func TestArtifactBuildRejectsActiveAndValidatesFinalizedSnapshot(t *testing.T) {
	t.Parallel()
	snapshot := exportSnapshot("recording-export", "artifact:export")
	active := snapshot
	active.Status.FinalizedAt = nil
	active.Status.State = recordings.RecordingActive
	snapshots := exportSnapshots{"recording-export": active}
	owner := newExportOwner(snapshots, nil)
	if _, err := owner.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: "recording-export"}); !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("active build = %v, want unavailable", err)
	}
	snapshots["recording-export"] = snapshot
	built, err := owner.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: "recording-export"})
	if err != nil {
		t.Fatalf("finalized build: %v", err)
	}
	if _, err := owner.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{Artifact: built.Artifact}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	encoded, err := owner.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{Artifact: built.Artifact})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("encode = (%d bytes, %v)", len(encoded.Payload), err)
	}
	if _, err := owner.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{Payload: []byte(`{`)}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("malformed decode = %v, want invalid", err)
	}
}

func TestArtifactRoundTripPreservesPublicSnapshotFacts(t *testing.T) {
	t.Parallel()
	snapshot := exportSnapshot("recording-round-trip", "artifact:reported-export")
	owner := newExportOwner(exportSnapshots{"recording-round-trip": snapshot}, nil)
	built, err := owner.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: "recording-round-trip"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if built.Artifact.SchemaVersion != recordings.PortableArtifactSchemaV1 || built.Artifact.Summary.Reference != snapshot.Status.Artifact ||
		built.Artifact.Summary.EventCount != 1 || !built.Artifact.Summary.Available {
		t.Fatalf("artifact = %#v", built.Artifact)
	}
	validated, err := owner.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{Artifact: built.Artifact})
	if err != nil || !reflect.DeepEqual(validated.Summary, built.Artifact.Summary) {
		t.Fatalf("validate = %#v, %v", validated, err)
	}
	encoded, err := owner.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{Artifact: built.Artifact})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := owner.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{Payload: encoded.Payload})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Artifact.Integrity != built.Artifact.Integrity || !reflect.DeepEqual(decoded.Artifact.Events, built.Artifact.Events) {
		t.Fatalf("decoded facts = %#v, want %#v", decoded.Artifact, built.Artifact)
	}
	summarized, err := owner.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{Artifact: decoded.Artifact})
	if err != nil || !reflect.DeepEqual(summarized.Summary, built.Artifact.Summary) {
		t.Fatalf("summary = %#v, %v", summarized, err)
	}
}

func TestFailedArtifactExportKeepsCauseAndNoReadableArtifact(t *testing.T) {
	t.Parallel()
	failure := errors.New("publication rejected destination")
	publication := &exportPublication{publishErr: failure}
	snapshot := exportSnapshot("recording-failed", "artifact:failed")
	owner := newExportOwner(exportSnapshots{"recording-failed": snapshot}, publication)
	result, err := owner.ExportPortableArtifact(context.Background(), recordings.ExportPortableArtifactRequest{RecordingID: "recording-failed"})
	if !errors.Is(err, recordings.ErrPortableArtifactExportFailed) || !errors.Is(err, failure) || result.Reference != "" {
		t.Fatalf("failed export = %#v, %v", result, err)
	}
	if publication.publishes != 1 {
		t.Fatalf("publishes = %d, want 1", publication.publishes)
	}
	_, err = owner.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{RecordingID: "recording-failed", Reference: snapshot.Status.Artifact})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("read after failure = %v, want unavailable", err)
	}
}

// Publication cleanup is proved at its own effect boundary, without a lifecycle or export graph.
func TestFailedPublicationPreservesDestinationAndRemovesTemporaryFile(t *testing.T) {
	t.Parallel()
	destination := filepath.Join(t.TempDir(), "destination-is-directory")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	publication, err := artifactsexportwire.NewPublication(os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove, os.Rename, os.ReadFile)
	if err != nil {
		t.Fatalf("construct publication: %v", err)
	}
	if err := publication.Publish(context.Background(), destination, []byte("portable artifact")); err == nil {
		t.Fatal("publish to directory succeeded")
	}
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() {
		t.Fatalf("destination = %v, %v, want directory", info, err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		t.Fatalf("read parent: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(destination) {
		t.Fatalf("temporary artifact remained: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(destination, filepath.Base(destination))); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("nested publish path = %v, want not exist", err)
	}
}

func TestArtifactReadRejectsMissingAndForeignHandles(t *testing.T) {
	t.Parallel()
	snapshot := exportSnapshot("recording-owner", "artifact:owner")
	other := exportSnapshot("recording-other", "artifact:other")
	publication := &exportPublication{}
	owner := newExportOwner(exportSnapshots{"recording-owner": snapshot, "recording-other": other}, publication)
	_, err := owner.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{RecordingID: "recording-owner", Reference: snapshot.Status.Artifact})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("missing read = %v, want unavailable", err)
	}
	_, err = owner.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{RecordingID: "recording-other", Reference: snapshot.Status.Artifact})
	if !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("foreign read = %v, want foreign", err)
	}
	if strings.Contains(err.Error(), string(snapshot.Status.Artifact)) {
		t.Fatalf("foreign error leaked reference: %v", err)
	}
	if publication.reads != 1 {
		t.Fatalf("reads = %d, foreign read must not reach publication", publication.reads)
	}
}

func TestArtifactCancellationPreventsPublicationAndRead(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"export", "read"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			snapshot := exportSnapshot("recording-cancel", "artifact:cancel")
			publication := &exportPublication{}
			owner := newExportOwner(exportSnapshots{"recording-cancel": snapshot}, publication)
			cause := errors.New("operator stopped " + operation)
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(cause)
			var err error
			if operation == "export" {
				_, err = owner.ExportPortableArtifact(ctx, recordings.ExportPortableArtifactRequest{RecordingID: "recording-cancel"})
			} else {
				_, err = owner.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{RecordingID: "recording-cancel", Reference: snapshot.Status.Artifact})
			}
			if !errors.Is(err, recordings.ErrPortableArtifactCancelled) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("cancelled %s = %v, want typed cause", operation, err)
			}
			if publication.publishes != 0 || publication.reads != 0 {
				t.Fatalf("cancelled operation admitted effects: %#v", publication)
			}
			if operation == "export" {
				_, err = owner.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{RecordingID: "recording-cancel", Reference: snapshot.Status.Artifact})
				if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
					t.Fatalf("read after cancellation = %v, want unavailable", err)
				}
			}
		})
	}
}
